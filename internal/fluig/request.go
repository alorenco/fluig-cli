package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

const restRequestsPath = "/process-management/api/v2/requests"

// RequestUser é o solicitante/responsável de solicitações e tarefas.
type RequestUser struct {
	Name  string `json:"name"`
	Login string `json:"login"`
	Code  string `json:"code"`
}

// RequestStep é um movimento corrente da solicitação (etapa onde ela está).
type RequestStep struct {
	Movement  int    `json:"movement"`  // movementSequence
	Sequence  int    `json:"sequence"`  // sequence da etapa (WKNumState)
	StateName string `json:"stateName"` // nome da etapa
	SLAStatus string `json:"slaStatus"`
}

// Request é uma solicitação de workflow (REST v2 process-management).
type Request struct {
	ID                 int            `json:"id"` // processInstanceId
	ProcessID          string         `json:"processId"`
	ProcessVersion     int            `json:"processVersion"`
	ProcessDescription string         `json:"processDescription"`
	Status             string         `json:"status"`    // OPEN | CANCELED | FINALIZED
	SLAStatus          string         `json:"slaStatus"` // ON_TIME | WARNING | EXPIRED
	Requester          *RequestUser   `json:"requester,omitempty"`
	StartDate          *time.Time     `json:"startDate,omitempty"`
	EndDate            *time.Time     `json:"endDate,omitempty"`
	FormRecordID       int64          `json:"formRecordId,omitempty"`
	FormID             int64          `json:"formId,omitempty"`
	CurrentSteps       []RequestStep  `json:"currentSteps,omitempty"`
}

// RequestTask é uma tarefa (movimentação) de uma solicitação.
type RequestTask struct {
	Movement  int          `json:"movement"`
	Sequence  int          `json:"sequence"`
	StateName string       `json:"stateName"`
	Assignee  *RequestUser `json:"assignee,omitempty"`
	Status    string       `json:"status"` // NOT_COMPLETED | PENDING_CONSENSUS | COMPLETED | TRANSFERRED | CANCELED
	SLAStatus string       `json:"slaStatus"`
	StartDate *time.Time   `json:"startDate,omitempty"`
	EndDate   *time.Time   `json:"endDate,omitempty"`
}

// RequestFilter parametriza a busca de solicitações (GET /v2/requests).
type RequestFilter struct {
	ProcessID string
	Status    string // OPEN | CANCELED | FINALIZED
	SLAStatus string // ON_TIME | WARNING | EXPIRED
	Assignee  string // login do responsável atual
	Requester string // login do solicitante
	Limit     int    // 0 = todas as páginas
}

// requestTime interpreta o formato de data do process-management, que vem com
// offset SEM dois-pontos ("2026-07-08T16:02:06.652-0400") — fora do RFC 3339
// que o encoding/json aceita; por isso as datas chegam como string e são
// convertidas aqui. Data ausente/nova variação → nil (não é erro).
func requestTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999-0700", time.RFC3339Nano} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t
		}
	}
	return nil
}

// requestItem é o item cru da API (schema Request do swagger, já com os
// expands de requester e currentMovements).
type requestItem struct {
	ProcessInstanceID  int          `json:"processInstanceId"`
	ProcessID          string       `json:"processId"`
	ProcessVersion     int          `json:"processVersion"`
	ProcessDescription string       `json:"processDescription"`
	Status             string       `json:"status"`
	SLAStatus          string       `json:"slaStatus"`
	Requester          *RequestUser `json:"requester"`
	StartDate          string       `json:"startDate"`
	EndDate            string       `json:"endDate"`
	FormRecordID       int64        `json:"formRecordId"`
	FormID             int64        `json:"formId"`
	CurrentMovements   []struct {
		MovementSequence int    `json:"movementSequence"`
		SLAStatus        string `json:"slaStatus"`
		State            struct {
			Sequence  int    `json:"sequence"`
			StateName string `json:"stateName"`
		} `json:"state"`
	} `json:"currentMovements"`
}

