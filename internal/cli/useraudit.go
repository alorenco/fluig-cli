package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// --- user audit ---

// auditDimensions são as dimensões de atuação que o comando reúne.
var auditDimensions = []string{"tasks", "requests", "documents"}

func newUserAuditCmd(app *App) *cobra.Command {
	var (
		day           string
		from          string
		to            string
		only          string
		outPath       string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "audit <login>",
		Short: "Atuação de um usuário num período (tarefas, solicitações, documentos)",
		Long: "Reúne a atuação de um usuário num intervalo de datas:\n" +
			"  • tarefas de workflow que ele concluiu (com horário);\n" +
			"  • solicitações que ele abriu;\n" +
			"  • documentos que ele criou no GED.\n\n" +
			"Sem --day/--from/--to, audita HOJE. Datas em dd/mm/aaaa ou aaaa-mm-dd.\n" +
			"Use --only para restringir as dimensões (ex.: --only tasks,requests).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			login := args[0]
			p := app.printerFor(cmd)

			fromDay, toDay, err := resolveAuditPeriod(day, from, to)
			if err != nil {
				return err
			}
			dims, err := parseAuditOnly(only)
			if err != nil {
				return err
			}
			outFormat, err := auditOutputFormat(outPath)
			if err != nil {
				return err
			}

			ctx := context.Background()
			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}

			// Resolve login → usuário (nome para o cabeçalho + userCode para o
			// dataset de documentos). Login inexistente = exit 4 limpo.
			user, err := client.FindUserByLogin(ctx, login)
			if err != nil {
				return mapFluigError(err)
			}

			// Strings de data para os filtros server-side (sem offset — o
			// servidor aplica o próprio fuso; validado na homologação).
			startStr := fromDay.Format("2006-01-02") + "T00:00:00"
			endStr := toDay.Format("2006-01-02") + "T23:59:59"

			var (
				tasks []fluig.TaskSummary
				reqs  []fluig.Request
				docs  []fluig.AuditDocument
			)
			if dims["tasks"] {
				// Status vazio + filtro de encerramento = tudo que ele encerrou
				// no período (concluído/transferido/cancelado).
				tasks, err = client.ListTasks(ctx, fluig.TaskFilter{
					Assignee:      login,
					AssignEndFrom: startStr,
					AssignEndTo:   endStr,
				})
				if err != nil {
					return mapFluigError(err)
				}
			}
			if dims["requests"] {
				reqs, err = client.ListRequests(ctx, fluig.RequestFilter{
					Requester: login,
					StartFrom: startStr,
					StartTo:   endStr,
				})
				if err != nil {
					return mapFluigError(err)
				}
			}
			if dims["documents"] {
				docs, err = client.DocumentsCreatedBy(ctx, user.Code, fromDay, toDay)
				if err != nil {
					return mapFluigError(err)
				}
			}

			// Ordenação cronológica (pedido do mantenedor): tarefas por
			// conclusão, solicitações por abertura, documentos por criação.
			sort.SliceStable(tasks, func(i, j int) bool { return auditBefore(tasks[i].EndDate, tasks[j].EndDate) })
			sort.SliceStable(reqs, func(i, j int) bool { return auditBefore(reqs[i].StartDate, reqs[j].StartDate) })
			sort.SliceStable(docs, func(i, j int) bool {
				if !auditSameTime(docs[i].CreatedAt, docs[j].CreatedAt) {
					return auditBefore(docs[i].CreatedAt, docs[j].CreatedAt)
				}
				return docs[i].DocumentID < docs[j].DocumentID
			})

			renderUserAudit(p, server.Name, login, user, fromDay, toDay, dims, tasks, reqs, docs)

			if outFormat != "" {
				if err := writeAuditFile(outPath, outFormat, server.Name, login, user, fromDay, toDay, dims, tasks, reqs, docs); err != nil {
					return err
				}
				p.Successf("Auditoria salva em %s", outPath)
			}

			p.Done(map[string]any{
				"user":      map[string]string{"login": login, "name": user.FullName, "code": user.Code},
				"from":      fromDay.Format("2006-01-02"),
				"to":        toDay.Format("2006-01-02"),
				"tasks":     tasks,
				"requests":  reqs,
				"documents": docs,
				"totals":    map[string]int{"tasks": len(tasks), "requests": len(reqs), "documents": len(docs)},
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&day, "day", "", "audita um único dia (dd/mm/aaaa ou aaaa-mm-dd)")
	cmd.Flags().StringVar(&from, "from", "", "início do período (dd/mm/aaaa ou aaaa-mm-dd)")
	cmd.Flags().StringVar(&to, "to", "", "fim do período (dd/mm/aaaa ou aaaa-mm-dd)")
	cmd.Flags().StringVar(&only, "only", "", "dimensões a incluir: tasks,requests,documents (default: todas)")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "salva a auditoria em arquivo .txt ou .xlsx")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// auditOutputFormat valida a extensão do --output e devolve "txt"/"xlsx" (ou ""
// quando não há --output).
func auditOutputFormat(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt":
		return "txt", nil
	case ".xlsx":
		return "xlsx", nil
	default:
		return "", output.Usagef("--output deve terminar em .txt ou .xlsx")
	}
}

