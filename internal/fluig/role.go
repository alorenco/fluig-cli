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

const restAdminRoles = "/admin/api/v1/roles"

// Role é um papel da plataforma (módulo /admin/api/v1). Um papel tem apenas
// `code` (identificador) e `description` (rótulo humano). Internamente o Fluig
// trata papéis como "applications" (ver o código da exceção de duplicidade).
type Role struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// RoleFilter parametriza a listagem. Como nos grupos, o servidor ignora
// filtros na query — `Search` é aplicado no cliente (substring em
// code/description, case-insensitive).
type RoleFilter struct {
	Search string
	Limit  int // 0 = todos
}

func matchRole(r Role, search string) bool {
	if search == "" {
		return true
	}
	s := strings.ToLower(search)
	return strings.Contains(strings.ToLower(r.Code), s) ||
		strings.Contains(strings.ToLower(r.Description), s)
}

// ListRoles lista os papéis da plataforma (paginado; busca aplicada no cliente).
func (c *Client) ListRoles(ctx context.Context, f RoleFilter) ([]Role, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []Role
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restAdminRoles)+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("admin/v1/roles", status, body)
		}
		var parsed struct {
			Items   []Role `json:"items"`
			HasNext bool   `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de admin/v1/roles: %w", err)
		}
		for _, r := range parsed.Items {
			if !matchRole(r, f.Search) {
				continue
			}
			out = append(out, r)
			if f.Limit > 0 && len(out) >= f.Limit {
				return out[:f.Limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// GetRole carrega um papel pelo código. Inexistente = 404
// FDNEntityNotFoundException → ErrNotFound (exit 4).
func (c *Client) GetRole(ctx context.Context, code string) (*Role, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restAdminRoles + "/" + url.PathEscape(code))
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, roleError(code, "admin/v1/roles/{code}", status, body)
	}
	var r Role
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("resposta inesperada do papel: %w", err)
	}
	return &r, nil
}

// CreateRole cria um papel (POST /v1/roles → 200). A `description` é OPCIONAL
// (sem ela o servidor grava null) — o chamador costuma passar o próprio código
// como default. Código duplicado = 400 FDNDuplicatedApplicationCodeException →
// errServerRejected (exit 5).
func (c *Client) CreateRole(ctx context.Context, code, description string) (*Role, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	payload := map[string]string{"code": code, "description": description}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restAdminRoles), data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, roleError(code, "admin/v1/roles", status, body)
	}
	var r Role
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("resposta inesperada da criação de papel: %w", err)
	}
	return &r, nil
}

// UpdateRole atualiza os campos dados de um papel (PUT /v1/roles/{code}).
// O PUT **mescla** (o código sobrevive).
func (c *Client) UpdateRole(ctx context.Context, code string, fields map[string]string) (*Role, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	endpoint := c.url(restAdminRoles + "/" + url.PathEscape(code))
	body, status, err := c.doJSON(ctx, http.MethodPut, endpoint, data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, roleError(code, "admin/v1/roles/{code}", status, body)
	}
	var r Role
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("resposta inesperada da atualização de papel: %w", err)
	}
	return &r, nil
}

// DeleteRole exclui um papel (DELETE /v1/roles/{code} → 204). Inexistente =
// 404 → ErrNotFound.
func (c *Client) DeleteRole(ctx context.Context, code string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	endpoint := c.url(restAdminRoles + "/" + url.PathEscape(code))
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return roleError(code, "admin/v1/roles/{code}", status, body)
	}
	return nil
}

// ListRoleUsers lista os usuários vinculados a um papel (paginado; reusa o
// shape de AdminUser).
func (c *Client) ListRoleUsers(ctx context.Context, code string, limit int) ([]AdminUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []AdminUser
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url(restAdminRoles+"/"+url.PathEscape(code)+"/users") + "?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, roleError(code, "admin/v1/roles/{code}/users", status, body)
		}
		var parsed struct {
			Items   []adminUserItem `json:"items"`
			HasNext bool            `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada dos usuários do papel: %w", err)
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

// AddRoleUser vincula um usuário a um papel (POST /v1/roles/{code}/users).
// Diferente dos grupos, o servidor VALIDA a existência (404 para login/papel
// inexistente) — mas com a MESMA exceção genérica (FDNEntityNotFoundException),
// sem dizer qual falta. Por isso pré-validamos papel e usuário para devolver a
// mensagem certa (exit 4).
func (c *Client) AddRoleUser(ctx context.Context, code, login string) error {
	if _, err := c.GetRole(ctx, code); err != nil {
		return err
	}
	if _, err := c.GetAdminUser(ctx, login); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"login": login})
	if err != nil {
		return err
	}
	endpoint := c.url(restAdminRoles + "/" + url.PathEscape(code) + "/users")
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, data)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return roleError(code, "admin/v1/roles/{code}/users", status, body)
	}
	return nil
}

// RemoveRoleUser desvincula um usuário de um papel (DELETE
// /v1/roles/{code}/users/{login} → 204). Valida o papel antes; um 404 do
// DELETE significa que o usuário não tem o papel.
func (c *Client) RemoveRoleUser(ctx context.Context, code, login string) error {
	if _, err := c.GetRole(ctx, code); err != nil {
		return err
	}
	endpoint := c.url(restAdminRoles + "/" + url.PathEscape(code) + "/users/" + url.PathEscape(login))
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: o usuário %q não tem o papel %q", ErrNotFound, login, code)
	}
	if status < 200 || status >= 300 {
		return roleError(code, "admin/v1/roles/{code}/users/{login}", status, body)
	}
	return nil
}

// roleError traduz os erros do módulo de papéis (espelha groupError). Papel
// inexistente = 404 FDNEntityNotFoundException; código duplicado = 400
// FDNDuplicatedApplicationCodeException (papel = "application" internamente).
func roleError(code, op string, status int, body []byte) error {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)
	switch parsed.Code {
	case "FDNEntityNotFoundException":
		return fmt.Errorf("%w: papel %q", ErrNotFound, code)
	case "FDNDuplicatedApplicationCodeException":
		return fmt.Errorf("%w: já existe um papel com o código %q", errServerRejected, code)
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: papel %q", ErrNotFound, code)
	}
	if parsed.Message == "" && parsed.Code != "" {
		return fmt.Errorf("%w: %s", errServerRejected, parsed.Code)
	}
	return restRequestError(op, status, body)
}