func (it requestItem) toRequest() Request {
	r := Request{
		ID:                 it.ProcessInstanceID,
		ProcessID:          it.ProcessID,
		ProcessVersion:     it.ProcessVersion,
		ProcessDescription: it.ProcessDescription,
		Status:             it.Status,
		SLAStatus:          it.SLAStatus,
		Requester:          it.Requester,
		StartDate:          requestTime(it.StartDate),
		EndDate:            requestTime(it.EndDate),
		FormRecordID:       it.FormRecordID,
		FormID:             it.FormID,
	}
	for _, m := range it.CurrentMovements {
		r.CurrentSteps = append(r.CurrentSteps, RequestStep{
			Movement:  m.MovementSequence,
			Sequence:  m.State.Sequence,
			StateName: m.State.StateName,
			SLAStatus: m.SLAStatus,
		})
	}
	return r
}

// resolveUserFilter converte login → userCode para os filtros assignee/
// requester: a API compara com o CÓDIGO do usuário, não o login — com login
// ela responde vazio em silêncio (validado na homologação em 2026-07-14).
// Login inexistente vira ErrNotFound (melhor que lista vazia enganosa).
func (c *Client) resolveUserFilter(ctx context.Context, login string) (string, error) {
	if login == "" {
		return "", nil
	}
	u, err := c.FindUserByLogin(ctx, login)
	if err != nil || u.Code == "" {
		return "", fmt.Errorf("%w: usuário %q (filtro de responsável/solicitante usa o login)", ErrNotFound, login)
	}
	return u.Code, nil
}

