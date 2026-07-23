package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/strsim"
)

// processNotFound monta o erro NOT_FOUND de um processo, enriquecido com
// sugestões de processIds próximos e, quando a identidade veio do arquivo/
// argumento local (derived), a dica do --process-id. A lista de processos é
// buscada só aqui (no caminho de erro) e a falha da busca é tolerada — o erro
// básico continua útil.
func processNotFound(ctx context.Context, client *fluig.Client, processID string, derived bool) error {
	msg := fmt.Sprintf("processo %q não encontrado no servidor", processID)
	var extra []string
	if procs, err := client.ListProcesses(ctx); err == nil {
		names := make([]string, 0, len(procs))
		for _, pr := range procs {
			names = append(names, pr.ID)
		}
		if sug := strsim.Suggest(processID, names, 3); len(sug) > 0 {
			quoted := make([]string, len(sug))
			for i, s := range sug {
				quoted[i] = fmt.Sprintf("%q", s)
			}
			extra = append(extra, "talvez: "+strings.Join(quoted, ", "))
		}
	}
	if derived {
		extra = append(extra, "use --process-id se o processId do servidor difere do arquivo/argumento local")
	}
	if len(extra) > 0 {
		msg += " (" + strings.Join(extra, "; ") + ")"
	}
	return output.NotFoundf("%s", msg)
}

func newWorkflowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Processos: listagem, versão e scripts de eventos (import = servidor → local; export = local → servidor)",
	}
	cmd.AddCommand(newWorkflowNewScriptCmd(app))
	cmd.AddCommand(newWorkflowListCmd(app))
	cmd.AddCommand(newWorkflowVersionCmd(app))
	cmd.AddCommand(newWorkflowImportCmd(app))
	cmd.AddCommand(newWorkflowExportCmd(app))
	cmd.AddCommand(newWorkflowDiffCmd(app))
	cmd.AddCommand(newWorkflowPublishCmd(app))
	return cmd
}

// --- workflow import (servidor → local) ---

func newWorkflowImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		eventsFlag    []string
		toStdout      bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <processId>... | --all",
		Short: "Baixa os scripts de eventos de processos para arquivos locais (servidor → local)",
		Long: "Baixa os scripts de eventos de processos do servidor para\n" +
			"workflow/scripts/<Processo>.<evento>.js. Um script local existente do\n" +
			"mesmo evento é sobrescrito no lugar, mesmo em subpasta.\n\n" +
			"A leitura usa o export nativo do processo (não requer o helper)\n" +
			"e traz os eventos da versão mais recente; eventos com script vazio são\n" +
			"ignorados. Com --all, importa os scripts de todos os processos do\n" +
			"servidor — é um export por processo, pode demorar.\n\n" +
			"Use --events para trazer só alguns eventos. Use --stdout para imprimir\n" +
			"os scripts no terminal sem gravar nada — bom para conferir o que está\n" +
			"publicado sem sobrescrever o repositório.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais processIds ou use --all")
			}
			if all && len(args) > 0 {
				return output.Usagef("use processIds ou --all, não os dois")
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}

			pids := args
			if all {
				processes, err := client.ListProcesses(ctx)
				if err != nil {
					return mapFluigError(err)
				}
				pids = nil
				for _, pr := range processes {
					pids = append(pids, pr.ID)
				}
				if len(pids) == 0 {
					p.Infof("Nenhum processo encontrado no servidor.")
				}
			}

			// --stdout: leitura pura, não toca no repositório.
			if toStdout {
				return app.dumpProcessScripts(ctx, client, p, pids, eventsFlag)
			}

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			var results []itemResult
			var lastErr error
			failures := 0
			for _, pid := range pids {
				r, f, perr := app.importProcessScripts(ctx, client, root, pid, eventsFlag)
				results = append(results, r...)
				failures += f
				if perr != nil {
					lastErr = perr
				}
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(pids))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa os scripts de todos os processos do servidor")
	cmd.Flags().StringSliceVar(&eventsFlag, "events", nil, "importa só os eventos indicados (separados por vírgula)")
	cmd.Flags().BoolVar(&toStdout, "stdout", false, "imprime os scripts no stdout em vez de gravar arquivos (não altera o repositório)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// scriptDump é um script publicado devolvido pelo import --stdout (contrato
// --json: data.scripts[]).
type scriptDump struct {
	ProcessID string `json:"processId"`
	Event     string `json:"event"`
	Contents  string `json:"contents"`
}

// dumpProcessScripts imprime no stdout os scripts de eventos dos processos, sem
// gravar arquivo (workflow import --stdout). No modo humano separa os eventos
// por um cabeçalho quando há mais de um; no modo --json devolve data.scripts[].
func (a *App) dumpProcessScripts(ctx context.Context, client *fluig.Client, p *output.Printer, pids, filter []string) error {
	want := eventFilterSet(filter)
	var dumps []scriptDump
	var lastErr error
	failures := 0
	for _, pid := range pids {
		events, err := client.ProcessEventScripts(ctx, pid)
		if err != nil {
			failures++
			lastErr = mapFluigError(err)
			p.Warnf("processo %q: %s", pid, output.AsError(lastErr).Message)
			continue
		}
		for _, ev := range selectedEventNames(events, want) {
			dumps = append(dumps, scriptDump{ProcessID: pid, Event: ev, Contents: events[ev]})
		}
	}

	// Modo humano: despeja o código no stdout (sem status), com cabeçalho por
	// evento só quando há mais de um — assim um único evento sai pipeável.
	if !p.JSON {
		multi := len(dumps) > 1
		for _, d := range dumps {
			if multi {
				p.Successf("// ==> %s.%s.js", d.ProcessID, d.Event)
			}
			p.Successf("%s", strings.TrimRight(d.Contents, "\n"))
		}
	}
	return finishBatch(p, lastErr, map[string]any{"scripts": dumps}, failures, len(pids))
}

// eventFilterSet transforma a lista de --events num conjunto (nil = todos).
func eventFilterSet(filter []string) map[string]bool {
	if len(filter) == 0 {
		return nil
	}
	set := make(map[string]bool, len(filter))
	for _, ev := range filter {
		set[ev] = true
	}
	return set
}

// selectedEventNames devolve os eventos com script (ordenados), filtrados por
// want quando não é nil. Eventos com código vazio são ignorados (igual ao
// import para arquivo).
func selectedEventNames(events map[string]string, want map[string]bool) []string {
	names := make([]string, 0, len(events))
	for ev, code := range events {
		if strings.TrimSpace(code) == "" {
			continue
		}
		if want != nil && !want[ev] {
			continue
		}
		names = append(names, ev)
	}
	sort.Strings(names)
	return names
}

// --- workflow publish ---

func newWorkflowPublishCmd(app *App) *cobra.Command {
	var (
		noRelease     bool
		processIDFlag string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "publish <processId>",
		Short: "Publica uma nova versão do processo com os scripts locais (nativo)",
		Long: "Cria uma NOVA versão do processo no servidor com os scripts de eventos\n" +
			"locais (workflow/scripts/<processId>.*.js) aplicados, e a libera para uso\n" +
			"— tudo pela API nativa, sem o componente auxiliar.\n\n" +
			"Diferença para o workflow export: o export atualiza os scripts na versão\n" +
			"corrente, sem criar versão (bom para desenvolvimento); o publish é o\n" +
			"deploy — sobe versão nova e libera (a versão anterior é desativada).\n\n" +
			"O publish NÃO cria eventos nem processos: scripts locais de eventos que\n" +
			"não existem no processo interrompem o comando antes de qualquer mudança.\n\n" +
			"Use --process-id quando o processId no servidor for diferente do prefixo\n" +
			"do arquivo local. O argumento continua a identificar os scripts locais. A\n" +
			"flag troca apenas o processo de destino no servidor.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			localPrefix := args[0]
			pid := localPrefix
			if processIDFlag != "" {
				pid = processIDFlag
			}
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			scripts, err := project.FindProcessScripts(root, localPrefix)
			if err != nil {
				return err
			}
			if len(scripts) == 0 {
				return output.Usagef("nenhum script local do processo %q (esperado %s/%s.<evento>.js)",
					localPrefix, project.WorkflowScriptsDir, localPrefix)
			}
			events, err := readWorkflowEvents(scripts)
			if err != nil {
				return err
			}
			byEvent := make(map[string]string, len(events))
			for _, e := range events {
				byEvent[e.Name] = e.Contents
			}

			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar uma nova versão do processo")
			if err != nil {
				return err
			}

			xmlData, err := client.ExportProcessXML(ctx, pid)
			if err != nil {
				if errors.Is(err, fluig.ErrNotFound) {
					return processNotFound(ctx, client, pid, processIDFlag == "")
				}
				return mapFluigError(err)
			}
			newXML, updated, missing := fluig.ApplyProcessEventScripts(xmlData, byEvent)
			if len(missing) > 0 {
				return output.NotFoundf(
					"evento(s) %s não existem no processo %q — o publish não cria eventos; crie-os no Fluig Studio (nada foi alterado)",
					strings.Join(missing, ", "), pid)
			}

			before, err := client.ProcessVersions(ctx, pid)
			if err != nil {
				return mapFluigError(err)
			}
			if err := client.ImportProcessXML(ctx, pid, newXML); err != nil {
				return mapFluigError(err)
			}
			after, err := client.ProcessVersions(ctx, pid)
			if err != nil {
				return mapFluigError(err)
			}
			prevVersion, newVersion := fluig.LatestProcessVersion(before), fluig.LatestProcessVersion(after)

			released := false
			if !noRelease {
				if err := client.ReleaseLatestProcessVersion(ctx, pid); err != nil {
					return output.ServerErrorf(
						"a versão %d do processo %q foi criada, mas não pôde ser liberada: %v — corrija o processo no Fluig Studio (ou use --no-release)",
						newVersion, pid, err).WithCause(err)
				}
				released = true
			}

			for _, ev := range updated {
				p.Successf("evento %q aplicado", ev)
			}
			if released {
				p.Successf("versão %d do processo %q criada e liberada (a v%d foi desativada)", newVersion, pid, prevVersion)
			} else {
				p.Successf("versão %d do processo %q criada em edição (libere com o publish sem --no-release ou no Fluig Studio)", newVersion, pid)
			}
			p.Done(map[string]any{
				"processId":       pid,
				"previousVersion": prevVersion,
				"version":         newVersion,
				"released":        released,
				"events":          updated,
			})
			return nil
		},
	}
	cmd.Flags().BoolVar(&noRelease, "no-release", false, "cria a versão nova em edição, sem liberá-la")
	cmd.Flags().StringVar(&processIDFlag, "process-id", "", "processId de destino no servidor, quando diferente do prefixo do arquivo local")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- workflow list ---

func newWorkflowListCmd(app *App) *cobra.Command {
	var activeOnly bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os processos do servidor (nativo, REST v2)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			processes, err := client.ListProcesses(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			shown := processes[:0]
			rows := make([][]string, 0, len(processes))
			for _, pr := range processes {
				if activeOnly && !pr.Active {
					continue
				}
				shown = append(shown, pr)
				ativo := "não"
				if pr.Active {
					ativo = "sim"
				}
				rows = append(rows, []string{pr.ID, pr.Description, pr.Category, ativo})
			}
			if len(shown) == 0 {
				p.Infof("Nenhum processo encontrado no servidor (processos são criados no Fluig Studio).")
			} else {
				// Padrão de listagem (ver CLAUDE.md): tabela com cabeçalho em
				// negrito; "sim" em verde destaca os processos ativos.
				p.Table(output.Table{
					Headers: []string{"ID", "Descrição", "Categoria", "Ativo"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 3 && shown[row].Active {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"processes": shown})
			return nil
		},
	}
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "mostra apenas processos ativos")
	return cmd
}

// --- workflow version ---

func newWorkflowVersionCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "version <processId>",
		Short: "Mostra a última versão de um processo no servidor (nativo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			processID := args[0]
			v, err := client.WorkflowVersion(ctx, processID)
			if err != nil {
				return mapFluigError(err)
			}
			if v == 0 {
				return processNotFound(ctx, client, processID, false)
			}
			p.Successf("%d", v)
			p.Done(map[string]any{"processId": processID, "version": v})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- workflow export ---

func newWorkflowExportCmd(app *App) *cobra.Command {
	var (
		processVersion int
		eventsFlag     []string
		allEvents      bool
		processIDFlag  string
		passwordStdin  bool
	)
	cmd := &cobra.Command{
		Use:   "export <arquivo|processId>",
		Short: "Atualiza scripts de eventos de um processo (via componente auxiliar)",
		Long: "Atualiza cirurgicamente os scripts de eventos de um processo, sem\n" +
			"reimportar o processo inteiro. Requer o componente auxiliar instalado\n" +
			"(server install-helper).\n\n" +
			"Alvos:\n" +
			"  workflow export workflow/scripts/Compras.beforeTaskSave.js   (um evento)\n" +
			"  workflow export Compras --all-events                          (todos os Compras.*.js)\n" +
			"  workflow export Compras --events beforeTaskSave,afterTaskComplete\n\n" +
			"Use --process-id quando o processId no servidor for diferente do prefixo\n" +
			"do arquivo local. O alvo (arquivo ou prefixo) continua a identificar os\n" +
			"scripts locais. A flag troca apenas o processo de destino no servidor.\n" +
			"  workflow export workflow/scripts/SolicitacaoAdiantamento.servicetask88.js \\\n" +
			"      --process-id \"Adiantamento ao Fornecedor\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}

			processID, scripts, err := resolveWorkflowTargets(root, args[0], eventsFlag, allEvents, processIDFlag)
			if err != nil {
				return err
			}
			events, err := readWorkflowEvents(scripts)
			if err != nil {
				return err
			}

			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "atualizar os scripts do processo")
			if err != nil {
				return err
			}

			// Pré-requisito: o helper precisa estar instalado (exit 7).
			installed, err := client.HelperInstalled(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			if !installed {
				return output.MissingHelperf(
					"o componente auxiliar não está instalado em %s; instale com: fluigcli server install-helper %s",
					p.Server, p.Server)
			}

			version := processVersion
			if version == 0 {
				version, err = client.WorkflowVersion(ctx, processID)
				if err != nil {
					return mapFluigError(err)
				}
				if version == 0 {
					return processNotFound(ctx, client, processID, processIDFlag == "")
				}
			}

			res, err := client.UpdateWorkflowEvents(ctx, processID, version, events)
			if err != nil {
				return mapFluigError(err)
			}
			for _, e := range events {
				p.Successf("evento %q atualizado (processo %s v%d)", e.Name, processID, version)
			}
			p.Done(map[string]any{
				"processId": processID,
				"version":   version,
				"updated":   len(events),
				"result":    res,
			})
			return nil
		},
	}
	cmd.Flags().IntVar(&processVersion, "process-version", 0, "versão do processo (default: a última do servidor)")
	cmd.Flags().StringSliceVar(&eventsFlag, "events", nil, "eventos a atualizar (separados por vírgula), quando o alvo é um processId")
	cmd.Flags().BoolVar(&allEvents, "all-events", false, "atualiza todos os scripts do processo (workflow/scripts/<processId>.*.js)")
	cmd.Flags().StringVar(&processIDFlag, "process-id", "", "processId de destino no servidor, quando diferente do prefixo do arquivo local")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- workflow diff ---

func newWorkflowDiffCmd(app *App) *cobra.Command {
	var (
		eventsFlag    []string
		allEvents     bool
		processIDFlag string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "diff <arquivo|processId>",
		Short: "Compara scripts de eventos locais com os publicados no servidor (nada é alterado)",
		Long: "Mostra o diff entre os scripts de eventos locais e o que está publicado\n" +
			"no servidor, sem gravar nada. Companheiro do export/publish: confirma se\n" +
			"o que está no ar é igual ao local.\n\n" +
			"A leitura usa o export nativo do processo (não requer o componente\n" +
			"auxiliar) e traz a versão mais recente. Diferenças só de quebra de linha\n" +
			"(CRLF/LF) não contam.\n\n" +
			"Alvos (iguais aos do export):\n" +
			"  workflow diff workflow/scripts/Compras.beforeTaskSave.js   (um evento)\n" +
			"  workflow diff Compras --all-events                          (todos os Compras.*.js)\n" +
			"  workflow diff Compras --events beforeTaskSave,afterTaskComplete\n\n" +
			"Use --process-id quando o processId no servidor for diferente do prefixo\n" +
			"do arquivo local.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			processID, scripts, err := resolveWorkflowTargets(root, args[0], eventsFlag, allEvents, processIDFlag)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			entries, err := diffProcessScripts(ctx, client, p, root, processID, scripts, false)
			if err != nil {
				return err
			}
			counts := renderDiffEntries(p, entries)
			p.Infof("%d igual(is), %d diferente(s), %d só local(is)",
				counts[diffEqual], counts[diffModified], counts[diffOnlyLocal])
			p.Done(map[string]any{"artifacts": entries, "counts": counts})
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&eventsFlag, "events", nil, "eventos a comparar (separados por vírgula), quando o alvo é um processId")
	cmd.Flags().BoolVar(&allEvents, "all-events", false, "compara todos os scripts do processo (workflow/scripts/<processId>.*.js)")
	cmd.Flags().StringVar(&processIDFlag, "process-id", "", "processId de destino no servidor, quando diferente do prefixo do arquivo local")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// resolveWorkflowTargets decide o processId de destino no servidor e os scripts
// a enviar a partir do argumento (um arquivo .js específico, ou um processId +
// --events/--all-events).
//
// O argumento identifica SEMPRE os arquivos locais. processIDOverride, quando
// não vazio, troca APENAS o processId devolvido para o servidor — a busca local
// continua pelo prefixo do argumento. Isso permite parear um arquivo
// X.<evento>.js com um processId Y ≠ X publicado no servidor (ver ROADMAP 1.7-A).
func resolveWorkflowTargets(root, arg string, eventsFlag []string, allEvents bool, processIDOverride string) (string, []project.ProcessScript, error) {
	serverPID := func(local string) string {
		if processIDOverride != "" {
			return processIDOverride
		}
		return local
	}

	// Caso 1: o argumento é um arquivo .js existente.
	if strings.HasSuffix(arg, ".js") {
		if _, err := os.Stat(arg); err == nil {
			pid, ev, ok := project.ParseWorkflowScriptName(arg)
			if !ok {
				return "", nil, output.Usagef("nome de script inválido %q (esperado <Processo>.<evento>.js)", filepath.Base(arg))
			}
			return serverPID(pid), []project.ProcessScript{{ProcessID: pid, Event: ev, Path: arg}}, nil
		}
		return "", nil, output.NotFoundf("arquivo %q não encontrado", arg)
	}

	// Caso 2: o argumento é um prefixo local; precisa de --all-events ou --events.
	localPrefix := arg
	all, err := project.FindProcessScripts(root, localPrefix)
	if err != nil {
		return "", nil, err
	}
	if allEvents {
		if len(all) == 0 {
			return "", nil, output.NotFoundf("nenhum script encontrado em %s/%s.*.js", project.WorkflowScriptsDir, localPrefix)
		}
		return serverPID(localPrefix), all, nil
	}
	if len(eventsFlag) > 0 {
		byEvent := make(map[string]project.ProcessScript, len(all))
		for _, s := range all {
			byEvent[s.Event] = s
		}
		var selected []project.ProcessScript
		for _, ev := range eventsFlag {
			s, ok := byEvent[ev]
			if !ok {
				return "", nil, output.NotFoundf("script do evento %q não encontrado (%s/%s.%s.js)", ev, project.WorkflowScriptsDir, localPrefix, ev)
			}
			selected = append(selected, s)
		}
		return serverPID(localPrefix), selected, nil
	}
	return "", nil, output.Usagef("informe um arquivo .js, ou um processId com --all-events ou --events a,b")
}

// writeProcessScript grava o código de um evento de processo: sobrescreve o
// script local existente (paths já encontrados sob workflow/scripts, recursivo)
// ou cria em workflow/scripts/<processId>.<evento>.js.
// importProcessScripts baixa os scripts de eventos de UM processo e grava nos
// arquivos locais, devolvendo um resultado por script (compartilhado entre o
// workflow import e o clone). Falha do processo inteiro (export indisponível)
// vira um único resultado failed com o id do processo.
func (a *App) importProcessScripts(ctx context.Context, client *fluig.Client, root, pid string, filter []string) (results []itemResult, failures int, lastErr error) {
	p := a.printer
	failProcess := func(err error) ([]itemResult, int, error) {
		mapped := mapFluigError(err)
		p.Warnf("processo %q: %s", pid, output.AsError(mapped).Message)
		return []itemResult{{ID: pid, Action: "failed", Success: false, Error: output.AsError(mapped).Message}}, 1, mapped
	}
	events, err := client.ProcessEventScripts(ctx, pid)
	if err != nil {
		return failProcess(err)
	}
	locals, err := project.FindProcessScripts(root, pid)
	if err != nil {
		return failProcess(err)
	}
	byEvent := make(map[string][]string, len(locals))
	for _, s := range locals {
		byEvent[s.Event] = append(byEvent[s.Event], s.Path)
	}

	// O export traz o registro de todo evento do processo; eventos sem script
	// (código vazio) não viram arquivo. --events restringe o conjunto.
	names := selectedEventNames(events, eventFilterSet(filter))
	if len(names) == 0 {
		p.Infof("processo %q não tem scripts de eventos no servidor.", pid)
		return nil, 0, nil
	}
	for _, ev := range names {
		id := pid + "." + ev
		action, werr := writeProcessScript(p, root, pid, ev, events[ev], byEvent[ev])
		if werr != nil {
			failures++
			lastErr = werr
			results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(werr).Message})
			p.Warnf("script %q: %s", id, output.AsError(werr).Message)
			continue
		}
		results = append(results, itemResult{ID: id, Action: action, Success: true})
		p.Successf("script %q %s", id, action)
	}
	return results, failures, lastErr
}

func writeProcessScript(p *output.Printer, root, processID, event, code string, existing []string) (action string, err error) {
	action = "updated"
	path := ""
	if len(existing) > 0 {
		path = existing[0]
		if len(existing) > 1 {
			p.Warnf("%s.%s: %d arquivos com esse nome; sobrescrevendo %s", processID, event, len(existing), path)
		}
	} else {
		path, err = project.SafeJoin(filepath.Join(root, project.WorkflowScriptsDir), processID+"."+event+".js")
		if err != nil {
			return "failed", err
		}
		action = "created"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "failed", err
	}
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		return "failed", err
	}
	return action, nil
}

func readWorkflowEvents(scripts []project.ProcessScript) ([]fluig.WorkflowEvent, error) {
	events := make([]fluig.WorkflowEvent, 0, len(scripts))
	for _, s := range scripts {
		content, err := os.ReadFile(s.Path)
		if err != nil {
			return nil, err
		}
		events = append(events, fluig.WorkflowEvent{Name: s.Event, Contents: string(content)})
	}
	return events, nil
}
