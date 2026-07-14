package cli

import (
	"context"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newTaskCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Tarefas de workflow: a sua fila e a dos outros usuários",
	}
	cmd.AddCommand(newTaskListCmd(app))
	return cmd
}

// --- task list ---

func newTaskListCmd(app *App) *cobra.Command {
	var (
		assignee      string
		everyone      bool
		status        string
		process       string
		requester     string
		sla           string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista tarefas de workflow (nativo, REST v2)",
		Long: "Sem flags, lista as SUAS tarefas em aberto (\"o que está comigo?\").\n" +
			"Use --assignee para ver a fila de outro usuário, --everyone para todos,\n" +
			"e --status para outros estados (completed, transferred... ou all).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			st, err := normalizeEnum("--status", status,
				"NOT_COMPLETED", "PENDING_CONSENSUS", "COMPLETED", "TRANSFERRED", "CANCELED", "ALL")
			if err != nil {
				return err
			}
			if st == "ALL" {
				st = ""
			}
			sl, err := normalizeEnum("--sla", sla, "ON_TIME", "WARNING", "EXPIRED")
			if err != nil {
				return err
			}
			if everyone && assignee != "" {
				return output.Usagef("use --assignee ou --everyone, não os dois")
			}
			ctx := context.Background()
			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			who := assignee
			if who == "" && !everyone {
				who = server.Username // default: as minhas tarefas
			}
			tasks, err := client.ListTasks(ctx, fluig.TaskFilter{
				Assignee:  who,
				Requester: requester,
				ProcessID: process,
				Status:    st,
				SLAStatus: sl,
				Limit:     limit,
			})
			if err != nil {
				return mapFluigError(err)
			}
			if len(tasks) == 0 {
				if who != "" && st == "NOT_COMPLETED" {
					p.Infof("Nenhuma tarefa em aberto para %q. 🎉", who)
				} else {
					p.Infof("Nenhuma tarefa encontrada com esses filtros.")
				}
			} else {
				rows := make([][]string, 0, len(tasks))
				for _, tk := range tasks {
					rows = append(rows, []string{
						strconv.Itoa(tk.RequestID), tk.ProcessID, tk.StateName,
						requestUserLabel(tk.Assignee), requestUserLabel(tk.Requester),
						tk.Status, tk.SLAStatus, fmtRequestTime(tk.StartDate),
					})
				}
				// Padrão de listagem (ver CLAUDE.md): em aberto em verde.
				p.Table(output.Table{
					Headers: []string{"Solicitação", "Processo", "Etapa", "Responsável", "Solicitante", "Status", "SLA", "Início"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 5 && tasks[row].Status == "NOT_COMPLETED" {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"tasks": tasks})
			return nil
		},
	}
	cmd.Flags().StringVar(&assignee, "assignee", "", "login do responsável (default: você)")
	cmd.Flags().BoolVar(&everyone, "everyone", false, "tarefas de todos os usuários (sem filtro de responsável)")
	cmd.Flags().StringVar(&status, "status", "not_completed", "status: not_completed, pending_consensus, completed, transferred, canceled ou all")
	cmd.Flags().StringVar(&process, "process", "", "filtra pelo processo (processId)")
	cmd.Flags().StringVar(&requester, "requester", "", "filtra pelo login do solicitante")
	cmd.Flags().StringVar(&sla, "sla", "", "filtra por SLA: on_time, warning ou expired")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de tarefas (0 = todas)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
