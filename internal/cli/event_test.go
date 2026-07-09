package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// eventStub simula os endpoints de evento global para os testes da CLI.
type eventStub struct {
	mu      sync.Mutex
	saved   []map[string]any // último saveEventList (lista completa)
	deleted []string
}

func (s *eventStub) server(t *testing.T) *httptest.Server {
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/getEventList", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("getEventList.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/saveEventList", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(body, &s.saved)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/deleteGlobalEvent", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.deleted = append(s.deleted, r.URL.Query().Get("eventName"))
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func eventProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "ev-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestEventImportWritesFile(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)
	code, _ := runMain(t, "event", "import", "displayCustomThemes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	data, err := os.ReadFile(filepath.Join(proj, "events", "displayCustomThemes.js"))
	if err != nil {
		t.Fatalf("arquivo não criado: %v", err)
	}
	if len(data) == 0 {
		t.Error("arquivo vazio")
	}
}

// Regra crítica: exportar um evento novo NÃO pode apagar os demais.
func TestEventExportKeepsOthers(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)

	dir := filepath.Join(proj, "events")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "meuEventoNovo.js")
	os.WriteFile(file, []byte("function meuEventoNovo(){}"), 0o644)

	code, _ := runMain(t, "event", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	// A lista salva deve conter os 2 eventos que já existiam + o novo = 3.
	if len(stub.saved) != 3 {
		t.Fatalf("saveEventList deveria conter 3 eventos (2 existentes + 1 novo), veio %d", len(stub.saved))
	}
	ids := map[string]bool{}
	for _, e := range stub.saved {
		pk, _ := e["globalEventPK"].(map[string]any)
		if pk != nil {
			ids[pk["eventId"].(string)] = true
		}
	}
	for _, want := range []string{"beforeConvertViewToPDF", "displayCustomThemes", "meuEventoNovo"} {
		if !ids[want] {
			t.Errorf("evento %q ausente na lista salva — export apagou eventos existentes!", want)
		}
	}
}

func TestEventDeleteRequiresConfirmation(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)

	// Sem --yes em modo não-interativo → exit 2, nada é excluído.
	code, _ := runMain(t, "event", "delete", "displayCustomThemes", "--json", "--non-interactive", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --yes deveria dar exit 2, veio %d", code)
	}
	stub.mu.Lock()
	n := len(stub.deleted)
	stub.mu.Unlock()
	if n != 0 {
		t.Errorf("não deveria ter excluído sem confirmação, deletados=%d", n)
	}

	// Com --yes → executa.
	code, _ = runMain(t, "event", "delete", "displayCustomThemes", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Errorf("com --yes deveria excluir (exit 0), veio %d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.deleted) != 1 || stub.deleted[0] != "displayCustomThemes" {
		t.Errorf("delete inesperado: %v", stub.deleted)
	}
}

func TestEventListJSON(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "event", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	events, _ := data["events"].([]any)
	if len(events) != 2 {
		t.Errorf("esperava 2 eventos no envelope, veio %d", len(events))
	}
}

// Modo humano: tabela com bordas (padrão de listas — ver CLAUDE.md).
func TestEventListTabela(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "event", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "ID", "Linhas", "beforeConvertViewToPDF", "displayCustomThemes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}
