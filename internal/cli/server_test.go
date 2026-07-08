package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// healthyServerStub simula um servidor saudável; helperInstalled controla o
// ping da fluiggersWidget.
func healthyServerStub(t *testing.T, helperInstalled bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":{"login":"u","fullName":"Fulano de Teste","email":"u@x","userCode":"uc"}}`)
	})
	mux.HandleFunc("/fluiggersWidget/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !helperInstalled {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func serverTestProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	s := config.Server{ID: "st-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(s, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestServerTestReportsHelperStatus(t *testing.T) {
	for _, installed := range []bool{true, false} {
		stub := healthyServerStub(t, installed)
		proj := serverTestProject(t, stub.URL)
		code, stdout := runMain(t, "server", "test", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("installed=%v exit=%d stdout=%s", installed, code, stdout)
		}
		var env output.Envelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("json inválido: %v", err)
		}
		data, _ := env.Data.(map[string]any)
		if got, _ := data["helperInstalled"].(bool); got != installed {
			t.Errorf("helperInstalled=%v, quer %v", got, installed)
		}
	}
}
