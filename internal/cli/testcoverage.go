package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/testrun"
)

// Cobertura de caminhos do BPMN (ROADMAP §5.15, fase C; spec em
// docs/interno/SPEC-TEST-C.md). A fonte das arestas percorridas é a
// HISTOR_PROCES (via helper), não a REST de tarefas: a REST devolve a etapa
// escolhida, não a etapa onde a tarefa aconteceu (Gate 0).

func newTestCoverageCmd(app *App) *cobra.Command {
	var (
		process       string
		version       int
		includeErrors bool
		minPct        float64
		fromResult    string
		requests      string
		scenarios     []string
		varFlags      []string
		keep          bool
		stepTimeout   time.Duration
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "coverage [arquivo.json|pasta]...",
		Short: "Cobertura de caminhos: quais transições do diagrama os testes percorreram",
		Long: "Calcula quais TRANSIÇÕES (arestas) do diagrama do processo as solicitações\n" +
			"percorreram, e quais nenhuma percorreu. É cobertura de caminhos do BPMN,\n" +
			"não de linhas de código.\n\n" +
			"Três fontes de solicitações (use uma):\n" +
			"  <alvo>...              roda as suítes, como o test run, e mede as\n" +
			"                         solicitações criadas (recusa produção)\n" +
			"  --from-result arq.json lê o --json de um test run anterior (só leitura)\n" +
			"  --requests 1,2,3       usa as solicitações informadas; exige --process\n" +
			"                         (só leitura)\n\n" +
			"As fontes de leitura aceitam produção: medir os caminhos do uso real não\n" +
			"escreve nada. Neste caso, o comando avisa no stderr.\n\n" +
			"As arestas têm duas categorias. \"main\" é o fluxo principal. \"error\" é o\n" +
			"tratamento de erro: as arestas que saem do evento de erro de uma service\n" +
			"task e tudo o que só é alcançável a partir dele. O percentual principal\n" +
			"conta só \"main\"; use --include-errors para somar as duas.\n\n" +
			"A versão de referência é a ativa (ou --version). Uma solicitação que rodou\n" +
			"em outra versão conta mesmo assim, com aviso. Um par percorrido que não\n" +
			"existe no diagrama de referência vai para a lista \"unknown\".\n\n" +
			"Requer o fluigcliHelper com a API de banco (lê a HISTOR_PROCES).\n\n" +
			"Exit codes: 0 = cobertura calculada; 1 = cenário falhou (TEST_FAILED) ou\n" +
			"abaixo do --min (COVERAGE_BELOW_MIN); 2 = uso; 4 = processo, versão ou\n" +
			"todas as solicitações inexistentes; 7 = helper ausente.",
		Example: "  # roda as suítes e mede a cobertura\n" +
			"  fluigcli test coverage tests/Compras/\n\n" +
			"  # mede um resultado gravado, exigindo 80% do fluxo principal\n" +
			"  fluigcli test run tests/Compras/ --json > r.json\n" +
			"  fluigcli test coverage --from-result r.json --min 80\n\n" +
			"  # mede solicitações já existentes (só leitura)\n" +
			"  fluigcli test coverage --process Compras --requests 1201,1202,1205",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			minSet := cmd.Flags().Changed("min")
			if minSet && (minPct < 0 || minPct > 100) {
				return output.Usagef("--min deve ficar entre 0 e 100")
			}
			if version < 0 {
				return output.Usagef("--version deve ser 1 ou mais")
			}
			fontes := 0
			for _, on := range []bool{len(args) > 0, fromResult != "", requests != ""} {
				if on {
					fontes++
				}
			}
			if fontes > 1 {
				return output.Usagef("use só uma fonte: alvos de suíte, --from-result ou --requests")
			}
			kind := "run"
			switch {
			case fromResult != "":
				kind = "result"
			case requests != "":
				kind = "requests"
			}
			if kind != "run" {
				for _, f := range []string{"scenario", "var", "keep", "step-timeout"} {
					if cmd.Flags().Changed(f) {
						return output.Usagef("--%s só vale quando o coverage roda as suítes (sem --from-result/--requests)", f)
					}
				}
			}

			var (
				ctx    context.Context
				eng    *testrun.FluigEngine
				refs   []testrun.RequestRef
				source = testrun.CoverageSource{Kind: kind, Requests: []int{}}
				run    *testrun.Report
				server *config.Server
			)
			switch kind {
			case "run":
				files, err := app.testTargets(args)
				if err != nil {
					return err
				}
				// Um processo por vez: confere antes de rodar qualquer coisa.
				vars, err := parseTestVars(varFlags)
				if err != nil {
					return err
				}
				suites, err := loadTestSuites(files, vars)
				if err != nil {
					return err
				}
				pid, err := singleProcess(suites, process)
				if err != nil {
					return err
				}
				process = pid
				ts, err := app.setupTestRun(p, args, testRunFlags{
					scenarios: scenarios, varFlags: varFlags, keep: keep, stepTimeout: stepTimeout,
					passwordStdin: passwordStdin,
				})
				if err != nil {
					return err
				}
				ctx, eng, server = ts.ctx, ts.eng, ts.server
				for _, s := range ts.suites {
					source.Suites = append(source.Suites, s.File)
				}
				run = executeTestSuites(ctx, app, p, eng, server.Name, ts.suites, ts.procs, ts.opts)
				refs = testrun.RefsFromReport(run)
			case "result":
				rep, err := readRunResult(fromResult)
				if err != nil {
					return err
				}
				pid, err := reportProcess(rep, process)
				if err != nil {
					return err
				}
				process = pid
				for _, su := range rep.Suites {
					source.Suites = append(source.Suites, su.File)
				}
				refs = testrun.RefsFromReport(rep)
			case "requests":
				if strings.TrimSpace(process) == "" {
					return output.Usagef("--requests exige --process <processId>")
				}
				ids, err := parseRequestList(requests)
				if err != nil {
					return err
				}
				for _, id := range ids {
					refs = append(refs, testrun.RequestRef{RequestID: id})
				}
			}
			if eng == nil {
				srv, err := app.resolveServer("")
				if err != nil {
					return err
				}
				server = srv
				p.Server = srv.Name
				if srv.Env == config.EnvProd {
					p.Warnf("leitura em servidor de PRODUÇÃO (%s): o coverage só lê, nada é escrito", srv.Name)
				}
				ctx = context.Background()
				app.raiseReadTimeout()
				client, err := app.authenticate(ctx, srv, passwordStdin)
				if err != nil {
					return err
				}
				eng = &testrun.FluigEngine{Client: client, Login: srv.Username}
			}

			pd, err := eng.Diagram(ctx, process, version)
			if err != nil {
				subject := fmt.Sprintf("processo %q", process)
				if version > 0 {
					subject = fmt.Sprintf("versão %d do processo %q", version, process)
				}
				return testEngineErr(err, subject)
			}
			if len(pd.States) == 0 {
				return output.NotFoundf("o processo %q não tem versão ativa; informe --version", process)
			}
			g := testrun.BuildGraph(pd)

			hist, warns, err := loadHistories(ctx, eng, refs, process, kind)
			if err != nil {
				return err
			}
			cov := testrun.ComputeCoverage(g, refs, hist, includeErrors)
			cov.Server = server.Name
			cov.Warnings = append(warns, cov.Warnings...)
			for _, r := range refs {
				if r.RequestID > 0 && !containsInt(source.Requests, r.RequestID) {
					source.Requests = append(source.Requests, r.RequestID)
				}
			}
			cov.Source = source
			cov.Run = run

			renderCoverage(p, &cov, app.Verbose)

			if run != nil {
				if msg := testRunFailMessage(run); msg != "" {
					p.FailData(cov, output.CodeTestFailed, msg)
					return output.TestFailedf("%s", msg)
				}
			}
			if minSet && cov.Summary.Headline.Percent < minPct {
				msg := fmt.Sprintf("cobertura de %s abaixo do mínimo: %s%% < %s%%",
					headlineLabel(cov.Summary.IncludeErrors), fmtPct(cov.Summary.Headline.Percent), fmtPct(minPct))
				p.FailData(cov, output.CodeCoverageBelowMin, msg)
				return output.CoverageBelowMinf("%s", msg)
			}
			p.Done(cov)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&process, "process", "", "processId (obrigatório com --requests; nas outras fontes vem da suíte)")
	f.IntVar(&version, "version", 0, "versão do diagrama usada como referência (default: a ativa)")
	f.BoolVar(&includeErrors, "include-errors", false, "soma as arestas de tratamento de erro ao percentual principal")
	f.Float64Var(&minPct, "min", 0, "percentual mínimo do fluxo principal; abaixo dele: exit 1 COVERAGE_BELOW_MIN")
	f.StringVar(&fromResult, "from-result", "", "lê o --json de um test run anterior (\"-\" = stdin); não escreve no servidor")
	f.StringVar(&requests, "requests", "", "solicitações separadas por vírgula (exige --process); não escreve no servidor")
	f.StringArrayVar(&scenarios, "scenario", nil, "roda só o cenário com este nome (só ao rodar as suítes)")
	f.StringArrayVar(&varFlags, "var", nil, "variável de suíte chave=valor (só ao rodar as suítes)")
	f.BoolVar(&keep, "keep", false, "não roda a limpeza (só ao rodar as suítes)")
	f.DurationVar(&stepTimeout, "step-timeout", 5*time.Minute, "tempo máximo de cada passo (só ao rodar as suítes)")
	f.BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

func singleProcess(suites []*testrun.Suite, flag string) (string, error) {
	pid := strings.TrimSpace(flag)
	for _, s := range suites {
		if pid == "" {
			pid = s.Process
			continue
		}
		if s.Process != pid {
			return "", output.Usagef("o coverage mede um processo por vez: %s é do processo %q, e não de %q", s.File, s.Process, pid)
		}
	}
	return pid, nil
}

func reportProcess(r *testrun.Report, flag string) (string, error) {
	pid := strings.TrimSpace(flag)
	for _, su := range r.Suites {
		if pid == "" {
			pid = su.Process
			continue
		}
		if su.Process != pid {
			return "", output.Usagef("o coverage mede um processo por vez: o resultado traz %q e %q", pid, su.Process)
		}
	}
	if pid == "" {
		return "", output.Usagef("o resultado não traz suítes; informe --process")
	}
	return pid, nil
}

// readRunResult lê o envelope --json de um test run (ou só o data).
func readRunResult(path string) (*testrun.Report, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil, output.NotFoundf("arquivo %q não encontrado", path)
		}
	}
	if err != nil {
		return nil, output.Usagef("não consegui ler %s: %v", path, err)
	}
	var env struct {
		Command string          `json:"command"`
		Data    json.RawMessage `json:"data"`
	}
	raw := data
	if json.Unmarshal(data, &env) == nil && len(env.Data) > 0 && string(env.Data) != "null" {
		raw = env.Data
	}
	var rep testrun.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		return nil, output.Usagef("%s não é um resultado do test run --json: %v", path, err)
	}
	if rep.DryRun {
		return nil, output.Usagef("%s é um resultado de --dry-run: não há solicitações para medir", path)
	}
	if len(rep.Suites) == 0 {
		return nil, output.Usagef("%s não traz suítes (esperado o --json de um fluigcli test run)", path)
	}
	return &rep, nil
}

