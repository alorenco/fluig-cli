package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

const watchImplServidor = "function createDataset(fields, constraints, sortFields) {\n  return null;\n}\n"

// watchStub simula o servidor para o watch: dataset ds_exemplo existente, com
// sinal em edits a cada editDataset.
func watchStub(t *testing.T) (*httptest.Server, chan struct{}) {
	t.Helper()
	edits := make(chan struct{}, 8)
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/loadDataset", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") != "ds_exemplo" {
			http.Error(w, "não existe", 500)
			return
		}
		_, _ = io.WriteString(w, `{"datasetPK":{"companyId":1,"datasetId":"ds_exemplo"},`+
			`"datasetDescription":"x","datasetImpl":`+jsonString(watchImplServidor)+`,"type":"CUSTOM"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/editDataset", func(w http.ResponseWriter, r *http.Request) {
		edits <- struct{}{}
		_, _ = io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, edits
}

func jsonString(s string) string {
	b := strings.NewReplacer("\n", `\n`, `"`, `\"`).Replace(s)
	return `"` + b + `"`
}

func TestWatchGuards(t *testing.T) {
	proj := projWithServers(t, doisServidores()...) // hml + producao (prod)

	// --json é recusado antes de qualquer coisa.
	code, _ := runMain(t, "watch", "--json", "--project", proj, "--server", "hml")
	if code != output.ExitUsage {
		t.Errorf("--json: exit = %d, quer %d", code, output.ExitUsage)
	}

	// Produção é recusada sem exceção.
	app := &App{}
	root := newRootCmd(app)
	root.SetArgs([]string{"watch", "--project", proj, "--server", "producao", "--yes"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "PRODUÇÃO") {
		t.Errorf("produção deveria ser recusada mesmo com --yes: %v", err)
	}

	// Servidor sem env marcado também é recusado, com dica do server update.
	semEnv := projWithServers(t, config.Server{ID: "9", Name: "solto", Host: "h", Port: 80, Username: "u", CompanyID: 1})
	app = &App{}
	root = newRootCmd(app)
	root.SetArgs([]string{"watch", "--project", semEnv, "--server", "solto"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "server update") {
		t.Errorf("sem env deveria orientar o server update: %v", err)
	}
}

func TestUpdateExisting(t *testing.T) {
	srv, edits := watchStub(t)
	client, err := fluig.NewClient(fluig.Options{BaseURL: srv.URL, Username: "u-updexist", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{}
	ctx := context.Background()

	// Conteúdo idêntico (modulo EOL) → unchanged, sem gravação.
	action, err := app.updateExisting(ctx, client, diffTarget{"dataset", "ds_exemplo", ""},
		strings.ReplaceAll(watchImplServidor, "\n", "\r\n"))
	if err != nil || action != "unchanged" {
		t.Errorf("conteúdo igual = (%q, %v), quer unchanged", action, err)
	}
	select {
	case <-edits:
		t.Error("conteúdo igual não deveria gravar no servidor")
	default:
	}

	// Conteúdo diferente → updated, com gravação.
	action, err = app.updateExisting(ctx, client, diffTarget{"dataset", "ds_exemplo", ""}, "outro código")
	if err != nil || action != "updated" {
		t.Errorf("conteúdo novo = (%q, %v), quer updated", action, err)
	}
	select {
	case <-edits:
	case <-time.After(time.Second):
		t.Error("conteúdo novo deveria ter gravado no servidor")
	}

	// Artefato inexistente → missing (o watch não cria).
	action, err = app.updateExisting(ctx, client, diffTarget{"dataset", "ds_novo", ""}, "x")
	if err != nil || action != "missing" {
		t.Errorf("inexistente = (%q, %v), quer missing", action, err)
	}
}

func TestWatchPublicaAoSalvar(t *testing.T) {
	srv, edits := watchStub(t)
	client, err := fluig.NewClient(fluig.Options{BaseURL: srv.URL, Username: "u-watch", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	file := filepath.Join(root, "datasets", "ds_exemplo.js")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(watchImplServidor), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	app.printer = output.NewPrinter(false, "watch")
	app.printer.Stdout, app.printer.Stderr = io.Discard, io.Discard

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.runWatch(ctx, client, root, "homolog", 20*time.Millisecond) }()

	// Salva conteúdo modificado até o watch publicar (tolerante ao tempo de
	// inicialização do observador).
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	published := false
	for !published {
		select {
		case <-edits:
			published = true
		case <-tick.C:
			if err := os.WriteFile(file, []byte("// conteúdo novo salvo pelo editor\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		case <-deadline:
			t.Fatal("o watch não publicou o salvamento em 5s")
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("runWatch deveria encerrar limpo no cancelamento: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runWatch não encerrou após o cancelamento")
	}
}
