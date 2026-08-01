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
	cmd.AddCommand(newTaskSummaryCmd(app))
	return cmd
}

// --- task summary ---

func newTaskSummaryCmd(app *App) *cobra.Command {
	var (
		user          string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Resumo da central de tarefas: contadores e pools visíveis",
		Long: "Mostra o resumo da central de tarefas: tarefas a concluir, solicitações,\n" +
			"tarefas sob gerência e os pools de grupo/papel que o usuário enxerga, com\n" +
			"a contagem de cada um. Use --user para ver o resumo de outro usuário.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			summary, err := client.TaskCentralSummary(ctx, user)
			if err != nil {
				return mapFluigError(err)
			}
			who := user
			if who == "" {
				who = server.Username
			}
			if len(summary) == 0 {
				p.Infof("A central de tarefas não tem resumo para %q. O Fluig monta o resumo quando o usuário abre a central no portal. Para ver a fila dele, use: fluigcli task list --assignee %s", who, who)
			} else {
				rows := make([][]string, 0, len(summary))
				poolRows := map[int]bool{}
				for _, cat := range summary {
					if cat.Type == "root" {
						continue // "Resumo de Tarefas" — cabeçalho da tela, sempre 0
					}
					rows = append(rows, []string{cat.Description, "", strconv.Itoa(cat.Total)})
					for _, child := range cat.Children {
						poolRows[len(rows)] = true
						rows = append(rows, []string{"  └ " + child.Description, child.Pool, strconv.Itoa(child.Total)})
					}
				}
				p.Table(output.Table{
					Headers: []string{"Categoria", "Pool", "Tarefas"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 1 && poolRows[row] {
							return output.Green(padded)
						}
						return padded
					}),
				})
				if len(poolRows) > 0 {
					p.Infof("Abra um pool com: fluigcli task list --group <código> ou --role <código> (a parte final da coluna Pool)")
				}
			}
			p.Done(map[string]any{"summary": summary})
			return nil
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "login do usuário (default: você)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- task list ---

func newTaskListCmd(app *App) *cobra.Command {
	var (
		assignee      string
		everyone      bool
		group         string
		role          string
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
			"--group ou --role para as tarefas paradas no pool de um grupo ou papel,\n" +
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
			if group != "" && role != "" {
				return output.Usagef("use --group ou --role, não os dois")
			}
			poolCode := ""
			if group != "" {
				poolCode = "Pool:Group:" + group
			} else if role != "" {
				poolCode = "Pool:Role:" + role
			}
			if poolCode != "" && (assignee != "" || everyone || requester != "" || sla != "" || cmd.Flags().Changed("status")) {
				return output.Usagef("--group/--role não combina com --assignee, --everyone, --requester, --sla nem --status: o pool só tem tarefas em aberto, sem responsável")
			}
			ctx := context.Background()
			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			var tasks []fluig.TaskSummary
			who := assignee
			if poolCode != "" {
				// A rota de pool não filtra por processo — com --process o
				// recorte é local, então o limite só entra depois dele.
				fetchLimit := limit
				if process != "" {
					fetchLimit = 0
				}
				tasks, err = client.ListPoolTasks(ctx, poolCode, fetchLimit)
				if err == nil && process != "" {
					kept := tasks[:0]
					for _, tk := range tasks {
						if tk.ProcessID == process {
							kept = append(kept, tk)
						}
					}
					tasks = kept
					if limit > 0 && len(tasks) > limit {
						tasks = tasks[:limit]
					}
				}
			} else {
				if who == "" && !everyone {
					who = server.Username // default: as minhas tarefas
				}
				tasks, err = client.ListTasks(ctx, fluig.TaskFilter{
					Assignee:  who,
					Requester: requester,
					ProcessID: process,
					Status:    st,
					SLAStatus: sl,
					Limit:     limit,
				})
			}
			if err != nil {
				return mapFluigError(err)
			}
			if len(tasks) == 0 {
				if group != "" {
					p.Infof("Nenhuma tarefa parada no pool do grupo %q. 🎉", group)
				} else if role != "" {
					p.Infof("Nenhuma tarefa parada no pool do papel %q. 🎉", role)
				} else if who != "" && st == "NOT_COMPLETED" {
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
	cmd.Flags().StringVar(&group, "group", "", "tarefas paradas no pool de um grupo (código do grupo, ex.: TI)")
	cmd.Flags().StringVar(&role, "role", "", "tarefas paradas no pool de um papel (código do papel, ex.: controladoria)")
	cmd.Flags().StringVar(&status, "status", "not_completed", "status: not_completed, pending_consensus, completed, transferred, canceled ou all")
	cmd.Flags().StringVar(&process, "process", "", "filtra pelo processo (processId)")
	cmd.Flags().StringVar(&requester, "requester", "", "filtra pelo login do solicitante")
	cmd.Flags().StringVar(&sla, "sla", "", "filtra por SLA: on_time, warning ou expired")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de tarefas (0 = todas)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
