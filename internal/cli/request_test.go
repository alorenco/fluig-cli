package cli

import (
	"encoding/base64"
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

	startBody      map[string]any
	moveBody       map[string]any
	needsAssignee  bool   // start responde 412 com possibleAssignees
	soapStartBody  string // envelope recebido no startProcess SOAP
	assigneesQuery url.Values
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
	// MoveResponse de sucesso (shape do swagger; o 200 real ainda não foi
	// capturado — o processo de teste exige anexo, sem API v2 de upload).
	moveResponse := `{"processInstanceId":196600,"processId":"compras_requisicao_abastecimento","processVersion":5,` +
		`"nextState":5,"nextStateName":"Aprovar Requisição","cardId":1111300,"toShowPossibleAssignees":false}`
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/start") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/quebrado/") {
			// Throw de evento chega como texto entre chaves, com HTML (real).
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "{Erro ao salvar dados de formulário: \n\n<b style='color:red'>Anexe a foto do Hodômetro (KM) antes de prosseguir!</b>}")
			return
		}
		json.NewDecoder(r.Body).Decode(&s.startBody)
		if s.needsAssignee {
			w.WriteHeader(http.StatusPreconditionFailed)
			io.WriteString(w, `{"processInstanceId":0,"toShowPossibleAssignees":true,"possibleAssignees":[`+
				`{"code":"c1","name":"Ana Andrade","login":"user1"},{"code":"c2","name":"Bruno Barros","login":"user2"}]}`)
			return
		}
		io.WriteString(w, moveResponse)
	})
	mux.HandleFunc("/process-management/api/v2/requests/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/process-management/api/v2/requests/196526":
			w.Write(readTD("rest_request_show.json"))
		case "/process-management/api/v2/requests/196526/tasks":
			w.Write(readTD("rest_request_tasks.json"))
		case "/process-management/api/v2/requests/196526/move":
			json.NewDecoder(r.Body).Decode(&s.moveBody)
			io.WriteString(w, moveResponse)
		case "/process-management/api/v2/requests/196540/attachments":
			w.Write(readTD("rest_request_attachments.json"))
		case "/process-management/api/v2/requests/196540/attachments/2/download":
			w.Write([]byte("PNG-BYTES-DE-TESTE"))
		case "/process-management/api/v2/requests/196526/possible-assignees":
			s.assigneesQuery = r.URL.Query()
			io.WriteString(w, `{"items":[{"code":"c1","name":"Ana Andrade","login":"user1"},`+
				`{"code":"c2","name":"Bruno Barros","login":"user2"}],"hasNext":false}`)
		default:
			// Formato real do 404 da homologação (2026-07-13).
			http.Error(w, `{"code":"BPMWorkflowProcessNotFoundException","message":"Solicitação não encontrada."}`, http.StatusNotFound)
		}
	})
	// SOAP startProcess (start --attach/--no-send) + findUserByLogin do
	// ResolveUserCode. Resposta com os pares chave/valor (shape validado ao
	// vivo na homologação em 2026-07-14 — iProcess = número criado).
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc"}}`)
	})
	mux.HandleFunc("/webdesk/ECMWorkflowEngineService", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("SOAPAction") != "startProcess" {
			http.Error(w, "op?", 500)
			return
		}
		b, _ := io.ReadAll(r.Body)
		s.soapStartBody = string(b)
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
			`<ns2:startProcessResponse xmlns:ns2="http://ws.workflow.ecm.technology.totvs.com/"><result>`+
			`<item><item>iProcess</item><item>196542</item></item>`+
			`<item><item>WKNumState</item><item>4</item></item>`+
			`</result></ns2:startProcessResponse></soap:Body></soap:Envelope>`)
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

