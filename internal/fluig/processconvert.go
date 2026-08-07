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
)

// Conversão de versão de solicitações abertas — a API é a legada do portal
// (/ecm/api/rest/ecm/processconvert), a mesma que o wizard "Converter
// processos" usa. Não existe equivalente nos swaggers (conferido em
// 2026-08-07: process-management, bpm, ecm e portal). A sessão (cookie)
// autentica; o Bearer que o navegador manda é dispensável.
//
// Semântica validada ao vivo na homologação (2026-08-07):
//   - convertProcess converte UMA solicitação por chamada e responde SEMPRE
//     HTTP 200 — sucesso e erro só se distinguem pelo texto de conversionLog
//     (localizado). Por isso o resultado é verificado pelo processVersion da
//     solicitação (REST v2), não pelo texto;
//   - o de-para (actualStates/newStates) só é validado na etapa em que a
//     solicitação está ABERTA — cobrir as demais etapas é inofensivo, e um
//     superconjunto das etapas "conversíveis" do wizard é aceito;
//   - a conversão consolida tarefas duplicadas da mesma etapa (uma
//     solicitação com duas tarefas abertas na mesma etapa fica com uma).
const restProcessConvertBase = "/ecm/api/rest/ecm/processconvert/"

// ConvertVersionInfo é uma versão liberada do processo com a contagem de
// tarefas abertas (o wizard chama de "solicitações", mas o número conta
// TAREFAS — uma solicitação com duas tarefas abertas conta duas).
type ConvertVersionInfo struct {
	Version   int `json:"version"`
	OpenTasks int `json:"openTasks"`
}

// ConvertVersions lista as versões liberadas do processo com as tarefas
// abertas de cada uma (getAllProcessVersions). A versão em edição fica fora.
func (c *Client) ConvertVersions(ctx context.Context, processID string) ([]ConvertVersionInfo, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restProcessConvertBase+"getAllProcessVersions") +
		"?processId=" + url.QueryEscape(processID)
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err := convertAPIGuard("getAllProcessVersions", status, body); err != nil {
		return nil, err
	}
	var parsed []struct {
		Version       int `json:"version"`
		OpenInstances int `json:"openInstances"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta inesperada de getAllProcessVersions: %w", err)
	}
	out := make([]ConvertVersionInfo, 0, len(parsed))
	for _, v := range parsed {
		out = append(out, ConvertVersionInfo{Version: v.Version, OpenTasks: v.OpenInstances})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// ConvertStateCounts devolve as tarefas abertas POR ETAPA de uma versão
// (getOpenProcessStateVersions), consultando as etapas informadas. Etapa sem
// tarefa aberta não aparece no mapa.
func (c *Client) ConvertStateCounts(ctx context.Context, processID string, version int, states []int) (map[int]int, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	list := make([]string, len(states))
	for i, s := range states {
		list[i] = strconv.Itoa(s)
	}
	q := url.Values{}
	q.Set("processId", processID)
	q.Set("version", strconv.Itoa(version))
	q.Set("states", "["+strings.Join(list, ",")+"]")
	endpoint := c.url(restProcessConvertBase+"getOpenProcessStateVersions") + "?" + q.Encode()
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if err := convertAPIGuard("getOpenProcessStateVersions", status, body); err != nil {
		return nil, err
	}
	var parsed struct {
		Content []struct {
			PK struct {
				Sequence int `json:"sequence"`
			} `json:"processStatePK"`
			OpenInstances int `json:"openInstances"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta inesperada de getOpenProcessStateVersions: %w", err)
	}
	out := map[int]int{}
	for _, s := range parsed.Content {
		if s.OpenInstances > 0 {
			out[s.PK.Sequence] = s.OpenInstances
		}
	}
	return out, nil
}

// ConvertInstanceSummary é uma solicitação aberta candidata à conversão
// (getInstancesToConvert). Deadline é texto pronto do servidor (localizado).
type ConvertInstanceSummary struct {
	ID        int    `json:"processInstanceId"`
	Requester string `json:"requester"`
	State     string `json:"state"`
	Assignee  string `json:"assignee"`
	Deadline  string `json:"deadline"`
}

