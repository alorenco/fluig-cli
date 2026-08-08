package fluig

// Vínculos usuário↔papel e usuário↔grupo pelo LADO DO USUÁRIO
// (ROADMAP §5.1).
//
// O projeto registrou por engano, em 2026-07-14, que estas rotas não existiam,
// e o erro chegou à documentação publicada. Elas existem no swagger do módulo
// admin e respondem (medido na homologação em 2026-08-08):
//
//	GET    /v1/users/{login}/roles           lista os papéis do usuário
//	POST   /v1/users/{login}/roles           {"code": "<papel>"}
//	DELETE /v1/users/{login}/roles/{code}
//	GET    /v1/users/{login}/groups          lista os grupos do usuário
//	POST   /v1/users/{login}/groups          {"code": "<grupo>"}
//	DELETE /v1/users/{login}/groups/{code}
//
// São o mesmo vínculo que o `role add-user` e o `group add-user` escrevem pelo
// outro lado — só muda por onde se entra. Quem administra costuma pensar no
// usuário, não no papel.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// userLinkKind distingue as duas famílias de vínculo. Os dois lados têm o
// mesmo contrato, então o código é um só e o tipo escolhe a rota, o rótulo das
// mensagens e como validar o alvo.
type userLinkKind struct {
	segmento string // "roles" | "groups"
	singular string // "papel" | "grupo"
}

var (
	userLinkRoles  = userLinkKind{segmento: "roles", singular: "papel"}
	userLinkGroups = userLinkKind{segmento: "groups", singular: "grupo"}
)

// userLinkPath monta /admin/api/v1/users/{login}/{roles|groups}[/{code}].
func (c *Client) userLinkPath(kind userLinkKind, login, code string) string {
	p := restAdminUsers + "/" + url.PathEscape(login) + "/" + kind.segmento
	if code != "" {
		p += "/" + url.PathEscape(code)
	}
	return c.url(p)
}

// listUserLinks pagina uma das duas listagens. O item devolvido pelo servidor
// carrega code + description (papel) e mais o type (grupo), então os dois
// parses cabem no mesmo shape.
func (c *Client) listUserLinks(ctx context.Context, kind userLinkKind, login string) ([]userLinkItem, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []userLinkItem
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.userLinkPath(kind, login, "") + "?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, userLinkError(kind, login, "", status, body)
		}
		var parsed struct {
			Items   []userLinkItem `json:"items"`
			HasNext bool           `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada dos %s do usuário: %w", kind.segmento, err)
		}
		out = append(out, parsed.Items...)
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

type userLinkItem struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// ListUserRoles lista os papéis vinculados a um usuário.
func (c *Client) ListUserRoles(ctx context.Context, login string) ([]Role, error) {
	items, err := c.listUserLinks(ctx, userLinkRoles, login)
	if err != nil {
		return nil, err
	}
	out := make([]Role, 0, len(items))
	for _, it := range items {
		out = append(out, Role{Code: it.Code, Description: it.Description})
	}
	return out, nil
}

// ListUserGroups lista os grupos de que um usuário participa.
func (c *Client) ListUserGroups(ctx context.Context, login string) ([]Group, error) {
	items, err := c.listUserLinks(ctx, userLinkGroups, login)
	if err != nil {
		return nil, err
	}
	out := make([]Group, 0, len(items))
	for _, it := range items {
		// userLinkItem tem exatamente os campos de Group, nessa ordem.
		out = append(out, Group(it))
	}
	return out, nil
}

// addUserLink vincula, pré-validando os dois lados.
//
// A pré-validação é OBRIGATÓRIA no caso dos grupos. Contrato medido na
// homologação em 2026-08-08:
//
//	                        papel                    grupo
//	POST novo               200                      200
//	POST duplicado          200 (idempotente)        400 FDNDuplicatedUserInGroup
//	POST código inexistente 404 FDNRole              200 — E NÃO FAZ NADA
//	POST login inexistente  404 FDNUser              200 — E NÃO FAZ NADA
//
// Ou seja: o lado dos grupos aceita em silêncio um código ou um login que não
// existem, responde 200 e não cria vínculo nenhum (confirmado relendo a lista
// depois). Sem a pré-validação, a CLI confirmaria uma associação que nunca
// aconteceu — o mesmo defeito que o §2.11-H corrigiu no dataset delete.
//
// No lado dos papéis o servidor valida e ainda diz QUAL entidade falta
// (FDNRole × FDNUser). Mesmo assim pré-validamos igual, para a mensagem não
// depender de texto em inglês do servidor.
func (c *Client) addUserLink(ctx context.Context, kind userLinkKind, login, code string, existe func(context.Context, string) error) error {
	if _, err := c.GetAdminUser(ctx, login); err != nil {
		return err
	}
	if err := existe(ctx, code); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.userLinkPath(kind, login, ""), data)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return userLinkError(kind, login, code, status, body)
	}
	return nil
}

// AddUserRole vincula um papel a um usuário. Mesmo vínculo do AddRoleUser.
func (c *Client) AddUserRole(ctx context.Context, login, code string) error {
	return c.addUserLink(ctx, userLinkRoles, login, code, func(ctx context.Context, code string) error {
		_, err := c.GetRole(ctx, code)
		return err
	})
}

// AddUserGroup vincula um grupo a um usuário. Mesmo vínculo do AddGroupUser.
func (c *Client) AddUserGroup(ctx context.Context, login, code string) error {
	return c.addUserLink(ctx, userLinkGroups, login, code, func(ctx context.Context, code string) error {
		_, err := c.GetGroup(ctx, code)
		return err
	})
}

// removeUserLink desvincula. O 404 do DELETE significa que o vínculo não
// existe — a mensagem diz isso, em vez de "não encontrado" solto.
func (c *Client) removeUserLink(ctx context.Context, kind userLinkKind, login, code string) error {
	if _, err := c.GetAdminUser(ctx, login); err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodDelete, c.userLinkPath(kind, login, code), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: o usuário %q não tem o %s %q", ErrNotFound, login, kind.singular, code)
	}
	if status < 200 || status >= 300 {
		return userLinkError(kind, login, code, status, body)
	}
	return nil
}

// RemoveUserRole desvincula um papel de um usuário.
func (c *Client) RemoveUserRole(ctx context.Context, login, code string) error {
	return c.removeUserLink(ctx, userLinkRoles, login, code)
}

// RemoveUserGroup desvincula um grupo de um usuário.
func (c *Client) RemoveUserGroup(ctx context.Context, login, code string) error {
	return c.removeUserLink(ctx, userLinkGroups, login, code)
}

// userLinkError traduz os erros destas rotas. Espelha o groupError/roleError de
// propósito: estes comandos são a MESMA operação por outra porta, então a
// mensagem e o exit code precisam bater com os do `group add-user` e
// `role add-user`. Duplicidade de grupo, por exemplo, sai como recusa do
// servidor (exit 5) nos dois caminhos.
func userLinkError(kind userLinkKind, login, code string, status int, body []byte) error {
	if status == http.StatusNotFound {
		alvo := fmt.Sprintf("usuário %q", login)
		if code != "" {
			alvo = fmt.Sprintf("%s %q do usuário %q", kind.singular, code, login)
		}
		return fmt.Errorf("%w: %s", ErrNotFound, alvo)
	}
	return restRequestError("admin/v1/users/{login}/"+kind.segmento, status, body)
}