// start: monta o corpo com formFields/comment e devolve o MoveResponse.
func TestRequestStart(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--field", "quantidade=10", "--field", "codEquipamento=1084",
		"--comment", "teste", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	ff, _ := stub.startBody["formFields"].(map[string]any)
	if ff["quantidade"] != "10" || ff["codEquipamento"] != "1084" || stub.startBody["comment"] != "teste" {
		t.Errorf("corpo do start inesperado: %+v", stub.startBody)
	}
	if _, tem := stub.startBody["targetState"]; tem {
		t.Error("targetState não deveria ir no corpo quando não informado")
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	res, _ := data["result"].(map[string]any)
	if res["requestId"].(float64) != 196600 || res["nextStateName"] != "Aprovar Requisição" {
		t.Errorf("resultado inesperado: %+v", res)
	}
}

// start com HTTP 412: lista os possíveis responsáveis e pede --assignee (exit 2).
func TestRequestStartPrecisaResponsavel(t *testing.T) {
	stub := &requestStub{needsAssignee: true}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--field", "a=b", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitUsage, stdout)
	}
	for _, want := range []string{"Ana Andrade (user1)", "Bruno Barros (user2)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem a opção %q:\n%s", want, stdout)
		}
	}
}

// Throw de evento no servidor (corpo não-JSON com HTML): mensagem limpa, exit 5.
func TestRequestStartErroEvento(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "start", "quebrado", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d", code, output.ExitServer)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "Anexe a foto do Hodômetro") {
		t.Errorf("mensagem deveria trazer o throw do evento: %+v", env.Error)
	}
	if strings.Contains(env.Error.Message, "<b") {
		t.Error("mensagem não deveria conter HTML")
	}
}

// move: descobre a tarefa em aberto sozinho (movementSequence do show).
func TestRequestMove(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196526",
		"--target-state", "13", "--field", "aprNivel1=aprovado", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.moveBody["movementSequence"].(float64) != 4 {
		t.Errorf("movementSequence deveria vir do currentMovements (4): %+v", stub.moveBody)
	}
	if stub.moveBody["targetState"].(float64) != 13 {
		t.Errorf("targetState não repassado: %+v", stub.moveBody)
	}
	ff, _ := stub.moveBody["formFields"].(map[string]any)
	if ff["aprNivel1"] != "aprovado" {
		t.Errorf("formFields não repassados: %+v", stub.moveBody)
	}
}

// assignees: tabela com os possíveis responsáveis.
func TestRequestAssignees(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "assignees", "196526", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Login", "Nome", "user1", "Ana Andrade", "user2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// start --attach: vai pelo SOAP startProcess com o anexo em base64 e
// completeTask=true; --no-send manda completeTask=false.
func TestRequestStartComAnexo(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	att := filepath.Join(t.TempDir(), "foto.png")
	os.WriteFile(att, []byte("conteudo-png"), 0o644)

	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--field", "quantidade=10", "--attach", att, "--target-state", "5",
		"--comment", "teste", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["requestId"].(float64) != 196542 || data["sent"] != true {
		t.Errorf("envelope inesperado: %+v", data)
	}
	body := stub.soapStartBody
	wantB64 := base64.StdEncoding.EncodeToString([]byte("conteudo-png"))
	for _, want := range []string{"<processId>compras_requisicao_abastecimento</processId>",
		"<choosedState>5</choosedState>", "<completeTask>true</completeTask>",
		"<fileName>foto.png</fileName>", "<filecontent>" + wantB64 + "</filecontent>",
		"<userId>uc</userId>", "<item><item>quantidade</item><item>10</item></item>"} {
		if !strings.Contains(body, want) {
			t.Errorf("envelope SOAP sem %q:\n%s", want, body)
		}
	}

	code, stdout = runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--field", "a=b", "--no-send", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--no-send exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stub.soapStartBody, "<completeTask>false</completeTask>") {
		t.Error("--no-send deveria mandar completeTask=false")
	}
	json.Unmarshal([]byte(stdout), &env)
	data, _ = env.Data.(map[string]any)
	if data["sent"] != false {
		t.Errorf("sent deveria ser false: %+v", data)
	}
}

// assignees --target-state: repassa a etapa (o servidor exige quando há mais
// de um destino possível).
func TestRequestAssigneesTargetState(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "request", "assignees", "196526", "--target-state", "13",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if stub.assigneesQuery.Get("targetState") != "13" {
		t.Errorf("targetState não repassado: %v", stub.assigneesQuery)
	}
}

