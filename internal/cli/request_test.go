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

// requestStub simula a REST v2 de solicitações com as fixtures reais
// sanitizadas da homologação.
type requestStub struct {
	listQuery url.Values
}

func (s *requestStub) server(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/process-management/api/v2/requests", func(w http.ResponseWriter, r *http.Request) {
		s.listQuery = r.URL.Query()
		w.Write(readTD("rest_requests_expand.json"))
	})
	mux.HandleFunc("/process-management/api/v2/requests/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/process-management/api/v2/requests/196526":
			w.Write(readTD("rest_request_show.json"))
		case "/process-management/api/v2/requests/196526/tasks":
			w.Write(readTD("rest_request_tasks.json"))
		default:
			// Formato real do 404 da homologação (2026-07-13).
			http.Error(w, `{"code":"BPMWorkflowProcessNotFoundException","message":"Solicitação não encontrada."}`, http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func requestProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "req-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// Modo humano: tabela com as solicitações da fixture (etapa expandida).
func TestRequestListTabela(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "Nº", "Processo", "Etapa atual", "Status", "SLA", "Solicitante", "Início",
		"196526", "contratos_taxa_limpeza", "Aguardar Assinatura", "OPEN", "FINALIZED", "ON_TIME", "Ana Andrade (user1)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// --json: envelope com as solicitações; filtros vão como query (expand sempre).
func TestRequestListJSON(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "list", "--process", "contratos_taxa_limpeza",
		"--status", "open", "--sla", "on_time", "--assignee", "user1", "--requester", "user2",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	requests, _ := data["requests"].([]any)
	if len(requests) != 3 {
		t.Fatalf("esperava 3 solicitações, veio %d", len(requests))
	}
	first, _ := requests[0].(map[string]any)
	if first["id"].(float64) != 196526 || first["status"] != "OPEN" || first["processId"] != "contratos_taxa_limpeza" {
		t.Errorf("request[0] inesperada: %+v", first)
	}
	steps, _ := first["currentSteps"].([]any)
	if len(steps) != 1 {
		t.Fatalf("esperava 1 etapa corrente, veio %d", len(steps))
	}
	step, _ := steps[0].(map[string]any)
	if step["stateName"] != "Aguardar Assinatura" || step["sequence"].(float64) != 14 {
		t.Errorf("etapa inesperada: %+v", step)
	}

	q := stub.listQuery
	if q.Get("processId") != "contratos_taxa_limpeza" || q.Get("status") != "OPEN" ||
		q.Get("slaStatus") != "ON_TIME" || q.Get("assignee") != "user1" || q.Get("requester") != "user2" {
		t.Errorf("filtros não repassados: %v", q)
	}
	if got := q["expand"]; len(got) != 2 || got[0] != "requester" || got[1] != "currentMovements" {
		t.Errorf("expand inesperado: %v", got)
	}
}

// Filtro com valor fora do enum: erro de uso (exit 2), sem chamar o servidor.
func TestRequestListFiltroInvalido(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "request", "list", "--status", "aberta", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
	if stub.listQuery != nil {
		t.Error("o servidor não deveria ter sido consultado")
	}
}

// show: detalhe da solicitação + tabela de movimentação.
func TestRequestShow(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "show", "196526", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Solicitação 196526", "contratos_taxa_limpeza v18", "Status: OPEN",
		"Etapa atual: Aguardar Assinatura (seq 14", "Mov", "Responsável", "COMPLETED", "NOT_COMPLETED"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "request", "show", "196526", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	reqData, _ := data["request"].(map[string]any)
	tasks, _ := data["tasks"].([]any)
	if reqData["id"].(float64) != 196526 || len(tasks) != 4 {
		t.Errorf("envelope inesperado: request=%v tasks=%d", reqData["id"], len(tasks))
	}
}

// show de solicitação inexistente: 404 real → exit 4; argumento não numérico → exit 2.
func TestRequestShowErros(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "request", "show", "999999", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
	code, _ = runMain(t, "request", "show", "abc", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("arg inválido: exit=%d, quer %d", code, output.ExitUsage)
	}
}
