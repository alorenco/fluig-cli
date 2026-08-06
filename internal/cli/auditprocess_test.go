package cli

// Testes do `audit --process` (regras WF001/WF002 — ROADMAP3 §4.12): cruzamento
// das classes activity-N do formulário com as etapas reais do processo. Usa a
// fixture real rest_process_export_full.xml (compras_entrada_documento,
// formId 263801; humanas: 5, 17, 20, 26, 31, 38, 45, 64, 72…).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// auditProcessStub sobe login/ping/version + o export XML do processo.
func auditProcessStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/api/public/wcm/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":"TOTVS Fluig Plataforma - Voyager 2.0.0-260707"}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes/compras_entrada_documento/export/xml", func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export_full.xml"))
		if err != nil {
			t.Error(err)
			return
		}
		w.Write(b)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// auditProcessProject monta o projeto: servidor cadastrado, formulário local e
// o vínculo formId↔pasta no forms.json (escopo host:porta/companyId).
func auditProcessProject(t *testing.T, stubURL, html string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "audit-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	form := filepath.Join(proj, "forms", "frm_entrada")
	if err := os.MkdirAll(form, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(form, "principal.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	formsJSON := fmt.Sprintf(`{"version":"2.0.0","servers":{"%s:%d/1":[`+
		`{"folder":"frm_entrada","documentId":263801,"name":"frm_entrada"}]}}`, u.host, u.port)
	if err := os.WriteFile(filepath.Join(proj, ".fluigcli", "forms.json"), []byte(formsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestAuditProcessCruzamento(t *testing.T) {
	stub := auditProcessStub(t)
	// activity-99 não existe no processo (erro); as humanas 5/17/20… sem seção
	// viram aviso — o form usa a convenção.
	html := `<form name="f"><div class="activity activity-0 activity-5 activity-99">x</div></form>`
	proj := auditProcessProject(t, stub.URL, html)

	code, stdout := runMain(t, "audit", "forms", "--process", "compras_entrada_documento",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("WF001 é erro e o default --fail-on error reprova: exit=%d\n%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	findings, _ := data["findings"].([]any)

	var wf001, wf002 int
	var msg99 string
	for _, raw := range findings {
		f, _ := raw.(map[string]any)
		switch f["rule"] {
		case "WF001":
			wf001++
			msg99, _ = f["message"].(string)
		case "WF002":
			wf002++
		}
	}
	if wf001 != 1 {
		t.Fatalf("esperava 1 WF001 (activity-99), veio %d\n%s", wf001, stdout)
	}
	if !strings.Contains(msg99, "activity-99") || !strings.Contains(msg99, "compras_entrada_documento") {
		t.Errorf("mensagem do WF001 sem o essencial: %s", msg99)
	}
	// A fixture tem 9+ atividades humanas; o HTML cobre só a 5.
	if wf002 < 5 {
		t.Errorf("esperava avisos WF002 para as humanas sem seção, veio %d", wf002)
	}
}

func TestAuditProcessFormularioCorreto(t *testing.T) {
	stub := auditProcessStub(t)
	// Cobre TODAS as humanas da fixture (bpmnType 80): 5,17,20,26,31,38,45,64,72.
	html := `<form name="f"><div class="activity activity-0 activity-5 activity-17 activity-20 activity-26 ` +
		`activity-31 activity-38 activity-45 activity-64 activity-72">x</div></form>`
	proj := auditProcessProject(t, stub.URL, html)

	code, stdout := runMain(t, "audit", "forms", "--process", "compras_entrada_documento",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("formulário correto não pode reprovar: exit=%d\n%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	findings, _ := data["findings"].([]any)
	for _, raw := range findings {
		f, _ := raw.(map[string]any)
		if f["rule"] == "WF001" || f["rule"] == "WF002" {
			t.Errorf("achado WF* inesperado: %+v", f)
		}
	}
}

// Sem vínculo no forms.json a regra não tem como achar o formulário — a
// mensagem diz como criar o vínculo.
func TestAuditProcessSemVinculo(t *testing.T) {
	stub := auditProcessStub(t)
	proj := auditProcessProject(t, stub.URL, `<form name="f"></form>`)
	if err := os.Remove(filepath.Join(proj, ".fluigcli", "forms.json")); err != nil {
		t.Fatal(err)
	}
	code, stdout := runMain(t, "audit", "forms", "--process", "compras_entrada_documento",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitNotFound, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "form import 263801") {
		t.Errorf("mensagem sem o caminho de correção: %+v", env.Error)
	}
}
