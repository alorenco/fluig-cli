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

// A central de tarefas do portal expõe as tarefas paradas num pool (grupo ou
// papel) pela API legada /ecm/api/rest/ecm/centralTasks — fora dos swaggers
// (ver FLUIG-APIS.md). A REST v2 (/v2/tasks) até aceita assignee=Pool:..., mas
// quebra com NullPointerException quando o resultado alcança alguma tarefa
// órfã de processo/versão apagado — validado ao vivo na homologação e na
// produção em 2026-08-01. A tela do Fluig usa esta rota, que não tropeça nos
// órfãos; por isso o `task list --group` também usa.
const restCentralTasksPoolPath = "/ecm/api/rest/ecm/centralTasks/getTasks/pool/"

// TaskCentralCategory é uma categoria do resumo da central de tarefas
// (fillTypeTasks): tarefas a concluir, pools de grupo/papel (cada pool vira um
// filho, com o código em Pool), minhas solicitações e tarefas sob gerência.
type TaskCentralCategory struct {
	Type        string                `json:"type"`
	Pool        string                `json:"pool,omitempty"` // código do pool ("Pool:Group:TI") nos filhos
	Description string                `json:"description"`
	Total       int                   `json:"total"`
	Known       int                   `json:"known"`
	Unknown     int                   `json:"unknown"`
	Children    []TaskCentralCategory `json:"children,omitempty"`
}

const restCentralTasksSummaryPath = "/ecm/api/rest/ecm/centralTasks/fillTypeTasks"

// centralCategoryWire é o shape da API; a conversão renomeia taskId/totalTask.
type centralCategoryWire struct {
	Type         string                `json:"type"`
	TaskID       string                `json:"taskId"`
	Description  string                `json:"description"`
	TotalTask    int                   `json:"totalTask"`
	TotalKnown   int                   `json:"totalKnown"`
	TotalUnknown int                   `json:"totalUnknown"`
	Children     []centralCategoryWire `json:"children"`
}

func centralCategories(wire []centralCategoryWire) []TaskCentralCategory {
	if len(wire) == 0 {
		return nil
	}
	out := make([]TaskCentralCategory, 0, len(wire))
	for _, w := range wire {
		out = append(out, TaskCentralCategory{
			Type:        w.Type,
			Pool:        w.TaskID,
			Description: w.Description,
			Total:       w.TotalTask,
			Known:       w.TotalKnown,
			Unknown:     w.TotalUnknown,
			Children:    centralCategories(w.Children),
		})
	}
	return out
}

// TaskCentralSummary devolve o resumo da central de tarefas de um usuário: os
// contadores por categoria e os pools que ele enxerga, com a contagem de cada
// um. login vazio = o usuário autenticado. ⚠️ O servidor responde uma lista
// vazia para usuários que nunca abriram a central no portal (comportamento
// observado na homologação em 2026-08-01) — não é erro.
func (c *Client) TaskCentralSummary(ctx context.Context, login string) ([]TaskCentralCategory, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	var code string
	var err error
	if login == "" {
		code, err = c.ResolveUserCode(ctx)
	} else {
		code, err = c.resolveUserFilter(ctx, login)
	}
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("taskUserId", code)
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restCentralTasksSummaryPath)+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("centralTasks/fillTypeTasks", status, body)
	}
	var wire []centralCategoryWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("resposta inesperada de centralTasks/fillTypeTasks: %w", err)
	}
	return centralCategories(wire), nil
}

// ListPoolTasks lista as tarefas em aberto paradas num pool — tarefas que
// nenhum usuário assumiu. O poolCode é o código completo do pool, como
// "Pool:Group:TI" ou "Pool:Role:financeiro". A rota só devolve tarefas em
// aberto; filtros de status/SLA/solicitante não se aplicam. O Fluig limita a
// consulta aos pools que o usuário autenticado enxerga na central de tarefas.
func (c *Client) ListPoolTasks(ctx context.Context, poolCode string, limit int) ([]TaskSummary, error) {
	me, err := c.ResolveUserCode(ctx)
	if err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []TaskSummary
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("taskId", poolCode)
		params.Set("offset", "0")
		params.Set("rows", strconv.Itoa(pageSize))
		params.Set("page", strconv.Itoa(page))
		params.Set("sidx", "processInstanceId")
		params.Set("sord", "asc")
		endpoint := c.url(restCentralTasksPoolPath+url.PathEscape(me)) + "?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("centralTasks/pool", status, body)
		}
		var parsed struct {
			Invdata []struct {
				ProcessInstanceID     int    `json:"processInstanceId"`
				ProcessID             string `json:"processId"`
				ProcessDescription    string `json:"processDescription"`
				RequesterID           string `json:"requesterId"`
				RequesterName         string `json:"requesterName"`
				RequesterLogin        string `json:"requesterLogin"`
				StateDescription      string `json:"stateDescription"`
				ColleagueName         string `json:"colleagueName"`
				MovementSequence      int    `json:"movementSequence"`
				StateSequence         int    `json:"stateSequence"`
				StartDateProcess      int64  `json:"startDateProcess"`
				Expired               bool   `json:"expired"`
				ApproachingExpiration bool   `json:"approachingExpiration"`
			} `json:"invdata"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de centralTasks/pool: %w", err)
		}
		items := parsed.Invdata
		// Paginação medida ao vivo: com página seguinte, a rota devolve
		// rows+1 itens e o ÚLTIMO repete como primeiro da próxima página —
		// descarta o excedente e segue. totalpages/totalrecords do corpo não
		// refletem o total real e ficam de fora.
		hasNext := len(items) > pageSize
		if hasNext {
			items = items[:pageSize]
		}
		for _, it := range items {
			sla := "ON_TIME"
			if it.Expired {
				sla = "EXPIRED"
			} else if it.ApproachingExpiration {
				sla = "WARNING"
			}
			var start *time.Time
			if it.StartDateProcess > 0 {
				ts := time.UnixMilli(it.StartDateProcess)
				start = &ts
			}
			out = append(out, TaskSummary{
				RequestID:          it.ProcessInstanceID,
				ProcessID:          it.ProcessID,
				ProcessDescription: it.ProcessDescription,
				Movement:           it.MovementSequence,
				Sequence:           it.StateSequence,
				StateName:          it.StateDescription,
				Assignee:           &RequestUser{Name: it.ColleagueName, Code: poolCode},
				Requester:          &RequestUser{Name: it.RequesterName, Login: it.RequesterLogin, Code: it.RequesterID},
				Status:             "NOT_COMPLETED",
				SLAStatus:          sla,
				StartDate:          start,
			})
			if limit > 0 && len(out) >= limit {
				return out[:limit], nil
			}
		}
		if !hasNext || len(items) == 0 {
			return out, nil
		}
	}
}
