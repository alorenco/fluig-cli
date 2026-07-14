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
	listQuery url.Values
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
		s.listQuery = r.URL.Query()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_admin_users.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})
	mux.HandleFunc("/admin/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		login := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/users/")
		if login != "user1" {
			// Formato real (2026-07-14).
			http.Error(w, `{"code":"FDNEntityNotFoundException","message":""}`, http.StatusNotFound)
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
	for _, want := range []string{"Login", "Nome", "E-mail", "Estado", "user1", "Ana Andrade", "ACTIVE", "INACTIVE"} {
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
