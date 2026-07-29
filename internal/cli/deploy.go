package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/sqlsplit"
)

// Release por manifesto (ROADMAP2 §3.11).
//
// O usuário do relato escreveu à mão um `docs/deploy_producao_juridico.md`: uma
// lista ORDENADA de scripts SQL, datasets (uns com `--new`, outros sem) e a
// widget. O pedido é transformar esse documento em algo executável e auditável.
//
// O formato é JSON, e não YAML: YAML exigiria dependência nova, e o projeto não
// usa YAML em nada (decisão do mantenedor, 2026-07-29).

// deployPlan é o manifesto lido do arquivo.
type deployPlan struct {
	// Server é o alvo default do plano. O --server da linha de comando sobrepõe.
	Server string       `json:"server,omitempty"`
	Steps  []deployStep `json:"steps"`
}

// deployStep é um passo do plano. Exatamente UMA das chaves de tipo
// (dataset/event/mechanism/widget/db) identifica o que o passo publica ou roda.
type deployStep struct {
	Name string `json:"name,omitempty"` // rótulo livre, só para o relatório

	Dataset   string `json:"dataset,omitempty"`
	Event     string `json:"event,omitempty"`
	Mechanism string `json:"mechanism,omitempty"`
	Widget    string `json:"widget,omitempty"`
	DB        string `json:"db,omitempty"`

	// Opções por tipo (espelham as flags do comando equivalente).
	New         bool   `json:"new,omitempty"`         // dataset
	Description string `json:"description,omitempty"` // dataset, mechanism
	Build       bool   `json:"build,omitempty"`       // widget
	Force       bool   `json:"force,omitempty"`       // widget
}

// deployStepResult é o resultado de um passo (contrato --json).
type deployStepResult struct {
	Index  int    `json:"index"` // 1-based, como no --from
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind"`   // dataset | event | mechanism | widget | db
	Target string `json:"target"` // arquivo, pasta ou código
	Status string `json:"status"` // ok | failed | skipped | validated
	Action string `json:"action,omitempty"`
	Error  string `json:"error,omitempty"`

	// plan guarda as opções do passo (não vai para o JSON: o plano já está no
	// arquivo, e repeti-lo no envelope só aumentaria a saída).
	plan deployStep
}

// Status possíveis de um passo.
const (
	deployOK        = "ok"
	deployFailed    = "failed"
	deploySkipped   = "skipped" // não tentado: o plano parou antes
	deployValidated = "validated"
)

// kindOf devolve o tipo e o alvo do passo, ou erro quando o passo não tem
// exatamente uma chave de tipo.
func (s deployStep) kindOf() (kind, target string, err error) {
	pares := []struct{ kind, target string }{
		{"dataset", s.Dataset}, {"event", s.Event}, {"mechanism", s.Mechanism},
		{"widget", s.Widget}, {"db", s.DB},
	}
	for _, p := range pares {
		if p.target == "" {
			continue
		}
		if kind != "" {
			return "", "", fmt.Errorf("o passo tem mais de um tipo (%s e %s): use um passo para cada", kind, p.kind)
		}
		kind, target = p.kind, p.target
	}
	if kind == "" {
		return "", "", fmt.Errorf("passo sem tipo: informe uma das chaves dataset, event, mechanism, widget ou db")
	}
	return kind, target, nil
}