func parseRequestList(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return nil, output.Usagef("--requests: %q não é um número de solicitação", part)
		}
		if !containsInt(out, n) {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil, output.Usagef("--requests vazio: informe os números separados por vírgula")
	}
	return out, nil
}

func containsInt(xs []int, n int) bool {
	for _, x := range xs {
		if x == n {
			return true
		}
	}
	return false
}

// loadHistories lê o histórico de cada solicitação. Inexistente = aviso;
// todas inexistentes = exit 4; outro processo = exit 2; helper ausente = 7.
func loadHistories(ctx context.Context, eng testrun.Engine, refs []testrun.RequestRef, process, kind string) (map[int][]fluig.HistoryMove, []string, error) {
	out := map[int][]fluig.HistoryMove{}
	var warns []string
	pedidas, faltando := 0, 0
	visto := map[int]bool{}
	for _, r := range refs {
		if r.RequestID <= 0 || visto[r.RequestID] {
			continue
		}
		visto[r.RequestID] = true
		pedidas++
		h, err := eng.History(ctx, r.RequestID)
		if err != nil {
			var ee *testrun.EngineError
			if errors.As(err, &ee) {
				switch ee.Code {
				case "NOT_FOUND":
					faltando++
					warns = append(warns, fmt.Sprintf("a solicitação %d não existe (ou não tem histórico)", r.RequestID))
					continue
				case "HELPER_NOT_INSTALLED":
					return nil, nil, output.MissingHelperf("%s", ee.Message)
				}
			}
			return nil, nil, testEngineErr(err, fmt.Sprintf("histórico da solicitação %d", r.RequestID))
		}
		if len(h) > 0 && h[0].ProcessID != "" && h[0].ProcessID != process {
			return nil, nil, output.Usagef("a solicitação %d é do processo %q, não de %q", r.RequestID, h[0].ProcessID, process)
		}
		out[r.RequestID] = h
	}
	if pedidas > 0 && faltando == pedidas {
		return nil, nil, output.NotFoundf("nenhuma das solicitações existe no servidor")
	}
	return out, warns, nil
}

