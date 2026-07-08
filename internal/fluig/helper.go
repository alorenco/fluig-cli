package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	helperPingPath      = "/fluiggersWidget/api/ping"
	helperWorkflowsBase = "/fluiggersWidget/api/workflows/"
)

// HelperInstalled verifica se a fluiggersWidget está publicada (ping → pong).
func (c *Client) HelperInstalled(ctx context.Context) (bool, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(helperPingPath), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, nil // inacessível = tratamos como ausente
	}
	body, err := readBody(resp, 4096)
	if err != nil {
		return false, nil
	}
	return resp.StatusCode == http.StatusOK && strings.Contains(strings.ToLower(body), "pong"), nil
}

// WorkflowEvent é um evento de processo a atualizar (name + código JS).
type WorkflowEvent struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

// WorkflowUpdateResult é a resposta do update de eventos da fluiggersWidget.
type WorkflowUpdateResult struct {
	ProcessID      string   `json:"processId"`
	Version        int      `json:"version"`
	HasError       bool     `json:"hasError"`
	TotalProcessed int      `json:"totalProcessed"`
	Errors         []string `json:"errors"`
	Successes      []string `json:"successes"`
}

// UpdateWorkflowEvents atualiza (cirurgicamente) eventos de um processo via
// fluiggersWidget: PUT /fluiggersWidget/api/workflows/{processId}/{version}/events
// com corpo [{name, contents}]. Requer a widget instalada.
func (c *Client) UpdateWorkflowEvents(ctx context.Context, processID string, version int, events []WorkflowEvent) (*WorkflowUpdateResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	endpoint := c.url(helperWorkflowsBase) + processID + "/" + strconv.Itoa(version) + "/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao atualizar eventos em %s: %w", c.base.Host, err)
	}
	body, err := readBody(resp, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		// A widget respondeu 404: ou o processo não existe, ou a rota sumiu.
		return nil, fmt.Errorf("%w: processo %q versão %d", ErrNotFound, processID, version)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: "fluiggersWidget events", Body: truncate(body, 512)}
	}
	var res WorkflowUpdateResult
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return nil, fmt.Errorf("resposta inesperada da fluiggersWidget: %w", err)
	}
	if res.HasError {
		return &res, fmt.Errorf("%w: %s", errServerRejected, strings.Join(res.Errors, "; "))
	}
	return &res, nil
}
