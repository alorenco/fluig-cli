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

const restTasksPath = "/process-management/api/v2/tasks"

// TaskSummary é uma tarefa retornada pela busca geral (GET /v2/tasks) —
// mesmo shape das tarefas de uma solicitação, mais o contexto do processo.
type TaskSummary struct {
	RequestID          int          `json:"requestId"` // processInstanceId
	ProcessID          string       `json:"processId"`
	ProcessDescription string       `json:"processDescription"`
	Movement           int          `json:"movement"`
	Sequence           int          `json:"sequence"`
	StateName          string       `json:"stateName"`
	Assignee           *RequestUser `json:"assignee,omitempty"`
	Requester          *RequestUser `json:"requester,omitempty"`
	Status             string       `json:"status"` // NOT_COMPLETED | PENDING_CONSENSUS | COMPLETED | TRANSFERRED | CANCELED
	SLAStatus          string       `json:"slaStatus"`
	StartDate          *time.Time   `json:"startDate,omitempty"`
	EndDate            *time.Time   `json:"endDate,omitempty"`
}

// TaskFilter parametriza a busca de tarefas (GET /v2/tasks). A busca é sobre
// as tarefas de TODOS os usuários — "minhas tarefas" = Assignee com o login.
type TaskFilter struct {
	Assignee  string
	Requester string
	ProcessID string
	Status    string
	SLAStatus string
	Limit     int // 0 = todas as páginas

	// Filtros de data server-side (usados por `user audit`): quando o
	// responsável ENCERROU a tarefa. Strings no formato date-time do Fluig
	// ("2006-01-02T15:04:05", sem offset — o servidor aplica o próprio fuso).
	AssignEndFrom string // initialAssignEndDate — concluída a partir de
	AssignEndTo   string // finalAssignEndDate — concluída até
}

// ListTasks busca tarefas com os filtros dados (paginado).
// ⚠️ NÃO usar /v2/tasks/count nem /v2/tasks/resume: na homologação (Voyager
// 2.0.0, 2026-07-14) essas rotas penduram a requisição e derrubaram a
// aplicação — ver FLUIG-APIS.md. A busca (esta rota) responde normalmente.
func (c *Client) ListTasks(ctx context.Context, f TaskFilter) ([]TaskSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	// Assignee/Requester são logins — a API filtra por userCode (ver
	// resolveUserFilter; login direto responde vazio em silêncio).
	assigneeCode, err := c.resolveUserFilter(ctx, f.Assignee)
	if err != nil {
		return nil, err
	}
	requesterCode, err := c.resolveUserFilter(ctx, f.Requester)
	if err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []TaskSummary
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		if assigneeCode != "" {
			params.Set("assignee", assigneeCode)
		}
		if requesterCode != "" {
			params.Set("requester", requesterCode)
		}
		if f.ProcessID != "" {
			params.Set("processId", f.ProcessID)
		}
		if f.Status != "" {
			params.Add("status", f.Status)
		}
		if f.SLAStatus != "" {
			params.Add("slaStatus", f.SLAStatus)
		}
		if f.AssignEndFrom != "" {
			params.Set("initialAssignEndDate", f.AssignEndFrom)
		}
		if f.AssignEndTo != "" {
			params.Set("finalAssignEndDate", f.AssignEndTo)
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restTasksPath)+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("v2/tasks", status, body)
		}
		var parsed struct {
			Items []struct {
				ProcessInstanceID  int          `json:"processInstanceId"`
				ProcessID          string       `json:"processId"`
				ProcessDescription string       `json:"processDescription"`
				MovementSequence   int          `json:"movementSequence"`
				Assignee           *RequestUser `json:"assignee"`
				Requester          *RequestUser `json:"requester"`
				Status             string       `json:"status"`
				SLAStatus          string       `json:"slaStatus"`
				StartDate          string       `json:"startDate"`
				EndDate            string       `json:"endDate"`
				State              struct {
					Sequence  int    `json:"sequence"`
					StateName string `json:"stateName"`
				} `json:"state"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/tasks: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, TaskSummary{
				RequestID:          it.ProcessInstanceID,
				ProcessID:          it.ProcessID,
				ProcessDescription: it.ProcessDescription,
				Movement:           it.MovementSequence,
				Sequence:           it.State.Sequence,
				StateName:          it.State.StateName,
				Assignee:           it.Assignee,
				Requester:          it.Requester,
				Status:             it.Status,
				SLAStatus:          it.SLAStatus,
				StartDate:          requestTime(it.StartDate),
				EndDate:            requestTime(it.EndDate),
			})
			if f.Limit > 0 && len(out) >= f.Limit {
				return out[:f.Limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}