// ListRequests busca solicitações com os filtros dados (paginado; expande
// requester e currentMovements — ~1,7 KB por item, validado na homologação).
// Assignee/Requester são LOGINS (resolvidos para userCode internamente).
func (c *Client) ListRequests(ctx context.Context, f RequestFilter) ([]Request, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	assigneeCode, err := c.resolveUserFilter(ctx, f.Assignee)
	if err != nil {
		return nil, err
	}
	requesterCode, err := c.resolveUserFilter(ctx, f.Requester)
	if err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []Request
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		params.Add("expand", "requester")
		params.Add("expand", "currentMovements")
		if f.ProcessID != "" {
			params.Add("processId", f.ProcessID)
		}
		if f.Status != "" {
			params.Add("status", f.Status)
		}
		if f.SLAStatus != "" {
			params.Add("slaStatus", f.SLAStatus)
		}
		if assigneeCode != "" {
			params.Set("assignee", assigneeCode)
		}
		if requesterCode != "" {
			params.Set("requester", requesterCode)
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restRequestsPath)+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: "v2/requests", Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items   []requestItem `json:"items"`
			HasNext bool          `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/requests: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, it.toRequest())
			if f.Limit > 0 && len(out) >= f.Limit {
				return out[:f.Limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// GetRequest carrega uma solicitação pelo número (processInstanceId).
func (c *Client) GetRequest(ctx context.Context, id int) (*Request, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restRequestsPath+"/"+strconv.Itoa(id)) +
		"?expand=requester&expand=currentMovements"
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: solicitação %d", ErrNotFound, id)
	}
	if status < 200 || status >= 300 {
		return nil, &HTTPError{StatusCode: status, URL: "v2/requests/{id}", Body: truncate(string(body), 512)}
	}
	var it requestItem
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/requests/{id}: %w", err)
	}
	r := it.toRequest()
	return &r, nil
}

// MoveResult é a resposta do start/move (schema MoveResponse do swagger).
type MoveResult struct {
	RequestID         int           `json:"requestId"` // processInstanceId
	ProcessID         string        `json:"processId"`
	ProcessVersion    int           `json:"processVersion"`
	NextState         int           `json:"nextState"`
	NextStateName     string        `json:"nextStateName"`
	CardID            int64         `json:"cardId,omitempty"`
	NeedsAssignee     bool          `json:"needsAssignee,omitempty"` // HTTP 412: escolha entre PossibleAssignees
	PossibleAssignees []RequestUser `json:"possibleAssignees,omitempty"`
}

type moveResponseRaw struct {
	ProcessInstanceID       int           `json:"processInstanceId"`
	ProcessID               string        `json:"processId"`
	ProcessVersion          int           `json:"processVersion"`
	NextState               int           `json:"nextState"`
	NextStateName           string        `json:"nextStateName"`
	CardID                  int64         `json:"cardId"`
	ToShowPossibleAssignees bool          `json:"toShowPossibleAssignees"`
	PossibleAssignees       []RequestUser `json:"possibleAssignees"`
}

func (raw moveResponseRaw) toResult(needsAssignee bool) *MoveResult {
	return &MoveResult{
		RequestID:         raw.ProcessInstanceID,
		ProcessID:         raw.ProcessID,
		ProcessVersion:    raw.ProcessVersion,
		NextState:         raw.NextState,
		NextStateName:     raw.NextStateName,
		CardID:            raw.CardID,
		NeedsAssignee:     needsAssignee || raw.ToShowPossibleAssignees,
		PossibleAssignees: raw.PossibleAssignees,
	}
}

// RequestStartOptions parametriza o início de uma solicitação.
type RequestStartOptions struct {
	TargetState    int
	TargetAssignee string
	Comment        string
	FormFields     map[string]string
	NoSend         bool // só no caminho SOAP: cria sem enviar (fica na atividade inicial)
}

// RequestMoveOptions parametriza a movimentação de uma solicitação.
type RequestMoveOptions struct {
	MovementSequence int // tarefa corrente a concluir (0 = descoberto pelo chamador)
	TargetState      int
	TargetAssignee   string
	Comment          string
	FormFields       map[string]string
}

// restRequestError converte a resposta de erro da REST v2 em erro amigável.
// Nem todo erro vem como ErrorResponse JSON: eventos de processo que dão throw
// chegam como texto entre chaves ("{Erro ao salvar dados de formulário: ...}"),
// possivelmente com HTML de destaque — validado na homologação em 2026-07-14.
func restRequestError(op string, status int, body []byte) error {
	var parsed struct {
		Message         string `json:"message"`
		DetailedMessage string `json:"detailedMessage"`
	}
	_ = json.Unmarshal(body, &parsed)
	msg := parsed.Message
	if msg == "" {
		msg = parsed.DetailedMessage
	}
	if msg == "" {
		if raw := plainServerText(body); raw != "" {
			msg = raw
		}
	}
	if msg != "" {
		return fmt.Errorf("%w: %s", errServerRejected, truncate(msg, 512))
	}
	return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
}

// plainServerText extrai o texto legível de um corpo de erro não-JSON:
// remove as chaves externas, tags HTML e espaço redundante.
func plainServerText(body []byte) string {
	s := strings.TrimSpace(string(body))
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && !json.Valid(body) {
		s = strings.TrimSpace(s[1 : len(s)-1])
	} else if json.Valid(body) {
		return ""
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// StartRequest inicia uma solicitação do processo (POST /v2/processes/{id}/start).
// HTTP 412 = o servidor exige escolher o responsável → MoveResult com
// NeedsAssignee e a lista PossibleAssignees (nada foi criado).
func (c *Client) StartRequest(ctx context.Context, processID string, o RequestStartOptions) (*MoveResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url("/process-management/api/v2/processes/" + url.PathEscape(processID) + "/start")
	payload := movePayload(o.TargetState, o.TargetAssignee, o.Comment, o.FormFields)
	return c.postMove(ctx, "v2/processes/{id}/start", endpoint, payload, fmt.Sprintf("processo %q", processID))
}

// MoveRequestTo movimenta uma solicitação (POST /v2/requests/{id}/move).
func (c *Client) MoveRequestTo(ctx context.Context, id int, o RequestMoveOptions) (*MoveResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restRequestsPath + "/" + strconv.Itoa(id) + "/move")
	payload := movePayload(o.TargetState, o.TargetAssignee, o.Comment, o.FormFields)
	if o.MovementSequence > 0 {
		payload["movementSequence"] = o.MovementSequence
	}
	return c.postMove(ctx, "v2/requests/{id}/move", endpoint, payload, fmt.Sprintf("solicitação %d", id))
}

// movePayload monta o corpo do start/move só com os campos preenchidos.
func movePayload(targetState int, targetAssignee, comment string, formFields map[string]string) map[string]any {
	payload := map[string]any{}
	if targetState != 0 {
		payload["targetState"] = targetState
	}
	if targetAssignee != "" {
		payload["targetAssignee"] = targetAssignee
	}
	if comment != "" {
		payload["comment"] = comment
	}
	if len(formFields) > 0 {
		payload["formFields"] = formFields
	}
	return payload
}

// postMove executa start/move e interpreta o MoveResponse (200 ok, 412 =
// escolher responsável) ou o ErrorResponse.
func (c *Client) postMove(ctx context.Context, op, endpoint string, payload map[string]any, subject string) (*MoveResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, data)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, subject)
	case http.StatusOK, http.StatusPreconditionFailed:
		var raw moveResponseRaw
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("resposta inesperada de %s: %w", op, err)
		}
		return raw.toResult(status == http.StatusPreconditionFailed), nil
	default:
		return nil, restRequestError(op, status, body)
	}
}

// ProcessAttachment é um anexo listado de uma solicitação. O próprio
// FORMULÁRIO aparece na lista como um "anexo" com MainForm=true e sem
// documentName (validado na homologação em 2026-07-14) — os arquivos reais
// têm MainForm=false.
type ProcessAttachment struct {
	Sequence    int          `json:"sequence"`
	DocumentID  int64        `json:"documentId"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Version     int          `json:"version"`
	Movement    int          `json:"movement"`
	MainForm    bool         `json:"mainForm"`
	Published   bool         `json:"published"`
	User        *RequestUser `json:"user,omitempty"`
	Date        *time.Time   `json:"date,omitempty"`
}