// ConvertInstances lista as solicitações abertas de uma versão do processo,
// percorrendo as páginas do grid. ⚠️ Conta SOLICITAÇÕES — número menor que o
// OpenTasks do ConvertVersions quando há tarefas duplicadas/consenso.
func (c *Client) ConvertInstances(ctx context.Context, processID string, version int) ([]ConvertInstanceSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []ConvertInstanceSummary
	seen := map[int]bool{} // defesa contra linha repetida do grid (evita POST duplo no --all)
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("processId", processID)
		q.Set("version", strconv.Itoa(version))
		q.Set("rows", strconv.Itoa(pageSize))
		q.Set("page", strconv.Itoa(page))
		q.Set("sidx", "processInstanceId")
		q.Set("sord", "asc")
		endpoint := c.url(restProcessConvertBase+"getInstancesToConvert") + "?" + q.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if err := convertAPIGuard("getInstancesToConvert", status, body); err != nil {
			return nil, err
		}
		var parsed struct {
			Rows []struct {
				ID        int    `json:"processInstanceId"`
				Requester string `json:"requesterName"`
				State     string `json:"stateDescription"`
				Assignee  string `json:"colleagueName"`
				Deadline  string `json:"deadlineText"`
			} `json:"invdata"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de getInstancesToConvert: %w", err)
		}
		// Paginação do grid legado (mesmo quirk do centralTasks/pool, medido em
		// 2026-08-07 com rows=15: 16 itens na página 1): com página seguinte a
		// rota devolve rows+1 itens e o ÚLTIMO repete como primeiro da próxima —
		// descarta o excedente e segue. totalpages/currpage ficam de fora.
		items := parsed.Rows
		hasNext := len(items) > pageSize
		if hasNext {
			items = items[:pageSize]
		}
		for _, r := range items {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, ConvertInstanceSummary{
				ID:        r.ID,
				Requester: r.Requester,
				State:     r.State,
				Assignee:  r.Assignee,
				Deadline:  r.Deadline,
			})
		}
		if !hasNext {
			return out, nil
		}
	}
}

// ConvertCall parametriza a conversão de UMA solicitação. ActualStates e
// NewStates são arrays paralelos (de-para posicional): a etapa aberta da
// solicitação é procurada em ActualStates e movida para o NewStates
// correspondente, na versão NewVersion.
type ConvertCall struct {
	InstanceID   int
	NewVersion   int
	ActualStates []int
	NewStates    []int
}

// ConvertInstanceVersion converte uma solicitação para outra versão do
// processo (POST convertProcess) e devolve o conversionLog do servidor.
// ⚠️ O HTTP é sempre 200 e o texto é localizado — NÃO decida sucesso por
// aqui: confirme pelo processVersion da solicitação (GetRequest).
func (c *Client) ConvertInstanceVersion(ctx context.Context, call ConvertCall) (string, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"processInstanceId":  call.InstanceID,
		"newVersion":         call.NewVersion,
		"actualStates":       call.ActualStates,
		"newStates":          call.NewStates,
		"conversionSequence": 0,
	})
	if err != nil {
		return "", err
	}
	endpoint := c.url(restProcessConvertBase + "convertProcess")
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return "", err
	}
	if err := convertAPIGuard("convertProcess", status, body); err != nil {
		return "", err
	}
	var parsed struct {
		ConversionLog string `json:"conversionLog"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("resposta inesperada de convertProcess: %w", err)
	}
	return parsed.ConversionLog, nil
}

// convertAPIGuard valida o transporte da API legada de conversão. Sessão sem
// acesso (ou sem papel de admin) não vira 401: o portal responde 200 com um
// HTML de redirect — detectado aqui para não virar "resposta inesperada".
func convertAPIGuard(op string, status int, body []byte) error {
	if status < 200 || status >= 300 {
		return restRequestError("processconvert/"+op, status, body)
	}
	if b := strings.TrimSpace(string(body)); strings.HasPrefix(b, "<") {
		return fmt.Errorf("%w: o servidor recusou o acesso a processconvert/%s (a conversão de versão exige papel de administrador)", ErrAuthFailed, op)
	}
	return nil
}
