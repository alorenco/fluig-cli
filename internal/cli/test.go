package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/browser"
	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/testrun"
)

// Teste de processo por cenários (ROADMAP §5.15, fase A). A spec mora em
// docs/interno/SPEC-TEST.md; as respostas do Gate 0 estão no DIARIO.

func newTestCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Teste de processo por cenários e teste de unidade dos scripts",
		Long: "Testa um processo no motor BPM real, pela API e sem navegador.\n\n" +
			"Uma suíte JSON descreve cenários. Cada cenário abre uma solicitação,\n" +
			"executa passos (move, assume, modo gestor, gravação de campos) e confere\n" +
			"o estado depois de cada passo: etapa, status, campos do card, caminho\n" +
			"percorrido e mensagens de erro dos eventos.\n\n" +
			"O test coverage mede quais transições do diagrama os testes percorreram.\n" +
			"O test new gera o esqueleto de uma suíte a partir do diagrama.\n" +
			"O test unit roda funções de script no Rhino local, sem servidor, com\n" +
			"falsos do form, do hAPI e do DatasetFactory.\n\n" +
			"O test run NUNCA roda em servidor de produção.",
	}
	cmd.AddCommand(newTestRunCmd(app), newTestCoverageCmd(app), newTestNewCmd(app), newTestUnitCmd(app))
	return cmd
}

