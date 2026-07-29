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

	"github.com/alorenco/fluig-cli/internal/config"
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
	// Workflow é o PREFIXO LOCAL dos scripts (workflow/scripts/<prefixo>.*.js).
	// O destino no servidor é ProcessID quando ele difere do prefixo.
	Workflow string `json:"workflow,omitempty"`
	// Form é a pasta do formulário (forms/<pasta>).
	Form string `json:"form,omitempty"`

	// Opções por tipo (espelham as flags do comando equivalente).
	New         bool   `json:"new,omitempty"`         // dataset
	Description string `json:"description,omitempty"` // dataset, mechanism
	Build       bool   `json:"build,omitempty"`       // widget
	Force       bool   `json:"force,omitempty"`       // widget
	ProcessID   string `json:"processId,omitempty"`   // workflow (espelha --process-id)

	// Opções do passo form (espelham as flags do `form export`). FormName é
	// `formName` e não `name` porque `name` já é o rótulo livre do passo.
	FormName        string `json:"formName,omitempty"`
	DocumentID      int    `json:"documentId,omitempty"`
	ParentID        int    `json:"parentId,omitempty"`
	DatasetName     string `json:"datasetName,omitempty"`
	CardDescription string `json:"cardDescription,omitempty"`
	PersistenceType string `json:"persistenceType,omitempty"`
	Version         string `json:"version,omitempty"` // keep | new
	// NoRelease espelha o --no-release: por padrão o plano LIBERA a versão nova.
	// A chave é negativa de propósito — `bool` em JSON tem default false, e
	// "release": false seria indistinguível de "release" ausente sem ponteiro.
	NoRelease bool `json:"noRelease,omitempty"` // workflow
}

// O `workflow export` (atualização cirúrgica na versão corrente) NÃO entra no
// plano por decisão do mantenedor (2026-07-29): ele não versiona, é ferramenta de
// desenvolvimento e exige o fluigcliHelper. Release é `publish`.

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
		{"widget", s.Widget}, {"db", s.DB}, {"workflow", s.Workflow}, {"form", s.Form},
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
		return "", "", fmt.Errorf(
			"passo sem tipo: informe uma das chaves dataset, event, mechanism, form, widget, workflow ou db")
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
			"      {\"widget\": \"processos_judiciais\", \"build\": true},\n" +
			"      {\"workflow\": \"Compras\"},\n" +
			"      {\"form\": \"forms/frm_pedido\"}\n" +
			"    ]\n" +
			"  }\n\n" +
			"O comando PARA no primeiro erro. Os passos seguintes saem como\n" +
			"\"skipped\" no relatório, então dá para ver exatamente onde o release\n" +
			"parou. Corrija e retome com --from N (a numeração é a do relatório).\n\n" +
			"Use --dry-run para validar o plano inteiro sem escrever nada: arquivos\n" +
			"presentes, auditoria dos scripts, colisão de código da widget e as\n" +
			"instruções de cada script SQL.\n\n" +
			"O plano NUNCA contém senha. A autenticação segue a precedência normal.\n" +
			"O passo workflow publica uma versão NOVA do processo com os scripts\n" +
			"locais e a libera (use \"noRelease\": true para não liberar). O alvo é o\n" +
			"prefixo dos arquivos locais; use \"processId\" quando o processo no\n" +
			"servidor tiver outro nome.\n\n" +
			"O passo form publica uma pasta de formulário. A criação exige\n" +
			"\"new\": true mais \"parentId\" e \"datasetName\" — num plano não há\n" +
			"pergunta. O vínculo pasta↔documentId é gravado no\n" +
			".fluigcli/forms.json, como no form export.",
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
			scripts := scriptTargetsDoPlano(steps, root)
			gate := app.auditBeforePublish(p, scripts, auditGateOpts{skip: noAudit})
			gate.report(p)
			if reprovados := gate.bloqueados(scripts); len(reprovados) > 0 {
				err := gate.blockedError(reprovados[0])
				return output.AuditFailedf("%s (%d script(s) reprovado(s); nada foi publicado)",
					err.Message, len(reprovados))
			}
			// Formulário audita com OUTRO recorte de regras (só runtime barra —
			// §3.13), por isso a segunda chamada.
			if pastas := formTargetsDoPlano(steps, root); len(pastas) > 0 {
				fgate := app.auditBeforePublish(p, pastas, auditGateOpts{skip: noAudit, regras: regrasDeRuntime})
				fgate.report(p)
				if reprovados := fgate.bloqueados(pastas); len(reprovados) > 0 {
					err := fgate.blockedError(reprovados[0])
					return output.AuditFailedf("%s (%d formulário(s) reprovado(s); nada foi publicado)",
						err.Message, len(reprovados))
				}
			}

			ctx := context.Background()
			// Um único guard de produção para o plano todo: perguntar por passo
			// seria pior que não perguntar.
			acao := "executar o plano de deploy " + filepath.Base(planPath)
			if dryRun {
				server, client, cerr := app.connect(ctx, passwordStdin)
				if cerr != nil {
					return cerr
				}
				return runDeployDryRun(ctx, app, p, server, client, root, planPath, steps, from, noAudit)
			}
			// O *config.Server é necessário: o passo form resolve o vínculo
			// pasta↔documentId pelo escopo do servidor (FormScopeKey).
			server, client, err := app.connectWrite(ctx, passwordStdin, acao)
			if err != nil {
				return err
			}
			return runDeployPlan(ctx, app, p, server, client, root, planPath, steps, from, noAudit)
		},
	}
	cmd.Flags().StringVar(&planPath, "plan", "", "arquivo JSON com o plano de release")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "valida o plano inteiro sem escrever nada no servidor")
	cmd.Flags().IntVar(&from, "from", 0, "retoma a partir do passo N (1-based, a numeração do relatório)")
	cmd.Flags().BoolVar(&noAudit, "no-audit", false, "não audita os scripts do plano antes de publicar")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// workflowTarget devolve o processId de destino do passo e se ele foi DERIVADO do
