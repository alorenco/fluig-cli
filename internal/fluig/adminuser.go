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
	FullName  string     `json:"fullName"`
	State     string     `json:"state"` // ACTIVE | ...
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Roles     []string   `json:"roles,omitempty"`
	Groups    []string   `json:"groups,omitempty"`
}

// adminUserItem é o shape cru da API.
type adminUserItem struct {
	Login          string   `json:"login"`
	Code           string   `json:"code"`
	Email          string   `json:"email"`
	FullName       string   `json:"fullName"`
	State          string   `json:"state"`
	LastUpdateDate string   `json:"lastUpdateDate"`
	Roles          []string `json:"roles"`
	Groups         []string `json:"groups"`
}

func (it adminUserItem) toUser() AdminUser {
	return AdminUser{
		Login:     it.Login,
		Code:      it.Code,
		Email:     it.Email,
		FullName:  it.FullName,
		State:     it.State,
		UpdatedAt: requestTime(it.LastUpdateDate),
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
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: usuário %q", ErrNotFound, login)
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("admin/v1/users/{login}", status, body)
	}
	var it adminUserItem
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, fmt.Errorf("resposta inesperada do usuário: %w", err)
	}
	u := it.toUser()
	return &u, nil
}
