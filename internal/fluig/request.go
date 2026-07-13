package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
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

// ListRequests busca solicitações com os filtros dados (paginado; expande
// requester e currentMovements — ~1,7 KB por item, validado na homologação).
func (c *Client) ListRequests(ctx context.Context, f RequestFilter) ([]Request, error) {
	if err := c.EnsureSession(ctx); err != nil {
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
		if f.Assignee != "" {
			params.Set("assignee", f.Assignee)
		}
		if f.Requester != "" {
			params.Set("requester", f.Requester)
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
