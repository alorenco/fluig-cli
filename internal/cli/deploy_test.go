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

// deployStub serve os endpoints dos tipos de passo do plano (§3.11).
type deployStub struct {
	mu sync.Mutex

	datasetsEditados []string
	datasetsCriados  []string
	eventosSalvos    int
	mecanismosSalvos int
	widgetsEnviados  []string
	sqlExecutado     []string

	processoImportado bool
	processoLiberado  bool

	// layouts responde o GET de layout por código (colisão do §3.1).
	layouts map[string]string
	// falhaDataset faz o editDataset recusar o dataset citado.
	falhaDataset string
}

func (s *deployStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})

	// dataset
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/loadDataset", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") == "ds_existente" {
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "loadDataset.json"))
			if err != nil {
				t.Fatal(err)
			}
			w.Write(b)
			return
		}
		w.WriteHeader(http.StatusInternalServerError) // quirk: inexistente = 500
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/editDataset", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.falhaDataset != "" {
			io.WriteString(w, `{"message":{"message":"Não foi possível compilar os scripts para customização Model."}}`)
			return
		}
		s.datasetsEditados = append(s.datasetsEditados, "ds_existente")
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/createDataset", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DatasetPK struct {
				DatasetID string `json:"datasetId"`
			} `json:"datasetPK"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		s.mu.Lock()
		s.datasetsCriados = append(s.datasetsCriados, body.DatasetPK.DatasetID)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})

	// evento global
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/getEventList", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":[{"globalEventPK":{"companyId":1,"eventId":"displayCustomThemes"},"eventDescription":"function displayCustomThemes(){}"}]}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/saveEventList", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.eventosSalvos++
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})

	// mecanismo
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/getCustomAttributionMechanismList", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":[]}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/createAttributionMechanism", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.mecanismosSalvos++
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})

	// widget: layouts (colisão) + upload
	mux.HandleFunc("/page-management/api/v2/layouts/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/page-management/api/v2/layouts/")
		if title, ok := s.layouts[code]; ok {
			io.WriteString(w, `{"id":1,"code":"`+code+`","title":"`+title+`"}`)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/page-management/api/v2/layouts", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[],"hasNext":false}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/product/uploadfile", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(20 << 20)
		s.mu.Lock()
		s.widgetsEnviados = append(s.widgetsEnviados, r.FormValue("fileName"))
		s.mu.Unlock()
		io.WriteString(w, `{}`)
	})

	// processo (publish): tudo pela REST v2 — export/xml, import/xml,
	// process-versions e release (o SOAP não entra neste caminho).
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/export/xml"):
			if !strings.Contains(path, "Compras") {
				http.Error(w, `{"code":"NotFound","message":"processo inexistente"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/xml;charset=UTF-8")
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export.xml"))
			if err != nil {
				t.Fatal(err)
			}
			w.Write(b)
		case strings.HasSuffix(path, "/import/xml"):
			s.mu.Lock()
			s.processoImportado = true
			s.mu.Unlock()
			io.WriteString(w, `{"processId":"Compras","versions":null}`)
		case strings.HasSuffix(path, "/process-versions/latest/release"):
			s.mu.Lock()
			s.processoLiberado = true
			s.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "/process-versions"):
			s.mu.Lock()
			v := 3
			if s.processoImportado {
				v = 4
			}
			s.mu.Unlock()
			io.WriteString(w, `{"items":[{"version":`+strconv.Itoa(v)+`,"active":true}],"hasNext":false}`)
		default:
			http.NotFound(w, r)
		}
	})

	// db (helper)
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.6.0"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/db/query", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SQL string `json:"sql"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &req)
		s.mu.Lock()
		s.sqlExecutado = append(s.sqlExecutado, req.SQL)
		s.mu.Unlock()
		if strings.Contains(req.SQL, "quebra") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "Invalid object name 'quebra'")
			return
		}
		io.WriteString(w, `{"columns":[{"name":"n","type":"int"}],"rows":[["1"]],"rowCount":1,"truncated":false}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// deployProject monta o projeto: servidor cadastrado + artefatos locais.
func deployProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	srv := config.Server{Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(srv, false); err != nil {
		t.Fatal(err)
	}
	escreve(t, proj, "datasets/ds_existente.js", dsLimpo)
	escreve(t, proj, "datasets/ds_novo.js", dsLimpo)
	escreve(t, proj, "events/meuEvento.js", "function meuEvento(){}")
	escreve(t, proj, "mechanisms/mec_novo.js", "function getUsers(){ return []; }")
	escreve(t, proj, "sql/diag.sql", "select 1 as n;\nselect 2 as n;\n")
	escreve(t, proj, "workflow/scripts/Compras.beforeTaskSave.js",
		"function beforeTaskSave(){ /* do plano */ }")
	escreve(t, proj, "wcm/widget/meu_painel/src/main/webapp/WEB-INF/application.xml", "<application/>")
	escreve(t, proj, "wcm/widget/meu_painel/src/main/resources/application.info", "code=meu_painel")
	return proj
}

// plano grava o manifesto e devolve o caminho.
func plano(t *testing.T, proj, conteudo string) string {
	t.Helper()
	return escreve(t, proj, "release.json", conteudo)
}

// O plano roda na ORDEM do arquivo, um passo por vez, e o relatório traz cada um.
func TestDeployExecutaNaOrdem(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{
	  "steps": [
	    {"name": "diagnóstico", "db": "sql/diag.sql"},
	    {"dataset": "datasets/ds_novo.js", "new": true},
	    {"dataset": "datasets/ds_existente.js"},
	    {"event": "events/meuEvento.js"},
	    {"mechanism": "mechanisms/mec_novo.js"},
	    {"widget": "meu_painel"}
	  ]
	}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	steps, _ := data["steps"].([]any)
	if len(steps) != 6 {
		t.Fatalf("esperava 6 passos no relatório, veio %d", len(steps))
	}
	for i, raw := range steps {
		s, _ := raw.(map[string]any)
		if s["status"] != deployOK {
			t.Errorf("passo %d não concluiu: %v", i+1, s)
		}
		if s["index"].(float64) != float64(i+1) {
			t.Errorf("índice fora de ordem no passo %d: %v", i+1, s["index"])
		}
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.sqlExecutado) != 2 {
		t.Errorf("esperava 2 instruções SQL, veio %v", stub.sqlExecutado)
	}
	if len(stub.datasetsCriados) != 1 || stub.datasetsCriados[0] != "ds_novo" {
		t.Errorf("dataset novo não criado: %v", stub.datasetsCriados)
	}
	if len(stub.datasetsEditados) != 1 {
		t.Errorf("dataset existente não atualizado: %v", stub.datasetsEditados)
	}
	if stub.eventosSalvos != 1 || stub.mecanismosSalvos != 1 {
		t.Errorf("evento/mecanismo não publicados: %d/%d", stub.eventosSalvos, stub.mecanismosSalvos)
	}
	if len(stub.widgetsEnviados) != 1 || stub.widgetsEnviados[0] != "meu_painel.war" {
		t.Errorf("widget não enviada: %v", stub.widgetsEnviados)
	}
}

