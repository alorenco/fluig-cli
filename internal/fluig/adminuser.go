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

const restAdminUsers = "/admin/api/v1/users"

// AdminUser é um usuário da plataforma (módulo /admin/api/v1).
type AdminUser struct {
	Login     string     `json:"login"`
	Code      string     `json:"code"`
	Email     string     `json:"email"`
	FirstName string     `json:"firstName,omitempty"`
	LastName  string     `json:"lastName,omitempty"`
	FullName  string     `json:"fullName"`
	State     string     `json:"state"` // ACTIVE | BLOCKED
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Roles     []string   `json:"roles,omitempty"`
	Groups    []string   `json:"groups,omitempty"`
}

// adminUserItem é o shape cru da API. lastUpdateDate vem como string ISO no
// GET e como número (epoch millis) no PUT/POST — por isso json.RawMessage +
// flexTime.
type adminUserItem struct {
	Login          string          `json:"login"`
	Code           string          `json:"code"`
	Email          string          `json:"email"`
	FirstName      string          `json:"firstName"`
	LastName       string          `json:"lastName"`
	FullName       string          `json:"fullName"`
	State          string          `json:"state"`
	LastUpdateDate json.RawMessage `json:"lastUpdateDate"`
	Roles          []string        `json:"roles"`
	Groups         []string        `json:"groups"`
}

func (it adminUserItem) toUser() AdminUser {
	return AdminUser{
		Login:     it.Login,
		Code:      it.Code,
		Email:     it.Email,
		FirstName: it.FirstName,
		LastName:  it.LastName,
		FullName:  it.FullName,
		State:     it.State,
		UpdatedAt: flexTime(it.LastUpdateDate),
		Roles:     it.Roles,
		Groups:    it.Groups,
	}
}

// AdminUserFilter parametriza a busca de usuários.
type AdminUserFilter struct {
	Pattern      string // busca textual (login/nome/e-mail)
	Role         string // usuários com o papel
	ShowInactive bool   // inclui usuários desativados
	Limit        int    // 0 = todas as páginas
}

// ListAdminUsers lista os usuários da plataforma (paginado).
func (c *Client) ListAdminUsers(ctx context.Context, f AdminUserFilter) ([]AdminUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []AdminUser
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		if f.Pattern != "" {
			params.Set("pattern", f.Pattern)
		}
		if f.Role != "" {
			params.Set("role", f.Role)
		}
		if f.ShowInactive {
			params.Set("showInactive", "true")
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restAdminUsers)+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("admin/v1/users", status, body)
		}
		var parsed struct {
			Items   []adminUserItem `json:"items"`
			HasNext bool            `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de admin/v1/users: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, it.toUser())
			if f.Limit > 0 && len(out) >= f.Limit {
				return out[:f.Limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// GetAdminUser carrega um usuário pelo login, com papéis e grupos expandidos.
func (c *Client) GetAdminUser(ctx context.Context, login string) (*AdminUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restAdminUsers+"/"+url.PathEscape(login)) + "?expand=roles&expand=groups"
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, adminUserError(login, "admin/v1/users/{login}", status, body)
	}
	var it adminUserItem
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, fmt.Errorf("resposta inesperada do usuário: %w", err)
	}
	u := it.toUser()
	return &u, nil
}

// adminUserError traduz os erros do módulo admin. As exceções vêm com
// `message` VAZIO — só o campo `code` (validado na homologação em 2026-07-14);
// e usuário inexistente responde 400 (FDNInvalidUserCodeException/
// FDNEntityNotFoundException), não 404. Mapeamos os códigos conhecidos para
// mensagens pt-BR e para ErrNotFound (exit 4) quando cabível.
func adminUserError(login, op string, status int, body []byte) error {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)
	switch parsed.Code {
	case "FDNInvalidUserCodeException", "FDNEntityNotFoundException":
		return fmt.Errorf("%w: usuário %q", ErrNotFound, login)
	case "FDNDuplicatedLoginException":
		return fmt.Errorf("%w: já existe um usuário com o login %q", errServerRejected, login)
	case "FDNEmptyPasswordException":
		return fmt.Errorf("%w: a senha do novo usuário é obrigatória", errServerRejected)
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: usuário %q", ErrNotFound, login)
	}
	if parsed.Message == "" && parsed.Code != "" {
		return fmt.Errorf("%w: %s", errServerRejected, parsed.Code)
	}
	return restRequestError(op, status, body)
}

// AdminUserCreate são os dados de um usuário novo (campos obrigatórios da API).
type AdminUserCreate struct {
	Login     string
	Code      string // userCode; se vazio, a CLI usa o login
	Email     string
	FirstName string
	LastName  string
	FullName  string
	Password  string
}

// CreateAdminUser cria um usuário na plataforma (POST /v1/users).
func (c *Client) CreateAdminUser(ctx context.Context, u AdminUserCreate) (*AdminUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	code := u.Code
	if code == "" {
		code = u.Login
	}
	payload := map[string]string{
		"login":     u.Login,
		"code":      code,
		"email":     u.Email,
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"password":  u.Password,
	}
	if u.FullName != "" {
		payload["fullName"] = u.FullName
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restAdminUsers), data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, adminUserError(u.Login, "admin/v1/users", status, body)
	}
	var it adminUserItem
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, fmt.Errorf("resposta inesperada da criação de usuário: %w", err)
	}
	created := it.toUser()
	return &created, nil
}

// UpdateAdminUser atualiza os campos dados de um usuário (PUT /v1/users/{login}).
// ⚠️ O PUT **mescla** (validado na homologação): campos não enviados
// sobrevivem — por isso o chamador envia só o que mudou. `fullName` é
// independente de firstName/lastName; envie-o se quiser mudá-lo.
func (c *Client) UpdateAdminUser(ctx context.Context, login string, fields map[string]string) (*AdminUser, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	endpoint := c.url(restAdminUsers + "/" + url.PathEscape(login))
	body, status, err := c.doJSON(ctx, http.MethodPut, endpoint, data)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, adminUserError(login, "admin/v1/users/{login}", status, body)
	}
	var it adminUserItem
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, fmt.Errorf("resposta inesperada da atualização de usuário: %w", err)
	}
	updated := it.toUser()
	return &updated, nil
}

// SetAdminUserActive ativa (activate) ou desativa (deactivate) um usuário.
// ⚠️ Desativado, o `state` vira **BLOCKED** (não "INACTIVE"); login
// inexistente responde 400 → ErrNotFound.
func (c *Client) SetAdminUserActive(ctx context.Context, login string, active bool) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	op := "deactivate"
	if active {
		op = "activate"
	}
	endpoint := c.url(restAdminUsers + "/" + url.PathEscape(login) + "/" + op)
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return adminUserError(login, "admin/v1/users/{login}/"+op, status, body)
	}
	return nil
}