func auditBefore(a, b *time.Time) bool {
	switch {
	case a == nil:
		return false
	case b == nil:
		return true
	default:
		return a.Before(*b)
	}
}

func auditSameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// processLabel prefere a descrição do processo ao id interno (pedido do
// mantenedor); cai no id quando a descrição vem vazia.
func processLabel(description, id string) string {
	if strings.TrimSpace(description) != "" {
		return description
	}
	return id
}

// resolveAuditPeriod converte as flags de data no intervalo [from, to] (dias em
// UTC). Sem nenhuma, audita hoje. --day tem precedência; --to default = --from.
func resolveAuditPeriod(day, from, to string) (time.Time, time.Time, error) {
	if day != "" {
		if from != "" || to != "" {
			return time.Time{}, time.Time{}, output.Usagef("use --day OU --from/--to, não os dois")
		}
		d, err := parseAuditDate(day)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return d, d, nil
	}
	if from == "" && to == "" {
		now := time.Now().UTC()
		d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return d, d, nil
	}
	var f, t time.Time
	var err error
	if from != "" {
		if f, err = parseAuditDate(from); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if to != "" {
		if t, err = parseAuditDate(to); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if from == "" {
		f = t // só --to → um dia
	}
	if to == "" {
		t = f // só --from → um dia
	}
	if t.Before(f) {
		return time.Time{}, time.Time{}, output.Usagef("--to (%s) é anterior a --from (%s)", to, from)
	}
	return f, t, nil
}

// parseAuditDate aceita dd/mm/aaaa e aaaa-mm-dd; devolve o dia à meia-noite UTC.
func parseAuditDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"02/01/2006", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, output.Usagef("data inválida %q (use dd/mm/aaaa ou aaaa-mm-dd)", s)
}

// parseAuditOnly interpreta --only (default: todas as dimensões).
func parseAuditOnly(only string) (map[string]bool, error) {
	dims := map[string]bool{}
	if strings.TrimSpace(only) == "" {
		for _, d := range auditDimensions {
			dims[d] = true
		}
		return dims, nil
	}
	valid := map[string]bool{"tasks": true, "requests": true, "documents": true}
	for _, part := range strings.Split(only, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if !valid[part] {
			return nil, output.Usagef("dimensão inválida em --only: %q (use tasks, requests ou documents)", part)
		}
		dims[part] = true
	}
	if len(dims) == 0 {
		return nil, output.Usagef("--only não selecionou nenhuma dimensão")
	}
	return dims, nil
}

// fmtAuditTime formata um instante com data e hora (segundos) — o mantenedor
// pediu o horário junto da data. nil → "—".
func fmtAuditTime(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

// fmtAuditDate formata só a data (documentos têm createDate sem hora).
func fmtAuditDate(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02")
}

func renderUserAudit(p *output.Printer, serverName, login string, user *fluig.User, from, to time.Time, dims map[string]bool, tasks []fluig.TaskSummary, reqs []fluig.Request, docs []fluig.AuditDocument) {
	name := user.FullName
	if name == "" {
		name = login
	}
	p.Infof("Auditoria de %s (%s) — %s — %s", name, login, serverName, auditPeriodLabel(from, to))

	if dims["tasks"] {
		p.Infof("\nTarefas atuadas (%d)", len(tasks))
		if len(tasks) == 0 {
			p.Infof("  nenhuma tarefa concluída no período.")
		} else {
			p.Table(output.Table{
				Headers: auditTaskHeaders,
				Rows:    auditTaskRows(tasks),
				Style:   output.BoldHeaderStyle(nil),
			})
		}
	}

	if dims["requests"] {
		p.Infof("\nSolicitações abertas (%d)", len(reqs))
		if len(reqs) == 0 {
			p.Infof("  nenhuma solicitação aberta no período.")
		} else {
			p.Table(output.Table{
				Headers: auditRequestHeaders,
				Rows:    auditRequestRows(reqs),
				Style:   output.BoldHeaderStyle(nil),
			})
		}
	}

	if dims["documents"] {
		p.Infof("\nDocumentos criados (%d)", len(docs))
		if len(docs) == 0 {
			p.Infof("  nenhum documento criado no período.")
		} else {
			p.Table(output.Table{
				Headers: auditDocHeaders,
				Rows:    auditDocRows(docs),
				Style:   output.BoldHeaderStyle(nil),
			})
		}
	}

	p.Infof("\nResumo: %d tarefa(s) · %d solicitação(ões) · %d documento(s)",
		len(tasks), len(reqs), len(docs))
}

// --- cabeçalhos e linhas (compartilhados entre tela e arquivo) ---

var (
	auditTaskHeaders    = []string{"Conclusão", "Início", "Nº", "Processo", "Etapa", "Status"}
	auditRequestHeaders = []string{"Abertura", "Nº", "Processo", "Status", "Etapa atual"}
	auditDocHeaders     = []string{"Criado em", "Documento", "Tipo", "Descrição"}
)

func auditTaskRows(tasks []fluig.TaskSummary) [][]string {
	rows := make([][]string, 0, len(tasks))
	for _, tk := range tasks {
		rows = append(rows, []string{
			fmtAuditTime(tk.EndDate), fmtAuditTime(tk.StartDate),
			strconv.Itoa(tk.RequestID), processLabel(tk.ProcessDescription, tk.ProcessID),
			tk.StateName, tk.Status,
		})
	}
	return rows
}

func auditRequestRows(reqs []fluig.Request) [][]string {
	rows := make([][]string, 0, len(reqs))
	for _, r := range reqs {
		rows = append(rows, []string{
			fmtAuditTime(r.StartDate), strconv.Itoa(r.ID),
			processLabel(r.ProcessDescription, r.ProcessID), r.Status, requestSteps(r),
		})
	}
	return rows
}

func auditDocRows(docs []fluig.AuditDocument) [][]string {
	rows := make([][]string, 0, len(docs))
	for _, d := range docs {
		desc := d.Description
		if d.Deleted {
			desc += " (excluído)"
		}
		typ := d.TypeLabel
		if typ == "" {
			typ = d.Type
		}
		rows = append(rows, []string{
			fmtAuditDate(d.CreatedAt), strconv.FormatInt(d.DocumentID, 10), typ, desc,
		})
	}
	return rows
}

func auditPeriodLabel(from, to time.Time) string {
	if to.Equal(from) {
		return from.Format("02/01/2006")
	}
	return from.Format("02/01/2006") + " a " + to.Format("02/01/2006")
}

// writeAuditFile grava a auditoria em .txt (texto puro) ou .xlsx.
func writeAuditFile(path, format, serverName, login string, user *fluig.User, from, to time.Time, dims map[string]bool, tasks []fluig.TaskSummary, reqs []fluig.Request, docs []fluig.AuditDocument) error {
	name := user.FullName
	if name == "" {
		name = login
	}
	f, err := os.Create(path)
	if err != nil {
		return output.Usagef("não consegui criar %s: %v", path, err)
	}
	defer f.Close()

	if format == "xlsx" {
		sheets := []output.XLSXSheet{{Name: "Resumo", Rows: [][]string{
			{"Auditoria de", name},
			{"Login", login},
			{"Servidor", serverName},
			{"Período", auditPeriodLabel(from, to)},
			{},
			{"Tarefas atuadas", strconv.Itoa(len(tasks))},
			{"Solicitações abertas", strconv.Itoa(len(reqs))},
			{"Documentos criados", strconv.Itoa(len(docs))},
		}}}
		if dims["tasks"] {
			sheets = append(sheets, output.XLSXSheet{Name: "Tarefas", Rows: append([][]string{auditTaskHeaders}, auditTaskRows(tasks)...)})
		}
		if dims["requests"] {
			sheets = append(sheets, output.XLSXSheet{Name: "Solicitações", Rows: append([][]string{auditRequestHeaders}, auditRequestRows(reqs)...)})
		}
		if dims["documents"] {
			sheets = append(sheets, output.XLSXSheet{Name: "Documentos", Rows: append([][]string{auditDocHeaders}, auditDocRows(docs)...)})
		}
		return output.WriteXLSX(f, sheets)
	}

	// txt puro
	var b bytes.Buffer
	fmt.Fprintf(&b, "Auditoria de %s (%s)\nServidor: %s\nPeríodo: %s\n",
		name, login, serverName, auditPeriodLabel(from, to))
	section := func(title string, headers []string, rows [][]string, empty string) {
		fmt.Fprintf(&b, "\n%s (%d)\n", title, len(rows))
		if len(rows) == 0 {
			fmt.Fprintf(&b, "  %s\n", empty)
			return
		}
		output.Table{Headers: headers, Rows: rows}.Render(&b)
	}
	if dims["tasks"] {
		section("Tarefas atuadas", auditTaskHeaders, auditTaskRows(tasks), "nenhuma tarefa concluída no período.")
	}
	if dims["requests"] {
		section("Solicitações abertas", auditRequestHeaders, auditRequestRows(reqs), "nenhuma solicitação aberta no período.")
	}
	if dims["documents"] {
		section("Documentos criados", auditDocHeaders, auditDocRows(docs), "nenhum documento criado no período.")
	}
	fmt.Fprintf(&b, "\nResumo: %d tarefa(s) · %d solicitação(ões) · %d documento(s)\n",
		len(tasks), len(reqs), len(docs))
	_, err = f.Write(b.Bytes())
	return err
}
