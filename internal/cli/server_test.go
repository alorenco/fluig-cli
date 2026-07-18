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

	helperwar "github.com/alorenco/fluig-cli/helper"
	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
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
		wantHelper := ""
		if installed {
			wantHelper = fluig.HelperFluiggers
		}
		if got, _ := data["helper"].(string); got != wantHelper {
			t.Errorf("helper=%q, quer %q", got, wantHelper)
		}
	}
}

// install-helper publica o WAR embutido do fluigcliHelper; se ele já responde
// ao ping, não reenvia (action=none).
func TestServerInstallHelperEmbutido(t *testing.T) {
	for _, jaInstalado := range []bool{false, true} {
		var uploadedName string
		var uploadedSize int
		mux := http.NewServeMux()
		mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
		})
		mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"message":"pong"}`)
		})
		mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
			if !jaInstalado {
				http.NotFound(w, r)
				return
			}
			io.WriteString(w, "pong")
		})
		mux.HandleFunc("/portal/api/rest/wcmservice/rest/product/uploadfile", func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseMultipartForm(20 << 20)
			uploadedName = r.FormValue("fileName")
			if f, _, err := r.FormFile("attachment"); err == nil {
				b, _ := io.ReadAll(f)
				uploadedSize = len(b)
			}
			io.WriteString(w, `{}`)
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		proj := serverTestProject(t, srv.URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("jaInstalado=%v exit=%d stdout=%s", jaInstalado, code, stdout)
		}
		var env output.Envelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("json inválido: %v", err)
		}
		data, _ := env.Data.(map[string]any)
		if jaInstalado {
			if data["action"] != "none" || uploadedName != "" {
				t.Errorf("já instalado: action=%v upload=%q (quer none, sem upload)", data["action"], uploadedName)
			}
			continue
		}
		if data["action"] != "uploaded" || data["helper"] != fluig.HelperFluigcli {
			t.Errorf("action=%v helper=%v, quer uploaded/fluigcliHelper", data["action"], data["helper"])
		}
		if uploadedName != helperwar.Name {
			t.Errorf("nome do WAR enviado = %q, quer %q", uploadedName, helperwar.Name)
		}
		if uploadedSize != len(helperwar.WAR) || uploadedSize == 0 {
			t.Errorf("tamanho enviado %d ≠ WAR embutido %d", uploadedSize, len(helperwar.WAR))
		}
	}
}

// server status: resumo + tabela de monitores (fixtures reais da homolog).
func TestServerStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux.HandleFunc("/environment/api/v2/monitors", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("rest_monitors.json"))
	})
	mux.HandleFunc("/environment/api/v2/statistics", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("rest_statistics.json"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u := mustParseHostPort(t, srv.URL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "st-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}

	code, stdout := runMain(t, "server", "status", "homolog", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Uptime:", "Usuários conectados: 35", "Threads: 385 (pico 454)",
		"Microsoft SQL Server", "Monitor", "LICENSE_SERVER_AVAILABILITY", "OK", "FAILURE", "100%"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "server", "status", "homolog", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	stats, _ := data["stats"].(map[string]any)
	monitors, _ := data["monitors"].([]any)
	if stats["connectedUsers"].(float64) != 35 || len(monitors) != 8 {
		t.Errorf("envelope inesperado: stats=%v monitors=%d", stats["connectedUsers"], len(monitors))
	}
}
