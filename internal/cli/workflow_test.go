package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// workflowStub simula version (SOAP nativo), ping do helper, o update de
// eventos e a listagem de processos (REST v2).
type workflowStub struct {
	helperInstalled bool
	version         int
}

func (s *workflowStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	// getWorkFlowProcessVersion (SOAP nativo) → versão.
	mux.HandleFunc("/webdesk/ECMWorkflowEngineService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
			`<ns2:getWorkFlowProcessVersionResponse xmlns:ns2="http://ws.workflow.ecm.technology.totvs.com/">`+
			`<result>`+strconv.Itoa(s.version)+`</result></ns2:getWorkFlowProcessVersionResponse></soap:Body></soap:Envelope>`)
	})
	mux.HandleFunc("/fluiggersWidget/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !s.helperInstalled {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluiggersWidget/api/workflows/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"processId":"Compras","version":5,"hasError":false,"totalProcessed":1,"errors":[],"successes":["beforeTaskSave"]}`)
	})
	// Listagem de processos (REST v2) — uma página com variedade real
	// (ativo/inativo, sem categoria), no envelope {items, hasNext}.
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[`+
			`{"processId":"Compras","processDescription":"Compras","active":true,"categoryId":"Suprimentos","public":false},`+
			`{"processId":"AprovacaoContrato","processDescription":"Aprovação de Contrato","active":false,"categoryId":"Jurídico","public":false},`+
			`{"processId":"FLUIGADHOCPROCESS","processDescription":"Processo Ad-hoc","active":true,"public":true}`+
			`],"hasNext":false}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func workflowProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "wf-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// Modo humano: tabela com bordas, cabeçalho e os processos do stub.
func TestWorkflowListTabela(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "ID", "Descrição", "Categoria", "Ativo", "Compras", "AprovacaoContrato", "sim", "não"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// --json: envelope com a lista completa; --active-only filtra os inativos.
func TestWorkflowListJSON(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	procs, _ := data["processes"].([]any)
	if len(procs) != 3 {
		t.Fatalf("esperava 3 processos no JSON, veio %d", len(procs))
	}
	first, _ := procs[0].(map[string]any)
	if first["id"] != "Compras" || first["category"] != "Suprimentos" || first["active"] != true {
		t.Errorf("processo[0] inesperado: %+v", first)
	}

	code, stdout = runMain(t, "workflow", "list", "--active-only", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--active-only exit=%d", code)
	}
	json.Unmarshal([]byte(stdout), &env)
	data, _ = env.Data.(map[string]any)
	procs, _ = data["processes"].([]any)
	if len(procs) != 2 {
		t.Errorf("--active-only esperava 2 processos, veio %d", len(procs))
	}
}

func TestWorkflowVersionNative(t *testing.T) {
	stub := &workflowStub{version: 57}
	proj := workflowProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "workflow", "version", "meu_processo",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["version"].(float64) != 57 {
		t.Errorf("versão = %v, quer 57", data["version"])
	}
}

func TestWorkflowVersionNotFound(t *testing.T) {
	stub := &workflowStub{version: 0}
	proj := workflowProject(t, stub.server(t).URL)
	code, _ := runMain(t, "workflow", "version", "inexistente",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("versão 0 deveria dar exit 4, veio %d", code)
	}
}

// Sem a widget, o export cirúrgico deve falhar com exit 7 (HELPER_NOT_INSTALLED).
func TestWorkflowExportRequiresHelper(t *testing.T) {
	stub := &workflowStub{version: 5, helperInstalled: false}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "Compras.beforeTaskSave.js")
	os.WriteFile(file, []byte("function beforeTaskSave(){}"), 0o644)

	code, stdout := runMain(t, "workflow", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitMissingHelper {
		t.Fatalf("sem helper deveria dar exit 7, veio %d\n%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || env.Error.Code != output.CodeMissingHelper {
		t.Errorf("erro inesperado: %+v", env.Error)
	}
}

// Com a widget instalada, o export cirúrgico de um arquivo específico funciona.
func TestWorkflowExportSingleFile(t *testing.T) {
	stub := &workflowStub{version: 5, helperInstalled: true}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "Compras.beforeTaskSave.js")
	os.WriteFile(file, []byte("function beforeTaskSave(){}"), 0o644)

	code, stdout := runMain(t, "workflow", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["updated"].(float64) != 1 || data["processId"] != "Compras" {
		t.Errorf("resultado inesperado: %+v", data)
	}
}
