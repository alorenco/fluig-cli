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

// roleStub simula o módulo /admin/api/v1/roles com as fixtures reais
// sanitizadas da homologação (2026-07-14).
type roleStub struct {
	createBody string
	updateBody string
	addBody    string
	deleted    []string
	removed    []string
}

func (s *roleStub) server(t *testing.T) *httptest.Server {
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

	mux.HandleFunc("/admin/api/v1/roles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			s.createBody = string(b)
			if strings.Contains(s.createBody, `"code":"dup"`) {
				http.Error(w, `{"code":"FDNDuplicatedApplicationCodeException","message":""}`, http.StatusBadRequest)
				return
			}
			// create devolve 200 (não 201) na homologação.
			io.WriteString(w, `{"id":78,"tenantId":1,"code":"rolenovo","description":"Papel novo","data":null}`)
			return
		}
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_admin_roles.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})

	mux.HandleFunc("/admin/api/v1/roles/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/roles/")
		parts := strings.Split(rest, "/")
		code := parts[0]
		if code != "role1" {
			notFound(w)
			return
		}
		switch {
		case len(parts) == 1:
			switch r.Method {
			case http.MethodPut:
				b, _ := io.ReadAll(r.Body)
				s.updateBody = string(b)
				io.WriteString(w, `{"id":78,"tenantId":1,"code":"role1","description":"Descrição nova","data":null}`)
			case http.MethodDelete:
				s.deleted = append(s.deleted, code)
				w.WriteHeader(http.StatusNoContent)
			default:
				io.WriteString(w, `{"id":78,"tenantId":1,"code":"role1","description":"Aprovadores","data":null}`)
			}
		case len(parts) == 2 && parts[1] == "users":
			if r.Method == http.MethodPost {
				b, _ := io.ReadAll(r.Body)
				s.addBody = string(b)
				w.Write(b) // 200 com echo
				return
			}
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_group_users.json"))
			if err != nil {
				t.Fatal(err)
			}
			w.Write(b)
		case len(parts) == 3 && parts[1] == "users":
			login := parts[2]
			if login != "user1" {
				notFound(w)
				return
			}
			s.removed = append(s.removed, login)
			w.WriteHeader(http.StatusNoContent)
		default:
			notFound(w)
		}
	})

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

// list: tabela (Código/Descrição), --search client-side e --json.
func TestRoleListTabela(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "role", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Código", "Descrição", "admin", "diretor", "aprovadores"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "role", "list", "--search", "dir", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || !strings.Contains(stdout, "diretor") || strings.Contains(stdout, "aprovadores") {
		t.Errorf("--search: exit=%d\n%s", code, stdout)
	}

	code, stdout = runMain(t, "role", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	roles, _ := data["roles"].([]any)
	if len(roles) != 3 {
		t.Errorf("esperava 3 papéis no json, veio %d", len(roles))
	}
}

// show: detalhe com usuários; inexistente → exit 4.
func TestRoleShow(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "role", "show", "role1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Aprovadores (role1)", "Usuários (1): user1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
	code, _ = runMain(t, "role", "show", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// create: --description no corpo; sem --description usa o code; dup → 5.
func TestRoleCreate(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "role", "create", "rolenovo", "--description", "Papel novo",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stub.createBody, `"code":"rolenovo"`) || !strings.Contains(stub.createBody, `"description":"Papel novo"`) {
		t.Errorf("corpo do create inesperado: %s", stub.createBody)
	}

	// sem --description: usa o próprio código.
	stub2 := &roleStub{}
	proj2 := adminUserProject(t, stub2.server(t).URL)
	code, _ = runMain(t, "role", "create", "rolenovo", "--json", "--project", proj2, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("create sem desc exit=%d", code)
	}
	if !strings.Contains(stub2.createBody, `"description":"rolenovo"`) {
		t.Errorf("sem --description deveria usar o código: %s", stub2.createBody)
	}

	code, _ = runMain(t, "role", "create", "dup", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Errorf("duplicado: exit=%d, quer %d", code, output.ExitServer)
	}
}

// update: só a description no corpo; sem campos → exit 2 sem tocar servidor.
func TestRoleUpdate(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "role", "update", "role1", "--description", "Descrição nova",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.updateBody != `{"description":"Descrição nova"}` {
		t.Errorf("corpo do update inesperado: %s", stub.updateBody)
	}

	stub2 := &roleStub{}
	proj2 := adminUserProject(t, stub2.server(t).URL)
	code, _ = runMain(t, "role", "update", "role1", "--json", "--project", proj2, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem campos: exit=%d, quer %d", code, output.ExitUsage)
	}
	if stub2.updateBody != "" {
		t.Error("update sem campos não deveria chamar o servidor")
	}
}

// delete: existente ok; inexistente → exit 4.
func TestRoleDelete(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, _ := runMain(t, "role", "delete", "role1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("delete exit=%d", code)
	}
	if strings.Join(stub.deleted, ",") != "role1" {
		t.Errorf("DELETE não chamado: %v", stub.deleted)
	}
	code, _ = runMain(t, "role", "delete", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// users: tabela; papel inexistente → exit 4.
func TestRoleUsers(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "role", "users", "role1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Login", "user1", "Ana Andrade"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	code, _ = runMain(t, "role", "users", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("papel inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// add-user / remove-user: pré-validação (papel+login) e endpoints certos.
func TestRoleAddRemoveUser(t *testing.T) {
	stub := &roleStub{}
	proj := adminUserProject(t, stub.server(t).URL)

	code, _ := runMain(t, "role", "add-user", "role1", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("add-user exit=%d", code)
	}
	if stub.addBody != `{"login":"user1"}` {
		t.Errorf("corpo do add-user inesperado: %s", stub.addBody)
	}

	code, _ = runMain(t, "role", "add-user", "sumiu", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("add em papel inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	code, _ = runMain(t, "role", "add-user", "role1", "fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("add de login inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}

	code, _ = runMain(t, "role", "remove-user", "role1", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("remove-user exit=%d", code)
	}
	if strings.Join(stub.removed, ",") != "user1" {
		t.Errorf("DELETE de vínculo não chamado: %v", stub.removed)
	}
	code, _ = runMain(t, "role", "remove-user", "role1", "fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("remove de não-vinculado: exit=%d, quer %d", code, output.ExitNotFound)
	}
}