// prefixo local (sem "processId" no plano) — a mensagem de processo inexistente
// só sugere o `processId` nesse caso.
func (s deployStep) workflowTarget() (pid string, derived bool) {
	if s.ProcessID != "" {
		return s.ProcessID, false
	}
	return s.Workflow, true
}

// scriptsDoPassoWorkflow lê os scripts locais do passo e devolve o mapa
// evento→código, mais os caminhos (para o audit).
func scriptsDoPassoWorkflow(root, prefixo string) (map[string]string, []string, error) {
	scripts, err := project.FindProcessScripts(root, prefixo)
	if err != nil {
		return nil, nil, err
	}
	if len(scripts) == 0 {
		return nil, nil, output.NotFoundf("nenhum script local do processo %q (esperado %s/%s.<evento>.js)",
			prefixo, project.WorkflowScriptsDir, prefixo)
	}
	events, err := readWorkflowEvents(scripts)
	if err != nil {
		return nil, nil, err
	}
	byEvent := make(map[string]string, len(events))
	for _, e := range events {
		byEvent[e.Name] = e.Contents
	}
	return byEvent, scriptPaths(scripts), nil
}

// formOptsDoPasso monta as opções do `form export` a partir do passo do plano.
//
// ConfirmCreate fica nil de propósito: num plano não há prompt. A criação tem de
// estar declarada com "new": true, senão o passo falha dizendo isso.
func (s deployStep) formOptsDoPasso(noAudit bool) (formExportOpts, error) {
	persist, err := parsePersistence(s.PersistenceType)
	if err != nil {
		return formExportOpts{}, err
	}
	versionOption, err := parseVersionMode(s.Version)
	if err != nil {
		return formExportOpts{}, err
	}
	return formExportOpts{
		MarkNew:         s.New,
		FormName:        s.FormName,
		DocumentID:      s.DocumentID,
		ParentID:        s.ParentID,
		DatasetName:     s.DatasetName,
		CardDescription: s.CardDescription,
		Persistence:     persist,
		VersionOption:   versionOption,
		// O audit das pastas de formulário roda no gate do plano (regras de
		// runtime), então aqui ele não repete.
		NoAudit: true,
	}, nil
}

