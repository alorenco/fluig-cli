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