// --fields-file: lê o objeto JSON, converte escalares para string e o --field
// sobrepõe o arquivo (template).
func TestRequestStartFieldsFile(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	file := filepath.Join(t.TempDir(), "campos.json")
	os.WriteFile(file, []byte(`{"codEquipamento":1084,"quantidade":"10","completaTanque":false,"observacao":null}`), 0o644)

	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--fields-file", file, "--field", "quantidade=20",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	ff, _ := stub.startBody["formFields"].(map[string]any)
	if ff["codEquipamento"] != "1084" || ff["completaTanque"] != "false" || ff["observacao"] != "" {
		t.Errorf("escalares do JSON não convertidos: %+v", ff)
	}
	if ff["quantidade"] != "20" {
		t.Errorf("--field deveria sobrepor o arquivo (quantidade=20): %+v", ff)
	}
}

// --fields-file -: lê o JSON do stdin (modo natural para agentes/pipelines).
func TestRequestStartFieldsStdin(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	io.WriteString(w, `{"descricao":"via stdin"}`)
	w.Close()

	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--fields-file", "-", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	ff, _ := stub.startBody["formFields"].(map[string]any)
	if ff["descricao"] != "via stdin" {
		t.Errorf("campos do stdin não repassados: %+v", stub.startBody)
	}
}

// --fields-file com problemas: JSON inválido e valor aninhado → exit 2;
// arquivo inexistente → exit 4. Nada chega ao servidor.
func TestRequestStartFieldsFileErros(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	dir := t.TempDir()

	bad := filepath.Join(dir, "invalido.json")
	os.WriteFile(bad, []byte(`{"a": `), 0o644)
	code, _ := runMain(t, "request", "start", "p", "--fields-file", bad, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("JSON inválido: exit=%d, quer %d", code, output.ExitUsage)
	}

	nested := filepath.Join(dir, "aninhado.json")
	os.WriteFile(nested, []byte(`{"filhos": [{"a":1}]}`), 0o644)
	code, _ = runMain(t, "request", "start", "p", "--fields-file", nested, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("valor aninhado: exit=%d, quer %d", code, output.ExitUsage)
	}

	code, _ = runMain(t, "request", "start", "p", "--fields-file", filepath.Join(dir, "nao_existe.json"), "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("arquivo inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	if stub.startBody != nil {
		t.Error("nada deveria ter chegado ao servidor")
	}
}

// move --fields-file: mesmo mecanismo do start.
func TestRequestMoveFieldsFile(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	file := filepath.Join(t.TempDir(), "aprovacao.json")
	os.WriteFile(file, []byte(`{"aprNivel1":"aprovado","comentarioNivel1":"ok"}`), 0o644)

	code, stdout := runMain(t, "request", "move", "196526", "--target-state", "13",
		"--fields-file", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	ff, _ := stub.moveBody["formFields"].(map[string]any)
	if ff["aprNivel1"] != "aprovado" || ff["comentarioNivel1"] != "ok" {
		t.Errorf("campos do arquivo não repassados: %+v", stub.moveBody)
	}
}

// attachments: lista com o formulário marcado; --download baixa só os
// arquivos (round-trip byte a byte validado ao vivo na homolog).
func TestRequestAttachments(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "request", "attachments", "196540", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Seq", "Arquivo", "Anexado por", "(formulário)", "hodometro_teste.png", "Ana Andrade (user1)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	dir := t.TempDir()
	code, stdout = runMain(t, "request", "attachments", "196540", "--download", "--dir", dir,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--download exit=%d stdout=%s", code, stdout)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hodometro_teste.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PNG-BYTES-DE-TESTE" {
		t.Errorf("conteúdo inesperado: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "anexo_1")); err == nil {
		t.Error("o (formulário) não deveria ser baixado no --download")
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	first, _ := results[0].(map[string]any)
	if len(results) != 1 || first["action"] != "downloaded" {
		t.Errorf("results inesperado: %+v", results)
	}
}

// attachments --seq inexistente: exit 4 validado ANTES do download (o
// servidor real responde 400 de "permissão", enganoso).
func TestRequestAttachmentsSeqInexistente(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "request", "attachments", "196540", "--seq", "9", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
}
