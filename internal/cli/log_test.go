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

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// logServerStub simula o Fluig com o fluigcliHelper 0.3.0 (rotas de log).
func logServerStub(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.3.0"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("helper_logs.json"))
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/tail", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("grep") == "Dataset query:" {
			w.Write(readTD("helper_log_tail_multiline.json"))
			return
		}
		w.Write(readTD("helper_log_tail.json"))
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/download", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "linha 1\nlinha 2\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLogFilesTabela(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "log", "files", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Arquivo", "Tamanho", "Modificado", "server.log", "server.log.2026-07-17", "2.0 MB", "7.0 MB"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "log", "files", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	files, _ := data["files"].([]any)
	if len(files) != 3 {
		t.Errorf("files=%d, quer 3", len(files))
	}
	first, _ := files[0].(map[string]any)
	if first["name"] != "server.log" || first["lastModified"] == nil {
		t.Errorf("primeiro arquivo inesperado: %v", first)
	}
}

func TestLogTail(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)

	// Modo humano: as entradas saem na íntegra (fixture real da homolog).
	code, stdout := runMain(t, "log", "tail", "-n", "3", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Registered web context", "Redeployed", "Replaced deployment"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "log", "tail", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	entries, _ := data["entries"].([]any)
	if data["file"] != "server.log" || len(entries) != 3 || data["truncated"] != false {
		t.Errorf("envelope inesperado: file=%v entries=%d truncated=%v", data["file"], len(entries), data["truncated"])
	}
}

// Entrada multi-linha (fixture real) sai com as continuações em linhas
// próprias no modo humano.
func TestLogTailMultilinha(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "tail", "--grep", "Dataset query:", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "Dataset query:\n((UT.USER_CODE") {
		t.Errorf("continuação não saiu em linha própria:\n%s", stdout)
	}
}

// --follow é contínuo: com --json é recusado (contrato do envelope único).
func TestLogTailFollowRecusaJSON(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _ := runMain(t, "log", "tail", "--follow", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d (usage)", code, output.ExitUsage)
	}
}

func TestLogTailLevelInvalido(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _ := runMain(t, "log", "tail", "--level", "gigante", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d (usage)", code, output.ExitUsage)
	}
}

func TestLogDownload(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)
	dest := filepath.Join(t.TempDir(), "logs", "server.log")

	code, stdout := runMain(t, "log", "download", "-o", dest, "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "linha 1\nlinha 2\n" {
		t.Errorf("conteúdo baixado: %q", b)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["file"] != "server.log" || data["path"] != dest || data["size"].(float64) != 16 {
		t.Errorf("envelope inesperado: %v", data)
	}
}

// Sem o helper (ping 404), os comandos de log orientam a instalação (exit 7).
func TestLogFilesSemHelper(t *testing.T) {
	stub := healthyServerStub(t, false)
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "files", "--json", "--project", proj)
	if code != output.ExitMissingHelper {
		t.Fatalf("exit=%d stdout=%s, quer %d", code, stdout, output.ExitMissingHelper)
	}
}

// garante que o helper de projeto compartilhado segue disponível aqui
var _ = config.Server{}