func newTestRunCmd(app *App) *cobra.Command {
	var (
		scenarios     []string
		dryRun        bool
		keep          bool
		varFlags      []string
		stepTimeout   time.Duration
		passwordStdin bool
		browserPath   string
		headed        bool
		uiArtifacts   string
	)
	cmd := &cobra.Command{
		Use:   "run [arquivo.json|pasta]...",
		Short: "Executa suítes de teste de processo contra um servidor de homologação",
		Long: "Executa suítes de teste de processo. Cada cenário abre a PRÓPRIA\n" +
			"solicitação e roda os passos em ordem. Os cenários rodam em série.\n\n" +
			"Alvo: um arquivo .json ou uma pasta (lê os *.json da pasta, sem\n" +
			"recursão). Sem alvo, o comando lê as suítes de tests/*/ do projeto.\n\n" +
			"Formato da suíte (resumo; a referência completa está em docs/test.md):\n" +
			"  {\n" +
			"    \"process\": \"Compras\",\n" +
			"    \"defaults\": { \"card\": \"fixtures/card_base.json\" },\n" +
			"    \"scenarios\": [\n" +
			"      { \"name\": \"aprovação simples\",\n" +
			"        \"steps\": [\n" +
			"          { \"start\": true, \"expect\": { \"state\": 5 } },\n" +
			"          { \"assume\": true },\n" +
			"          { \"move\": { \"target\": 9, \"fields\": { \"decisao\": \"aprovado\" } },\n" +
			"            \"expect\": { \"state\": 12, \"card\": { \"aprovador\": { \"notEmpty\": true } } } }\n" +
			"        ] }\n" +
			"    ]\n" +
			"  }\n\n" +
			"Passos: start, move, assume, manager, setFields, check, wait, cancel e ui.\n" +
			"Asserções (expect): state, stateName, status, error, card, path e\n" +
			"assignee. Expressões: {{today}}, {{today(+N)}}, {{unique}},\n" +
			"{{var.nome}}, {{actors.eu.login}}, {{actors.eu.code}}, {{requestId}} e\n" +
			"{{steps.<id>.nextState}}. \"Hoje\" é a data do SERVIDOR; o fuso vem de\n" +
			"--var tz=America/Sao_Paulo ou, sem ela, do fuso local.\n\n" +
			"Limpeza: no fim de cada cenário a solicitação é cancelada, se ainda\n" +
			"estiver aberta (\"cleanup\": [\"cancel\"], o padrão). O cancelamento NÃO\n" +
			"desfaz integração externa. Use --keep para deixar as solicitações\n" +
			"abertas para inspeção.\n\n" +
			"Limites conhecidos: os eventos de formulário (validateForm,\n" +
			"inputFields) rodam no start, no move e no setFields. O displayFields\n" +
			"não roda nos passos de API (o passo ui confere a tela). As service\n" +
			"tasks rodam DEPOIS da resposta do move: use um\n" +
			"passo wait antes de conferir a etapa seguinte a uma service task.\n\n" +
			"Passo ui: abre a tarefa REAL num navegador headless (Chrome, Chromium ou\n" +
			"Edge; a sessão do fluigcli vale no navegador) e confere a tela: visible,\n" +
			"hidden, enabled, disabled, value, text, css, message (a recusa do envio) e\n" +
			"changed/unchanged (hiddens). Ações: open (task|view), fill, click, send\n" +
			"<etapa> e save. Um passo ui que falha grava um screenshot (--ui-artifacts).\n\n" +
			"Exit codes: 0 = todos os cenários passaram; 1 = algum cenário falhou\n" +
			"(TEST_FAILED, com o relatório completo no --json); 2 = suíte inválida ou\n" +
			"servidor de produção; 3 = autenticação; 4 = arquivo, fixture ou processo\n" +
			"inexistente; 7 = suíte com passo ui e nenhum navegador (BROWSER_NOT_FOUND).",
		Example: "  # roda as suítes de tests/*/\n" +
			"  fluigcli test run --server homologacao\n\n" +
			"  # um cenário de um arquivo, com variável\n" +
			"  fluigcli test run tests/Compras/aprovacao.json --scenario \"aprovação simples\" --var venc=10/11/2026\n\n" +
			"  # valida a suíte contra o diagrama, sem abrir solicitação\n" +
			"  fluigcli test run tests/Compras/ --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ts, err := app.setupTestRun(p, args, testRunFlags{
				scenarios: scenarios, varFlags: varFlags, keep: keep, stepTimeout: stepTimeout,
				passwordStdin: passwordStdin, readOnly: dryRun,
				browser: browserPath, headed: headed, uiArtifacts: uiArtifacts,
			})
			if err != nil {
				return err
			}
			if dryRun {
				return runTestDryRun(ts.ctx, p, ts.eng, ts.server.Name, ts.suites, ts.procs, ts.opts, app.Verbose)
			}
			return runTestSuites(ts.ctx, app, p, ts.eng, ts.server.Name, ts.suites, ts.procs, ts.opts)
		},
	}
	cmd.Flags().StringArrayVar(&scenarios, "scenario", nil, "roda só o cenário com este nome (comparação exata; pode repetir)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "valida as suítes contra o diagrama do processo, sem abrir solicitação")
	cmd.Flags().BoolVar(&keep, "keep", false, "não roda a limpeza: as solicitações ficam como estão, para inspeção")
	cmd.Flags().StringArrayVar(&varFlags, "var", nil, "variável de suíte chave=valor (pode repetir; vence o \"vars\" do arquivo)")
	cmd.Flags().DurationVar(&stepTimeout, "step-timeout", 5*time.Minute, "tempo máximo de cada passo")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	cmd.Flags().StringVar(&browserPath, "browser", "", "navegador dos passos ui (default: $FLUIGCLI_BROWSER, Chrome/Chromium/Edge instalado ou o Chromium do Playwright)")
	cmd.Flags().BoolVar(&headed, "headed", false, "mostra a janela do navegador nos passos ui (depuração)")
	cmd.Flags().StringVar(&uiArtifacts, "ui-artifacts", "", "pasta dos screenshots de passo ui que falha (default: uma pasta temporária)")
	return cmd
}

// testRunFlags são as opções comuns do test run e da fonte "executar" do
// test coverage.
type testRunFlags struct {
	scenarios     []string
	varFlags      []string
	keep          bool
	stepTimeout   time.Duration
	passwordStdin bool
	readOnly      bool // --dry-run: sem o piso de tempo limite da escrita
	browser       string
	headed        bool
	uiArtifacts   string
}

// testRunSetup é o estado pronto para executar as suítes.
type testRunSetup struct {
	ctx    context.Context
	server *config.Server
	eng    *testrun.FluigEngine
	suites []*testrun.Suite
	procs  []*testrun.ProcessInfo
	opts   testrun.Options
}

