package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const restAdminGroups = "/admin/api/v1/groups"

// Group é um grupo da plataforma (módulo /admin/api/v1). Um grupo tem apenas
// `code` (identificador), `description` (rótulo humano — NÃO há campo "name") e
// `type`: "user" para grupos criados pelos administradores e "community" para
// os grupos automáticos das comunidades (prefixos MODERATOR_/MEMBER_).
type Group struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// GroupFilter parametriza a listagem. ⚠️ O servidor IGNORA os parâmetros
// `pattern` e `type` na query (validado na homologação em 2026-07-14: todas as
// variações devolvem a lista inteira) — por isso ambos os filtros são aplicados
// no cliente, sobre as páginas já buscadas.
type GroupFilter struct {
	Type   string // "user" | "community" (filtro client-side; vazio = todos)
	Search string // substring em code/description, case-insensitive (client-side)
	Limit  int    // 0 = todos
}

func matchGroup(g Group, search string) bool {
	if search == "" {
		return true
	}
	s := strings.ToLower(search)
	return strings.Contains(strings.ToLower(g.Code), s) ||
		strings.Contains(strings.ToLower(g.Description), s)
}

// ListGroups lista os grupos da plataforma (paginado; filtros aplicados no
// cliente — ver GroupFilter).
func (c *Client) ListGroups(ctx context.Context, f GroupFilter) ([]Group, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []Group
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restAdminGroups)+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("admin/v1/groups", status, body)
		}
		var parsed struct {
			Items   []Group `json:"items"`
			HasNext bool    `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de admin/v1/groups: %w", err)
		}
		for _, g := range parsed.Items {
			if f.Type != "" && !strings.EqualFold(g.Type, f.Type) {
				continue
			}
			if !matchGroup(g, f.Search) {
				continue
			}
			out = append(out, g)
			if f.Limit > 0 && len(out) >= f.Limit {
				return out[:f.Limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// GetGroup carrega um grupo pelo código. Inexistente = 404
// FDNEntityNotFoundException → ErrNotFound (exit 4).
func (c *Client) GetGroup(ctx context.Context, code string) (*Group, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restAdminGroups + "/" + url.PathEscape(code))
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, groupError(code, "admin/v1/groups/{code}", status, body)
	}
	var g Group
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, fmt.Errorf("resposta inesperada do grupo: %w", err)
	}
	return &g, nil
}

// CreateGroup cria um grupo (POST /v1/groups → 201). `description` é
// obrigatória (sem ela o servidor responde 500 EJBException); `groupType`
// vazio assume "user" (default do servidor). Código duplicado = 400
// FDNDuplicatedGroupCodeException → errServerRejected (exit 5).
func (c *Client) CreateGroup(ctx context.Context, code, description, groupType string) (*Group, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	if groupType == "" {
		groupType = "user"
	}
	payload := map[string]string{"code": code, "description": description, "type": groupType}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restAdminGroups), data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, groupError(code, "admin/v1/groups", status, body)
	}
	var g Group
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, fmt.Errorf("resposta inesperada da criação de grupo: %w", err)
	}
	return &g, nil
}

// UpdateGroup atualiza os campos dados de um grupo (PUT /v1/groups/{code}).
// ⚠️ O PUT **mescla** (validado na homologação): campos não enviados sobrevivem.
func (c *Client) UpdateGroup(ctx context.Context, code string, fields map[string]string) (*Group, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	endpoint := c.url(restAdminGroups + "/" + url.PathEscape(code))
	body, status, err := c.doJSON(ctx, http.MethodPut, endpoint, data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, groupError(code, "admin/v1/groups/{code}", status, body)
	}
	var g Group
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, fmt.Errorf("resposta inesperada da atualização de grupo: %w", err)
	}
	return &g, nil
}

// DeleteGroup exclui um grupo (DELETE /v1/groups/{code} → 204). Inexistente =
// 404 → ErrNotFound.
func (c *Client) DeleteGroup(ctx context.Context, code string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	endpoint := c.url(restAdminGroups + "/" + url.PathEscape(code))
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return groupError(code, "admin/v1/groups/{code}", status, body)
	}
	return nil
}

// ListGroupUsers lista os usuários membros de um grupo (paginado). Reaproveita
// o shape de AdminUser (a resposta traz o usuário completo).
func (c *Client) ListGroupUsers(ctx context.Context, code string, limit int) ([]AdminUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []AdminUser
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url(restAdminGroups+"/"+url.PathEscape(code)+"/users") + "?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, groupError(code, "admin/v1/groups/{code}/users", status, body)
		}
		var parsed struct {
			Items   []adminUserItem `json:"items"`
			HasNext bool            `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada dos membros do grupo: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, it.toUser())
			if limit > 0 && len(out) >= limit {
				return out[:limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// AddGroupUser adiciona um usuário a um grupo (POST /v1/groups/{code}/users).
// ⚠️ O servidor NÃO valida a existência do grupo nem do login — responde 201
// mesmo para inexistentes, criando uma associação órfã (validado na
// homologação em 2026-07-14). Para não deixar essa armadilha vazar, validamos
// grupo e usuário ANTES (GetGroup + GetAdminUser), devolvendo exit 4 limpo.
func (c *Client) AddGroupUser(ctx context.Context, code, login string) error {
	if _, err := c.GetGroup(ctx, code); err != nil {
		return err
	}
	if _, err := c.GetAdminUser(ctx, login); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"login": login})
	if err != nil {
		return err
	}
	endpoint := c.url(restAdminGroups + "/" + url.PathEscape(code) + "/users")
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, data)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return groupError(code, "admin/v1/groups/{code}/users", status, body)
	}
	return nil
}

// RemoveGroupUser remove um usuário de um grupo (DELETE
// /v1/groups/{code}/users/{login} → 204). Valida o grupo antes (para
// distinguir grupo inexistente de "usuário não é membro"); um 404 do DELETE
// significa que o usuário não é membro do grupo.
func (c *Client) RemoveGroupUser(ctx context.Context, code, login string) error {
	if _, err := c.GetGroup(ctx, code); err != nil {
		return err
	}
	endpoint := c.url(restAdminGroups + "/" + url.PathEscape(code) + "/users/" + url.PathEscape(login))
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: o usuário %q não é membro do grupo %q", ErrNotFound, login, code)
	}
	if status < 200 || status >= 300 {
		return groupError(code, "admin/v1/groups/{code}/users/{login}", status, body)
	}
	return nil
}

// groupError traduz os erros do módulo de grupos. Como no módulo de usuários,
// as exceções FDN chegam com `message` VAZIO (só o `code`); grupo inexistente
// responde 404 FDNEntityNotFoundException e código duplicado 400
// FDNDuplicatedGroupCodeException.
func groupError(code, op string, status int, body []byte) error {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)
	switch parsed.Code {
	case "FDNEntityNotFoundException":
		return fmt.Errorf("%w: grupo %q", ErrNotFound, code)
	case "FDNDuplicatedGroupCodeException":
		return fmt.Errorf("%w: já existe um grupo com o código %q", errServerRejected, code)
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: grupo %q", ErrNotFound, code)
	}
	if parsed.Message == "" && parsed.Code != "" {
		return fmt.Errorf("%w: %s", errServerRejected, parsed.Code)
	}
	return restRequestError(op, status, body)
}
