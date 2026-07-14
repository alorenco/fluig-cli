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

// taskStub simula GET /v2/tasks com a fixture real sanitizada da homolog.
type taskStub struct {
	query url.Values
}

func (s *taskStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/process-management/api/v2/tasks", func(w http.ResponseWriter, r *http.Request) {
		s.query = r.URL.Query()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_tasks.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func taskProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "task-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "user1", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// Sem flags: minhas tarefas (assignee = usuário do servidor) em aberto.
func TestTaskListDefaults(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.query.Get("assignee") != "user1" || stub.query.Get("status") != "NOT_COMPLETED" {
		t.Errorf("defaults não aplicados: %v", stub.query)
	}
	for _, want := range []string{"│", "Solicitação", "Processo", "Etapa", "Responsável", "Status",
		"196542", "compras_requisicao_abastecimento", "Aprovar Requisição", "NOT_COMPLETED"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// Flags: --everyone tira o assignee; --status all tira o status; demais filtros passam.
func TestTaskListFiltros(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--everyone", "--status", "all",
		"--process", "compras_requisicao_abastecimento", "--requester", "user2", "--sla", "on_time",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	q := stub.query
	if q.Get("assignee") != "" || q.Get("status") != "" {
		t.Errorf("--everyone/--status all deveriam remover os filtros: %v", q)
	}
	if q.Get("processId") != "compras_requisicao_abastecimento" || q.Get("requester") != "user2" || q.Get("slaStatus") != "ON_TIME" {
		t.Errorf("filtros não repassados: %v", q)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("esperava 2 tarefas, veio %d", len(tasks))
	}
	first, _ := tasks[0].(map[string]any)
	if first["requestId"].(float64) != 196542 || first["processId"] != "compras_requisicao_abastecimento" ||
		first["stateName"] != "Aprovar Requisição" {
		t.Errorf("task[0] inesperada: %+v", first)
	}

	code, _ = runMain(t, "task", "list", "--everyone", "--assignee", "x", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("--everyone + --assignee: exit=%d, quer %d", code, output.ExitUsage)
	}
	code, _ = runMain(t, "task", "list", "--status", "pendente", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("status inválido: exit=%d, quer %d", code, output.ExitUsage)
	}
}