// setupTestRun carrega e valida as suítes, aplica o guard de produção,
// autentica e lê o diagrama de cada processo. Erro de carga sai antes de
// qualquer escrita.
func (app *App) setupTestRun(p *output.Printer, args []string, f testRunFlags) (*testRunSetup, error) {
	vars, err := parseTestVars(f.varFlags)
	if err != nil {
		return nil, err
	}
	if f.stepTimeout <= 0 {
		return nil, output.Usagef("--step-timeout deve ser maior que zero")
	}
	files, err := app.testTargets(args)
	if err != nil {
		return nil, err
	}
	suites, err := loadTestSuites(files, vars)
	if err != nil {
		return nil, err
	}
	if err := checkScenarioFilter(suites, f.scenarios); err != nil {
		return nil, err
	}
	if err := app.pickTestServer(suites); err != nil {
		return nil, err
	}

	// Guard de produção ANTES de autenticar, sem flag para liberar: uma
	// suíte abre e move dezenas de solicitações e dispara integrações.
	server, err := app.resolveServer("")
	if err != nil {
		return nil, err
	}
	p.Server = server.Name
	if server.Env == config.EnvProd {
		return nil, output.Usagef("o fluigcli test não roda em produção. Use um servidor de homologação (o servidor %q está marcado como produção)", server.Name)
	}
	if !f.readOnly {
		app.raiseWriteTimeout()
	}
	ctx := context.Background()
	client, err := app.authenticate(ctx, server, f.passwordStdin)
	if err != nil {
		return nil, err
	}
	eng := &testrun.FluigEngine{Client: client, Login: server.Username}

	code, err := client.ResolveUserCode(ctx)
	if err != nil {
		return nil, app.mapErr(err)
	}
	today, err := testToday(ctx, client.ServerTime, vars["tz"])
	if err != nil {
		return nil, err
	}
	opts := testrun.Options{
		Today: today, SelfLogin: server.Username, SelfCode: code,
		Keep: f.keep, StepTimeout: f.stepTimeout, Scenarios: f.scenarios,
	}
	// Passo ui: o navegador é conferido ANTES de abrir qualquer solicitação.
	if !f.readOnly && suitesHaveUI(suites) {
		path, err := findBrowser(f.browser)
		if err != nil {
			return nil, output.BrowserNotFoundf("%v. A suíte tem passo ui: instale o Chrome, o Chromium ou o Edge, ou informe --browser <caminho>", err)
		}
		dir := f.uiArtifacts
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "fluigcli-ui-"+time.Now().Format("20060102-150405"))
		}
		opts.UI = newUIDriver(path, f.headed, client)
		opts.UIArtifacts = dir
	}

	// Processo inexistente é erro de carga (exit 4), antes do primeiro
	// cenário: nada roda pela metade.
	procs := make([]*testrun.ProcessInfo, len(suites))
	for i, s := range suites {
		pi, err := eng.Process(ctx, s.Process)
		if err != nil {
			return nil, testEngineErr(err, fmt.Sprintf("suíte %s: processo %q", s.File, s.Process))
		}
		if len(pi.States) == 0 {
			return nil, output.NotFoundf("suíte %s: o processo %q não tem versão ativa", s.File, s.Process)
		}
		procs[i] = pi
	}
	return &testRunSetup{ctx: ctx, server: server, eng: eng, suites: suites, procs: procs, opts: opts}, nil
}

func parseTestVars(flags []string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range flags {
		k, v, ok := strings.Cut(f, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, output.Usagef("--var %q inválida: use chave=valor", f)
		}
		out[k] = v
	}
	return out, nil
}

