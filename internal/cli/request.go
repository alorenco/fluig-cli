package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newRequestCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "request",
		Short: "Solicitações de workflow: consultar, iniciar, movimentar e baixar anexos",
	}
	cmd.AddCommand(newRequestListCmd(app))
	cmd.AddCommand(newRequestShowCmd(app))
	cmd.AddCommand(newRequestStartCmd(app))
	cmd.AddCommand(newRequestMoveCmd(app))
	cmd.AddCommand(newRequestAssigneesCmd(app))
	cmd.AddCommand(newRequestAttachmentsCmd(app))
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

// loadFormFields junta os campos do --fields-file (objeto JSON plano; "-" lê
// do stdin) com os --field. O --field tem precedência — assim o arquivo pode
// servir de template e a flag variar um campo pontual. Valores escalares do
// JSON (número/bool/null) são convertidos para a string que a API espera.
func loadFormFields(fieldsFile string, fieldFlags []string) (map[string]string, error) {
	flags, err := parseFormFields(fieldFlags)
	if err != nil {
		return nil, err
	}
	if fieldsFile == "" {
		return flags, nil
	}
	var data []byte
	if fieldsFile == "-" {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
	} else {
		data, err = os.ReadFile(fieldsFile)
		if os.IsNotExist(err) {
			return nil, output.NotFoundf("arquivo %q não encontrado", fieldsFile)
		}
		if err != nil {
			return nil, err
		}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, output.Usagef(`--fields-file: JSON inválido (%v) — esperado um objeto plano {"campo": "valor"}`, err)
	}
	out := make(map[string]string, len(raw)+len(flags))
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case json.Number:
			out[k] = t.String()
		case bool:
			out[k] = strconv.FormatBool(t)
		case nil:
			out[k] = ""
		default:
			return nil, output.Usagef("--fields-file: o campo %q tem valor aninhado (objeto/array) — a API aceita só valores simples", k)
		}
	}
	for k, v := range flags {
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

// discoverMovement descobre qual movimento concluir quando o usuário não passou
// --movement. Devolve erro de uso quando a escolha é genuinamente ambígua.
func discoverMovement(ctx context.Context, p *output.Printer, client *fluig.Client, id int) (int, error) {
	req, err := client.GetRequest(ctx, id)
	if err != nil {
		return 0, mapFluigError(err)
	}
	// A MESMA etapa pode aparecer duas vezes na etapa corrente — a tarefa do
	// pool e a do usuário que a assumiu compartilham o movementSequence
	// (visto em produção: duas opções idênticas "movimento 15"). Isso não é
	// ambiguidade: o movimento é um só.
	steps := dedupStepsByMovement(req.CurrentSteps)
	switch len(steps) {
	case 0:
		return 0, output.Usagef("a solicitação %d não tem tarefa em aberto (status %s)", id, req.Status)
	case 1:
		return steps[0].Movement, nil
	}

	// Movimentos diferentes = atividades paralelas: aí a escolha é do usuário.
	// Responsável e status vêm das tarefas (o expand da etapa corrente não os
	// traz). Best-effort: sem eles, a lista sai só com etapa e SLA.
	tasks, terr := client.RequestTasks(ctx, id)
	if terr != nil {
		p.Warnf("não foi possível detalhar as tarefas (%s) — a lista sai sem responsável", output.AsError(mapFluigError(terr)).Message)
	}
	rows := make([][]string, 0, len(steps))
	options := make([]map[string]any, 0, len(steps))
	for _, s := range steps {
		who, status := taskInfoForMovement(tasks, s.Movement)
		rows = append(rows, []string{strconv.Itoa(s.Movement), s.StateName, who, status, s.SLAStatus})
		options = append(options, map[string]any{
			"movement": s.Movement, "stateName": s.StateName,
			"assignee": who, "status": status, "slaStatus": s.SLAStatus,
		})
	}
	p.Table(output.Table{
		Headers: []string{"Movimento", "Etapa", "Responsável", "Status", "SLA"},
		Rows:    rows,
		Style:   output.BoldHeaderStyle(nil),
	})
	msg := fmt.Sprintf("a solicitação %d tem %d tarefas em aberto em movimentos diferentes — escolha com --movement %d (ou %d)",
		id, len(steps), steps[0].Movement, steps[1].Movement)
	p.FailData(map[string]any{"requestId": id, "options": options}, output.CodeUsage, msg)
	return 0, output.Usagef("%s", msg)
}

// dedupStepsByMovement remove passos repetidos do mesmo movimento, preservando a
// ordem em que o servidor devolveu.
func dedupStepsByMovement(steps []fluig.RequestStep) []fluig.RequestStep {
	out := make([]fluig.RequestStep, 0, len(steps))
	visto := make(map[int]bool, len(steps))
	for _, s := range steps {
		if visto[s.Movement] {
			continue
		}
		visto[s.Movement] = true
		out = append(out, s)
	}
	return out
}

// taskInfoForMovement resume as tarefas de um movimento: quem responde por ele e
// em que status. Um movimento pode ter várias tarefas (pool + usuário), então as
// EM ABERTO têm preferência — são as que interessam para movimentar.
func taskInfoForMovement(tasks []fluig.RequestTask, movement int) (assignee, status string) {
	var abertas, todas []fluig.RequestTask
	for _, t := range tasks {
		if t.Movement != movement {
			continue
		}
		todas = append(todas, t)
		if t.Status != "COMPLETED" && t.Status != "CANCELED" {
			abertas = append(abertas, t)
		}
	}
	escolhidas := abertas
	if len(escolhidas) == 0 {
		escolhidas = todas
	}
	var nomes, situacoes []string
	for _, t := range escolhidas {
		if label := requestUserLabel(t.Assignee); label != "" && !slices.Contains(nomes, label) {
			nomes = append(nomes, label)
		}
		if t.Status != "" && !slices.Contains(situacoes, t.Status) {
			situacoes = append(situacoes, t.Status)
		}
	}
	if len(nomes) == 0 {
		nomes = []string{"(pool, sem responsável)"}
	}
	return strings.Join(nomes, ", "), strings.Join(situacoes, "/")
}

// Veredictos da pós-checagem de timeout do `request move`. São valores do
// contrato --json (campo `outcome`).
const (
	moveOutcomeMoved    = "moved"     // a tarefa alvo não está mais em aberto
	moveOutcomeNotMoved = "not_moved" // a tarefa alvo continua em aberto
	moveOutcomeUnknown  = "unknown"   // não foi possível reler o estado
)

// reportMoveTimeout trata o timeout do move: o cliente desistiu de esperar, mas
// o servidor pode ter concluído a movimentação (visto em produção — o move
// respondeu depois de ~80 s e movimentou). Repetir às cegas movimenta duas
// vezes, então a CLI relê o estado e diz o que encontrou. O exit segue 5.
func reportMoveTimeout(ctx context.Context, app *App, p *output.Printer, client *fluig.Client, id, movement int, cause error) error {
	p.Warnf("o move da solicitação %d excedeu o tempo limite de %s — consultando o estado atual", id, app.Timeout)

	outcome, detail := moveOutcomeUnknown, ""
	var steps []fluig.RequestStep
	req, gerr := client.GetRequest(ctx, id)
	switch {
	case gerr != nil:
		detail = fmt.Sprintf("a releitura do estado também falhou (%s)", output.AsError(mapFluigError(gerr)).Message)
	case stepWithMovement(req.CurrentSteps, movement) != nil:
		outcome = moveOutcomeNotMoved
		steps = req.CurrentSteps
		detail = fmt.Sprintf("a tarefa do movimento %d continua em aberto (status %s)", movement, req.Status)
	default:
		outcome = moveOutcomeMoved
		steps = req.CurrentSteps
		detail = fmt.Sprintf("a tarefa do movimento %d não está mais em aberto (status %s)", movement, req.Status)
		if len(req.CurrentSteps) > 0 {
			detail += fmt.Sprintf(", agora em %q (movimento %d)", req.CurrentSteps[0].StateName, req.CurrentSteps[0].Movement)
		}
	}

	// Cada veredicto tem uma orientação diferente: o erro do "moved" avisa para
	// NÃO repetir, e é o caso perigoso do relato original.
	var advice string
	switch outcome {
	case moveOutcomeMoved:
		advice = "a movimentação provavelmente foi concluída no servidor — NÃO repita o comando"
	case moveOutcomeNotMoved:
		advice = fmt.Sprintf("a movimentação provavelmente não aconteceu — repita com um tempo limite maior (--timeout %s)", app.Timeout*2)
	default:
		advice = fmt.Sprintf("estado indeterminado — confira com: fluigcli request show %d", id)
	}
	p.Infof("%s: %s.", detail, advice)

	msg := fmt.Sprintf("o move da solicitação %d excedeu o tempo limite de %s; %s (%s)", id, app.Timeout, detail, advice)
	p.FailData(map[string]any{
		"requestId": id, "movement": movement,
		"outcome": outcome, "currentSteps": steps,
	}, output.CodeTimeout, msg)
	return output.Timeoutf("%s", msg).WithCause(cause)
}

// reportStartTimeout trata o timeout do start. Aqui não dá para conferir
// sozinho: o número da solicitação nasce na resposta que não chegou. Então a
// CLI entrega o comando que responde "criou ou não" em vez de arriscar um
// veredicto — e avisa para não repetir às cegas.
func reportStartTimeout(app *App, p *output.Printer, processID, login string, cause error) error {
	check := fmt.Sprintf("fluigcli request list --process %s --requester %s --status open", processID, login)
	p.Warnf("o start do processo %s excedeu o tempo limite de %s", processID, app.Timeout)
	p.Infof("A solicitação pode ter sido criada no servidor. Confira antes de repetir: %s", check)
	msg := fmt.Sprintf("o start do processo %s excedeu o tempo limite de %s; a solicitação pode ter sido criada — confira com: %s",
		processID, app.Timeout, check)
	p.FailData(map[string]any{
		"processId": processID, "outcome": moveOutcomeUnknown, "checkCommand": check,
	}, output.CodeTimeout, msg)
	return output.Timeoutf("%s", msg).WithCause(cause)
}

// stepWithMovement acha o passo corrente de um movimento (nil se não houver).
func stepWithMovement(steps []fluig.RequestStep, movement int) *fluig.RequestStep {
	for i := range steps {
		if steps[i].Movement == movement {
			return &steps[i]
		}
	}
	return nil
}

// --- request start ---

func newRequestStartCmd(app *App) *cobra.Command {
	var (
		fields        []string
		fieldsFile    string
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
			"formulário com os --field dados.\n\n" +
			"Os eventos do PROCESSO rodam no servidor normalmente. Um throw de evento\n" +
			"vira exit 5 com a mensagem do servidor.\n\n" +
			"⚠️ Os eventos do FORMULÁRIO não rodam: displayFields e validateForm ficam\n" +
			"de fora, porque a CLI grava o card pela API REST. O card recebe só os\n" +
			"valores que você enviou. Uma solicitação com campos obrigatórios vazios é\n" +
			"aceita sem crítica. Por isso não use este comando para testar a validação\n" +
			"do formulário — use o navegador (fluigcli dev). O beforeSendValidate é\n" +
			"client-side e também não roda.\n\n" +
			"Com --attach, os arquivos vão como anexos da solicitação — necessário nos\n" +
			"processos que exigem anexo no início (a REST v2 não tem upload de anexo;\n" +
			"nesse modo a CLI usa o SOAP startProcess, que exige --target-state).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			formFields, err := loadFormFields(fieldsFile, fields)
			if err != nil {
				return err
			}
			ctx := context.Background()
			server, client, err := app.connectWrite(ctx, passwordStdin, "iniciar uma solicitação")
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
					if isTimeoutErr(serr) {
						return reportStartTimeout(app, p, args[0], server.Username, serr)
					}
					return app.mapErr(serr)
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
				if isTimeoutErr(err) {
					return reportStartTimeout(app, p, args[0], server.Username, err)
				}
				return app.mapErr(err)
			}
			return reportMoveResult(p, res, fmt.Sprintf("solicitação %d criada (%s → etapa %q)",
				res.RequestID, res.ProcessID, res.NextStateName))
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "campo do formulário: campo=valor (pode repetir; sobrepõe o --fields-file)")
	cmd.Flags().StringVar(&fieldsFile, "fields-file", "", `campos do formulário em JSON plano {"campo":"valor"}; linha de tabela-filha usa o sufixo campo___N (arquivo ou "-" para stdin)`)
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
		fieldsFile    string
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
			"informar quando houver mais de uma, ex.: atividades paralelas).\n\n" +
			"⚠️ Como no request start, os eventos do FORMULÁRIO não rodam\n" +
			"(displayFields, validateForm). Os eventos do processo rodam.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			id, err := strconv.Atoi(args[0])
			if err != nil || id <= 0 {
				return output.Usagef("número de solicitação inválido %q", args[0])
			}
			formFields, err := loadFormFields(fieldsFile, fields)
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
				seq, err = discoverMovement(ctx, p, client, id)
				if err != nil {
					return err
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
				if isTimeoutErr(err) {
					return reportMoveTimeout(ctx, app, p, client, id, seq, err)
				}
				return app.mapErr(err)
			}
			// O 200 real pode vir com nextStateName vazio (validado na homolog).
			dest := fmt.Sprintf("etapa %d", res.NextState)
			if res.NextStateName != "" {
				dest = fmt.Sprintf("etapa %q (seq %d)", res.NextStateName, res.NextState)
			}
			return reportMoveResult(p, res, fmt.Sprintf("solicitação %d movimentada → %s", id, dest))
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "campo do formulário a atualizar: campo=valor (pode repetir; sobrepõe o --fields-file)")
	cmd.Flags().StringVar(&fieldsFile, "fields-file", "", `campos do formulário em JSON plano {"campo":"valor"}; linha de tabela-filha usa o sufixo campo___N (arquivo ou "-" para stdin)`)
	cmd.Flags().StringVar(&comment, "comment", "", "comentário do movimento")
	cmd.Flags().IntVar(&targetState, "target-state", 0, "etapa de destino (sequence; default: o fluxo do diagrama)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "login do responsável pela próxima atividade")
	cmd.Flags().IntVar(&movement, "movement", 0, "movimento (tarefa) a concluir, quando houver mais de um em aberto")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- request attachments ---

func newRequestAttachmentsCmd(app *App) *cobra.Command {
	var (
		download      bool
		seq           int
		dir           string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "attachments <número>",
		Short: "Lista e baixa os anexos de uma solicitação (nativo, REST v2)",
		Long: "Lista os anexos de uma solicitação; com --download, baixa os arquivos\n" +
			"para o diretório atual (ou --dir). O formulário da solicitação aparece na\n" +
			"lista como \"(formulário)\" e não é baixado — só os arquivos anexados.\n" +
			"--seq baixa um anexo específico.",
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
			atts, err := client.RequestAttachments(ctx, id)
			if err != nil {
				return mapFluigError(err)
			}

			if !download && seq == 0 {
				if len(atts) == 0 {
					p.Infof("A solicitação %d não tem anexos.", id)
				} else {
					rows := make([][]string, 0, len(atts))
					for _, a := range atts {
						name := a.Name
						if a.MainForm {
							name = "(formulário)"
						}
						rows = append(rows, []string{strconv.Itoa(a.Sequence), name,
							strconv.Itoa(a.Version), requestUserLabel(a.User), fmtRequestTime(a.Date)})
					}
					p.Table(output.Table{
						Headers: []string{"Seq", "Arquivo", "Versão", "Anexado por", "Em"},
						Rows:    rows,
						Style:   output.BoldHeaderStyle(nil),
					})
				}
				p.Done(map[string]any{"attachments": atts})
				return nil
			}

			// Download: --seq baixa um; sem --seq baixa todos os arquivos
			// (o "(formulário)" fica de fora).
			var targets []fluig.ProcessAttachment
			if seq > 0 {
				found := false
				for _, a := range atts {
					if a.Sequence == seq {
						targets, found = []fluig.ProcessAttachment{a}, true
						break
					}
				}
				// Valida antes de baixar: sequence inexistente responde 400 de
				// "permissão" no servidor — enganoso (comportamento real).
				if !found {
					return output.NotFoundf("anexo %d não existe na solicitação %d (veja request attachments %d)", seq, id, id)
				}
			} else {
				for _, a := range atts {
					if !a.MainForm {
						targets = append(targets, a)
					}
				}
				if len(targets) == 0 {
					p.Infof("A solicitação %d não tem arquivos anexados para baixar.", id)
					p.Done(map[string]any{"results": []any{}})
					return nil
				}
			}

			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			var results []itemResult
			var lastErr error
			failures := 0
			for _, a := range targets {
				name := a.Name
				if name == "" {
					name = fmt.Sprintf("anexo_%d", a.Sequence)
				}
				content, derr := client.DownloadRequestAttachment(ctx, id, a.Sequence)
				if derr == nil {
					var path string
					if path, derr = project.SafeJoin(dir, name); derr == nil {
						derr = os.WriteFile(path, content, 0o644)
					}
				}
				if derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: name, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("anexo %q: %s", name, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: name, Action: "downloaded", Success: true})
				p.Successf("anexo %q salvo em %s (%d bytes)", name, dir, len(content))
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(targets))
		},
	}
	cmd.Flags().BoolVar(&download, "download", false, "baixa os arquivos anexados (todos, exceto o formulário)")
	cmd.Flags().IntVar(&seq, "seq", 0, "baixa só o anexo com esse sequence")
	cmd.Flags().StringVar(&dir, "dir", ".", "diretório de destino dos downloads")
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
