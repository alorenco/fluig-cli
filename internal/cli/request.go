package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	cmd.AddCommand(newRequestStartCmd(app))
	cmd.AddCommand(newRequestMoveCmd(app))
	cmd.AddCommand(newRequestAssigneesCmd(app))
	return cmd
}

// parseFormFields converte flags --field campo=valor no mapa da API.
func parseFormFields(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, f := range raw {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, output.Usagef("--field inválido %q (use campo=valor)", f)
		}
		out[k] = v
	}
	return out, nil
}

// reportMoveResult trata o resultado de start/move: sucesso ou a exigência de
// escolher responsável (HTTP 412 — nada foi movimentado).
func reportMoveResult(p *output.Printer, res *fluig.MoveResult, successMsg string) error {
	if res.NeedsAssignee {
		for _, u := range res.PossibleAssignees {
			p.Infof("  - %s", requestUserLabel(&u))
		}
		return output.Usagef("a próxima atividade exige escolher o responsável — repita com --assignee <login> (opções acima)")
	}
	p.Successf("%s", successMsg)
	p.Done(map[string]any{"result": res})
	return nil
}

// --- request start ---

func newRequestStartCmd(app *App) *cobra.Command {
	var (
		fields        []string
		attach        []string
		comment       string
		targetState   int
		assignee      string
		noSend        bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "start <processId>",
		Short: "Inicia uma solicitação do processo (nativo)",
		Long: "Inicia (abre e envia) uma solicitação do processo, preenchendo o\n" +
			"formulário com os --field dados. Os eventos do processo rodam no servidor\n" +
			"normalmente (validateForm pode rejeitar — a mensagem é repassada).\n\n" +
			"Com --attach, os arquivos vão como anexos da solicitação — necessário nos\n" +
			"processos que exigem anexo no início (a REST v2 não tem upload de anexo;\n" +
			"nesse modo a CLI usa o SOAP startProcess, que exige --target-state).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			formFields, err := parseFormFields(fields)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "iniciar uma solicitação")
			if err != nil {
				return err
			}

			opts := fluig.RequestStartOptions{
				TargetState:    targetState,
				TargetAssignee: assignee,
				Comment:        comment,
				FormFields:     formFields,
				NoSend:         noSend,
			}

			// Com anexos ou --no-send: SOAP startProcess (a REST v2 não tem
			// upload de anexo nem "salvar sem enviar").
			if len(attach) > 0 || noSend {
				atts := make([]fluig.RequestAttachment, 0, len(attach))
				for _, path := range attach {
					content, rerr := os.ReadFile(path)
					if rerr != nil {
						if os.IsNotExist(rerr) {
							return output.NotFoundf("anexo %q não encontrado", path)
						}
						return rerr
					}
					atts = append(atts, fluig.RequestAttachment{FileName: filepath.Base(path), Content: content})
				}
				id, _, serr := client.StartRequestWithAttachments(ctx, args[0], opts, atts)
				if serr != nil {
					return mapFluigError(serr)
				}
				detail := fmt.Sprintf("%d anexo(s)", len(atts))
				if noSend {
					detail += ", sem enviar — está na atividade inicial com você"
				}
				p.Successf("solicitação %d criada (%s, %s)", id, args[0], detail)
				p.Done(map[string]any{"requestId": id, "processId": args[0], "attachments": len(atts), "sent": !noSend})
				return nil
			}

			res, err := client.StartRequest(ctx, args[0], opts)
			if err != nil {
				return mapFluigError(err)
			}
			return reportMoveResult(p, res, fmt.Sprintf("solicitação %d criada (%s → etapa %q)",
				res.RequestID, res.ProcessID, res.NextStateName))
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "campo do formulário: campo=valor (pode repetir)")
	cmd.Flags().StringArrayVar(&attach, "attach", nil, "arquivo para anexar à solicitação (pode repetir)")
	cmd.Flags().StringVar(&comment, "comment", "", "comentário do movimento")
	cmd.Flags().IntVar(&targetState, "target-state", 0, "etapa de destino (sequence; default: o fluxo do diagrama)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "login do responsável pela próxima atividade")
	cmd.Flags().BoolVar(&noSend, "no-send", false, "cria a solicitação sem enviá-la (fica na atividade inicial, com você)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- request move ---

func newRequestMoveCmd(app *App) *cobra.Command {
	var (
		fields        []string
		comment       string
		targetState   int
		assignee      string
		movement      int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "move <número>",
		Short: "Movimenta uma solicitação para a próxima etapa (nativo, REST v2)",
		Long: "Conclui a tarefa corrente da solicitação e a envia adiante. Sem\n" +
			"--movement, a CLI descobre a tarefa em aberto sozinha (obrigatório\n" +
			"informar quando houver mais de uma, ex.: atividades paralelas).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			id, err := strconv.Atoi(args[0])
			if err != nil || id <= 0 {
				return output.Usagef("número de solicitação inválido %q", args[0])
			}
			formFields, err := parseFormFields(fields)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "movimentar uma solicitação")
			if err != nil {
				return err
			}

			seq := movement
			if seq == 0 {
				req, gerr := client.GetRequest(ctx, id)
				if gerr != nil {
					return mapFluigError(gerr)
				}
				switch len(req.CurrentSteps) {
				case 0:
					return output.Usagef("a solicitação %d não tem tarefa em aberto (status %s)", id, req.Status)
				case 1:
					seq = req.CurrentSteps[0].Movement
				default:
					for _, s := range req.CurrentSteps {
						p.Infof("  - movimento %d: %s", s.Movement, s.StateName)
					}
					return output.Usagef("a solicitação %d tem %d tarefas em aberto — escolha com --movement (opções acima)", id, len(req.CurrentSteps))
				}
			}

			res, err := client.MoveRequestTo(ctx, id, fluig.RequestMoveOptions{
				MovementSequence: seq,
				TargetState:      targetState,
				TargetAssignee:   assignee,
				Comment:          comment,
				FormFields:       formFields,
			})
			if err != nil {
				return mapFluigError(err)
			}
			// O 200 real pode vir com nextStateName vazio (validado na homolog).
			dest := fmt.Sprintf("etapa %d", res.NextState)
			if res.NextStateName != "" {
				dest = fmt.Sprintf("etapa %q (seq %d)", res.NextStateName, res.NextState)
			}
			return reportMoveResult(p, res, fmt.Sprintf("solicitação %d movimentada → %s", id, dest))
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "campo do formulário a atualizar: campo=valor (pode repetir)")
	cmd.Flags().StringVar(&comment, "comment", "", "comentário do movimento")
	cmd.Flags().IntVar(&targetState, "target-state", 0, "etapa de destino (sequence; default: o fluxo do diagrama)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "login do responsável pela próxima atividade")
	cmd.Flags().IntVar(&movement, "movement", 0, "movimento (tarefa) a concluir, quando houver mais de um em aberto")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- request assignees ---

func newRequestAssigneesCmd(app *App) *cobra.Command {
	var (
		targetState   int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "assignees <número>",
		Short: "Lista quem pode assumir a próxima atividade da solicitação",
		Long: "Lista os possíveis responsáveis pela próxima atividade. Quando o diagrama\n" +
			"oferece mais de um destino, o servidor exige a etapa: use --target-state\n" +
			"(sequence — veja request show ou workflow list).",
		Args: cobra.ExactArgs(1),
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
			users, err := client.PossibleAssignees(ctx, id, targetState)
			if err != nil {
				return mapFluigError(err)
			}
			if len(users) == 0 {
				p.Infof("Nenhum responsável possível para a próxima atividade (pode ser automática).")
			} else {
				rows := make([][]string, 0, len(users))
				for _, u := range users {
					rows = append(rows, []string{u.Login, u.Name})
				}
				p.Table(output.Table{
					Headers: []string{"Login", "Nome"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"assignees": users})
			return nil
		},
	}
	cmd.Flags().IntVar(&targetState, "target-state", 0, "etapa de destino (sequence), quando o diagrama tem mais de uma saída")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
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
