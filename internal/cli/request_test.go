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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// requestStub simula a REST v2 de solicitações com as fixtures reais
// sanitizadas da homologação.
type requestStub struct {
	listQuery url.Values

	version        string // versão do produto devolvida (vazio → Voyager 2.0)
	legacyList     bool   // serve a fixture 1.8 (activities) na listagem
	startBody      map[string]any
	moveBody       map[string]any
	needsAssignee  bool   // start responde 412 com possibleAssignees
	soapStartBody  string // envelope recebido no startProcess SOAP
	assigneesQuery url.Values
	// writeDelay atrasa move/start para o cliente estourar o --timeout (o
	// servidor real continua processando depois disso — ROADMAP §2.10-B).
	writeDelay time.Duration
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
	// Versão do produto (ServerVersion): default = Fluig 2.0 (currentMovements).
	mux.HandleFunc("/api/public/wcm/version", func(w http.ResponseWriter, r *http.Request) {
		v := s.version
		if v == "" {
			v = "TOTVS Fluig Plataforma - Voyager 2.0.0-260707"
		}
		io.WriteString(w, `{"value":"`+v+`"}`)
	})
	mux.HandleFunc("/process-management/api/v2/requests", func(w http.ResponseWriter, r *http.Request) {
		s.listQuery = r.URL.Query()
		if s.legacyList {
			// Fluig 1.8: sem currentMovements; etapa atual vem de activities.
			w.Write([]byte(`{"items":[` + string(readTD("rest_request_show_legacy.json")) + `],"hasNext":false}`))
			return
		}
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
		if s.writeDelay > 0 {
			time.Sleep(s.writeDelay)
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
		if s.writeDelay > 0 && strings.HasSuffix(r.URL.Path, "/move") {
			time.Sleep(s.writeDelay)
		}
		if strings.HasSuffix(r.URL.Path, "/move") {
			// O servidor responde 404 no /move tanto para solicitação
			// inexistente quanto para "a tarefa não é sua" (pool sem dono,
			// atividade automática). 1965xx abaixo cobrem os três casos.
			if strings.HasPrefix(r.URL.Path, "/process-management/api/v2/requests/1965") &&
				strings.Contains("|196530|196531|196532|", strings.TrimSuffix(strings.TrimPrefix(r.URL.Path,
					"/process-management/api/v2/requests/"), "/move")) {
				http.Error(w, `{"code":"BPMWorkflowProcessNotFoundException","message":"Solicitação não encontrada."}`,
					http.StatusNotFound)
				return
			}
			json.NewDecoder(r.Body).Decode(&s.moveBody)
			io.WriteString(w, moveResponse)
			return
		}
		// Tarefas em aberto dos três casos de 404 no move. Shapes REAIS da
		// homologação (2026-08-06): no pool e na atividade automática o
		// `login` vem VAZIO e quem identifica é o `code`.
		const tarefaPool = `{"items":[{"processInstanceId":196530,"movementSequence":4,"status":"NOT_COMPLETED",` +
			`"slaStatus":"ON_TIME","assignee":{"code":"Pool:Role:sucesso_cliente","name":"Sucesso do Cliente","login":""},` +
			`"state":{"sequence":21,"stateName":"Acompanhar Retornos"}}],"hasNext":false}`
		const tarefaAuto = `{"items":[{"processInstanceId":196531,"movementSequence":1,"status":"NOT_COMPLETED",` +
			`"slaStatus":"ON_TIME","assignee":{"code":"System:Auto","name":"","login":""},` +
			`"state":{"sequence":7,"stateName":"Mover Documentos"}}],"hasNext":false}`
		// Etapa corrente com o MESMO movimento duas vezes (tarefa do pool + a do
		// usuário que assumiu) — o caso real de produção que gerava duas opções
		// idênticas. Não é ambiguidade.
		const movDuplicado = `{"processInstanceId":196527,"processId":"contratos_taxa_limpeza","status":"OPEN",` +
			`"currentMovements":[` +
			`{"movementSequence":15,"active":true,"slaStatus":"ON_TIME","state":{"sequence":22,"stateName":"Corrigir Integração"}},` +
			`{"movementSequence":15,"active":true,"slaStatus":"ON_TIME","state":{"sequence":22,"stateName":"Corrigir Integração"}}]}`
		// Movimentos DIFERENTES em aberto (atividades paralelas): aí sim é escolha.
		const movParalelo = `{"processInstanceId":196528,"processId":"contratos_taxa_limpeza","status":"OPEN",` +
			`"currentMovements":[` +
			`{"movementSequence":15,"active":true,"slaStatus":"ON_TIME","state":{"sequence":22,"stateName":"Corrigir Integração"}},` +
			`{"movementSequence":16,"active":true,"slaStatus":"EXPIRED","state":{"sequence":30,"stateName":"Aprovar Diretoria"}}]}`
		const tarefasParalelo = `{"items":[` +
			`{"processInstanceId":196528,"movementSequence":15,"status":"TRANSFERRED","slaStatus":"ON_TIME","state":{"sequence":22,"stateName":"Corrigir Integração"}},` +
			`{"processInstanceId":196528,"movementSequence":15,"status":"NOT_COMPLETED","slaStatus":"ON_TIME","assignee":{"code":"c3","name":"João Silva","login":"jsilva"},"state":{"sequence":22,"stateName":"Corrigir Integração"}},` +
			`{"processInstanceId":196528,"movementSequence":16,"status":"NOT_COMPLETED","slaStatus":"EXPIRED","assignee":{"code":"c4","name":"Maria Souza","login":"msouza"},"state":{"sequence":30,"stateName":"Aprovar Diretoria"}}],"hasNext":false}`
		// Solicitação sem tarefa em aberto.
		const semTarefa = `{"processInstanceId":196529,"processId":"contratos_taxa_limpeza","status":"FINALIZED","currentMovements":[]}`
		switch r.URL.Path {
		case "/process-management/api/v2/requests/196526":
			w.Write(readTD("rest_request_show.json"))
		case "/process-management/api/v2/requests/196526/tasks":
			w.Write(readTD("rest_request_tasks.json"))
		case "/process-management/api/v2/requests/196527":
			io.WriteString(w, movDuplicado)
		case "/process-management/api/v2/requests/196528":
			io.WriteString(w, movParalelo)
		case "/process-management/api/v2/requests/196528/tasks":
			io.WriteString(w, tarefasParalelo)
		case "/process-management/api/v2/requests/196529":
			io.WriteString(w, semTarefa)
		case "/process-management/api/v2/requests/196530/tasks":
			io.WriteString(w, tarefaPool)
		case "/process-management/api/v2/requests/196531/tasks":
			io.WriteString(w, tarefaAuto)
		// 196532 cai no default (404 também nas tarefas) = solicitação
		// inexistente de verdade.
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
	// assignee/requester chegam como userCode (o stub resolve qualquer login
	// para "uc" — a API real filtra por código, não por login).
	if q.Get("processId") != "contratos_taxa_limpeza" || q.Get("status") != "OPEN" ||
		q.Get("slaStatus") != "ON_TIME" || q.Get("assignee") != "uc" || q.Get("requester") != "uc" {
		t.Errorf("filtros não repassados: %v", q)
	}
	if got := q["expand"]; len(got) != 2 || got[0] != "requester" || got[1] != "currentMovements" {
		t.Errorf("expand inesperado: %v", got)
	}
}

// Fluig 1.8: currentMovements não existe → a listagem pede expand=activities e
// a "Etapa atual" é derivada da activity ativa (regressão real de produção).
func TestRequestListLegado18(t *testing.T) {
	stub := &requestStub{version: "TOTVS Fluig Plataforma - Crystal Mist 1.8.2-260707", legacyList: true}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "list", "--process", "compras_requisicao_abastecimento",
		"--status", "open", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	// O expand da etapa vira "activities" (não currentMovements) no 1.8.
	if got := stub.listQuery["expand"]; len(got) != 2 || got[1] != "activities" {
		t.Errorf("expand devia ser [requester activities] no 1.8: %v", got)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	requests, _ := data["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("esperava 1 solicitação, veio %d", len(requests))
	}
	first, _ := requests[0].(map[string]any)
	steps, _ := first["currentSteps"].([]any)
	// Só a activity ativa vira etapa atual (a inativa "Início" é descartada).
	if len(steps) != 1 {
		t.Fatalf("esperava 1 etapa (a ativa), veio %d: %+v", len(steps), first["currentSteps"])
	}
	step, _ := steps[0].(map[string]any)
	if step["stateName"] != "Aprovar Requisição" || step["sequence"].(float64) != 5 {
		t.Errorf("etapa ativa inesperada: %+v", step)
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

// Linha de tabela-filha: a convenção do Fluig é o sufixo `___<rowId>` no nome
// do campo, a mesma que a API usa na LEITURA do card (ver FLUIG-APIS.md). A CLI
// não pode transformar nem rejeitar essas chaves — ela repassa como vieram.
// Este teste tranca isso: um "sanitizador" de nome de campo no --fields-file
// quebraria silenciosamente todo teste de processo com anexo (ROADMAP3 §4.8).
func TestRequestStartTabelaFilha(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	arq := filepath.Join(t.TempDir(), "campos.json")
	os.WriteFile(arq, []byte(`{
		"numeroNotificacaoAuto": "TESTE-001",
		"anxNotificacaoFileId___1": "1247035",
		"anxNotificacaoNome___1": "Notificação nº TESTE-001.pdf",
		"anxNotificacaoFileId___2": "1247036"
	}`), 0o644)

	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--fields-file", arq, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	ff, _ := stub.startBody["formFields"].(map[string]any)
	for campo, quer := range map[string]string{
		"numeroNotificacaoAuto":    "TESTE-001",
		"anxNotificacaoFileId___1": "1247035",
		"anxNotificacaoNome___1":   "Notificação nº TESTE-001.pdf",
		"anxNotificacaoFileId___2": "1247036",
	} {
		if ff[campo] != quer {
			t.Errorf("campo %q chegou como %v, quer %q", campo, ff[campo], quer)
		}
	}
}

// O 404 do move é ambíguo no servidor. A CLI desambigua com um GET de tarefas
// (só no caminho de erro) e devolve um código próprio no lugar do NOT_FOUND
// genérico, que mandava depurar permissão e cache de papel (ROADMAP3 §4.4).
func TestRequestMove404Desambiguado(t *testing.T) {
	casos := []struct {
		nome    string
		id      string
		code    string
		trechos []string
	}{
		{"pool sem dono", "196530", output.CodePoolTaskNotAssigned,
			[]string{"pool", "Sucesso do Cliente", "Pool:Role:sucesso_cliente", "Acompanhar Retornos", "21"}},
		{"atividade automática", "196531", output.CodeNoHumanTask,
			[]string{"atividade automática", "Mover Documentos", "7", "não há tarefa humana"}},
		{"solicitação inexistente continua NOT_FOUND", "196532", output.CodeNotFound, nil},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			stub := &requestStub{}
			proj := requestProject(t, stub.server(t).URL)
			code, stdout := runMain(t, "request", "move", tc.id, "--movement", "4",
				"--json", "--project", proj, "--server", "homolog")
			// O exit NÃO muda: a tarefa que seria SUA realmente não existe.
			if code != output.ExitNotFound {
				t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitNotFound, stdout)
			}
			var env output.Envelope
			json.Unmarshal([]byte(stdout), &env)
			if env.Error == nil || env.Error.Code != tc.code {
				t.Fatalf("código do erro = %+v, quer %s", env.Error, tc.code)
			}
			for _, want := range tc.trechos {
				if !strings.Contains(env.Error.Message, want) {
					t.Errorf("mensagem sem %q: %s", want, env.Error.Message)
				}
			}
		})
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

// Duas tarefas no MESMO movimento (pool + usuário que assumiu) não são
// ambiguidade: o movimento é um só e o move segue (ROADMAP §2.10-C — antes a
// CLI pedia --movement listando duas opções idênticas).
func TestRequestMoveMovimentoDuplicado(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196527", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d, quer 0 (movimento repetido não é escolha)\n%s", code, stdout)
	}
	if stub.moveBody["movementSequence"].(float64) != 15 {
		t.Errorf("movementSequence=%v, quer 15", stub.moveBody["movementSequence"])
	}
}

// Movimentos DIFERENTES em aberto: aí a escolha é do usuário, e cada opção sai
// com responsável e status para dar para diferenciar.
func TestRequestMoveMovimentosParalelos(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)

	// Modo humano: tabela com responsável e status.
	code, stdout := runMain(t, "request", "move", "196528", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitUsage, stdout)
	}
	for _, want := range []string{"Movimento", "Responsável", "Status", "SLA",
		"Corrigir Integração", "João Silva (jsilva)", "NOT_COMPLETED",
		"Aprovar Diretoria", "Maria Souza (msouza)", "EXPIRED"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if stub.moveBody != nil {
		t.Error("nada deveria ter sido movimentado no caso ambíguo")
	}

	// --json: as opções vão no envelope, para o agente escolher sem parsear texto.
	code, stdout = runMain(t, "request", "move", "196528", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("--json exit=%d, quer %d\n%s", code, output.ExitUsage, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != output.CodeUsage {
		t.Fatalf("envelope inesperado: %+v", env)
	}
	if !strings.Contains(env.Error.Message, "--movement 15") {
		t.Errorf("a mensagem deveria citar um --movement concreto: %q", env.Error.Message)
	}
	data, _ := env.Data.(map[string]any)
	options, _ := data["options"].([]any)
	if len(options) != 2 {
		t.Fatalf("options=%d, quer 2: %+v", len(options), data)
	}
	primeira, _ := options[0].(map[string]any)
	if primeira["movement"].(float64) != 15 || primeira["assignee"] != "João Silva (jsilva)" ||
		primeira["stateName"] != "Corrigir Integração" {
		t.Errorf("options[0] inesperada: %+v", primeira)
	}
	// A tarefa TRANSFERRED do pool não tem responsável: o status resume as duas.
	if st, _ := primeira["status"].(string); !strings.Contains(st, "TRANSFERRED") || !strings.Contains(st, "NOT_COMPLETED") {
		t.Errorf("status deveria resumir as tarefas do movimento: %q", st)
	}
}

// Solicitação sem tarefa em aberto continua sendo erro de uso claro.
func TestRequestMoveSemTarefaAberta(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196529", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitUsage, stdout)
	}
}

func TestDedupStepsByMovement(t *testing.T) {
	steps := []fluig.RequestStep{
		{Movement: 15, StateName: "Corrigir Integração"},
		{Movement: 15, StateName: "Corrigir Integração"},
		{Movement: 16, StateName: "Aprovar Diretoria"},
		{Movement: 15, StateName: "Corrigir Integração"},
	}
	got := dedupStepsByMovement(steps)
	if len(got) != 2 || got[0].Movement != 15 || got[1].Movement != 16 {
		t.Errorf("dedup preservando a ordem do servidor: %+v", got)
	}
	if got := dedupStepsByMovement(nil); len(got) != 0 {
		t.Errorf("nil não deveria virar entrada: %+v", got)
	}
}

// Timeout no move (ROADMAP §2.10-B): o cliente desiste, mas o servidor pode ter
// concluído. A CLI relê o estado e dá o veredicto, sempre com exit 5 e código
// TIMEOUT. O --timeout explícito também prova que o piso de escrita não o
// sobrepõe (senão o teste esperaria 2 min).
func TestRequestMoveTimeout(t *testing.T) {
	cases := []struct {
		nome        string
		id          string
		movement    string
		wantOutcome string
		wantNaSaida []string
	}{
		{
			nome: "tarefa alvo continua em aberto → not_moved",
			id:   "196526", movement: "4", wantOutcome: "not_moved",
			wantNaSaida: []string{"continua em aberto", "--timeout"},
		},
		{
			nome: "tarefa alvo não está mais em aberto → moved",
			id:   "196526", movement: "99", wantOutcome: "moved",
			wantNaSaida: []string{"não está mais em aberto", "NÃO repita"},
		},
		{
			nome: "releitura falha → unknown",
			id:   "999999", movement: "7", wantOutcome: "unknown",
			wantNaSaida: []string{"request show 999999"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			stub := &requestStub{writeDelay: 300 * time.Millisecond}
			proj := requestProject(t, stub.server(t).URL)
			code, stdout := runMain(t, "request", "move", tc.id, "--movement", tc.movement,
				"--timeout", "80ms", "--json", "--project", proj, "--server", "homolog")
			if code != output.ExitServer {
				t.Fatalf("exit=%d, quer %d (timeout é erro de servidor)\n%s", code, output.ExitServer, stdout)
			}
			var env output.Envelope
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("envelope inválido: %v\n%s", err, stdout)
			}
			if env.OK || env.Error == nil || env.Error.Code != output.CodeTimeout {
				t.Fatalf("envelope deveria falhar com %s: %+v", output.CodeTimeout, env)
			}
			data, _ := env.Data.(map[string]any)
			if data == nil {
				t.Fatalf("o envelope de timeout precisa levar o estado consultado: %s", stdout)
			}
			if data["outcome"] != tc.wantOutcome {
				t.Errorf("outcome=%v, quer %q", data["outcome"], tc.wantOutcome)
			}
			if data["requestId"].(float64) != float64(mustAtoi(t, tc.id)) {
				t.Errorf("requestId no envelope: %v", data["requestId"])
			}
			for _, want := range tc.wantNaSaida {
				if !strings.Contains(env.Error.Message, want) {
					t.Errorf("mensagem sem %q: %s", want, env.Error.Message)
				}
			}
		})
	}
}

// Modo humano do timeout do move: o veredicto e a orientação saem legíveis,
// sem envelope.
func TestRequestMoveTimeoutModoHumano(t *testing.T) {
	stub := &requestStub{writeDelay: 300 * time.Millisecond}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196526", "--movement", "4",
		"--timeout", "80ms", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	for _, want := range []string{"movimento 4 continua em aberto", "repita com um tempo limite maior"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída humana sem %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, `{"ok"`) {
		t.Errorf("modo humano não deve emitir envelope:\n%s", stdout)
	}
}

// Timeout no start: sem número de solicitação para conferir, a CLI entrega o
// comando de verificação em vez de arriscar um veredicto.
func TestRequestStartTimeout(t *testing.T) {
	stub := &requestStub{writeDelay: 300 * time.Millisecond}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "start", "contratos_taxa_limpeza",
		"--field", "nome=teste", "--timeout", "80ms", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != output.CodeTimeout {
		t.Fatalf("esperava %s: %+v", output.CodeTimeout, env)
	}
	data, _ := env.Data.(map[string]any)
	check, _ := data["checkCommand"].(string)
	// A dica precisa citar flags que existem de verdade (é --process, não
	// --process-id).
	for _, want := range []string{"request list", "--process contratos_taxa_limpeza", "--requester u", "--status open"} {
		if !strings.Contains(check, want) {
			t.Errorf("checkCommand sem %q: %q", want, check)
		}
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
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