// Erro no meio: o plano PARA, e os passos seguintes saem como skipped — é o que
// permite ver onde o release parou e retomar.
func TestDeployParaNoPrimeiroErro(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	escreve(t, proj, "sql/quebra.sql", "select * from quebra;\n")
	p := plano(t, proj, `{
	  "steps": [
	    {"dataset": "datasets/ds_existente.js"},
	    {"db": "sql/quebra.sql"},
	    {"widget": "meu_painel"}
	  ]
	}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d (parcial); stdout=%s", code, output.ExitPartial, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	steps, _ := data["steps"].([]any)
	got := make([]string, 0, 3)
	for _, raw := range steps {
		s, _ := raw.(map[string]any)
		got = append(got, s["status"].(string))
	}
	quer := []string{deployOK, deployFailed, deploySkipped}
	for i := range quer {
		if got[i] != quer[i] {
			t.Errorf("passo %d = %q, quer %q (%v)", i+1, got[i], quer[i], got)
		}
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.widgetsEnviados) != 0 {
		t.Errorf("a widget foi publicada depois da falha: %v", stub.widgetsEnviados)
	}
}

// --from retoma: os passos anteriores não são executados de novo.
func TestDeployFromRetoma(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{
	  "steps": [
	    {"dataset": "datasets/ds_existente.js"},
	    {"widget": "meu_painel"}
	  ]
	}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--from", "2", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.datasetsEditados) != 0 {
		t.Errorf("--from 2 executou o passo 1: %v", stub.datasetsEditados)
	}
	if len(stub.widgetsEnviados) != 1 {
		t.Errorf("--from 2 não executou o passo 2: %v", stub.widgetsEnviados)
	}
}

// --dry-run não escreve NADA e diz o que cada passo faria.
func TestDeployDryRun(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{
	  "steps": [
	    {"dataset": "datasets/ds_novo.js", "new": true},
	    {"dataset": "datasets/ds_existente.js"},
	    {"db": "sql/diag.sql"},
	    {"widget": "meu_painel"}
	  ]
	}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["dryRun"] != true {
		t.Errorf("dryRun devia ser true: %v", data["dryRun"])
	}
	steps, _ := data["steps"].([]any)
	for i, raw := range steps {
		s, _ := raw.(map[string]any)
		if s["status"] != deployValidated {
			t.Errorf("passo %d não validado: %v", i+1, s)
		}
		if s["action"] == nil || s["action"] == "" {
			t.Errorf("passo %d sem a ação prevista: %v", i+1, s)
		}
	}
	// O passo 1 é criação e o 2 é atualização: o dry-run distingue os dois.
	primeiro, _ := steps[0].(map[string]any)
	segundo, _ := steps[1].(map[string]any)
	if !strings.Contains(primeiro["action"].(string), "criaria") {
		t.Errorf("passo 1 devia prever criação: %v", primeiro["action"])
	}
	if !strings.Contains(segundo["action"].(string), "atualizaria") {
		t.Errorf("passo 2 devia prever atualização: %v", segundo["action"])
	}

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.datasetsCriados)+len(stub.datasetsEditados)+len(stub.widgetsEnviados)+
		stub.eventosSalvos+stub.mecanismosSalvos+len(stub.sqlExecutado) != 0 {
		t.Errorf("--dry-run escreveu no servidor: %+v", stub)
	}
}

// --dry-run reprova o plano com arquivo faltando, e ainda não escreve nada.
func TestDeployDryRunAcusaArquivoFaltando(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"steps": [{"dataset": "datasets/nao_existe.js"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
	}
	if !strings.Contains(stdout, "não encontrado") {
		t.Errorf("mensagem sem o motivo: %s", stdout)
	}
}

// A guarda de colisão do §3.1 vale dentro do plano: a widget não pode
// sobrescrever um layout só porque o deploy é automatizado.
func TestDeployRespeitaColisaoDeLayout(t *testing.T) {
	stub := &deployStub{layouts: map[string]string{"meu_painel": "Painel Layout"}}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"steps": [{"widget": "meu_painel"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d (colisão); stdout=%s", code, output.ExitUsage, stdout)
	}
	if !strings.Contains(stdout, "LAYOUT") {
		t.Errorf("mensagem sem a explicação da colisão: %s", stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.widgetsEnviados) != 0 {
		t.Errorf("a widget foi enviada apesar da colisão: %v", stub.widgetsEnviados)
	}
}

// O audit do §3.2/§3.13 também vale no plano, e barra ANTES de qualquer escrita.
func TestDeployAuditaAntesDeComecar(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	escreve(t, proj, "datasets/ds_ruim.js", dsConstEmLaco)
	p := plano(t, proj, `{
	  "steps": [
	    {"dataset": "datasets/ds_existente.js"},
	    {"dataset": "datasets/ds_ruim.js", "new": true}
	  ]
	}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d (audit); stdout=%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || env.Error.Code != output.CodeAuditFailed {
		t.Fatalf("esperava %s, veio %+v", output.CodeAuditFailed, env.Error)
	}
	if !strings.Contains(env.Error.Message, "nada foi publicado") {
		t.Errorf("a mensagem não deixa claro que nada rodou: %q", env.Error.Message)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.datasetsEditados) != 0 {
		t.Errorf("o primeiro passo rodou apesar do erro de audit no segundo: %v", stub.datasetsEditados)
	}
}

// Erros de plano: nada é executado e a mensagem aponta o passo.
func TestDeployPlanoInvalido(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)

	casos := []struct {
		nome, conteudo, quer string
	}{
		{"sem passos", `{"steps": []}`, "não tem passos"},
		{"passo sem tipo", `{"steps": [{"name": "x"}]}`, "passo sem tipo"},
		{"dois tipos no passo", `{"steps": [{"dataset": "a.js", "widget": "w"}]}`, "mais de um tipo"},
		{"chave desconhecida", `{"steps": [{"datasets": "a.js"}]}`, "inválido"},
		{"json quebrado", `{"steps": [`, "inválido"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			p := plano(t, proj, c.conteudo)
			code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
			if code != output.ExitUsage {
				t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
			}
			if !strings.Contains(stdout, c.quer) {
				t.Errorf("mensagem sem %q: %s", c.quer, stdout)
			}
		})
	}
}

