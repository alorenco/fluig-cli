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
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// procStub simula o processo compras_entrada_documento (diagrama real da
// fixture rest_process_export_full.xml): start → 76 (gateway) → 45 (pool
// revisor_despesas); move 50 (gateway) → 5. As respostas de erro são as
// REAIS do Gate 0 (testdata/test/).
type procStub struct {
	mu        sync.Mutex
	hits      int
	authFails bool
	current   int
	status    string
	moves     []map[string]any
	canceled  bool
	cardPut   map[string]any
	noHelper  bool
	// history: solicitação → linhas da HISTOR_PROCES (MOVTO, ANT, ESTADO,
	// THREAD, VERSÃO, PROCESSO), na ordem do SQL do RequestHistory.
	history map[int][][]string
}

func readTestTD(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (s *procStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	count := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			s.hits++
			s.mu.Unlock()
			h(w, r)
		}
	}
	mux.HandleFunc("/portal/api/servlet/login.do", count(func(w http.ResponseWriter, r *http.Request) {
		if s.authFails {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	}))
	mux.HandleFunc("/portal/p/api/servlet/ping", count(func(w http.ResponseWriter, r *http.Request) {
		if s.authFails {
			io.WriteString(w, `{}`)
			return
		}
		io.WriteString(w, `{"message":"pong"}`)
	}))
	mux.HandleFunc("/api/public/wcm/version", count(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"value":"TOTVS Fluig Plataforma - Voyager 2.0.0-260707"}`)
	}))
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", count(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc"}}`)
	}))
	mux.HandleFunc("/process-management/api/v2/processes/", count(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/inexistente/") {
			http.Error(w, `{"code":"BPMProcessNotFoundException","message":"Processo não encontrado"}`, http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/export/xml") {
			if strings.Contains(r.URL.Path, "/process-versions/99/") {
				http.Error(w, `{"code":"NotFound","message":"versão não encontrada"}`, http.StatusNotFound)
				return
			}
			w.Write(readTestTD(t, "rest_process_export_full.xml"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/process-versions") {
			io.WriteString(w, `{"items":[{"version":24,"active":false},{"version":25,"active":true}],"hasNext":false}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		ff, _ := body["formFields"].(map[string]any)
		if ff["forcarErro"] == "S" {
			// validateForm real: 500, texto entre chaves, com HTML.
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(readTestTD(t, "test/start_error_validateform.txt"))
			return
		}
		s.mu.Lock()
		s.current, s.status = 45, "OPEN"
		s.mu.Unlock()
		io.WriteString(w, `{"processInstanceId":700001,"processId":"compras_entrada_documento","processVersion":25,`+
			`"nextState":45,"nextStateName":null,"cardId":1247334,"toShowPossibleAssignees":false}`)
	}))
	mux.HandleFunc("/process-management/api/v2/requests/", count(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch r.URL.Path {
		case "/process-management/api/v2/requests/700001":
			io.WriteString(w, `{"processInstanceId":700001,"processId":"compras_entrada_documento","processVersion":25,`+
				`"status":"`+s.status+`","formId":157153,"formRecordId":1247334,"currentMovements":[`+
				`{"movementSequence":3,"active":true,"slaStatus":"ON_TIME","state":{"sequence":`+strconv.Itoa(s.current)+`,"stateName":"Etapa `+strconv.Itoa(s.current)+`"}}]}`)
		case "/process-management/api/v2/requests/700001/tasks":
			io.WriteString(w, `{"items":[`+
				`{"movementSequence":1,"status":"COMPLETED","assignee":{"code":"uc","login":"u"},"state":{"sequence":76,"stateName":"Passar por Revisão"}},`+
				`{"movementSequence":2,"status":"NOT_COMPLETED","assignee":{"code":"Pool:Role:revisor_despesas","login":""},"state":{"sequence":`+strconv.Itoa(s.current)+`,"stateName":"x"}}`+
				`],"hasNext":false}`)
		case "/process-management/api/v2/requests/700001/move":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			s.moves = append(s.moves, body)
			ff, _ := body["formFields"].(map[string]any)
			if ff["decisao"] == "incoerente" {
				// beforeTaskSave real: 400 JSON BPMBeforeTaskSaveException.
				w.WriteHeader(http.StatusBadRequest)
				w.Write(readTestTD(t, "test/move_error_beforetasksave.json"))
				return
			}
			s.current = 5
			io.WriteString(w, `{"processInstanceId":700001,"nextState":5,"nextStateName":null,"cardId":1247334}`)
		default:
			http.Error(w, `{"code":"BPMWorkflowProcessNotFoundException","message":"Solicitação não encontrada."}`, http.StatusNotFound)
		}
	}))
	mux.HandleFunc("/ecm-forms/api/v2/cardindex/157153/cards/1247334", count(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			s.mu.Lock()
			json.NewDecoder(r.Body).Decode(&s.cardPut)
			s.mu.Unlock()
		}
		w.Write(readTestTD(t, "test/form_record_children.json"))
	}))
	mux.HandleFunc("/webdesk/ECMWorkflowEngineService", count(func(w http.ResponseWriter, r *http.Request) {
		op := r.Header.Get("SOAPAction")
		if op == "cancelInstance" {
			s.mu.Lock()
			s.canceled, s.status = true, "CANCELED"
			s.mu.Unlock()
		}
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
			`<ns2:`+op+`Response xmlns:ns2="http://ws.workflow.ecm.technology.totvs.com/"><result>OK</result></ns2:`+op+
			`Response></soap:Body></soap:Envelope>`)
	}))
	mux.HandleFunc("/fluigcliHelper/api/ping", count(func(w http.ResponseWriter, r *http.Request) {
		if s.noHelper {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	}))
	mux.HandleFunc("/fluigcliHelper/api/version", count(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.12.0"}`)
	}))
	mux.HandleFunc("/fluigcliHelper/api/db/query", count(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SQL    string   `json:"sql"`
			Params []string `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		id, _ := strconv.Atoi(body.Params[0])
		s.mu.Lock()
		rows := s.history[id]
		s.mu.Unlock()
		b, _ := json.Marshal(map[string]any{"columns": []any{}, "rows": rows, "rowCount": len(rows)})
		w.Write(b)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// testProject cria o projeto com o servidor e a suíte em tests/compras/.
func testProject(t *testing.T, stubURL, env, suite string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "t-srv", Name: "homolog", Host: u.host, Port: u.port, Username: "u", CompanyID: 1, Env: env}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(proj, "tests", "compras")
	os.MkdirAll(filepath.Join(dir, "fixtures"), 0o755)
	os.WriteFile(filepath.Join(dir, "fixtures", "card.json"), []byte(`{"valor":"50,00","doc":"{{unique}}","emissao":"{{today}}"}`), 0o644)
	if suite != "" {
		os.WriteFile(filepath.Join(dir, "suite.json"), []byte(suite), 0o644)
	}
	return proj
}

const suiteCompras = `{"process":"compras_entrada_documento","defaults":{"card":"fixtures/card.json"},"scenarios":[
 {"name":"aprova","steps":[
   {"start":true,"expect":{"state":45,"assignee":"Pool:Role:revisor_despesas"}},
   {"assume":true},
   {"move":{"target":50,"fields":{"decisao":"aprovado"}},"expect":{"state":5,"path":{"visited":[4,76,5]},"card":{"pgtoUrgente":"N"}}}
 ]}]}`

func envelope(t *testing.T, stdout string) (output.Envelope, map[string]any) {
	t.Helper()
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é um envelope JSON único: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	return env, data
}

func TestTestRunPassaJSON(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	env, data := envelope(t, stdout)
	if !env.OK || env.Error != nil {
		t.Fatalf("envelope: %+v", env)
	}
	sum := data["summary"].(map[string]any)
	if sum["passed"].(float64) != 1 || sum["failed"].(float64) != 0 {
		t.Errorf("resumo: %+v", sum)
	}
	sc := data["suites"].([]any)[0].(map[string]any)["scenarios"].([]any)[0].(map[string]any)
	if sc["requestId"].(float64) != 700001 || sc["cleanup"].(map[string]any)["status"] != "ok" || !stub.canceled {
		t.Errorf("limpeza: %+v canceled=%v", sc, stub.canceled)
	}
	// As expressões da fixture chegam resolvidas ao start? (via move não; o
	// start é o primeiro POST em /processes) — e o move manda os campos.
	if len(stub.moves) != 1 || stub.moves[0]["targetState"].(float64) != 50 {
		t.Errorf("move: %+v", stub.moves)
	}
}

// Modo humano: tabela com bordas, cabeçalho e o resultado (regra 7).
func TestTestRunTabela(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout := runMain(t, "test", "run", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "Cenário", "Resultado", "Solicitação", "Tempo", "aprova", "passou", "700001",
		"Processo: compras_entrada_documento (versão 25)", "Resumo: 1 passaram · 0 falharam"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
}

func TestTestRunFalhaTestFailed(t *testing.T) {
	stub := &procStub{}
	suite := strings.Replace(suiteCompras, `"state":5,`, `"state":99,`, 1)
	proj := testProject(t, stub.server(t).URL, "hml", suite)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer 1\n%s", code, stdout)
	}
	env, data := envelope(t, stdout)
	if env.OK || env.Error == nil || env.Error.Code != output.CodeTestFailed {
		t.Fatalf("envelope: %+v", env.Error)
	}
	steps := data["suites"].([]any)[0].(map[string]any)["scenarios"].([]any)[0].(map[string]any)["steps"].([]any)
	as := steps[2].(map[string]any)["assertions"].([]any)
	a := as[0].(map[string]any)
	if a["type"] != "state" || a["expected"].(float64) != 99 || a["actual"].(float64) != 5 || a["passed"] != false {
		t.Errorf("asserção esperado × obtido: %+v", a)
	}
	// Asserções que passaram também vêm no JSON.
	if len(as) != 5 {
		t.Errorf("esperava 5 asserções (state, path×3, card), veio %d", len(as))
	}
}

// O erro REAL do beforeTaskSave (400 JSON com HTML) atravessa a CLI inteira.
func TestTestRunErroEsperadoFixtureReal(t *testing.T) {
	stub := &procStub{}
	suite := `{"process":"compras_entrada_documento","defaults":{"card":"fixtures/card.json"},"scenarios":[
	 {"name":"bloqueia","steps":[{"start":true},{"assume":true},
	   {"move":{"target":50,"fields":{"decisao":"incoerente"}},"expect":{"error":"Decisão da aprovação é incompatível","state":45}}]},
	 {"name":"validateForm","steps":[{"start":{"fields":{"forcarErro":"S"}},"expect":{"error":{"contains":"Tipo de Pagamento","code":"SERVER_ERROR"}}}]}
	]}`
	proj := testProject(t, stub.server(t).URL, "hml", suite)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
}

func TestTestRunRecusaProducao(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, config.EnvProd, suiteCompras)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog", "--yes")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer 2", code)
	}
	env, _ := envelope(t, stdout)
	if env.Error.Code != output.CodeUsage || !strings.Contains(env.Error.Message, "não roda em produção") {
		t.Errorf("erro: %+v", env.Error)
	}
	if stub.hits != 0 {
		t.Errorf("o guard tem de vir antes de qualquer requisição: %d hits", stub.hits)
	}
	// O --dry-run também respeita a recusa.
	code, _ = runMain(t, "test", "run", "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("dry-run em produção: exit=%d", code)
	}
}

func TestTestRunSuiteInvalida(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", `{"process":"x","scenarios":[{"name":"a","steps":[{"start":true,"expcet":{}}]}]}`)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	env, _ := envelope(t, stdout)
	if code != output.ExitUsage || !strings.Contains(env.Error.Message, `chave desconhecida "expcet"`) {
		t.Fatalf("exit=%d erro=%+v", code, env.Error)
	}
	if stub.hits != 0 {
		t.Errorf("arquivo inválido falha antes de conectar: %d hits", stub.hits)
	}
}

func TestTestRunFixtureEProcessoInexistentes(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml",
		`{"process":"x","scenarios":[{"name":"a","card":"fixtures/nao.json","steps":[{"start":true}]}]}`)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	env, _ := envelope(t, stdout)
	if code != output.ExitNotFound || !strings.Contains(env.Error.Message, "nao.json") {
		t.Fatalf("fixture: exit=%d erro=%+v", code, env.Error)
	}

	proj = testProject(t, stub.server(t).URL, "hml",
		`{"process":"inexistente","scenarios":[{"name":"a","steps":[{"start":true}]}]}`)
	code, stdout = runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	env, _ = envelope(t, stdout)
	if code != output.ExitNotFound || !strings.Contains(env.Error.Message, `processo "inexistente"`) {
		t.Fatalf("processo: exit=%d erro=%+v", code, env.Error)
	}
	if stub.moves != nil {
		t.Error("nada pode rodar com processo inexistente")
	}

	code, _ = runMain(t, "test", "run", filepath.Join(proj, "nada.json"), "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("suíte inexistente: exit=%d", code)
	}
}

func TestTestRunAutenticacaoFalha(t *testing.T) {
	stub := &procStub{authFails: true}
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	t.Setenv("FLUIGCLI_NO_SESSION_CACHE", "1")
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitAuth {
		t.Fatalf("exit=%d, quer 3\n%s", code, stdout)
	}
}

func TestTestRunDryRun(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout := runMain(t, "test", "run", "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.moves != nil || stub.canceled {
		t.Error("o dry-run não pode escrever")
	}
	_, data := envelope(t, stdout)
	if data["dryRun"] != true {
		t.Errorf("data: %+v", data)
	}

	// Destino inexistente e transição impossível: exit 2 com a lista em data.
	ruim := strings.Replace(suiteCompras, `"target":50`, `"target":15`, 1)
	proj = testProject(t, stub.server(t).URL, "hml", ruim)
	code, stdout = runMain(t, "test", "run", "--dry-run", "--json", "--project", proj, "--server", "homolog")
	env, data := envelope(t, stdout)
	if code != output.ExitUsage || env.Error.Code != output.CodeUsage || data == nil {
		t.Fatalf("exit=%d env=%+v", code, env.Error)
	}
	steps := data["suites"].([]any)[0].(map[string]any)["scenarios"].([]any)[0].(map[string]any)["steps"].([]any)
	if e, _ := steps[2].(map[string]any)["error"].(string); !strings.Contains(e, "não há transição da etapa 45") {
		t.Errorf("problema da transição: %v", steps[2])
	}
}

func TestTestRunFlagsInvalidas(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	for _, args := range [][]string{
		{"--scenario", "nao existe"},
		{"--var", "semIgual"},
		{"--step-timeout", "0s"},
	} {
		full := append([]string{"test", "run", "--json", "--project", proj, "--server", "homolog"}, args...)
		if code, _ := runMain(t, full...); code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer 2", args, code)
		}
	}
	if stub.hits != 0 {
		t.Errorf("flag inválida falha antes de conectar: %d hits", stub.hits)
	}
}
