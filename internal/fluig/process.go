package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// API REST v2 de process-management (validada na homologação em 2026-07-09).
// A resposta é paginada no envelope {items, hasNext}; o parâmetro fields não
// funciona neste endpoint (devolve itens vazios) e expand=versions multiplica
// o payload por ~25× — a listagem usa a resposta enxuta padrão.
const restProcessesPath = "/process-management/api/v2/processes"

// ProcessSummary é um processo listado pela API de process-management.
type ProcessSummary struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
	Active      bool   `json:"active"`
	Public      bool   `json:"public"`
}

// ListProcesses retorna todos os processos do servidor, percorrendo as páginas
// até hasNext=false. Processos sem categoria vêm sem a chave categoryId
// (observado na homologação: FLUIGADHOCPROCESS) — Category fica vazia.
func (c *Client) ListProcesses(ctx context.Context) ([]ProcessSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []ProcessSummary
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url(restProcessesPath) + "?" + q.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: restProcessesPath, Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items []struct {
				ProcessID          string `json:"processId"`
				ProcessDescription string `json:"processDescription"`
				CategoryID         string `json:"categoryId"`
				Active             bool   `json:"active"`
				Public             bool   `json:"public"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de %s: %w", restProcessesPath, err)
		}
		for _, it := range parsed.Items {
			out = append(out, ProcessSummary{
				ID:          it.ProcessID,
				Description: it.ProcessDescription,
				Category:    it.CategoryID,
				Active:      it.Active,
				Public:      it.Public,
			})
		}
		// Página vazia encerra mesmo com hasNext=true — defesa contra loop.
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}
