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
	query        url.Values
	summaryEmpty bool // fillTypeTasks responde [] (usuário sem central)
}

func (s *taskStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	// Os filtros de usuário resolvem login → userCode antes da busca.
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"`+r.URL.Query().Get("login")+`","userCode":"uc-`+r.URL.Query().Get("login")+`"}}`)
	})
	mux.HandleFunc("/process-management/api/v2/tasks", func(w http.ResponseWriter, r *http.Request) {
		s.query = r.URL.Query()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_tasks.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})
	// Rota legada da central de tarefas: o resumo por categoria (task summary).
	mux.HandleFunc("/ecm/api/rest/ecm/centralTasks/fillTypeTasks", func(w http.ResponseWriter, r *http.Request) {
		s.query = r.URL.Query()
		if s.summaryEmpty {
			io.WriteString(w, `[]`)
			return
		}
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_central_tasks_filltype.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})
	// Rota legada da central de tarefas: as tarefas paradas num pool (--group).
	mux.HandleFunc("/ecm/api/rest/ecm/centralTasks/getTasks/pool/", func(w http.ResponseWriter, r *http.Request) {
		s.query = r.URL.Query()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_central_tasks_pool.json"))
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
	// O login default (user1) é resolvido para o userCode antes da busca.
	if stub.query.Get("assignee") != "uc-user1" || stub.query.Get("status") != "NOT_COMPLETED" {
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
	if q.Get("processId") != "compras_requisicao_abastecimento" || q.Get("requester") != "uc-user2" || q.Get("slaStatus") != "ON_TIME" {
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

// --group lista as tarefas paradas no pool do grupo pela rota legada da
// central de tarefas, com o pool como responsável na tabela.
func TestTaskListGroup(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--group", "TI", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.query.Get("taskId") != "Pool:Group:TI" {
		t.Errorf("o pool não foi repassado no taskId: %v", stub.query)
	}
	for _, want := range []string{"219876", "contratos_troca_contribuinte", "Corrigir Integração",
		"Para o Grupo TI", "João Silva (jsilva)", "NOT_COMPLETED", "ON_TIME"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	// Contrato --json: data.tasks com o pool no assignee.
	code, stdout = runMain(t, "task", "list", "--group", "TI", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("esperava 3 tarefas, veio %d", len(tasks))
	}
	first, _ := tasks[0].(map[string]any)
	assignee, _ := first["assignee"].(map[string]any)
	if first["requestId"].(float64) != 219876 || assignee["code"] != "Pool:Group:TI" {
		t.Errorf("task[0] inesperada: %+v", first)
	}

	// --process recorta no cliente (a rota legada não filtra por processo).
	code, stdout = runMain(t, "task", "list", "--group", "TI", "--process", "outro_processo",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	env = output.Envelope{}
	json.Unmarshal([]byte(stdout), &env)
	data, _ = env.Data.(map[string]any)
	if tasks, _ := data["tasks"].([]any); len(tasks) != 0 {
		t.Errorf("--process deveria zerar o recorte, veio %d tarefas", len(tasks))
	}
}

// --role lista o pool de um papel pela mesma rota legada (taskId=Pool:Role:x).
func TestTaskListRole(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--role", "controladoria",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.query.Get("taskId") != "Pool:Role:controladoria" {
		t.Errorf("o pool do papel não foi repassado no taskId: %v", stub.query)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	tasks, _ := data["tasks"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("esperava 3 tarefas, veio %d", len(tasks))
	}
	first, _ := tasks[0].(map[string]any)
	assignee, _ := first["assignee"].(map[string]any)
	if assignee["code"] != "Pool:Role:controladoria" {
		t.Errorf("assignee deveria ser o pool do papel: %+v", assignee)
	}
}

// --group/--role não combinam entre si nem com filtros que a rota de pool não tem.
func TestTaskListGroupConflitos(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	for _, flags := range [][]string{
		{"--group", "TI", "--assignee", "user2"},
		{"--group", "TI", "--everyone"},
		{"--group", "TI", "--requester", "user2"},
		{"--group", "TI", "--sla", "expired"},
		{"--group", "TI", "--status", "completed"},
		{"--group", "TI", "--role", "controladoria"},
		{"--role", "controladoria", "--everyone"},
	} {
		args := append([]string{"task", "list"}, flags...)
		args = append(args, "--json", "--project", proj, "--server", "homolog")
		code, _ := runMain(t, args...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d", flags, code, output.ExitUsage)
		}
	}
}

// Um código de pool no --assignee passa direto para a busca v2 (sem resolver
// como login) — o caminho avançado para pool de papel, com --process junto.
func TestTaskListAssigneePool(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--assignee", "Pool:Role:financeiro",
		"--process", "compras_requisicao_abastecimento", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.query.Get("assignee") != "Pool:Role:financeiro" {
		t.Errorf("o código de pool deveria passar direto no assignee: %v", stub.query)
	}
}

// task summary: tabela com as categorias e os pools (com código), pulando a
// linha root; --user resolve o login; resumo vazio vira mensagem, não erro.
func TestTaskSummary(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "summary", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.query.Get("taskUserId") != "uc-user1" {
		t.Errorf("default deveria ser o usuário do servidor: %v", stub.query)
	}
	for _, want := range []string{"Categoria", "Pool", "Tarefas",
		"Tarefas a concluir", "Tarefas em pool: Papel", "└ Departamento Pessoal",
		"Pool:Role:departamento_pessoal", "Tarefas sob minha gerência", "2195"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Resumo de Tarefas") {
		t.Errorf("a linha root não deveria aparecer:\n%s", stdout)
	}

	// Contrato --json: data.summary com a árvore completa (root incluído).
	code, stdout = runMain(t, "task", "summary", "--user", "user2", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.query.Get("taskUserId") != "uc-user2" {
		t.Errorf("--user deveria resolver o login: %v", stub.query)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	summary, _ := data["summary"].([]any)
	if len(summary) != 5 {
		t.Fatalf("esperava 5 categorias, veio %d", len(summary))
	}
	poolrole, _ := summary[2].(map[string]any)
	children, _ := poolrole["children"].([]any)
	if poolrole["type"] != "poolrole" || len(children) != 2 {
		t.Errorf("poolrole inesperado: %+v", poolrole)
	}
	child, _ := children[1].(map[string]any)
	if child["pool"] != "Pool:Role:controladoria" || child["total"].(float64) != 10 {
		t.Errorf("pool filho inesperado: %+v", child)
	}

	// Resumo vazio: mensagem amigável e envelope ok (exit 0).
	stub.summaryEmpty = true
	code, stdout = runMain(t, "task", "summary", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("resumo vazio deveria ser exit 0: exit=%d stdout=%s", code, stdout)
	}
}
