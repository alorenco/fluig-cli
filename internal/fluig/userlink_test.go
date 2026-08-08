package fluig

// Testes dos vínculos pelo lado do usuário (ROADMAP §5.1). O stub reproduz o
// contrato MEDIDO na homologação em 2026-08-08 — em especial as duas
// diferenças entre papéis e grupos, que são o motivo de a pré-validação
// existir:
//
//   - papel inexistente no POST → 404
//   - grupo inexistente no POST → 200 E NÃO FAZ NADA (silencioso)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type userLinkStub struct {
	postsRoles  []string // códigos recebidos no POST de papéis
	postsGroups []string
	deletes     []string // "roles/<code>" | "groups/<code>"
	semVinculo  bool     // DELETE responde 404 (usuário não tem o vínculo)
	duplicado   bool     // POST de grupo responde 400 (já é membro)
}

func (s *userLinkStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	// Usuário: existe só o jsilva (a pré-validação passa por aqui).
	mux.HandleFunc("/admin/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		resto := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/users/")
		partes := strings.SplitN(resto, "/", 3)
		login := partes[0]
		if login != "jsilva" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"FDNEntityNotFoundException","message":"Entity 'FDNUser' not found"}`)
			return
		}
		if len(partes) == 1 { // GET do usuário (pré-validação)
			io.WriteString(w, `{"login":"jsilva","fullName":"João Silva","state":"ACTIVE"}`)
			return
		}
		familia := partes[1]
		switch r.Method {
		case http.MethodGet:
			if familia == "roles" {
				w.Write(testdata(t, "rest_user_roles.json"))
			} else {
				w.Write(testdata(t, "rest_user_groups.json"))
			}
		case http.MethodPost:
			var corpo struct {
				Code string `json:"code"`
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &corpo)
			if familia == "roles" {
				s.postsRoles = append(s.postsRoles, corpo.Code)
			} else {
				if s.duplicado {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, `{"code":"FDNDuplicatedUserInGroupException","message":"User %q is already associated to group %q."}`, login, corpo.Code)
					return
				}
				s.postsGroups = append(s.postsGroups, corpo.Code)
			}
			fmt.Fprintf(w, `{"code":%q}`, corpo.Code)
		case http.MethodDelete:
			if s.semVinculo {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"code":"FDNEntityNotFoundException","message":"Entity 'FDNUserRole' not found"}`)
				return
			}
			s.deletes = append(s.deletes, familia+"/"+partes[2])
			w.WriteHeader(http.StatusNoContent)
		}
	})
	// Papéis e grupos: existe só o "faturista" / "TI".
	mux.HandleFunc("/admin/api/v1/roles/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/admin/api/v1/roles/") != "faturista" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"FDNEntityNotFoundException"}`)
			return
		}
		io.WriteString(w, `{"code":"faturista","description":"Faturista"}`)
	})
	mux.HandleFunc("/admin/api/v1/groups/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/admin/api/v1/groups/") != "TI" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"FDNEntityNotFoundException"}`)
			return
		}
		io.WriteString(w, `{"code":"TI","description":"Tecnologia da Informação","type":"user"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func userLinkClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-link-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListUserRolesEGroups(t *testing.T) {
	stub := &userLinkStub{}
	c := userLinkClient(t, stub.server(t).URL)
	ctx := context.Background()

	roles, err := c.ListUserRoles(ctx, "jsilva")
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 3 || roles[1].Code != "gestor_compras" || roles[1].Description != "Gestor de Compras" {
		t.Errorf("papéis inesperados: %+v", roles)
	}

	groups, err := c.ListUserGroups(ctx, "jsilva")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 || groups[2].Type != "community" {
		t.Errorf("grupos inesperados: %+v", groups)
	}
	// O grupo carrega o `type`; o papel não tem esse campo.
	if groups[1].Code != "TI" || groups[1].Type != "user" {
		t.Errorf("o type do grupo precisa vir preenchido: %+v", groups[1])
	}
}

func TestUserLinkCicloPapel(t *testing.T) {
	stub := &userLinkStub{}
	c := userLinkClient(t, stub.server(t).URL)
	ctx := context.Background()

	if err := c.AddUserRole(ctx, "jsilva", "faturista"); err != nil {
		t.Fatalf("AddUserRole: %v", err)
	}
	if len(stub.postsRoles) != 1 || stub.postsRoles[0] != "faturista" {
		t.Errorf("o POST devia mandar o código do papel: %v", stub.postsRoles)
	}
	if err := c.RemoveUserRole(ctx, "jsilva", "faturista"); err != nil {
		t.Fatalf("RemoveUserRole: %v", err)
	}
	if len(stub.deletes) != 1 || stub.deletes[0] != "roles/faturista" {
		t.Errorf("DELETE inesperado: %v", stub.deletes)
	}
}

// A pré-validação é o ponto do item: o servidor aceita grupo inexistente com
// 200 e não cria vínculo nenhum. Sem a checagem, a CLI confirmaria uma
// associação que nunca aconteceu.
func TestUserLinkAlvoInexistenteNaoEnviaPOST(t *testing.T) {
	casos := []struct {
		nome string
		fn   func(*Client, context.Context) error
		post func(*userLinkStub) []string
	}{
		{"papel", func(c *Client, ctx context.Context) error {
			return c.AddUserRole(ctx, "jsilva", "zz_nao_existe")
		}, func(s *userLinkStub) []string { return s.postsRoles }},
		{"grupo", func(c *Client, ctx context.Context) error {
			return c.AddUserGroup(ctx, "jsilva", "zz_nao_existe")
		}, func(s *userLinkStub) []string { return s.postsGroups }},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			stub := &userLinkStub{}
			c := userLinkClient(t, stub.server(t).URL)
			err := caso.fn(c, context.Background())
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("esperava ErrNotFound, veio %v", err)
			}
			if n := len(caso.post(stub)); n != 0 {
				t.Errorf("o POST não podia ter sido enviado: %d", n)
			}
		})
	}
}

// Usuário inexistente para antes de qualquer escrita, nas duas famílias.
func TestUserLinkUsuarioInexistente(t *testing.T) {
	stub := &userLinkStub{}
	c := userLinkClient(t, stub.server(t).URL)
	ctx := context.Background()
	for _, err := range []error{
		c.AddUserRole(ctx, "zz_ninguem", "faturista"),
		c.AddUserGroup(ctx, "zz_ninguem", "TI"),
		c.RemoveUserRole(ctx, "zz_ninguem", "faturista"),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("esperava ErrNotFound, veio %v", err)
		}
	}
	if len(stub.postsRoles)+len(stub.postsGroups)+len(stub.deletes) != 0 {
		t.Error("nenhuma escrita podia ter saído")
	}
}

// DELETE sem vínculo: 404 vira mensagem que diz o que falta, não "não
// encontrado" solto.
func TestUserLinkRemoveSemVinculo(t *testing.T) {
	stub := &userLinkStub{semVinculo: true}
	c := userLinkClient(t, stub.server(t).URL)

	err := c.RemoveUserRole(context.Background(), "jsilva", "faturista")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
	for _, quer := range []string{"jsilva", "faturista", "não tem o papel"} {
		if !strings.Contains(err.Error(), quer) {
			t.Errorf("mensagem sem %q: %v", quer, err)
		}
	}
}

// Grupo duplicado: o servidor recusa com 400. A mensagem precisa ser a mesma
// do `group add-user` — é o mesmo vínculo pela outra porta.
func TestUserLinkGrupoDuplicado(t *testing.T) {
	stub := &userLinkStub{duplicado: true}
	c := userLinkClient(t, stub.server(t).URL)

	err := c.AddUserGroup(context.Background(), "jsilva", "TI")
	if err == nil {
		t.Fatal("esperava recusa do servidor")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("duplicidade não é NOT_FOUND: %v", err)
	}
	if !strings.Contains(err.Error(), "already associated") {
		t.Errorf("a mensagem do servidor tem de aparecer: %v", err)
	}
}
