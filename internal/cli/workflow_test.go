package cli

import (
	"encoding/json"
	"fmt"
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
// eventos, a listagem de processos e o ciclo de publish (REST v2).
type workflowStub struct {
	helperInstalled bool
	version         int

	pubVersion   int    // última versão REST (import incrementa; default 3)
	importedXML  []byte // corpo do import/xml
	releaseCalls int
	releaseFail  bool
}

func (s *workflowStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	// SOAP nativo: getWorkFlowProcessVersion → versão; exportProcess → zip do
	// processo (reusa a fixture comprasProcessXML do diff; Fantasma = vazio).
	mux.HandleFunc("/webdesk/ECMWorkflowEngineService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		switch r.Header.Get("SOAPAction") {
		case "getWorkFlowProcessVersion":
			io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
				`<ns2:getWorkFlowProcessVersionResponse xmlns:ns2="http://ws.workflow.ecm.technology.totvs.com/">`+
				`<result>`+strconv.Itoa(s.version)+`</result></ns2:getWorkFlowProcessVersionResponse></soap:Body></soap:Envelope>`)
		case "exportProcess":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), ">Fantasma<") {
				// Processo inexistente: falha real da homologação (resultado nulo).
				io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
					`<soap:Fault><faultcode>soap:Server</faultcode>`+
					`<faultstring>Cannot write part result. RPC/Literal parts cannot be null. (WS-I BP R2211)</faultstring>`+
					`</soap:Fault></soap:Body></soap:Envelope>`)
				return
			}
			io.WriteString(w, wfExportEnvelope(processZipBase64(t, comprasProcessXML)))
		default:
			http.Error(w, "op?", 500)
		}
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !s.helperInstalled {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/workflows/", func(w http.ResponseWriter, r *http.Request) {
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
	// Ciclo de publish (REST v2): export/import/versions/release por processo.
	if s.pubVersion == 0 {
		s.pubVersion = 3
	}
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/export/xml"):
			if strings.Contains(path, "Fantasma") {
				http.Error(w, `{"code":"NotFound","message":"processo inexistente"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/xml;charset=UTF-8")
			// Fixture real da homologação: evento beforeTaskSave com /* v1 fluigcli */.
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export.xml"))
			if err != nil {
				t.Fatal(err)
			}
			w.Write(b)
		case strings.HasSuffix(path, "/import/xml"):
			s.importedXML, _ = io.ReadAll(r.Body)
			s.pubVersion++
			io.WriteString(w, `{"processId":"Compras","versions":null}`)
		case strings.HasSuffix(path, "/process-versions/latest/release"):
			if s.releaseFail {
				http.Error(w, `{"code":"BPMProcessDefinitionVersionOnReleaseException","message":"Versão do processo contém erros"}`, http.StatusBadRequest)
				return
			}
			s.releaseCalls++
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "/process-versions"):
			fmt.Fprintf(w, `{"items":[{"version":%d,"active":true,"editing":true}],"hasNext":false}`, s.pubVersion)
		default:
			http.NotFound(w, r)
		}
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

// publish: aplica o script local, importa (nova versão) e libera.
func TestWorkflowPublish(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Compras.beforeTaskSave.js"),
		[]byte("function beforeTaskSave(){ /* publicado */ }"), 0o644)

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["previousVersion"].(float64) != 3 || data["version"].(float64) != 4 || data["released"] != true {
		t.Errorf("resultado inesperado: %+v", data)
	}
	events, _ := data["events"].([]any)
	if len(events) != 1 || events[0] != "beforeTaskSave" {
		t.Errorf("eventos inesperados: %v", events)
	}
	if !strings.Contains(string(stub.importedXML), "function beforeTaskSave(){ /* publicado */ }") {
		t.Error("XML importado não contém o script local")
	}
	if strings.Contains(string(stub.importedXML), "/* v1 fluigcli */") {
		t.Error("XML importado ainda tem o script antigo")
	}
	if stub.releaseCalls != 1 {
		t.Errorf("release chamado %d vezes, quer 1", stub.releaseCalls)
	}
}

// --no-release: cria a versão em edição e não chama o release.
func TestWorkflowPublishNoRelease(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Compras.beforeTaskSave.js"), []byte("function beforeTaskSave(){}"), 0o644)

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--no-release",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["released"] != false || stub.releaseCalls != 0 {
		t.Errorf("released=%v releaseCalls=%d", data["released"], stub.releaseCalls)
	}
}

// Script local de evento que não existe no processo: falha ANTES do import.
func TestWorkflowPublishEventoInexistente(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Compras.naoTem.js"), []byte("function naoTem(){}"), 0o644)

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitNotFound, stdout)
	}
	if stub.importedXML != nil {
		t.Error("import não deveria ter acontecido")
	}
	if stub.releaseCalls != 0 {
		t.Error("release não deveria ter acontecido")
	}
}

// Falha na liberação: a versão fica criada e a mensagem explica isso (exit 5).
func TestWorkflowPublishReleaseFalha(t *testing.T) {
	stub := &workflowStub{releaseFail: true}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Compras.beforeTaskSave.js"), []byte("function beforeTaskSave(){}"), 0o644)

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "foi criada, mas não pôde ser liberada") {
		t.Errorf("mensagem deveria avisar que a versão foi criada: %+v", env.Error)
	}
}

// Sem script local do processo: erro de uso (exit 2), sem tocar no servidor.
func TestWorkflowPublishSemScripts(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	code, _ := runMain(t, "workflow", "publish", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
	if stub.importedXML != nil {
		t.Error("import não deveria ter acontecido")
	}
}

// Processo inexistente no servidor: exit 4.
func TestWorkflowPublishProcessoInexistente(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Fantasma.beforeTaskSave.js"), []byte("function beforeTaskSave(){}"), 0o644)

	code, _ := runMain(t, "workflow", "publish", "Fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
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

// import: baixa os scripts do processo para workflow/scripts/<pid>.<evento>.js.
func TestWorkflowImportCriaArquivos(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "workflow", "import", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("esperava 3 scripts importados, veio %d\n%s", len(results), stdout)
	}
	first, _ := results[0].(map[string]any)
	if first["id"] != "Compras.afterProcessFinish" || first["action"] != "created" {
		t.Errorf("results[0] inesperado: %+v", first)
	}
	got, err := os.ReadFile(filepath.Join(proj, "workflow", "scripts", "Compras.beforeTaskSave.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "function beforeTaskSave(){ /* codigo A */ }" {
		t.Errorf("conteúdo inesperado: %q", got)
	}
	for _, name := range []string{"Compras.afterProcessFinish.js", "Compras.validateForm.js"} {
		if _, err := os.Stat(filepath.Join(proj, "workflow", "scripts", name)); err != nil {
			t.Errorf("arquivo %s não foi criado", name)
		}
	}
}

// import: script local existente (mesmo em subpasta) é sobrescrito no lugar.
func TestWorkflowImportSobrescreveNoLugar(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	sub := filepath.Join(proj, "workflow", "scripts", "compras")
	os.MkdirAll(sub, 0o755)
	existing := filepath.Join(sub, "Compras.beforeTaskSave.js")
	os.WriteFile(existing, []byte("function beforeTaskSave(){ /* local antigo */ }"), 0o644)

	code, stdout := runMain(t, "workflow", "import", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "function beforeTaskSave(){ /* codigo A */ }" {
		t.Errorf("script na subpasta não foi sobrescrito: %q", got)
	}
	if _, err := os.Stat(filepath.Join(proj, "workflow", "scripts", "Compras.beforeTaskSave.js")); err == nil {
		t.Error("não deveria criar duplicata no caminho default quando o script já existe em subpasta")
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	for _, r := range results {
		m, _ := r.(map[string]any)
		if m["id"] == "Compras.beforeTaskSave" && m["action"] != "updated" {
			t.Errorf("beforeTaskSave deveria ser updated, veio %v", m["action"])
		}
	}
}

// import --all: importa os scripts de todos os processos do servidor.
func TestWorkflowImportAll(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "workflow", "import", "--all", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	if len(results) != 9 { // 3 processos × 3 eventos (o stub exporta o mesmo XML)
		t.Fatalf("esperava 9 scripts importados, veio %d\n%s", len(results), stdout)
	}
	for _, name := range []string{"Compras.beforeTaskSave.js", "AprovacaoContrato.beforeTaskSave.js", "FLUIGADHOCPROCESS.validateForm.js"} {
		if _, err := os.Stat(filepath.Join(proj, "workflow", "scripts", name)); err != nil {
			t.Errorf("arquivo %s não foi criado", name)
		}
	}
}

// import de processo inexistente: exit 4, nada gravado.
func TestWorkflowImportProcessoInexistente(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)

	code, _ := runMain(t, "workflow", "import", "Fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
	if _, err := os.Stat(filepath.Join(proj, "workflow", "scripts")); err == nil {
		t.Error("nenhum arquivo deveria ter sido criado")
	}
}

// import em lote com uma falha: exit 6 (parcial) e os demais importados.
func TestWorkflowImportParcial(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "workflow", "import", "Compras", "Fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitPartial, stdout)
	}
	if _, err := os.Stat(filepath.Join(proj, "workflow", "scripts", "Compras.beforeTaskSave.js")); err != nil {
		t.Error("Compras deveria ter sido importado mesmo com a falha do Fantasma")
	}
}

// import sem argumentos e sem --all: erro de uso (exit 2).
func TestWorkflowImportSemArgs(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	code, _ := runMain(t, "workflow", "import", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
}

// resolveWorkflowTargets: --process-id (override) desacopla a identidade local
// da do servidor. A busca dos arquivos continua pelo prefixo do argumento; só
// o processId devolvido ao servidor muda (ROADMAP 1.7-A).
func TestResolveWorkflowTargetsProcessIDOverride(t *testing.T) {
	const serverPID = "Adiantamento ao Fornecedor"
	root := t.TempDir()
	dir := filepath.Join(root, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "SolicitacaoAdiantamento.servicetask88.js")
	os.WriteFile(file, []byte("function servicetask88(){}"), 0o644)

	// Caso arquivo: o alvo é o .js local; a flag troca só o pid do servidor.
	pid, scripts, err := resolveWorkflowTargets(root, file, nil, false, serverPID)
	if err != nil {
		t.Fatalf("caso arquivo: %v", err)
	}
	if pid != serverPID {
		t.Errorf("caso arquivo: pid do servidor = %q, quer %q", pid, serverPID)
	}
	if len(scripts) != 1 || scripts[0].Event != "servicetask88" || scripts[0].Path != file {
		t.Errorf("caso arquivo: script inesperado: %+v", scripts)
	}

	// Caso prefixo + --events: FindProcessScripts usa o prefixo local, não o pid.
	pid, scripts, err = resolveWorkflowTargets(root, "SolicitacaoAdiantamento", []string{"servicetask88"}, false, serverPID)
	if err != nil {
		t.Fatalf("caso prefixo: %v", err)
	}
	if pid != serverPID {
		t.Errorf("caso prefixo: pid do servidor = %q, quer %q", pid, serverPID)
	}
	if len(scripts) != 1 || scripts[0].Path != file {
		t.Errorf("caso prefixo: script inesperado: %+v", scripts)
	}

	// Sem override, o pid do servidor volta a ser o prefixo local.
	pid, _, err = resolveWorkflowTargets(root, file, nil, false, "")
	if err != nil {
		t.Fatalf("sem override: %v", err)
	}
	if pid != "SolicitacaoAdiantamento" {
		t.Errorf("sem override: pid = %q, quer o prefixo local", pid)
	}
}

// export --process-id: o arquivo local tem prefixo X, mas o processo publicado
// no servidor é Y. O envelope reporta o pid do servidor (Y).
func TestWorkflowExportProcessIDOverride(t *testing.T) {
	stub := &workflowStub{version: 5, helperInstalled: true}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "SolicitacaoAdiantamento.beforeTaskSave.js")
	os.WriteFile(file, []byte("function beforeTaskSave(){}"), 0o644)

	code, stdout := runMain(t, "workflow", "export", file,
		"--process-id", "Adiantamento ao Fornecedor",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["processId"] != "Adiantamento ao Fornecedor" {
		t.Errorf("processId do servidor inesperado: %+v", data["processId"])
	}
	if data["updated"].(float64) != 1 {
		t.Errorf("updated = %v, quer 1", data["updated"])
	}
}

// publish --process-id: scripts locais com prefixo X publicam no processo Y.
func TestWorkflowPublishProcessIDOverride(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "workflow", "scripts")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SolicitacaoAdiantamento.beforeTaskSave.js"),
		[]byte("function beforeTaskSave(){ /* override */ }"), 0o644)

	code, stdout := runMain(t, "workflow", "publish", "SolicitacaoAdiantamento",
		"--process-id", "Compras",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["processId"] != "Compras" {
		t.Errorf("processId do servidor inesperado: %+v", data["processId"])
	}
	if data["version"].(float64) != 4 || data["released"] != true {
		t.Errorf("resultado inesperado: %+v", data)
	}
	if !strings.Contains(string(stub.importedXML), "/* override */") {
		t.Error("XML importado não contém o script local")
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