// formTargetsDoPlano lista as pastas de formulário do plano. Elas são auditadas
// numa chamada SEPARADA de gate, porque o recorte de regras é outro: no
// formulário só RHINO*/FL* barram (§3.13).
func formTargetsDoPlano(steps []deployStepResult, root string) []string {
	var out []string
	for _, s := range steps {
		if s.Kind == "form" {
			out = append(out, resolveDeployPath(root, s.Target))
		}
	}
	return out
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
		case "workflow":
			// Os scripts do processo também são auditados. Erro de leitura aqui
			// não interrompe a auditoria: o passo falha na execução com a
			// mensagem própria.
			if _, paths, err := scriptsDoPassoWorkflow(root, s.Target); err == nil {
				out = append(out, paths...)
			}
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
func runDeployDryRun(ctx context.Context, app *App, p *output.Printer, server *config.Server,
	client *fluig.Client, root, planPath string, steps []deployStepResult, from int, noAudit bool) error {
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
		if err := checkDeployStep(ctx, app, server, client, root, s); err != nil {
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
func checkDeployStep(ctx context.Context, app *App, server *config.Server, client *fluig.Client,
	root string, s *deployStepResult) error {
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

	case "form":
		dir := resolveDeployPath(root, s.Target)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return output.NotFoundf("pasta de formulário %q não encontrada", s.Target)
		}
		if _, err := s.plan.formOptsDoPasso(true); err != nil {
			return err
		}
		// Resolve o alvo como o export faria (read-only) para dizer se criaria ou
		// atualizaria, e com que documentId.
		pub, err := client.ResolveUserCode(ctx)
		if err != nil {
			return mapFluigError(err)
		}
		fmap, err := project.LoadFormMap(root, server.FormScopeKey())
		if err != nil {
			return output.Genericf("falha ao ler .fluigcli/forms.json: %v", err)
		}
		forms, err := client.ListForms(ctx, pub)
		if err != nil {
			return mapFluigError(err)
		}
		folderKey := filepath.Base(filepath.Clean(dir))
		existing, found := resolveExportTarget(forms, fmap, folderKey, s.plan.FormName, s.plan.DocumentID)
		switch {
		case found && !s.plan.New:
			s.Action = fmt.Sprintf("atualizaria o formulário %q (documentId %d)",
				existing.Description, existing.DocumentID)
		case !s.plan.New:
			return output.Usagef(
				"o formulário da pasta %q não existe no servidor: declare \"new\": true no passo, "+
					"ou aponte o alvo com \"documentId\"/\"formName\"", s.Target)
		case s.plan.ParentID == 0:
			return output.Usagef("o passo do formulário %q precisa de \"parentId\" para criar", s.Target)
		case s.plan.DatasetName == "":
			return output.Usagef("o passo do formulário %q precisa de \"datasetName\" para criar", s.Target)
		default:
			s.Action = "criaria o formulário " + folderKey
		}
		return nil

	case "workflow":
		byEvent, _, err := scriptsDoPassoWorkflow(root, s.Target)
		if err != nil {
			return err
		}
		pid, derived := s.plan.workflowTarget()
		// prepareProcessXML é read-only: aqui ele acusa evento local que NÃO
		// existe no processo, antes de qualquer publicação. Esse erro, hoje, só
		// aparece no meio do publish — com a versão já em jogo.
		_, updated, err := prepareProcessXML(ctx, client, pid, byEvent, derived)
		if err != nil {
			return err
		}
		acao := fmt.Sprintf("criaria uma versão nova de %q com %d evento(s): %s",
			pid, len(updated), strings.Join(updated, ", "))
		if s.plan.NoRelease {
			acao += " (sem liberar)"
		}
		s.Action = acao
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
func runDeployPlan(ctx context.Context, app *App, p *output.Printer, server *config.Server,
	client *fluig.Client, root, planPath string, steps []deployStepResult, from int, noAudit bool) error {
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
		action, err := execDeployStep(ctx, app, p, server, client, root, s, noAudit)
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
func execDeployStep(ctx context.Context, app *App, p *output.Printer, server *config.Server,
	client *fluig.Client, root string, s *deployStepResult, noAudit bool) (string, error) {
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

	case "form":
		opts, err := step.formOptsDoPasso(noAudit)
		if err != nil {
			return "failed", err
		}
		res, err := app.exportOneForm(ctx, p, server, client, root, resolveDeployPath(root, s.Target), opts)
		if err != nil {
			return "failed", err
		}
		return fmt.Sprintf("formulário %q %s (documentId %d)", res.Name, res.Action, res.DocumentID), nil

	case "workflow":
		byEvent, _, err := scriptsDoPassoWorkflow(root, s.Target)
		if err != nil {
			return "failed", err
		}
		pid, derived := step.workflowTarget()
		res, err := publishOneWorkflow(ctx, client, pid, byEvent, step.NoRelease, derived)
		if err != nil {
			return "failed", err
		}
		estado := "criada e liberada"
		if !res.Released {
			estado = "criada em edição"
		}
		return fmt.Sprintf("versão %d do processo %q %s (%d evento(s))",
			res.Version, pid, estado, len(res.Events)), nil

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
