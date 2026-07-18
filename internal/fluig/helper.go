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

// Context-roots dos componentes auxiliares reconhecidos. O fluigcli publica o
// fluigcliHelper (próprio, embutido no binário); a fluiggersWidget da
// comunidade continua aceita como fallback em servidores que já a tenham.
const (
	HelperFluigcli  = "fluigcliHelper"
	HelperFluiggers = "fluiggersWidget"
)

// helperRoots em ordem de preferência na resolução.
var helperRoots = []string{HelperFluigcli, HelperFluiggers}

// ResolveHelper descobre qual componente auxiliar está publicado no servidor
// (ping → pong em cada context-root, preferindo o fluigcliHelper) e devolve o
// context-root; "" = nenhum instalado. Resultado cacheado por execução.
func (c *Client) ResolveHelper(ctx context.Context) (string, error) {
	if c.helperRoot != nil {
		return *c.helperRoot, nil
	}
	if err := c.EnsureSession(ctx); err != nil {
		return "", err
	}
	root := ""
	for _, r := range helperRoots {
		if c.helperPong(ctx, r) {
			root = r
			break
		}
	}
	c.helperRoot = &root
	return root, nil
}

// helperPong verifica se GET /<root>/api/ping responde 200 "pong".
func (c *Client) helperPong(ctx context.Context, root string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/"+root+"/api/ping"), nil)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	body, err := readBody(resp, 4096)
	if err != nil {
		return false
	}
	return resp.StatusCode == http.StatusOK && strings.Contains(strings.ToLower(body), "pong")
}

// HelperInstalled informa se algum componente auxiliar está publicado
// (fluigcliHelper ou fluiggersWidget).
func (c *Client) HelperInstalled(ctx context.Context) (bool, error) {
	root, err := c.ResolveHelper(ctx)
	return root != "", err
}

// WorkflowEvent é um evento de processo a atualizar (name + código JS).
type WorkflowEvent struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

// WorkflowUpdateResult é a resposta do update de eventos do helper.
type WorkflowUpdateResult struct {
	ProcessID      string   `json:"processId"`
	Version        int      `json:"version"`
	HasError       bool     `json:"hasError"`
	TotalProcessed int      `json:"totalProcessed"`
	Errors         []string `json:"errors"`
	Successes      []string `json:"successes"`
}

// UpdateWorkflowEvents atualiza (cirurgicamente) eventos de um processo via
// componente auxiliar: PUT /<helper>/api/workflows/{processId}/{version}/events
// com corpo [{name, contents}]. Requer o helper instalado.
func (c *Client) UpdateWorkflowEvents(ctx context.Context, processID string, version int, events []WorkflowEvent) (*WorkflowUpdateResult, error) {
	root, err := c.ResolveHelper(ctx)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, ErrHelperMissing
	}
	payload, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	endpoint := c.url("/"+root+"/api/workflows/") + processID + "/" + strconv.Itoa(version) + "/events"
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
		// O helper respondeu 404: ou o processo não existe, ou a rota sumiu.
		return nil, fmt.Errorf("%w: processo %q versão %d", ErrNotFound, processID, version)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: root + " events", Body: truncate(body, 512)}
	}
	var res WorkflowUpdateResult
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return nil, fmt.Errorf("resposta inesperada do %s: %w", root, err)
	}
	if res.HasError {
		return &res, fmt.Errorf("%w: %s", errServerRejected, strings.Join(res.Errors, "; "))
	}
	return &res, nil
}