func newDeployCmd(app *App) *cobra.Command {
	var (
		planPath      string
		dryRun        bool
		from          int
		noAudit       bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "deploy --plan <arquivo.json>",
		Short: "Executa um plano de release na ordem, passo a passo (manifesto JSON)",
		Long: "Executa um plano de release descrito em JSON: datasets, eventos,\n" +
			"mecanismos, widgets e scripts SQL de diagnóstico, na ORDEM do arquivo.\n\n" +
			"O plano é um arquivo versionável no repositório. Ele troca o roteiro de\n" +
			"deploy escrito à mão por algo executável e auditável.\n\n" +
			"Formato (JSON — o projeto não usa YAML):\n" +
			"  {\n" +
			"    \"server\": \"producao\",\n" +
			"    \"steps\": [\n" +
			"      {\"name\": \"diagnóstico\", \"db\": \"sql/001_check.sql\"},\n" +
			"      {\"dataset\": \"datasets/ds_agenda.js\", \"new\": true},\n" +
			"      {\"dataset\": \"datasets/ds_processos.js\"},\n" +
			"      {\"widget\": \"processos_judiciais\", \"build\": true}\n" +
			"    ]\n" +
			"  }\n\n" +
			"O comando PARA no primeiro erro. Os passos seguintes saem como\n" +
			"\"skipped\" no relatório, então dá para ver exatamente onde o release\n" +
			"parou. Corrija e retome com --from N (a numeração é a do relatório).\n\n" +
			"Use --dry-run para validar o plano inteiro sem escrever nada: arquivos\n" +
			"presentes, auditoria dos scripts, colisão de código da widget e as\n" +
			"instruções de cada script SQL.\n\n" +
			"O plano NUNCA contém senha. A autenticação segue a precedência normal.\n" +
			"Passos de formulário e de processo ainda não são suportados.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if planPath == "" {
				return output.Usagef("informe o plano com --plan <arquivo.json>")
			}
			if from < 0 {
				return output.Usagef("--from deve ser 1 ou mais (a numeração começa em 1)")
			}

			plan, err := readDeployPlan(planPath)
			if err != nil {
				return err
			}
			steps, err := validateDeployPlan(plan, planPath)
			if err != nil {
				return err
			}
			if from > len(steps) {
				return output.Usagef("--from %d não existe: o plano tem %d passo(s)", from, len(steps))
			}
			// O --server da linha de comando vence o do arquivo.
			if app.Server == "" && plan.Server != "" {
				app.Server = plan.Server
				p.Server = plan.Server
			}

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}

			// Pré-checagem local dos scripts do plano INTEIRO, antes de conectar:
			// um release não deve começar para descobrir no meio que um script não
			// compila.
			gate := app.auditBeforePublish(p, scriptTargetsDoPlano(steps, root), auditGateOpts{skip: noAudit})
			gate.report(p)
			if reprovados := gate.bloqueados(scriptTargetsDoPlano(steps, root)); len(reprovados) > 0 {
				err := gate.blockedError(reprovados[0])
				return output.AuditFailedf("%s (%d script(s) reprovado(s); nada foi publicado)",
					err.Message, len(reprovados))
			}

			ctx := context.Background()
			// Um único guard de produção para o plano todo: perguntar por passo
			// seria pior que não perguntar.
			acao := "executar o plano de deploy " + filepath.Base(planPath)
			if dryRun {
				_, client, cerr := app.connect(ctx, passwordStdin)
				if cerr != nil {
					return cerr
				}
				return runDeployDryRun(ctx, app, p, client, root, planPath, steps, from)
			}
			_, client, err := app.connectWrite(ctx, passwordStdin, acao)
			if err != nil {
				return err
			}
			return runDeployPlan(ctx, app, p, client, root, planPath, steps, from)
		},
	}
	cmd.Flags().StringVar(&planPath, "plan", "", "arquivo JSON com o plano de release")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "valida o plano inteiro sem escrever nada no servidor")
	cmd.Flags().IntVar(&from, "from", 0, "retoma a partir do passo N (1-based, a numeração do relatório)")
	cmd.Flags().BoolVar(&noAudit, "no-audit", false, "não audita os scripts do plano antes de publicar")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// readDeployPlan lê e desserializa o manifesto.
func readDeployPlan(path string) (*deployPlan, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, output.NotFoundf("plano %q não encontrado", path)
		}
		return nil, output.Usagef("não consegui ler %s: %v", path, err)
	}
	var plan deployPlan
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields() // chave escrita errado é erro, não silêncio
	if err := dec.Decode(&plan); err != nil {
		return nil, output.Usagef("plano %s inválido: %v", path, err)
	}
	return &plan, nil
}

// validateDeployPlan confere o plano inteiro antes de qualquer execução e
// devolve os passos já resolvidos (tipo + alvo).
func validateDeployPlan(plan *deployPlan, path string) ([]deployStepResult, error) {
	if len(plan.Steps) == 0 {
		return nil, output.Usagef("o plano %s não tem passos", path)
	}
	steps := make([]deployStepResult, 0, len(plan.Steps))
	for i, s := range plan.Steps {
		kind, target, err := s.kindOf()
		if err != nil {
			return nil, output.Usagef("passo %d do plano: %v", i+1, err)
		}
		steps = append(steps, deployStepResult{
			Index: i + 1, Name: s.Name, Kind: kind, Target: target, Status: deploySkipped, plan: s,
		})
	}
	return steps, nil
}

// scriptTargetsDoPlano lista os arquivos de script do plano (dataset, event,
// mechanism) para a auditoria. Widget e db ficam de fora: o widget é auditado
// pelo `audit` da pasta e o db não é script de plataforma.
func scriptTargetsDoPlano(steps []deployStepResult, root string) []string {
	var out []string
	for _, s := range steps {
		switch s.Kind {
		case "dataset", "event", "mechanism":
			out = append(out, resolveDeployPath(root, s.Target))
		}
	}
	return out
}

// resolveDeployPath resolve o caminho do alvo relativo à raiz do projeto (o
// plano é versionado com o projeto, então caminhos relativos são o normal).
func resolveDeployPath(root, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	if _, err := os.Stat(target); err == nil {
		return target
	}
	return filepath.Join(root, target)
}