// RequestAttachments lista os anexos de uma solicitação (paginado).
func (c *Client) RequestAttachments(ctx context.Context, id int) ([]ProcessAttachment, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []ProcessAttachment
	for page := 1; ; page++ {
		endpoint := c.url(restRequestsPath+"/"+strconv.Itoa(id)+"/attachments") +
			"?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("%w: solicitação %d", ErrNotFound, id)
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("v2/requests/{id}/attachments", status, body)
		}
		var parsed struct {
			Items []struct {
				AttachmentSequence  int          `json:"attachmentSequence"`
				DocumentID          int64        `json:"documentId"`
				DocumentName        string       `json:"documentName"`
				DocumentDescription string       `json:"documentDescription"`
				DocumentVersion     int          `json:"documentVersion"`
				MovementSequence    int          `json:"movementSequence"`
				MainForm            bool         `json:"mainForm"`
				Published           bool         `json:"published"`
				User                *RequestUser `json:"user"`
				Date                string       `json:"date"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/requests/{id}/attachments: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, ProcessAttachment{
				Sequence:    it.AttachmentSequence,
				DocumentID:  it.DocumentID,
				Name:        it.DocumentName,
				Description: it.DocumentDescription,
				Version:     it.DocumentVersion,
				Movement:    it.MovementSequence,
				MainForm:    it.MainForm,
				Published:   it.Published,
				User:        it.User,
				Date:        requestTime(it.Date),
			})
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// DownloadRequestAttachment baixa o conteúdo de um anexo (bytes verbatim —
// round-trip byte a byte validado na homologação). O chamador deve validar o
// sequence contra RequestAttachments antes: sequence inexistente responde 400
// com uma exceção de PERMISSÃO enganosa (comportamento real do servidor).
func (c *Client) DownloadRequestAttachment(ctx context.Context, id, sequence int) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restRequestsPath + "/" + strconv.Itoa(id) + "/attachments/" +
		strconv.Itoa(sequence) + "/download")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao chamar %s: %w", c.base.Host, err)
	}
	// Anexos podem ser grandes — limite próprio, maior que o dos JSONs.
	body, err := readBody(resp, 256<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: solicitação %d", ErrNotFound, id)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, restRequestError("v2/requests/{id}/attachments/{seq}/download", resp.StatusCode, []byte(body))
	}
	return []byte(body), nil
}

// RequestAttachment é um anexo para o início de solicitação.
type RequestAttachment struct {
	FileName string
	Content  []byte
}

// StartRequestWithAttachments inicia uma solicitação COM anexos pelo SOAP
// startProcess — a REST v2 de requests não tem upload de anexo (só download;
// validado no swagger e na homologação em 2026-07-14, onde processos com
// anexo obrigatório no início não podem ser iniciados pela REST). Devolve o
// número da solicitação criada e o mapa bruto do resultado.
func (c *Client) StartRequestWithAttachments(ctx context.Context, processID string, o RequestStartOptions, atts []RequestAttachment) (int, map[string]string, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return 0, nil, err
	}
	userCode, err := c.ResolveUserCode(ctx)
	if err != nil {
		return 0, nil, err
	}
	var assignees []string
	if o.TargetAssignee != "" {
		u, uerr := c.FindUserByLogin(ctx, o.TargetAssignee)
		if uerr != nil {
			return 0, nil, uerr
		}
		assignees = []string{u.Code}
	}
	keys := make([]string, 0, len(o.FormFields))
	for k := range o.FormFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	soapAtts := make([]soap.StartAttachment, 0, len(atts))
	for _, a := range atts {
		soapAtts = append(soapAtts, soap.StartAttachment{FileName: a.FileName, Content: a.Content})
	}
	reqBody, err := soap.BuildStartProcess(c.opts.CompanyID, c.opts.Username, c.opts.Password,
		processID, o.TargetState, assignees, o.Comment, userCode, !o.NoSend, soapAtts, o.FormFields, keys)
	if err != nil {
		return 0, nil, err
	}
	respBody, err := c.postSOAP(ctx, soapWorkflowPath, "startProcess", reqBody)
	if err != nil {
		return 0, nil, err
	}
	result, err := soap.ParseStartProcess(respBody)
	if err != nil {
		return 0, nil, mapSOAPError(err)
	}
	// Erro de negócio vem como par ERROR → mensagem (validado na homologação:
	// ex. responsável não apto para a tarefa).
	if msg := strings.TrimSpace(result["ERROR"]); msg != "" {
		return 0, result, fmt.Errorf("%w: %s", errServerRejected, msg)
	}
	id, _ := strconv.Atoi(strings.TrimSpace(result["iProcess"]))
	if id == 0 {
		return 0, result, fmt.Errorf("%w: startProcess não devolveu o número da solicitação (%v)", errServerRejected, result)
	}
	return id, result, nil
}

// PossibleAssignees lista quem pode assumir a próxima atividade da solicitação
// (GET /v2/requests/{id}/possible-assignees). targetState é obrigatório quando
// o diagrama oferece mais de um destino (o servidor rejeita sem ele —
// validado na homologação em 2026-07-14); 0 = deixa o servidor decidir.
func (c *Client) PossibleAssignees(ctx context.Context, id, targetState int) ([]RequestUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restRequestsPath + "/" + strconv.Itoa(id) + "/possible-assignees")
	if targetState > 0 {
		endpoint += "?targetState=" + strconv.Itoa(targetState)
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: solicitação %d", ErrNotFound, id)
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("v2/requests/{id}/possible-assignees", status, body)
	}
	var parsed struct {
		Items []RequestUser `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta inesperada de possible-assignees: %w", err)
	}
	return parsed.Items, nil
}

// RequestTasks devolve as tarefas (movimentações) de uma solicitação, na ordem
// do servidor (histórico completo, incluindo as concluídas).
func (c *Client) RequestTasks(ctx context.Context, id int) ([]RequestTask, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []RequestTask
	for page := 1; ; page++ {
		endpoint := c.url(restRequestsPath+"/"+strconv.Itoa(id)+"/tasks") +
			"?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("%w: solicitação %d", ErrNotFound, id)
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: "v2/requests/{id}/tasks", Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items []struct {
				MovementSequence int          `json:"movementSequence"`
				Assignee         *RequestUser `json:"assignee"`
				Status           string       `json:"status"`
				SLAStatus        string       `json:"slaStatus"`
				StartDate        string       `json:"startDate"`
				EndDate          string       `json:"endDate"`
				State            struct {
					Sequence  int    `json:"sequence"`
					StateName string `json:"stateName"`
				} `json:"state"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/requests/{id}/tasks: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, RequestTask{
				Movement:  it.MovementSequence,
				Sequence:  it.State.Sequence,
				StateName: it.State.StateName,
				Assignee:  it.Assignee,
				Status:    it.Status,
				SLAStatus: it.SLAStatus,
				StartDate: requestTime(it.StartDate),
				EndDate:   requestTime(it.EndDate),
			})
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}
