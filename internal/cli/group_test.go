package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// groupStub simula o módulo /admin/api/v1/groups com as fixtures reais
// sanitizadas da homologação (2026-07-14).
type groupStub struct {
	createBody string
	updateBody string
	addBody    string
	deleted    []string // DELETEs em /groups/{code}
	removed    []string // DELETEs em /groups/{code}/users/{login}
}

func (s *groupStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	notFound := func(w http.ResponseWriter) {
		http.Error(w, `{"code":"FDNEntityNotFoundException","message":""}`, http.StatusNotFound)
	}

	// Listagem + criação.
	mux.HandleFunc("/admin/api/v1/groups", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			s.createBody = string(b)
			if strings.Contains(s.createBody, `"code":"dup"`) {
				http.Error(w, `{"code":"FDNDuplicatedGroupCodeException","message":""}`, http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"id":42,"tenantId":1,"code":"grpnovo","description":"Grupo novo","type":"user","data":null}`)
			return
		}
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_admin_groups.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})

	// Detalhe / membros / associações.
	mux.HandleFunc("/admin/api/v1/groups/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/groups/")
		parts := strings.Split(rest, "/")
		code := parts[0]
		if code != "grp1" {
			notFound(w)
			return
		}
		switch {
		case len(parts) == 1: // /groups/grp1
			switch r.Method {
			case http.MethodPut:
				b, _ := io.ReadAll(r.Body)
				s.updateBody = string(b)
				io.WriteString(w, `{"id":42,"tenantId":1,"code":"grp1","description":"Descrição nova","type":"user","data":null}`)
			case http.MethodDelete:
				s.deleted = append(s.deleted, code)
				w.WriteHeader(http.StatusNoContent)
			default:
				io.WriteString(w, `{"id":42,"tenantId":1,"code":"grp1","description":"Grupo de TI","type":"user","data":null}`)
			}
		case len(parts) == 2 && parts[1] == "users": // /groups/grp1/users
			if r.Method == http.MethodPost {
				b, _ := io.ReadAll(r.Body)
				s.addBody = string(b)
				w.WriteHeader(http.StatusCreated)
				w.Write(b)
				return
			}
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_group_users.json"))
			if err != nil {
				t.Fatal(err)
			}
			w.Write(b)
		case len(parts) == 3 && parts[1] == "users": // /groups/grp1/users/{login}
			login := parts[2]
			if login != "user1" { // só user1 é membro
				notFound(w)
				return
			}
			s.removed = append(s.removed, login)
			w.WriteHeader(http.StatusNoContent)
		default:
			notFound(w)
		}
	})

	// Usuários (para a pré-validação de add-user).
	mux.HandleFunc("/admin/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		login := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/users/")
		if login != "user1" {
			http.Error(w, `{"code":"FDNEntityNotFoundException","message":""}`, http.StatusNotFound)
			return
		}
		io.WriteString(w, `{"id":3,"login":"user1","code":"user1","email":"user1@exemplo.com",`+
			`"fullName":"Ana Andrade","state":"ACTIVE","lastUpdateDate":"2026-02-18T12:54:46.074-0400"}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// list: tabela (Código/Descrição/Tipo), filtro --type client-side e --json.
func TestGroupListTabela(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "group", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Código", "Descrição", "Tipo", "Compras", "TotvsRM", "MEMBER_departamento-de-ti", "community", "user"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	// --type user (client-side): não deve listar o grupo community.
	code, stdout = runMain(t, "group", "list", "--type", "user", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--type exit=%d", code)
	}
	if strings.Contains(stdout, "MEMBER_departamento-de-ti") {
		t.Errorf("--type user não deveria listar grupo community:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Compras") {
		t.Errorf("--type user deveria listar grupos user:\n%s", stdout)
	}

	// --search substring em descrição/código.
	code, stdout = runMain(t, "group", "list", "--search", "totvs", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || !strings.Contains(stdout, "TotvsRM") || strings.Contains(stdout, "Compras") {
		t.Errorf("--search: exit=%d\n%s", code, stdout)
	}

	// --json
	code, stdout = runMain(t, "group", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	groups, _ := data["groups"].([]any)
	if len(groups) != 3 {
		t.Errorf("esperava 3 grupos no json, veio %d", len(groups))
	}
	// tipo inválido → exit 2.
	code, _ = runMain(t, "group", "list", "--type", "banana", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("--type inválido: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// show: detalhe com membros; inexistente → exit 4.
func TestGroupShow(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "group", "show", "grp1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Grupo de TI (grp1) — tipo user", "Membros (1): user1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
	code, _ = runMain(t, "group", "show", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// create: corpo {code,description,type}; type default user; dup → 5; sem desc → 2.
func TestGroupCreate(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "group", "create", "grpnovo", "--description", "Grupo novo",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{`"code":"grpnovo"`, `"description":"Grupo novo"`, `"type":"user"`} {
		if !strings.Contains(stub.createBody, want) {
			t.Errorf("corpo do create sem %q: %s", want, stub.createBody)
		}
	}

	code, _ = runMain(t, "group", "create", "dup", "--description", "x",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Errorf("duplicado: exit=%d, quer %d", code, output.ExitServer)
	}

	code, _ = runMain(t, "group", "create", "semdesc", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem descrição: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// update: só a description vai no corpo; sem campos → exit 2 sem tocar servidor.
func TestGroupUpdate(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "group", "update", "grp1", "--description", "Descrição nova",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.updateBody != `{"description":"Descrição nova"}` {
		t.Errorf("corpo do update inesperado: %s", stub.updateBody)
	}

	stub2 := &groupStub{}
	proj2 := adminUserProject(t, stub2.server(t).URL)
	code, _ = runMain(t, "group", "update", "grp1", "--json", "--project", proj2, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem campos: exit=%d, quer %d", code, output.ExitUsage)
	}
	if stub2.updateBody != "" {
		t.Error("update sem campos não deveria chamar o servidor")
	}
}

// delete: existente ok; inexistente → exit 4.
func TestGroupDelete(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, _ := runMain(t, "group", "delete", "grp1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("delete exit=%d", code)
	}
	if strings.Join(stub.deleted, ",") != "grp1" {
		t.Errorf("DELETE não chamado: %v", stub.deleted)
	}
	code, _ = runMain(t, "group", "delete", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// users: tabela de membros; grupo inexistente → exit 4.
func TestGroupUsers(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "group", "users", "grp1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Login", "user1", "Ana Andrade", "ACTIVE"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela de membros sem %q:\n%s", want, stdout)
		}
	}
	code, _ = runMain(t, "group", "users", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("grupo inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// add-user / remove-user: pré-validação (grupo+login) e endpoints certos.
func TestGroupAddRemoveUser(t *testing.T) {
	stub := &groupStub{}
	proj := adminUserProject(t, stub.server(t).URL)

	code, _ := runMain(t, "group", "add-user", "grp1", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("add-user exit=%d", code)
	}
	if stub.addBody != `{"login":"user1"}` {
		t.Errorf("corpo do add-user inesperado: %s", stub.addBody)
	}

	// grupo inexistente → exit 4 (pré-validação, sem POST).
	code, _ = runMain(t, "group", "add-user", "sumiu", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("add em grupo inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	// login inexistente → exit 4 (pré-validação do usuário).
	code, _ = runMain(t, "group", "add-user", "grp1", "fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("add de login inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}

	// remove membro real.
	code, _ = runMain(t, "group", "remove-user", "grp1", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("remove-user exit=%d", code)
	}
	if strings.Join(stub.removed, ",") != "user1" {
		t.Errorf("DELETE de membro não chamado: %v", stub.removed)
	}
	// remove não-membro → exit 4.
	code, _ = runMain(t, "group", "remove-user", "grp1", "fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("remove de não-membro: exit=%d, quer %d", code, output.ExitNotFound)
	}
}
