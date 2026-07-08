package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const restMechanismBase = "/ecm/api/rest/ecm/mechanism/"

// Valores fixos de um mecanismo de atribuição customizado
const (
	customAssignmentType = 1
	customControlClass   = "com.datasul.technology.webdesk.workflow.assignment.customization.CustomAssignmentImpl"
)

// Campos do DTO (nomes do Fluig — "Mecanism" sem "h"). O código JS fica em
// attributionMecanismDescription; name/description são metadados.
const (
	fieldMechCode = "attributionMecanismDescription"
	fieldMechName = "name"
	fieldMechDesc = "description"
	fieldMechPK   = "attributionMecanismPK"
)

// Mechanism é um mecanismo de atribuição customizado. raw preserva o JSON
// completo para o round-trip no update (troca só o código).
type Mechanism struct {
	CompanyID   int
	ID          string
	Name        string
	Description string
	Code        string // attributionMecanismDescription: código JS

	raw map[string]json.RawMessage
}

// ListMechanisms retorna os mecanismos customizados (GET getCustomAttributionMechanismList).
func (c *Client) ListMechanisms(ctx context.Context) ([]Mechanism, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restMechanismBase+"getCustomAttributionMechanismList"), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "getCustomAttributionMechanismList", Body: truncate(string(body), 512)}
	}
	rawItems, err := parseMechanismList(body)
	if err != nil {
		return nil, err
	}
	out := make([]Mechanism, 0, len(rawItems))
	for _, raw := range rawItems {
		out = append(out, mechanismFromRaw(raw))
	}
	return out, nil
}

// parseMechanismList aceita lista crua ou {"content":[...]}/{"items":[...]}.
func parseMechanismList(body []byte) ([]map[string]json.RawMessage, error) {
	var direct []map[string]json.RawMessage
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, nil
	}
	var wrapped struct {
		Content []map[string]json.RawMessage `json:"content"`
		Items   []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("resposta inesperada de getCustomAttributionMechanismList: %w", err)
	}
	if len(wrapped.Content) > 0 {
		return wrapped.Content, nil
	}
	return wrapped.Items, nil
}

func mechanismFromRaw(raw map[string]json.RawMessage) Mechanism {
	m := Mechanism{raw: raw}
	m.Name = jsonString(raw, fieldMechName)
	m.Description = jsonString(raw, fieldMechDesc)
	m.Code = jsonString(raw, fieldMechCode)
	if pk, ok := raw[fieldMechPK]; ok {
		var p struct {
			CompanyID             int    `json:"companyId"`
			AttributionMecanismID string `json:"attributionMecanismId"`
		}
		_ = json.Unmarshal(pk, &p)
		m.CompanyID, m.ID = p.CompanyID, p.AttributionMecanismID
	}
	return m
}

// UpdateMechanism reenvia o mecanismo existente com o código trocado (POST
// updateAttributionMechanism), preservando os demais campos do DTO.
func (c *Client) UpdateMechanism(ctx context.Context, m *Mechanism, code string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	raw := make(map[string]json.RawMessage, len(m.raw))
	for k, v := range m.raw {
		raw[k] = v
	}
	enc, err := json.Marshal(code)
	if err != nil {
		return err
	}
	raw[fieldMechCode] = enc
	return c.postMechanismWrite(ctx, "updateAttributionMechanism", raw)
}

// CreateMechanism cria um mecanismo customizado (POST createAttributionMechanism).
func (c *Client) CreateMechanism(ctx context.Context, id, name, description, code string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	payload := map[string]any{
		fieldMechPK: map[string]any{
			"companyId":             c.opts.CompanyID,
			"attributionMecanismId": id,
		},
		"assignmentType":     customAssignmentType,
		"controlClass":       customControlClass,
		"preSelectionClass":  nil,
		"configurationClass": "",
		fieldMechName:        name,
		fieldMechDesc:        description,
		fieldMechCode:        code,
	}
	return c.postMechanismWrite(ctx, "createAttributionMechanism", payload)
}

// DeleteMechanism exclui um mecanismo (DELETE deleteAttributionMechanism).
func (c *Client) DeleteMechanism(ctx context.Context, id string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	endpoint := c.url(restMechanismBase+"deleteAttributionMechanism") + "?mechanismId=" + url.QueryEscape(id)
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: mecanismo %q", ErrNotFound, id)
	}
	return checkWriteResponse("deleteAttributionMechanism", body, status)
}

func (c *Client) postMechanismWrite(ctx context.Context, op string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restMechanismBase+op), data)
	if err != nil {
		return err
	}
	return checkWriteResponse(op, body, status)
}
