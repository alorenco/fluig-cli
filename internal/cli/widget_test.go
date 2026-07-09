package cli

import (
	"archive/zip"
	"bytes"
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

var widgetBinary = []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0x42}

type widgetStub struct {
	mu           sync.Mutex
	uploadedWAR  []byte
	uploadedName string
}

func (s *widgetStub) widgetZip(t *testing.T) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, content []byte) {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		w.Write(content)
	}
	add("resources/js/app.js", []byte("console.log(1)"))
	add("resources/img/logo.png", widgetBinary)
	add("WEB-INF/classes/application.info", []byte("info"))
	add("WEB-INF/application.xml", []byte("<application/>"))
	add("pom.xml", []byte("<project/>"))
	zw.Close()
	return buf.Bytes()
}

func (s *widgetStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluiggersWidget/api/widgets", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"code":"meu_widget","title":"Meu Widget","description":"d","filename":"meu_widget.war"}]`)
	})
	mux.HandleFunc("/fluiggersWidget/api/widgets/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(s.widgetZip(t))
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/product/uploadfile", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(20 << 20)
		s.mu.Lock()
		s.uploadedName = r.FormValue("fileName")
		if f, _, err := r.FormFile("attachment"); err == nil {
			s.uploadedWAR, _ = io.ReadAll(f)
		}
		s.mu.Unlock()
		io.WriteString(w, `{}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func widgetProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	srv := config.Server{ID: "wg-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(srv, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// Import desempacota o WAR no layout local, preservando binário.
func TestWidgetImportUnpacks(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)
	code, _ := runMain(t, "widget", "import", "meu_widget", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	base := filepath.Join(proj, "wcm", "widget", "meu_widget")
	checks := map[string][]byte{
		filepath.Join(base, "src", "main", "webapp", "resources", "js", "app.js"):    []byte("console.log(1)"),
		filepath.Join(base, "src", "main", "webapp", "resources", "img", "logo.png"): widgetBinary,
		filepath.Join(base, "src", "main", "resources", "application.info"):          []byte("info"),
		filepath.Join(base, "src", "main", "webapp", "WEB-INF", "application.xml"):   []byte("<application/>"),
		filepath.Join(base, "pom.xml"):                                               []byte("<project/>"),
	}
	for path, want := range checks {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("arquivo não gravado: %s (%v)", path, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("conteúdo divergente em %s", path)
		}
	}
}

// Export empacota o WAR do layout local e faz upload; binário intacto e STORE.
func TestWidgetExportPacks(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)
	base := filepath.Join(proj, "wcm", "widget", "meu_widget")
	write := func(rel string, content []byte) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, content, 0o644)
	}
	write("src/main/webapp/WEB-INF/application.xml", []byte("<application/>"))
	write("src/main/resources/application.info", []byte("info"))
	write("src/main/webapp/resources/img/logo.png", widgetBinary)

	code, _ := runMain(t, "widget", "export", "meu_widget", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.uploadedName != "meu_widget.war" {
		t.Errorf("nome do WAR = %q", stub.uploadedName)
	}
	zr, err := zip.NewReader(bytes.NewReader(stub.uploadedWAR), int64(len(stub.uploadedWAR)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, f := range zr.File {
		if f.Method != zip.Store {
			t.Errorf("%s não está em STORE", f.Name)
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		entries[f.Name] = b
	}
	if !bytes.Equal(entries["resources/img/logo.png"], widgetBinary) {
		t.Errorf("binário corrompido no WAR exportado")
	}
	if _, ok := entries["WEB-INF/classes/application.info"]; !ok {
		t.Errorf("resources → WEB-INF/classes não mapeado; entradas: %v", keysOf(entries))
	}
	if _, ok := entries["WEB-INF/application.xml"]; !ok {
		t.Errorf("WEB-INF/application.xml ausente")
	}
}

func keysOf(m map[string][]byte) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

func TestWidgetListJSON(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "widget", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if ws, _ := data["widgets"].([]any); len(ws) != 1 {
		t.Errorf("esperava 1 widget, veio %d", len(ws))
	}
}

// Modo humano: tabela com bordas (padrão de listas — ver CLAUDE.md).
func TestWidgetListTabela(t *testing.T) {
	stub := &widgetStub{}
	proj := widgetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "widget", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "Código", "Título", "meu_widget", "Meu Widget"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}