// testTargets resolve os arquivos de suíte a partir dos argumentos.
func (a *App) testTargets(args []string) ([]string, error) {
	if len(args) == 0 {
		root, err := a.projectRootForFiles()
		if err != nil {
			return nil, err
		}
		files, err := project.DefaultSuites(root)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, output.NotFoundf("pasta %s/ não encontrada em %s; informe o arquivo ou a pasta da suíte", project.TestsDir, root)
			}
			return nil, output.Usagef("não consegui ler %s/: %v", project.TestsDir, err)
		}
		if len(files) == 0 {
			return nil, output.NotFoundf("nenhuma suíte em %s/*/ (esperado tests/<processo>/<suíte>.json)", project.TestsDir)
		}
		return files, nil
	}
	var out []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, output.NotFoundf("suíte %q não encontrada", arg)
			}
			return nil, output.Usagef("não consegui ler %s: %v", arg, err)
		}
		if !info.IsDir() {
			out = append(out, arg)
			continue
		}
		files, err := project.SuitesInDir(arg)
		if err != nil {
			return nil, output.Usagef("não consegui ler %s: %v", arg, err)
		}
		if len(files) == 0 {
			return nil, output.NotFoundf("nenhuma suíte *.json em %s", arg)
		}
		out = append(out, files...)
	}
	return out, nil
}

func loadTestSuites(files []string, vars map[string]string) ([]*testrun.Suite, error) {
	suites := make([]*testrun.Suite, 0, len(files))
	for _, f := range files {
		s, err := testrun.LoadSuite(f, testrun.LoadOptions{Vars: vars})
		if err != nil {
			if errors.Is(err, testrun.ErrMissing) {
				return nil, output.NotFoundf("%s", err.Error())
			}
			return nil, output.Usagef("%s", strings.TrimPrefix(err.Error(), testrun.ErrUsage.Error()+": "))
		}
		suites = append(suites, s)
	}
	return suites, nil
}

func checkScenarioFilter(suites []*testrun.Suite, names []string) error {
	for _, n := range names {
		achou := false
		for _, s := range suites {
			for _, sc := range s.Scenarios {
				if sc.Name == n {
					achou = true
				}
			}
		}
		if !achou {
			return output.Usagef("--scenario %q não existe nas suítes informadas (a comparação é exata)", n)
		}
	}
	return nil
}

// pickTestServer aplica o "server" das suítes quando --server não veio. Suítes
// com servidores diferentes exigem --server: rodar metade em cada um seria
// surpresa.
func (a *App) pickTestServer(suites []*testrun.Suite) error {
	if a.Server != "" {
		return nil
	}
	nome := ""
	for _, s := range suites {
		if s.Server == "" {
			continue
		}
		if nome != "" && s.Server != nome {
			return output.Usagef("as suítes apontam servidores diferentes (%q e %q); escolha um com --server", nome, s.Server)
		}
		nome = s.Server
	}
	a.Server = nome
	return nil
}

// testToday devolve a data do servidor no fuso informado (ou no local).
func testToday(ctx context.Context, serverTime func(context.Context) (time.Time, error), tz string) (time.Time, error) {
	loc := time.Local
	if tz != "" {
		l, err := time.LoadLocation(tz)
		if err != nil {
			return time.Time{}, output.Usagef("--var tz=%s: fuso desconhecido (ex.: America/Sao_Paulo)", tz)
		}
		loc = l
	}
	t, err := serverTime(ctx)
	if err != nil {
		return time.Time{}, mapFluigError(err)
	}
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), nil
}

func testEngineErr(err error, subject string) error {
	var ee *testrun.EngineError
	if errors.As(err, &ee) {
		switch ee.Code {
		case "NOT_FOUND":
			return output.NotFoundf("%s não encontrado no servidor", subject)
		case "AUTH_FAILED":
			return output.AuthFailedf("%s", ee.Message)
		case "TIMEOUT":
			return output.Timeoutf("%s: %s", subject, ee.Message)
		}
		return output.ServerErrorf("%s: %s", subject, ee.Message).WithCause(err)
	}
	return mapFluigError(err)
}

// --- execução ---