// Plano inexistente → NOT_FOUND (exit 4), não erro genérico.
func TestDeployPlanoInexistente(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)

	code, _ := runMain(t, "deploy", "--plan", filepath.Join(proj, "nao_existe.json"),
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// O servidor do plano vale quando --server não é informado.
func TestDeployUsaServidorDoPlano(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"server": "homolog", "steps": [{"dataset": "datasets/ds_existente.js"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, `"server":"homolog"`) {
		t.Errorf("o envelope não registrou o servidor do plano: %s", stdout)
	}
}

// --- passo workflow no plano (§3.14) ---

// O passo publica uma versão nova e libera, reaproveitando a MESMA sequência do
// `workflow publish`.
func TestDeployPassoWorkflowPublica(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"steps": [{"workflow": "Compras"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if !stub.processoImportado {
		t.Error("o processo não foi importado")
	}
	if !stub.processoLiberado {
		t.Error("a versão nova não foi liberada (o default do plano é liberar)")
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	steps, _ := data["steps"].([]any)
	primeiro, _ := steps[0].(map[string]any)
	if !strings.Contains(primeiro["action"].(string), "criada e liberada") {
		t.Errorf("ação inesperada: %v", primeiro["action"])
	}
}

// "noRelease": true cria a versão em edição, sem liberar. A chave é negativa
// porque bool em JSON tem default false.
func TestDeployPassoWorkflowNoRelease(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"steps": [{"workflow": "Compras", "noRelease": true}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if !stub.processoImportado {
		t.Error("o processo não foi importado")
	}
	if stub.processoLiberado {
		t.Error("a versão foi liberada apesar de noRelease")
	}
}

// O maior ganho do item: o --dry-run detecta evento local que NÃO existe no
// processo, SEM publicar nada. Hoje esse erro só aparece no meio do publish.
func TestDeployDryRunAcusaEventoInexistente(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	escreve(t, proj, "workflow/scripts/Compras.eventoQueNaoExiste.js", "function eventoQueNaoExiste(){}")
	p := plano(t, proj, `{"steps": [{"workflow": "Compras"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
	}
	if !strings.Contains(stdout, "eventoQueNaoExiste") || !strings.Contains(stdout, "não existem no processo") {
		t.Errorf("o dry-run não acusou o evento inexistente: %s", stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.processoImportado {
		t.Error("--dry-run importou o processo")
	}
}

// O dry-run do caminho feliz diz quantos eventos a versão levaria, sem publicar.
func TestDeployDryRunWorkflowPrevêEventos(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"steps": [{"workflow": "Compras"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	steps, _ := data["steps"].([]any)
	primeiro, _ := steps[0].(map[string]any)
	acao, _ := primeiro["action"].(string)
	if !strings.Contains(acao, "criaria uma versão nova") || !strings.Contains(acao, "beforeTaskSave") {
		t.Errorf("ação prevista inesperada: %q", acao)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.processoImportado {
		t.Error("--dry-run importou o processo")
	}
}

// "processId" aponta outro processo no servidor (o prefixo local continua
// identificando os arquivos).
func TestDeployPassoWorkflowProcessIDDiferente(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	escreve(t, proj, "workflow/scripts/LocalPrefixo.beforeTaskSave.js", "function beforeTaskSave(){}")
	p := plano(t, proj, `{"steps": [{"workflow": "LocalPrefixo", "processId": "Compras"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if !stub.processoImportado {
		t.Error("o processo do processId não foi importado")
	}
}

// Sem script local para o prefixo: NOT_FOUND, e nada é publicado.
func TestDeployPassoWorkflowSemScripts(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	p := plano(t, proj, `{"steps": [{"workflow": "NaoTemScript"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitNotFound, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.processoImportado {
		t.Error("importou apesar de não haver script local")
	}
}

// Os scripts do processo também passam pelo audit do plano, antes de conectar.
func TestDeployAuditaScriptsDeProcesso(t *testing.T) {
	stub := &deployStub{}
	proj := deployProject(t, stub.server(t).URL)
	escreve(t, proj, "workflow/scripts/Compras.beforeTaskSave.js", dsConstEmLaco)
	p := plano(t, proj, `{"steps": [{"workflow": "Compras"}]}`)

	code, stdout := runMain(t, "deploy", "--plan", p, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d (audit); stdout=%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || env.Error.Code != output.CodeAuditFailed {
		t.Errorf("esperava %s, veio %+v", output.CodeAuditFailed, env.Error)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.processoImportado {
		t.Error("publicou apesar do erro de audit")
	}
}
