package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const restGlobalEventBase = "/ecm/api/rest/ecm/globalevent/"

// GlobalEvent é um evento global: ID e o código JS (eventDescription na API).
type GlobalEvent struct {
	CompanyID int    `json:"-"`
	ID        string `json:"id"`
	Code      string `json:"code"`
}

// globalEventDTO é o formato de fio de getEventList/saveEventList.
type globalEventDTO struct {
	GlobalEventPK struct {
		CompanyID int    `json:"companyId"`
		EventID   string `json:"eventId"`
	} `json:"globalEventPK"`
	EventDescription string `json:"eventDescription"`
}

// ListGlobalEvents retorna todos os eventos globais (GET getEventList).
func (c *Client) ListGlobalEvents(ctx context.Context) ([]GlobalEvent, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restGlobalEventBase+"getEventList"), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "getEventList", Body: truncate(string(body), 512)}
	}
	dtos, err := parseEventList(body)
	if err != nil {
		return nil, err
	}
	out := make([]GlobalEvent, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, GlobalEvent{
			CompanyID: d.GlobalEventPK.CompanyID,
			ID:        d.GlobalEventPK.EventID,
			Code:      d.EventDescription,
		})
	}
	return out, nil
}

// parseEventList aceita tanto uma lista crua quanto {"content":[...]} ou
// {"items":[...]} — variações defensivas até confirmar o formato na homologação.
func parseEventList(body []byte) ([]globalEventDTO, error) {
	var direct []globalEventDTO
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, nil
	}
	var wrapped struct {
		Content []globalEventDTO `json:"content"`
		Items   []globalEventDTO `json:"items"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("resposta inesperada de getEventList: %w", err)
	}
	if len(wrapped.Content) > 0 {
		return wrapped.Content, nil
	}
	return wrapped.Items, nil
}

// SaveGlobalEvents grava a LISTA COMPLETA de eventos: o Fluig substitui o
// conjunto inteiro, então quem edita um evento precisa reenviar todos.
func (c *Client) SaveGlobalEvents(ctx context.Context, events []GlobalEvent) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	dtos := make([]globalEventDTO, 0, len(events))
	for _, e := range events {
		var d globalEventDTO
		d.GlobalEventPK.CompanyID = c.opts.CompanyID
		d.GlobalEventPK.EventID = e.ID
		d.EventDescription = e.Code
		dtos = append(dtos, d)
	}
	payload, err := json.Marshal(dtos)
	if err != nil {
		return err
	}
	// O corpo é JSON, mas o Content-Type enviado é
	// application/x-www-form-urlencoded. ⚠️ Confirmar na homologação se
	// application/json também funciona.
	body, status, err := c.doWithContentType(ctx, http.MethodPost,
		c.url(restGlobalEventBase+"saveEventList"), payload, "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	return checkWriteResponse("saveEventList", body, status)
}

// DeleteGlobalEvent exclui um evento global (DELETE deleteGlobalEvent).
func (c *Client) DeleteGlobalEvent(ctx context.Context, id string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	endpoint := c.url(restGlobalEventBase+"deleteGlobalEvent") + "?eventName=" + url.QueryEscape(id)
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: evento global %q", ErrNotFound, id)
	}
	return checkWriteResponse("deleteGlobalEvent", body, status)
}

// checkWriteResponse interpreta respostas de escrita do Fluig: erro quando o
// status não é 2xx ou quando há {"message":{"message":"..."}} no corpo.
func checkWriteResponse(op string, body []byte, status int) error {
	var parsed struct {
		Message *struct {
			Message string `json:"message"`
		} `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)
	if parsed.Message != nil && parsed.Message.Message != "" {
		return fmt.Errorf("%w: %s", errServerRejected, parsed.Message.Message)
	}
	if status < 200 || status >= 300 {
		return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
	}
	return nil
}

// doWithContentType é como doJSON mas com Content-Type explícito.
func (c *Client) doWithContentType(ctx context.Context, method, endpoint string, body []byte, contentType string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("falha ao chamar %s: %w", c.base.Host, err)
	}
	respBody, err := readBody(resp, 8<<20)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return []byte(respBody), resp.StatusCode, nil
}