// renderDeploySteps imprime o plano no modo humano.
func renderDeploySteps(p *output.Printer, steps []deployStepResult, titulo string) {
	rows := make([][]string, 0, len(steps))
	for _, s := range steps {
		nome := s.Name
		if nome == "" {
			nome = "—"
		}
		rows = append(rows, []string{strconv.Itoa(s.Index), s.Kind, s.Target, nome})
	}
	p.Infof("%s", titulo)
	p.Table(output.Table{
		Headers: []string{"#", "Tipo", "Alvo", "Nome"},
		Rows:    rows,
		Style:   output.BoldHeaderStyle(nil),
	})
}

// runDeployDryRun valida o plano contra o servidor sem escrever nada.
func runDeployDryRun(ctx context.Context, app *App, p *output.Printer, client *fluig.Client,
	root, planPath string, steps []deployStepResult, from int) error {
	renderDeploySteps(p, steps, fmt.Sprintf("Plano %s — %d passo(s), nada será escrito (--dry-run):", planPath, len(steps)))

	var lastErr error
	failures := 0
	for i := range steps {
		s := &steps[i]
		if from > 0 && s.Index < from {
			s.Status = deploySkipped
			s.Error = "fora do intervalo (--from)"
			continue
		}
		if err := checkDeployStep(ctx, app, client, root, s); err != nil {
			failures++
			lastErr = err
			s.Status, s.Error = deployFailed, output.AsError(err).Message
			p.Warnf("passo %d (%s %s): %s", s.Index, s.Kind, s.Target, s.Error)
			continue
		}
		s.Status = deployValidated
		p.Successf("── [%d] %s %s: %s", s.Index, s.Kind, s.Target, s.Action)
	}
	data := map[string]any{
		"plan": planPath, "dryRun": true, "steps": steps,
		"counts": contaDeploy(steps),
	}
	if failures == 0 {
		p.Infof("Plano validado: %d passo(s) prontos para executar.", len(steps))
		p.Done(data)
		return nil
	}
	p.FailData(data, output.AsError(lastErr).Code, output.AsError(lastErr).Message)
	return output.Usagef("o plano tem %d passo(s) com problema — corrija antes de executar", failures)
}

// checkDeployStep faz a checagem read-only de um passo e preenche a ação prevista.
func checkDeployStep(ctx context.Context, app *App, client *fluig.Client, root string, s *deployStepResult) error {
	switch s.Kind {
	case "dataset", "event", "mechanism":
		path := resolveDeployPath(root, s.Target)
		if _, err := os.Stat(path); err != nil {
			return output.NotFoundf("arquivo %q não encontrado", s.Target)
		}
		if s.Kind == "dataset" {
			id := project.ArtifactName(path)
			if _, err := client.LoadDataset(ctx, id); err == nil {
				s.Action = "atualizaria o dataset " + id
			} else {
				s.Action = "criaria o dataset " + id + " (exige \"new\": true)"
			}
			return nil
		}
		s.Action = "publicaria " + s.Kind + " " + project.ArtifactName(path)
		return nil

	case "widget":
		dir := project.WidgetDir(root, s.Target)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return output.NotFoundf("widget %q não encontrado em %s", s.Target, project.WidgetsDir)
		}
		// Colisão de código com layout (§3.1): read-only, e é o erro que
		// derrubou um servidor.
		if err := checkLayoutCollision(ctx, discardPrinter(), client, s.Target, false); err != nil {
			return err
		}
		s.Action = "publicaria a widget " + s.Target
		return nil

	case "db":
		path := resolveDeployPath(root, s.Target)
		raw, err := os.ReadFile(path)
		if err != nil {
			return output.NotFoundf("script %q não encontrado", s.Target)
		}
		stmts := sqlsplit.Split(string(raw))
		if len(stmts) == 0 {
			return output.Usagef("o script %q não tem instrução SQL", s.Target)
		}
		s.Action = fmt.Sprintf("rodaria %d instrução(ões) de leitura", len(stmts))
		return nil
	}
	return output.Usagef("tipo de passo não suportado: %s", s.Kind)
}

// discardPrinter devolve um Printer que joga a saída fora. Serve para
// reaproveitar uma checagem que, no dry-run, emitiria mensagem fora de contexto.
func discardPrinter() *output.Printer {
	return &output.Printer{JSON: true, Stdout: discardWriter{}, Stderr: discardWriter{}}
}

type discardWriter struct{}

func (discardWriter) Write(b []byte) (int, error) { return len(b), nil }

