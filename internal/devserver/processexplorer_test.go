package devserver

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// processExplorerUpstream simula os endpoints do process-management que o
// explorador usa: sessão, lista de processos, export XML (a fixture real) e
// versões. ListForms (SOAP) não é servido de propósito — a resolução do nome
// do formulário é best-effort e não pode derrubar o detalhe.
func processExplorerUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export_full.xml"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[
			{"processId":"compras_entrada_documento","processDescription":"Entrada de Documento","active":true},
			{"processId":"rh_justificativa_ponto","processDescription":"Justificativa de Ponto","active":false}],
			"hasNext":false}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes/compras_entrada_documento/export/xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write(fixture)
	})
	mux.HandleFunc("/process-management/api/v2/processes/compras_entrada_documento/process-versions", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[{"version":24,"active":false},{"version":25,"active":true}],"hasNext":false}`)
	})
	// Processo inexistente no export → 404 (BPMWorkflowProcessNotFoundException).
	mux.HandleFunc("/process-management/api/v2/processes/nao_existe/export/xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"code":"BPMWorkflowProcessNotFoundException","message":"processo não encontrado"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newProcessExplorerServer monta o dev server com um root próprio que contém
// um script local de evento (para exercer a presença local) e um forms.json
// vinculando o formId 263801 a uma pasta (para o card do formulário).
func newProcessExplorerServer(t *testing.T, upstream *httptest.Server, withClient bool) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Só o beforeTaskSave existe localmente — os demais eventos ficam "só no servidor".
	write("workflow/scripts/compras_entrada_documento.beforeTaskSave.js", "function beforeTaskSave(){}\n")
	write(".fluigcli/forms.json", `{"version":"2.0.0","servers":{"testscope":[
		{"folder":"frm_entrada","documentId":263801,"name":"Entrada de Documento"}]}}`)

	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Root: root, Upstream: u, Port: 0, Debounce: 10 * time.Millisecond, FormScope: "testscope"}
	jar, _ := cookiejar.New(nil)
	opts.Jar = jar
	if withClient {
		client, err := fluig.NewClient(fluig.Options{BaseURL: upstream.URL, Username: "dev-" + t.Name(), Password: "p", CompanyID: 1})
		if err != nil {
			t.Fatal(err)
		}
		opts.Client = client
		opts.CompanyID = 1
	}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts
}

// A página do explorador é servida com os marcadores esperados.
func TestProcessExplorerPage(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), true)
	status, body := getBody(t, ts.URL+"/_dev/processes/")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{"fluigcli", "/_dev/api/process/list", "WKNumState", "Diagrama", "Quem atua"} {
		if !strings.Contains(body, want) {
			t.Errorf("página não contém %q", want)
		}
	}
}

// list devolve os processos.
func TestProcessExplorerList(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), true)
	status, body := getBody(t, ts.URL+"/_dev/api/process/list")
	if status != http.StatusOK || !strings.Contains(body, `"compras_entrada_documento"`) {
		t.Fatalf("list inesperado (%d): %s", status, body)
	}
}

// detail agrega etapas, atribuição, transições, versões, formulário (pasta
// local via forms.json) e presença local dos scripts.
func TestProcessExplorerDetail(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), true)
	status, body := getBody(t, ts.URL+"/_dev/api/process/detail?id=compras_entrada_documento")
	if status != http.StatusOK {
		t.Fatalf("detail status %d: %s", status, body)
	}
	for _, want := range []string{
		`"version":25`,
		`"formId":263801`,
		`"folder":"frm_entrada"`,     // vínculo local do forms.json
		`"sequence":17`,              // Faturar Documento
		`"faturista"`,                // atribuição parseada (Pool Papel)
		`"diretorAprovador"`,         // Campo Formulário
		`"aprNivel1"`,                // regra de gateway
		`"versions"`,                 // seletor de versão
		`"manager"`,                  // gestor do processo
		`"gestor_entrada_documento"`, // papel do gestor
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail sem %q", want)
		}
	}
	// beforeTaskSave existe localmente; servicetask19 não.
	if !strings.Contains(body, "workflow/scripts/compras_entrada_documento.beforeTaskSave.js") &&
		!strings.Contains(body, "workflow\\\\scripts\\\\compras_entrada_documento.beforeTaskSave.js") {
		t.Errorf("detail não trouxe o localPath do beforeTaskSave: %s", body)
	}
}

// detail de versão específica usa o endpoint de versão.
func TestProcessExplorerDetailVersion(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), true)
	// O upstream só serve export/xml (última) — a rota de versão específica
	// não existe, então pedir v24 deve falhar com 4xx/5xx, não entregar 200.
	status, _ := getBody(t, ts.URL+"/_dev/api/process/detail?id=compras_entrada_documento&version=24")
	if status == http.StatusOK {
		t.Errorf("detail de versão sem endpoint deveria falhar, deu 200")
	}
}

// detail cacheia por id|version — o segundo request não bate no upstream.
func TestProcessExplorerDetailCache(t *testing.T) {
	upstream := processExplorerUpstream(t)
	ts := newProcessExplorerServer(t, upstream, true)
	if status, _ := getBody(t, ts.URL+"/_dev/api/process/detail?id=compras_entrada_documento"); status != http.StatusOK {
		t.Fatalf("primeiro detail falhou: %d", status)
	}
	// Fecha o upstream: se o cache funciona, o segundo request ainda responde 200.
	upstream.Close()
	status, body := getBody(t, ts.URL+"/_dev/api/process/detail?id=compras_entrada_documento")
	if status != http.StatusOK || !strings.Contains(body, `"sequence":17`) {
		t.Errorf("cache não usado (status %d)", status)
	}
}

// Processo inexistente → 404 limpo.
func TestProcessExplorerDetail404(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), true)
	status, _ := getBody(t, ts.URL+"/_dev/api/process/detail?id=nao_existe")
	if status != http.StatusNotFound {
		t.Errorf("processo inexistente deveria dar 404, deu %d", status)
	}
	// Sem id → 400.
	if status, _ := getBody(t, ts.URL+"/_dev/api/process/detail"); status != http.StatusBadRequest {
		t.Errorf("detail sem id deveria dar 400, deu %d", status)
	}
}

// Sem cliente, a API responde 503 (a página segue servindo).
func TestProcessExplorerSemCliente(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), false)
	if status, _ := getBody(t, ts.URL+"/_dev/api/process/list"); status != http.StatusServiceUnavailable {
		t.Errorf("sem cliente deveria dar 503, deu %d", status)
	}
	if status, _ := getBody(t, ts.URL+"/_dev/processes/"); status != http.StatusOK {
		t.Errorf("página deveria abrir mesmo sem cliente, deu %d", status)
	}
}

// O tile do explorador aparece no dashboard.
func TestProcessExplorerTileNoDashboard(t *testing.T) {
	ts := newProcessExplorerServer(t, processExplorerUpstream(t), true)
	_, body := getBody(t, ts.URL+"/")
	if !strings.Contains(body, "/_dev/processes/") {
		t.Errorf("dashboard sem o tile de processos")
	}
}