// --- saída humana ---

func fmtPct(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".", ",", 1)
}

func headlineLabel(includeErrors bool) string {
	if includeErrors {
		return "fluxo principal + tratamento de erro"
	}
	return "fluxo principal"
}

func stateLabel(seq int, name string) string {
	if name == "" {
		return strconv.Itoa(seq)
	}
	return strconv.Itoa(seq) + " " + name
}

func hitsLabel(hits []testrun.RequestRef) string {
	if len(hits) == 0 {
		return ""
	}
	cenarios := []string{}
	for _, h := range hits {
		if h.Scenario != "" && !containsStr(cenarios, h.Scenario) {
			cenarios = append(cenarios, h.Scenario)
		}
	}
	if len(cenarios) == 0 {
		ids := []string{}
		for _, h := range hits {
			ids = append(ids, strconv.Itoa(h.RequestID))
		}
		if len(ids) > 4 {
			return fmt.Sprintf("%d solicitações", len(ids))
		}
		return strings.Join(ids, ", ")
	}
	if len(cenarios) > 2 {
		return fmt.Sprintf("%d cenários", len(cenarios))
	}
	for i, c := range cenarios {
		if r := []rune(c); len(r) > 28 {
			cenarios[i] = string(r[:27]) + "…"
		}
	}
	return strings.Join(cenarios, "; ")
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// conditionLabel junta as alternativas de condição: regras com "e", as
// alternativas com "ou".
func conditionLabel(cs []testrun.EdgeCondition) string {
	alts := []string{}
	for _, c := range cs {
		if c.Expression != "" {
			alts = append(alts, c.Expression)
			continue
		}
		parts := []string{}
		for _, r := range c.Rules {
			parts = append(parts, strings.TrimSpace(r.Field+" "+r.Operator+" "+r.Value))
		}
		alts = append(alts, strings.Join(parts, " e "))
	}
	return strings.Join(alts, " ou ")
}

// truncRunes corta o texto em n caracteres visíveis.
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func renderCoverage(p *output.Printer, c *testrun.CoverageReport, verbose bool) {
	p.Infof("Cobertura de %s (versão %d) · servidor: %s", c.Process, c.Version, c.Server)
	switch c.Source.Kind {
	case "requests":
		p.Infof("Fonte: %d solicitação(ões) informada(s)", len(c.Source.Requests))
	default:
		cen := 0
		if c.Run != nil {
			for _, su := range c.Run.Suites {
				cen += len(su.Scenarios)
			}
		}
		suites := strings.Join(c.Source.Suites, ", ")
		if cen > 0 {
			p.Infof("Fonte: %d cenário(s) de %s (%d solicitações)", cen, suites, len(c.Source.Requests))
		} else {
			p.Infof("Fonte: %s (%d solicitações)", suites, len(c.Source.Requests))
		}
	}
	for _, w := range c.Warnings {
		p.Warnf("%s", w)
	}
	sm := c.Summary
	p.Infof("")
	if sm.IncludeErrors {
		p.Infof("Fluxo principal + tratamento de erro: %d de %d arestas (%s%%) · etapas humanas: %d de %d",
			sm.Headline.Covered, sm.Headline.Total, fmtPct(sm.Headline.Percent), sm.States.Covered, sm.States.Total)
	} else {
		p.Infof("Fluxo principal: %d de %d arestas (%s%%) · etapas humanas: %d de %d",
			sm.Main.Covered, sm.Main.Total, fmtPct(sm.Main.Percent), sm.States.Covered, sm.States.Total)
	}

	var rows [][]string
	var covered []bool
	for _, e := range c.Edges {
		if e.Category == testrun.CatError && !verbose && !sm.IncludeErrors {
			continue
		}
		cob := "NÃO"
		if e.Covered {
			cob = "sim"
		}
		origem := stateLabel(e.From, e.FromName)
		if e.Category == testrun.CatError {
			origem += " (erro)"
		}
		rows = append(rows, []string{origem, stateLabel(e.To, e.ToName), cob, hitsLabel(e.Hits)})
		covered = append(covered, e.Covered)
		if cond := conditionLabel(e.Conditions); cond != "" {
			rows = append(rows, []string{"  condição: " + truncRunes(cond, 40), "", "", ""})
			covered = append(covered, e.Covered)
		}
	}
	p.Table(output.Table{
		Headers: []string{"Origem", "Destino", "Coberta", "Cenários"},
		Rows:    rows,
		Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
			if col != 2 || !output.ColorEnabled() || row < 0 || row >= len(rows) || strings.TrimSpace(padded) == "" {
				return padded
			}
			if covered[row] {
				return output.Green(padded)
			}
			return output.Red(padded)
		}),
	})

	if len(c.Gateways) > 0 {
		p.Infof("Gateways:")
		for _, gw := range c.Gateways {
			partes := []string{}
			for _, o := range gw.Outputs {
				marca := "✗"
				if o.Covered {
					marca = "✔"
				}
				partes = append(partes, fmt.Sprintf("%d %s", o.To, marca))
			}
			p.Infof("  %s: %s", stateLabel(gw.Sequence, gw.Name), strings.Join(partes, "  "))
			for _, o := range gw.Outputs {
				if cond := conditionLabel(o.Conditions); cond != "" {
					p.Infof("      → %d quando %s", o.To, cond)
				}
			}
		}
	}
	if len(c.UncoveredStates) > 0 {
		nomes := []string{}
		for _, s := range c.UncoveredStates {
			nomes = append(nomes, stateLabel(s.Sequence, s.Name))
		}
		p.Infof("Etapas humanas nunca abertas: %s", strings.Join(nomes, ", "))
	}
	if !sm.IncludeErrors {
		p.Infof("Tratamento de erro: %d de %d arestas (%s%%). Use --include-errors para somar.",
			sm.Error.Covered, sm.Error.Total, fmtPct(sm.Error.Percent))
	}
	if len(c.Unknown) > 0 {
		pares := []string{}
		for _, u := range c.Unknown {
			pares = append(pares, fmt.Sprintf("%d→%d", u.From, u.To))
		}
		sort.Strings(pares)
		p.Infof("Transições fora do diagrama de referência: %s (veja \"unknown\" no --json)", strings.Join(pares, ", "))
	}
}