// runDeployPlan executa os passos na ordem, parando no primeiro erro.
func runDeployPlan(ctx context.Context, app *App, p *output.Printer, client *fluig.Client,
	root, planPath string, steps []deployStepResult, from int) error {
	renderDeploySteps(p, steps, fmt.Sprintf("Plano %s — %d passo(s):", planPath, len(steps)))

	var lastErr error
	executados, failures := 0, 0
	parou := false
	for i := range steps {
		s := &steps[i]
		if parou {
			s.Status = deploySkipped
			continue
		}
		if from > 0 && s.Index < from {
			s.Status = deploySkipped
			s.Error = "fora do intervalo (--from)"
			continue
		}
		action, err := execDeployStep(ctx, app, p, client, root, s)
		if err != nil {
			failures++
			lastErr = err
			s.Status, s.Error, s.Action = deployFailed, output.AsError(err).Message, action
			p.Warnf("passo %d (%s %s): %s", s.Index, s.Kind, s.Target, s.Error)
			// Para no primeiro erro: o resto do plano depende deste passo até
			// prova em contrário, e um release meio aplicado é pior que um
			// release interrompido num ponto conhecido.
			parou = true
			continue
		}
		executados++
		s.Status, s.Action = deployOK, action
		p.Successf("── [%d] %s %s: %s", s.Index, s.Kind, s.Target, action)
	}

	counts := contaDeploy(steps)
	data := map[string]any{"plan": planPath, "dryRun": false, "steps": steps, "counts": counts}
	if failures == 0 {
		p.Infof("Plano concluído: %d passo(s) executados.", executados)
		p.Done(data)
		return nil
	}
	p.Infof("Plano interrompido no passo %d: %d executado(s), %d não tentado(s). Retome com --from %d.",
		primeiroFalho(steps), executados, counts[deploySkipped], primeiroFalho(steps))
	// Nada executado: devolve o erro real, com o exit code dele. Não há release
	// parcial para reportar.
	if executados == 0 {
		p.FailData(data, output.AsError(lastErr).Code, output.AsError(lastErr).Message)
		return lastErr
	}
	p.Partial(data)
	return output.Partialf("o plano parou no passo %d de %d", primeiroFalho(steps), len(steps))
}

// execDeployStep executa um passo e devolve a ação realizada.
func execDeployStep(ctx context.Context, app *App, p *output.Printer, client *fluig.Client,
	root string, s *deployStepResult) (string, error) {
	step := s.plan
	switch s.Kind {
	case "dataset":
		path := resolveDeployPath(root, s.Target)
		id := project.ArtifactName(path)
		action, err := app.exportOneDataset(ctx, client, path, id, step.Description, step.New)
		if err != nil {
			return action, mapFluigError(err)
		}
		return "dataset " + id + " " + action, nil

	case "event":
		path := resolveDeployPath(root, s.Target)
		action, err := app.exportOneEvent(ctx, client, path)
		if err != nil {
			return "failed", err
		}
		return action, nil

	case "mechanism":
		path := resolveDeployPath(root, s.Target)
		id := project.ArtifactName(path)
		mechs, err := client.ListMechanisms(ctx)
		if err != nil {
			return "failed", mapFluigError(err)
		}
		byID := make(map[string]fluig.Mechanism, len(mechs))
		for i := range mechs {
			byID[mechs[i].ID] = mechs[i]
		}
		action, err := app.exportOneMechanism(ctx, client, byID, path, id, "", step.Description)
		if err != nil {
			return action, mapFluigError(err)
		}
		return "mecanismo " + id + " " + action, nil

	case "widget":
		if err := app.exportOneWidget(ctx, p, client, root, s.Target, step.Build, step.Force); err != nil {
			return "failed", err
		}
		return "widget " + s.Target + " enviada", nil

	case "db":
		path := resolveDeployPath(root, s.Target)
		n, err := app.runDeploySQL(ctx, client, path)
		if err != nil {
			return "failed", err
		}
		return fmt.Sprintf("%d instrução(ões) de leitura executada(s)", n), nil
	}
	return "failed", output.Usagef("tipo de passo não suportado: %s", s.Kind)
}

// runDeploySQL roda as instruções de um script .sql (leitura) e devolve quantas
// rodaram. A primeira falha interrompe o script.
func (a *App) runDeploySQL(ctx context.Context, client *fluig.Client, path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, output.NotFoundf("script %q não encontrado", path)
	}
	stmts := sqlsplit.Split(string(raw))
	for i, st := range stmts {
		if _, err := client.DbQuery(ctx, fluig.DbQueryOptions{SQL: st.SQL}); err != nil {
			return i, output.AsError(mapFluigError(err))
		}
	}
	return len(stmts), nil
}

// contaDeploy agrupa os passos por status.
func contaDeploy(steps []deployStepResult) map[string]int {
	counts := map[string]int{}
	for _, s := range steps {
		counts[s.Status]++
	}
	return counts
}

// primeiroFalho devolve o índice (1-based) do passo que falhou.
func primeiroFalho(steps []deployStepResult) int {
	for _, s := range steps {
		if s.Status == deployFailed {
			return s.Index
		}
	}
	return 0
}
