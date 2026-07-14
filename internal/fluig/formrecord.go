package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

// restCardsBase monta o caminho dos registros de um formulário.
func restCardsBase(documentID int) string {
	return "/ecm-forms/api/v2/cardindex/" + strconv.Itoa(documentID) + "/cards"
}

// FormRecord é um registro (card) de formulário. Values é o mapa campo →
// valor (a API devolve array {fieldId, value}; valor null vira "").
type FormRecord struct {
	CardID   int64               `json:"cardId"`
	Version  int                 `json:"version"`
	FormID   int64               `json:"formId"` // parentDocumentId
	Active   bool                `json:"active"`
	Values   map[string]string   `json:"values"`
	Children []map[string]string `json:"children,omitempty"` // linhas de tabela pai×filho
}

// cardField é o par campo/valor cru da API.
type cardField struct {
	FieldID string  `json:"fieldId"`
	Value   *string `json:"value"`
}

func fieldsToMap(fields []cardField) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		v := ""
		if f.Value != nil {
			v = *f.Value
		}
		out[f.FieldID] = v
	}
	return out
}

// mapToFields converte o mapa em array {fieldId, value} com ordem estável.
func mapToFields(values map[string]string) []map[string]string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]string{"fieldId": k, "value": values[k]})
	}
	return out
}

// cardFind é o CardFind cru da API.
type cardFind struct {
	CardID           int64       `json:"cardId"`
	Version          int         `json:"version"`
	ParentDocumentID int64       `json:"parentDocumentId"`
	ActiveVersion    bool        `json:"activeVersion"`
	Values           []cardField `json:"values"`
	Children         []struct {
		Values []cardField `json:"values"`
	} `json:"children"`
}

func (c cardFind) toRecord() FormRecord {
	rec := FormRecord{
		CardID:  c.CardID,
		Version: c.Version,
		FormID:  c.ParentDocumentID,
		Active:  c.ActiveVersion,
		Values:  fieldsToMap(c.Values),
	}
	for _, ch := range c.Children {
		rec.Children = append(rec.Children, fieldsToMap(ch.Values))
	}
	return rec
}

// ListFormRecords lista os registros de um formulário (paginado). filter é a
// expressão `$filter` da API, repassada crua; limit 0 = todas as páginas.
func (c *Client) ListFormRecords(ctx context.Context, documentID int, filter string, limit int) ([]FormRecord, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []FormRecord
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		if filter != "" {
			params.Set("$filter", filter)
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restCardsBase(documentID))+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("%w: formulário %d", ErrNotFound, documentID)
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("v2/cardindex/{id}/cards", status, body)
		}
		var parsed struct {
			Items   []cardFind `json:"items"`
			HasNext bool       `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de cardindex/cards: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, it.toRecord())
			if limit > 0 && len(out) >= limit {
				return out[:limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// GetFormRecord carrega um registro (com as linhas filhas, se houver).
func (c *Client) GetFormRecord(ctx context.Context, documentID, cardID int) (*FormRecord, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restCardsBase(documentID) + "/" + strconv.Itoa(cardID))
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: registro %d do formulário %d", ErrNotFound, cardID, documentID)
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("v2/cardindex/{id}/cards/{cardId}", status, body)
	}
	var cf cardFind
	if err := json.Unmarshal(body, &cf); err != nil {
		return nil, fmt.Errorf("resposta inesperada do registro: %w", err)
	}
	rec := cf.toRecord()
	return &rec, nil
}

// CreateFormRecord cria um registro com os valores dados.
func (c *Client) CreateFormRecord(ctx context.Context, documentID int, values map[string]string) (*FormRecord, error) {
	return c.writeFormRecord(ctx, http.MethodPost, restCardsBase(documentID), documentID, 0, values)
}

// UpdateFormRecord atualiza um registro existente (PUT — a semântica
// substituir × mesclar é do servidor; ver a nota no CLAUDE.md após o E2E).
func (c *Client) UpdateFormRecord(ctx context.Context, documentID, cardID int, values map[string]string) (*FormRecord, error) {
	return c.writeFormRecord(ctx, http.MethodPut, restCardsBase(documentID)+"/"+strconv.Itoa(cardID), documentID, cardID, values)
}

func (c *Client) writeFormRecord(ctx context.Context, method, path string, documentID, cardID int, values map[string]string) (*FormRecord, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{"values": mapToFields(values)})
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, method, c.url(path), payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		if cardID > 0 {
			return nil, fmt.Errorf("%w: registro %d do formulário %d", ErrNotFound, cardID, documentID)
		}
		return nil, fmt.Errorf("%w: formulário %d", ErrNotFound, documentID)
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("v2/cardindex/{id}/cards", status, body)
	}
	var cf cardFind
	if err := json.Unmarshal(body, &cf); err != nil {
		return nil, fmt.Errorf("resposta inesperada da gravação do registro: %w", err)
	}
	rec := cf.toRecord()
	return &rec, nil
}

// DeleteFormRecord exclui um registro do formulário.
func (c *Client) DeleteFormRecord(ctx context.Context, documentID, cardID int) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	endpoint := c.url(restCardsBase(documentID) + "/" + strconv.Itoa(cardID))
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: registro %d do formulário %d", ErrNotFound, cardID, documentID)
	}
	if status < 200 || status >= 300 {
		return restRequestError("v2/cardindex/{id}/cards/{cardId}", status, body)
	}
	return nil
}
