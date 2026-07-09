package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

type formStub struct {
	mu         sync.Mutex
	createBody string
	updateBody string
}

func (s *formStub) server(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc-real-123"}}`)
	})
	mux.HandleFunc("/webdesk/ECMCardIndexService", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		switch r.Header.Get("SOAPAction") {
		case "getCardIndexesWithoutApprover":
			w.Write(readTD("soap_listForms.xml"))
		case "getAttachmentsList":
			w.Write(readTD("soap_attachmentsList.xml"))
		case "getCardIndexContent":
			w.Write(readTD("soap_cardContent.xml"))
		case "getCustomizationEvents":
			w.Write(readTD("soap_customEvents.xml"))
		case "createSimpleCardIndexWithDatasetPersisteType":
			s.mu.Lock()
			s.createBody = string(body)
			s.mu.Unlock()
			w.Write(readTD("soap_writeForm.xml"))
		case "updateSimpleCardIndexWithDatasetAndGeneralInfo":
			s.mu.Lock()
			s.updateBody = string(body)
			s.mu.Unlock()
			w.Write(readTD("soap_writeForm.xml"))
		default:
			http.Error(w, "op?", 500)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func formProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "form-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", UserCode: "uc", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// Import baixa anexos (base64→binário) e eventos para forms/<desc>/{,events/}.
func TestFormImportWritesFilesAndEvents(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)

	code, _ := runMain(t, "form", "import", "42", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	formDir := filepath.Join(proj, "forms", "Formulario de Teste")

	html, err := os.ReadFile(filepath.Join(formDir, "Formulario de Teste.html"))
	if err != nil {
		t.Fatalf("html não gravado: %v", err)
	}
	if string(html) != "conteudo" {
		t.Errorf("conteúdo base64 não decodificado: %q", html)
	}
	if _, err := os.Stat(filepath.Join(formDir, "script.js")); err != nil {
		t.Errorf("anexo script.js não gravado: %v", err)
	}
	ev, err := os.ReadFile(filepath.Join(formDir, "events", "onNotify.js"))
	if err != nil {
		t.Fatalf("evento não gravado: %v", err)
	}
	if !strings.Contains(string(ev), "onNotify") {
		t.Errorf("evento com conteúdo errado: %q", ev)
	}
}

// Export de um formulário existente → update; anexo .html com nome do form = principal.
func TestFormExportUpdateMarksPrincipal(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)

	// A pasta precisa bater com o nome do form existente na fixture ("Formulario de Teste").
	formDir := filepath.Join(proj, "forms", "Formulario de Teste")
	os.MkdirAll(filepath.Join(formDir, "events"), 0o755)
	os.WriteFile(filepath.Join(formDir, "Formulario de Teste.html"), []byte("<html>x</html>"), 0o644)
	os.WriteFile(filepath.Join(formDir, "estilo.css"), []byte("body{}"), 0o644)
	os.WriteFile(filepath.Join(formDir, "events", "onLoad.js"), []byte("function onLoad(){}"), 0o644)

	code, _ := runMain(t, "form", "export", formDir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.updateBody == "" {
		t.Fatal("update não foi chamado (deveria, o form já existe)")
	}
	// O .html com o nome do formulário deve ir como principal=true; o .css não.
	if !strings.Contains(stub.updateBody, "<fileName>Formulario de Teste.html</fileName>") {
		t.Errorf("html não enviado no update")
	}
	if !strings.Contains(stub.updateBody, "<principal>true</principal>") {
		t.Errorf("html deveria ser principal=true")
	}
	// O evento deve estar presente.
	if !strings.Contains(stub.updateBody, "<eventId>onLoad</eventId>") {
		t.Errorf("evento onLoad não enviado no update")
	}
}

// Export de pasta nova → create exige --new + --parent-id + --dataset-name.
func TestFormExportCreateRequiresFlags(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	formDir := filepath.Join(proj, "forms", "Form Novo")
	os.MkdirAll(formDir, 0o755)
	os.WriteFile(filepath.Join(formDir, "Form Novo.html"), []byte("<html></html>"), 0o644)

	// Sem --new → recusa (exit 2).
	code, _ := runMain(t, "form", "export", formDir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --new deveria dar exit 2, veio %d", code)
	}

	// Com --new mas sem --parent-id → exit 2.
	code, _ = runMain(t, "form", "export", formDir, "--new", "--dataset-name", "ds_x", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --parent-id deveria dar exit 2, veio %d", code)
	}

	// Completo → cria.
	code, _ = runMain(t, "form", "export", formDir, "--new", "--parent-id", "10", "--dataset-name", "ds_x", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Errorf("com todas as flags deveria criar (exit 0), veio %d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if !strings.Contains(stub.createBody, "<principal>true</principal>") {
		t.Errorf("Form Novo.html deveria ser principal=true no create")
	}
	if !strings.Contains(stub.createBody, "<persistenceType>1</persistenceType>") {
		t.Errorf("persistenceType default (db=1) ausente")
	}
}

// Opção C: import com --folder grava o mapeamento; export de uma pasta com
// nome técnico (≠ nome no servidor) reencontra o formulário pelo mapeamento.
func TestFormMappingRoundTrip(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)

	// import documentId 42 ("Formulario de Teste") para uma pasta técnica.
	code, _ := runMain(t, "form", "import", "42", "--folder", "frm_tecnico",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("import exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, "forms", "frm_tecnico", "Formulario de Teste.html")); err != nil {
		t.Fatalf("arquivo não gravado na pasta técnica: %v", err)
	}
	// O mapeamento deve ter sido gravado.
	mapData, err := os.ReadFile(filepath.Join(proj, ".fluigcli", "forms.json"))
	if err != nil {
		t.Fatalf("forms.json não gravado: %v", err)
	}
	if !strings.Contains(string(mapData), "frm_tecnico") || !strings.Contains(string(mapData), "\"documentId\": 42") {
		t.Errorf("mapeamento incompleto:\n%s", mapData)
	}

	// export da pasta técnica: o basename ("frm_tecnico") não é o nome do form,
	// mas o mapeamento aponta para o documentId 42 → deve atualizar.
	code, _ = runMain(t, "form", "export", filepath.Join(proj, "forms", "frm_tecnico"),
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("export exit=%d", code)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.updateBody == "" {
		t.Error("export deveria ter atualizado (via mapeamento), mas update não foi chamado")
	}
	if stub.createBody != "" {
		t.Error("não deveria ter criado — o mapeamento aponta um form existente")
	}
}

// Segurança: um anexo com nome de path traversal vindo do servidor NÃO pode
// escrever fora do diretório do projeto (SafeJoin).
func TestFormImportRejectsTraversal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc"}}`)
	})
	mux.HandleFunc("/webdesk/ECMCardIndexService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		switch r.Header.Get("SOAPAction") {
		case "getCardIndexesWithoutApprover":
			io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
				`<r xmlns="http://ws.dm.ecm.technology.totvs.com/"><result><item>`+
				`<documentId>7</documentId><documentDescription>Malicioso</documentDescription>`+
				`<version>1</version></item></result></r></s:Body></s:Envelope>`)
		case "getAttachmentsList":
			io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
				`<r xmlns="http://ws.dm.ecm.technology.totvs.com/"><result>`+
				`<item>../../../../pwned.txt</item></result></r></s:Body></s:Envelope>`)
		default:
			io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body></s:Body></s:Envelope>`)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	proj := formProject(t, srv.URL)
	code, _ := runMain(t, "form", "import", "7", "--json", "--project", proj, "--server", "homolog")
	if code == output.ExitOK {
		t.Errorf("import com anexo traversal deveria falhar, veio exit 0")
	}
	// O arquivo NÃO pode existir fora do projeto.
	if _, err := os.Stat(filepath.Join(proj, "..", "..", "..", "..", "pwned.txt")); err == nil {
		t.Fatal("VULNERÁVEL: arquivo escrito fora do projeto por path traversal")
	}
}

func TestFormListJSON(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	forms, _ := data["forms"].([]any)
	if len(forms) != 1 {
		t.Errorf("esperava 1 form no envelope, veio %d", len(forms))
	}
}

// Modo humano: tabela com bordas (padrão de listas — ver CLAUDE.md).
func TestFormListTabela(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "ID", "Nome", "Dataset", "Formulario de Teste", "ds_teste"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}
