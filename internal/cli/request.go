package cli

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newRequestCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "request",
		Short: "Solicitações de workflow: consulta e acompanhamento",
	}
	cmd.AddCommand(newRequestListCmd(app))
	cmd.AddCommand(newRequestShowCmd(app))
	return cmd
}

// requestUserLabel formata solicitante/responsável: "Nome (login)"; contas de
// sistema (System:Auto) só têm code.
func requestUserLabel(u *fluig.RequestUser) string {
	switch {
	case u == nil:
		return ""
	case u.Name != "" && u.Login != "":
		return u.Name + " (" + u.Login + ")"
	case u.Name != "":
		return u.Name
	case u.Login != "":
		return u.Login
	default:
		return u.Code
	}
}

// requestSteps junta os nomes das etapas correntes ("" para finalizada).
func requestSteps(r fluig.Request) string {
	names := make([]string, 0, len(r.CurrentSteps))
	for _, s := range r.CurrentSteps {
		names = append(names, s.StateName)
	}
	return strings.Join(names, " + ")
}

func fmtRequestTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// --- request list ---

func newRequestListCmd(app *App) *cobra.Command {
	var (
		process       string
		status        string
		sla           string
		assignee      string
		requester     string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Busca solicitações do servidor (nativo, REST v2)",
		Long: "Busca solicitações de workflow com filtros. Sem filtros, lista as\n" +
			"solicitações mais recentes (--limit controla quantas; 0 = todas).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			st, err := normalizeEnum("--status", status, "OPEN", "CANCELED", "FINALIZED")
			if err != nil {
				return err
			}
			sl, err := normalizeEnum("--sla", sla, "ON_TIME", "WARNING", "EXPIRED")
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			requests, err := client.ListRequests(ctx, fluig.RequestFilter{
				ProcessID: process,
				Status:    st,
				SLAStatus: sl,
				Assignee:  assignee,
				Requester: requester,
				Limit:     limit,
			})
			if err != nil {
				return mapFluigError(err)
			}
			if len(requests) == 0 {
				p.Infof("Nenhuma solicitação encontrada com esses filtros.")
			} else {
				rows := make([][]string, 0, len(requests))
				for _, r := range requests {
					rows = append(rows, []string{
						strconv.Itoa(r.ID), r.ProcessID, requestSteps(r),
						r.Status, r.SLAStatus,
						requestUserLabel(r.Requester), fmtRequestTime(r.StartDate),
					})
				}
				// Padrão de listagem (ver CLAUDE.md): OPEN em verde — são as
				// solicitações em andamento.
				p.Table(output.Table{
					Headers: []string{"Nº", "Processo", "Etapa atual", "Status", "SLA", "Solicitante", "Início"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 3 && requests[row].Status == "OPEN" {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"requests": requests})
			return nil
		},
	}
	cmd.Flags().StringVar(&process, "process", "", "filtra pelo processo (processId)")
	cmd.Flags().StringVar(&status, "status", "", "filtra por status: open, canceled ou finalized")
	cmd.Flags().StringVar(&sla, "sla", "", "filtra por SLA: on_time, warning ou expired")
	cmd.Flags().StringVar(&assignee, "assignee", "", "filtra pelo login do responsável atual")
	cmd.Flags().StringVar(&requester, "requester", "", "filtra pelo login do solicitante")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de solicitações (0 = todas)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// normalizeEnum aceita o valor em qualquer caixa e valida contra as opções.
func normalizeEnum(flag, value string, allowed ...string) (string, error) {
	if value == "" {
		return "", nil
	}
	up := strings.ToUpper(value)
	for _, a := range allowed {
		if up == a {
			return a, nil
		}
	}
	return "", output.Usagef("%s inválido %q (use %s)", flag, value, strings.ToLower(strings.Join(allowed, ", ")))
}

// --- request show ---

func newRequestShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <número>",
		Short: "Mostra uma solicitação com o histórico de movimentação (nativo, REST v2)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			id, err := strconv.Atoi(args[0])
			if err != nil || id <= 0 {
				return output.Usagef("número de solicitação inválido %q", args[0])
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			req, err := client.GetRequest(ctx, id)
			if err != nil {
				return mapFluigError(err)
			}
			tasks, err := client.RequestTasks(ctx, id)
			if err != nil {
				return mapFluigError(err)
			}

			p.Successf("Solicitação %d — %s v%d (%s)", req.ID, req.ProcessID, req.ProcessVersion, req.ProcessDescription)
			p.Successf("Status: %s (SLA %s)", req.Status, req.SLAStatus)
			if req.Requester != nil {
				p.Successf("Solicitante: %s", requestUserLabel(req.Requester))
			}
			if req.StartDate != nil {
				period := fmtRequestTime(req.StartDate)
				if req.EndDate != nil {
					period += " → " + fmtRequestTime(req.EndDate)
				}
				p.Successf("Período: %s", period)
			}
			for _, s := range req.CurrentSteps {
				p.Successf("Etapa atual: %s (seq %d, SLA %s)", s.StateName, s.Sequence, s.SLAStatus)
			}
			if len(tasks) > 0 {
				rows := make([][]string, 0, len(tasks))
				for _, tk := range tasks {
					rows = append(rows, []string{
						strconv.Itoa(tk.Movement), tk.StateName, requestUserLabel(tk.Assignee),
						tk.Status, tk.SLAStatus, fmtRequestTime(tk.StartDate), fmtRequestTime(tk.EndDate),
					})
				}
				// Tarefa em aberto em verde — é onde a solicitação está agora.
				p.Table(output.Table{
					Headers: []string{"Mov", "Etapa", "Responsável", "Status", "SLA", "Início", "Fim"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 3 && tasks[row].Status == "NOT_COMPLETED" {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"request": req, "tasks": tasks})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
