package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// adminUserStub simula o módulo /admin/api/v1/users com a fixture real
// sanitizada da homologação.
type adminUserStub struct {
	listQuery  url.Values
	createBody string
	updateBody string
	posted     []string // POSTs em activate/deactivate
}

func (s *adminUserStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/admin/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			s.createBody = string(b)
			if strings.Contains(s.createBody, `"login":"duplicado"`) {
				http.Error(w, `{"code":"FDNDuplicatedLoginException","message":""}`, http.StatusBadRequest)
				return
			}
			// create devolve lastUpdateDate como STRING.
			io.WriteString(w, `{"id":354,"login":"novo","code":"novo","email":"novo@exemplo.com",`+
				`"firstName":"Teste","lastName":"CLI","fullName":"Teste CLI","state":"ACTIVE",`+
				`"lastUpdateDate":"2026-07-14T13:06:58.054-0400"}`)
			return
		}
		s.listQuery = r.URL.Query()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_admin_users.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})
	mux.HandleFunc("/admin/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/users/")
		// activate/deactivate: login inexistente responde 400, não 404 (real).
		if strings.HasSuffix(rest, "/activate") || strings.HasSuffix(rest, "/deactivate") {
			login := rest[:strings.LastIndex(rest, "/")]
			if login != "user1" {
				http.Error(w, `{"code":"FDNInvalidUserCodeException","message":""}`, http.StatusBadRequest)
				return
			}
			s.posted = append(s.posted, rest)
			w.WriteHeader(http.StatusOK)
			return
		}
		login := rest
		if login != "user1" {
			// Formato real (2026-07-14).
			http.Error(w, `{"code":"FDNEntityNotFoundException","message":""}`, http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut {
			b, _ := io.ReadAll(r.Body)
			s.updateBody = string(b)
			// PUT devolve lastUpdateDate como NÚMERO (epoch millis) — quirk real.
			io.WriteString(w, `{"id":5,"login":"user1","code":"user1","email":"editado@exemplo.com",`+
				`"firstName":"Ana","lastName":"Andrade","fullName":"Ana Andrade","state":"ACTIVE",`+
				`"lastUpdateDate":1784048837693}`)
			return
		}
		io.WriteString(w, `{"id":5,"login":"user1","code":"user1","email":"user1@exemplo.com",`+
			`"firstName":"Ana","lastName":"Andrade","fullName":"Ana Andrade","state":"ACTIVE",`+
			`"lastUpdateDate":"2026-02-18T12:54:46.074-0400",`+
			`"roles":["admin","user"],"groups":["DefaultGroup-1","TI"]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func adminUserProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "adm-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// list: tabela com estado (ativos em verde) e filtros repassados.
func TestUserList(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "list", "--search", "Ana", "--role", "admin", "--inactive",
		"--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Login", "Nome", "E-mail", "Estado", "user1", "Ana Andrade", "ACTIVE", "BLOCKED"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	q := stub.listQuery
	if q.Get("pattern") != "Ana" || q.Get("role") != "admin" || q.Get("showInactive") != "true" {
		t.Errorf("filtros não repassados: %v", q)
	}

	code, stdout = runMain(t, "user", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	users, _ := data["users"].([]any)
	first, _ := users[0].(map[string]any)
	if len(users) != 2 || first["login"] != "user1" || first["state"] != "ACTIVE" {
		t.Errorf("users inesperado: %+v", users)
	}
	if stub.listQuery.Get("showInactive") != "" {
		t.Error("sem --inactive não deveria mandar showInactive")
	}
}

// show: detalhe com papéis/grupos; inexistente → exit 4.
func TestUserShow(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "show", "user1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Ana Andrade (user1) — ACTIVE", "E-mail: user1@exemplo.com",
		"Papéis (2): admin, user", "Grupos (2): DefaultGroup-1, TI"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, _ = runMain(t, "user", "show", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// create: corpo com os campos (code = login quando omitido); senha da env.
func TestUserCreate(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	t.Setenv("FLUIGCLI_NEW_USER_PASSWORD", "Seg@123456")
	code, stdout := runMain(t, "user", "create", "novo",
		"--email", "novo@exemplo.com", "--first-name", "Teste", "--last-name", "CLI",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{`"login":"novo"`, `"code":"novo"`, `"password":"Seg@123456"`,
		`"firstName":"Teste"`, `"lastName":"CLI"`} {
		if !strings.Contains(stub.createBody, want) {
			t.Errorf("corpo do create sem %q: %s", want, stub.createBody)
		}
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	u, _ := data["user"].(map[string]any)
	if u["state"] != "ACTIVE" {
		t.Errorf("user inesperado: %+v", u)
	}

	// login duplicado → exit 5; sem senha e sem env, não-interativo → exit 2.
	code, _ = runMain(t, "user", "create", "duplicado",
		"--email", "d@e.com", "--first-name", "D", "--last-name", "E",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Errorf("duplicado: exit=%d, quer %d", code, output.ExitServer)
	}
	t.Setenv("FLUIGCLI_NEW_USER_PASSWORD", "")
	code, _ = runMain(t, "user", "create", "semsenha",
		"--email", "s@e.com", "--first-name", "S", "--last-name", "E",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem senha: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// update: só os campos alterados vão no corpo; a resposta do PUT tem
// lastUpdateDate NUMÉRICO (não pode quebrar o parse).
func TestUserUpdate(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "user", "update", "user1",
		"--email", "editado@exemplo.com", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.updateBody != `{"email":"editado@exemplo.com"}` {
		t.Errorf("corpo do update deveria conter só o email: %s", stub.updateBody)
	}

	// Sem nenhum campo: erro de uso, sem tocar o servidor.
	stub2 := &adminUserStub{}
	proj2 := adminUserProject(t, stub2.server(t).URL)
	code, _ = runMain(t, "user", "update", "user1", "--json", "--project", proj2, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem campos: exit=%d, quer %d", code, output.ExitUsage)
	}
	if stub2.updateBody != "" {
		t.Error("update sem campos não deveria chamar o servidor")
	}
}

// activate/deactivate: POST no endpoint certo; inexistente (400 real) → exit 4.
func TestUserActivateDeactivate(t *testing.T) {
	stub := &adminUserStub{}
	proj := adminUserProject(t, stub.server(t).URL)
	code, _ := runMain(t, "user", "deactivate", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("deactivate exit=%d", code)
	}
	code, _ = runMain(t, "user", "activate", "user1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("activate exit=%d", code)
	}
	want := []string{"user1/deactivate", "user1/activate"}
	if strings.Join(stub.posted, ",") != strings.Join(want, ",") {
		t.Errorf("POSTs = %v, quer %v", stub.posted, want)
	}

	code, _ = runMain(t, "user", "deactivate", "sumiu", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}