// executeTestSuites roda as suítes, imprime o relatório humano e devolve o
// Report (sem fechar o envelope: quem chama decide o resultado).
func executeTestSuites(ctx context.Context, app *App, p *output.Printer, eng testrun.Engine, serverName string,
	suites []*testrun.Suite, procs []*testrun.ProcessInfo, opts testrun.Options) *testrun.Report {
	inicio := time.Now()
	total := 0
	for _, s := range suites {
		for _, sc := range s.Scenarios {
			if opts.Selected(sc.Name) {
				total++
			}
		}
	}
	n := 0
	opts.Progress = func(s *testrun.Suite, r testrun.ScenarioResult) {
		n++
		fmt.Fprintf(p.Stderr, "[%d/%d] %s · %s: %s (%s)\n", n, total, filepath.Base(s.File), r.Name,
			resultLabel(r.Result, false), fmtDuration(r.DurationMs))
	}
	report := testrun.Report{Server: serverName}
	for i, s := range suites {
		report.Suites = append(report.Suites, testrun.RunSuite(ctx, eng, s, procs[i], opts))
	}
	report.Summary = testrun.Summarize(report.Suites)
	report.Summary.DurationMs = time.Since(inicio).Milliseconds()

	renderTestReport(p, &report, app.Verbose)
	for _, su := range report.Suites {
		for _, sc := range su.Scenarios {
			if sc.Cleanup != nil && sc.Cleanup.Status == "failed" {
				p.Warnf("cenário %q: limpeza falhou: %s", sc.Name, sc.Cleanup.Message)
			}
		}
	}
	return &report
}

// testRunFailMessage devolve a mensagem do TEST_FAILED, ou "" se tudo passou.
func testRunFailMessage(r *testrun.Report) string {
	sm := r.Summary
	if sm.Failed+sm.Errored == 0 {
		return ""
	}
	return fmt.Sprintf("%d de %d cenário(s) não passaram (%d falharam, %d com erro)",
		sm.Failed+sm.Errored, sm.Passed+sm.Failed+sm.Errored+sm.Skipped, sm.Failed, sm.Errored)
}

func runTestSuites(ctx context.Context, app *App, p *output.Printer, eng testrun.Engine, serverName string,
	suites []*testrun.Suite, procs []*testrun.ProcessInfo, opts testrun.Options) error {
	report := executeTestSuites(ctx, app, p, eng, serverName, suites, procs, opts)
	msg := testRunFailMessage(report)
	if msg == "" {
		p.Done(report)
		return nil
	}
	p.FailData(report, output.CodeTestFailed, msg)
	return output.TestFailedf("%s", msg)
}

func runTestDryRun(ctx context.Context, p *output.Printer, eng testrun.Engine, serverName string,
	suites []*testrun.Suite, procs []*testrun.ProcessInfo, opts testrun.Options, verbose bool) error {
	report := testrun.Report{Server: serverName, DryRun: true}
	var problemas []string
	for i, s := range suites {
		sr, probs := testrun.DryRunSuite(ctx, eng, s, procs[i], opts)
		report.Suites = append(report.Suites, sr)
		problemas = append(problemas, probs...)
	}
	report.Summary = testrun.Summarize(report.Suites)
	renderTestReport(p, &report, verbose)
	if len(problemas) == 0 {
		p.Infof("Suítes validadas: nada foi escrito no servidor (--dry-run).")
		p.Done(report)
		return nil
	}
	for _, pr := range problemas {
		p.Warnf("%s", pr)
	}
	msg := fmt.Sprintf("%d problema(s) nas suítes; corrija antes de executar", len(problemas))
	p.FailData(report, output.CodeUsage, msg)
	return output.Usagef("%s", msg)
}

// --- saída humana ---

func resultLabel(r string, upper bool) string {
	switch r {
	case testrun.ResPassed:
		return "passou"
	case testrun.ResFailed:
		if upper {
			return "FALHOU"
		}
		return "falhou"
	case testrun.ResErrored:
		return "erro"
	case testrun.ResSkipped:
		return "pulado"
	case testrun.ResValidated:
		return "validado"
	}
	return r
}

// fmtDuration formata milissegundos em pt-BR ("12,4 s").
func fmtDuration(ms int64) string {
	s := strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64)
	return strings.Replace(s, ".", ",", 1) + " s"
}

