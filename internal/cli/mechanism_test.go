package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

type mechStub struct {
	mu      sync.Mutex
	created map[string]any
	updated map[string]any
}

func (s *mechStub) server(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/getCustomAttributionMechanismList", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("getMechanismList.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/createAttributionMechanism", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.created)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/updateAttributionMechanism", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.updated)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func mechProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "mec-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestMechanismImportWritesFile(t *testing.T) {
	stub := &mechStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, _ := runMain(t, "mechanism", "import", "mec_gestor_area", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, "mechanisms", "mec_gestor_area.js")); err != nil {
		t.Fatalf("arquivo não criado: %v", err)
	}
}

// Ao criar um mecanismo novo, usa os valores fixos (assignmentType=1, controlClass).
func TestMechanismExportCreateUsesFixedFields(t *testing.T) {
	stub := &mechStub{}
	proj := mechProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "mechanisms")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "mec_novo.js")
	os.WriteFile(file, []byte("function getUsers(){ return []; }"), 0o644)

	code, _ := runMain(t, "mechanism", "export", file, "--name", "Mec Novo", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.created == nil {
		t.Fatal("createAttributionMechanism não foi chamado")
	}
	if stub.created["assignmentType"].(float64) != 1 {
		t.Errorf("assignmentType = %v, quer 1", stub.created["assignmentType"])
	}
	if s, _ := stub.created["controlClass"].(string); s == "" {
		t.Errorf("controlClass não definido: %v", stub.created["controlClass"])
	}
	if stub.created["attributionMecanismDescription"] != "function getUsers(){ return []; }" {
		t.Errorf("código não foi para attributionMecanismDescription: %v", stub.created["attributionMecanismDescription"])
	}
}

func TestMechanismExportUpdatesExisting(t *testing.T) {
	stub := &mechStub{}
	proj := mechProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "mechanisms")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "mec_gestor_area.js")
	os.WriteFile(file, []byte("function getUsers(){ return ['novo']; }"), 0o644)

	code, _ := runMain(t, "mechanism", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.updated == nil {
		t.Fatal("updateAttributionMechanism não foi chamado")
	}
	if stub.created != nil {
		t.Error("não deveria ter criado um mecanismo que já existe")
	}
}
