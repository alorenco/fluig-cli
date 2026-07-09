package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newWorkflowCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Processos: listagem, versão e deploy de scripts de eventos (export = local → servidor)",
	}
	cmd.AddCommand(newWorkflowListCmd(app))
	cmd.AddCommand(newWorkflowVersionCmd(app))
	cmd.AddCommand(newWorkflowExportCmd(app))
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
				return output.NotFoundf("processo %q não encontrado no servidor", processID)
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
		passwordStdin  bool
	)
	cmd := &cobra.Command{
		Use:   "export <arquivo|processId>",
		Short: "Atualiza scripts de eventos de um processo (via fluiggersWidget)",
		Long: "Atualiza cirurgicamente os scripts de eventos de um processo, sem\n" +
			"reimportar o processo inteiro. Requer a fluiggersWidget instalada\n" +
			"(server install-helper).\n\n" +
			"Alvos:\n" +
			"  workflow export workflow/scripts/Compras.beforeTaskSave.js   (um evento)\n" +
			"  workflow export Compras --all-events                          (todos os Compras.*.js)\n" +
			"  workflow export Compras --events beforeTaskSave,afterTaskComplete",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}

			processID, scripts, err := resolveWorkflowTargets(root, args[0], eventsFlag, allEvents)
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

			// Pré-requisito: a widget precisa estar instalada (exit 7).
			installed, err := client.HelperInstalled(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			if !installed {
				return output.MissingHelperf(
					"a fluiggersWidget não está instalada em %s; instale com: fluigcli server install-helper %s",
					p.Server, p.Server)
			}

			version := processVersion
			if version == 0 {
				version, err = client.WorkflowVersion(ctx, processID)
				if err != nil {
					return mapFluigError(err)
				}
				if version == 0 {
					return output.NotFoundf("processo %q não encontrado no servidor", processID)
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
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// resolveWorkflowTargets decide o processId e os scripts a enviar a partir do
// argumento (um arquivo .js específico, ou um processId + --events/--all-events).
func resolveWorkflowTargets(root, arg string, eventsFlag []string, allEvents bool) (string, []project.ProcessScript, error) {
	// Caso 1: o argumento é um arquivo .js existente.
	if strings.HasSuffix(arg, ".js") {
		if _, err := os.Stat(arg); err == nil {
			pid, ev, ok := project.ParseWorkflowScriptName(arg)
			if !ok {
				return "", nil, output.Usagef("nome de script inválido %q (esperado <Processo>.<evento>.js)", filepath.Base(arg))
			}
			return pid, []project.ProcessScript{{ProcessID: pid, Event: ev, Path: arg}}, nil
		}
		return "", nil, output.NotFoundf("arquivo %q não encontrado", arg)
	}

	// Caso 2: o argumento é um processId; precisa de --all-events ou --events.
	processID := arg
	all, err := project.FindProcessScripts(root, processID)
	if err != nil {
		return "", nil, err
	}
	if allEvents {
		if len(all) == 0 {
			return "", nil, output.NotFoundf("nenhum script encontrado em %s/%s.*.js", project.WorkflowScriptsDir, processID)
		}
		return processID, all, nil
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
				return "", nil, output.NotFoundf("script do evento %q não encontrado (%s/%s.%s.js)", ev, project.WorkflowScriptsDir, processID, ev)
			}
			selected = append(selected, s)
		}
		return processID, selected, nil
	}
	return "", nil, output.Usagef("informe um arquivo .js, ou um processId com --all-events ou --events a,b")
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