func renderTestReport(p *output.Printer, r *testrun.Report, verbose bool) {
	for _, su := range r.Suites {
		p.Infof("Suíte: %s", su.File)
		p.Infof("Processo: %s (versão %d) · servidor: %s", su.Process, su.ProcessVersion, r.Server)
		for _, w := range su.Warnings {
			p.Warnf("%s", w)
		}
		if len(su.Scenarios) == 0 {
			p.Infof("Nenhum cenário selecionado nesta suíte.")
			continue
		}
		rows := make([][]string, 0, len(su.Scenarios))
		for i, sc := range su.Scenarios {
			req := "—"
			if sc.RequestID > 0 {
				req = strconv.Itoa(sc.RequestID)
			}
			tempo := "—"
			if !r.DryRun {
				tempo = fmtDuration(sc.DurationMs)
			}
			rows = append(rows, []string{strconv.Itoa(i + 1), sc.Name, resultLabel(sc.Result, true), req, tempo})
		}
		results := make([]string, len(su.Scenarios))
		for i, sc := range su.Scenarios {
			results[i] = sc.Result
		}
		p.Table(output.Table{
			Headers: []string{"#", "Cenário", "Resultado", "Solicitação", "Tempo"},
			Rows:    rows,
			Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
				if col != 2 || !output.ColorEnabled() || row < 0 || row >= len(results) {
					return padded
				}
				switch results[row] {
				case testrun.ResPassed, testrun.ResValidated:
					return output.Green(padded)
				case testrun.ResFailed:
					return output.Red(padded)
				case testrun.ResErrored:
					return output.Yellow(padded)
				}
				return padded
			}),
		})
		for i, sc := range su.Scenarios {
			falhou := sc.Result == testrun.ResFailed || sc.Result == testrun.ResErrored
			if !falhou && !verbose {
				continue
			}
			marca := "✓"
			if falhou {
				marca = "✗"
			}
			p.Infof("%s %d · %s", marca, i+1, sc.Name)
			for _, st := range sc.Steps {
				if st.Result == testrun.ResSkipped && !verbose {
					continue
				}
				ok := st.Result == testrun.ResPassed || st.Result == testrun.ResValidated
				if ok && !verbose {
					continue
				}
				linha := fmt.Sprintf("  passo %d (%s): %s", st.Index, st.Name, resultLabel(st.Result, false))
				if st.DurationMs > 0 {
					linha += " em " + fmtDuration(st.DurationMs)
				}
				p.Infof("%s", linha)
				if st.Error != "" && (!ok || len(st.Assertions) == 0) {
					p.Infof("    erro: %s", st.Error)
				}
				for _, a := range st.Assertions {
					if a.Passed && !verbose {
						continue
					}
					p.Infof("    %s", testrun.FormatAssertion(a))
				}
				if st.Screenshot != "" {
					p.Infof("    tela: %s", st.Screenshot)
				}
			}
		}
		p.Infof("")
	}
	sm := r.Summary
	if r.DryRun {
		p.Infof("Resumo: %d validados · %d com problema", sm.Validated, sm.Failed)
	} else {
		p.Infof("Resumo: %d passaram · %d falharam · %d com erro · %d pulados (%s)",
			sm.Passed, sm.Failed, sm.Errored, sm.Skipped, fmtDuration(sm.DurationMs))
	}
	var abertas []string
	for _, su := range r.Suites {
		for _, sc := range su.Scenarios {
			if sc.Cleanup != nil && sc.Cleanup.Action == "keep" && sc.RequestID > 0 {
				abertas = append(abertas, strconv.Itoa(sc.RequestID))
			}
		}
	}
	if len(abertas) > 0 {
		p.Infof("Solicitações mantidas (--keep): %s", strings.Join(abertas, ", "))
	}
}

func suitesHaveUI(suites []*testrun.Suite) bool {
	for _, s := range suites {
		if s.HasUI() {
			return true
		}
	}
	return false
}

// findBrowser e newUIDriver são pontos de troca para os testes.
var (
	findBrowser = browser.Find
	newUIDriver = func(path string, headed bool, client *fluig.Client) testrun.UIDriver {
		return &testrun.BrowserDriver{ExecPath: path, Headed: headed, Base: client.BaseURL(), Jar: client.SessionJar()}
	}
)
