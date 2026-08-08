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

// --- WF003: scripts do processo (ROADMAP §4.12-b) ---

// escreveScript grava um script de evento do processo no projeto.
func escreveScript(t *testing.T, proj, nome, js string) {
	t.Helper()
	dir := filepath.Join(proj, "workflow", "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nome), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
}

// O audit --process também varre os scripts do processo. A fixture real tem as
// etapas 5, 17, 20, 26… — 166 não é etapa nenhuma.
func TestAuditProcessScriptEtapaInexistente(t *testing.T) {
	stub := auditProcessStub(t)
	proj := auditProcessProject(t, stub.URL, `<form name="f"></form>`)
	escreveScript(t, proj, "compras_entrada_documento.beforeStateEntry.js",
		`function beforeStateEntry(sequenceId) {
    var cancelaState = 166;
    var aprovar = 5;
    if (sequenceId == cancelaState) { cancela(); }
    if (sequenceId == aprovar) { ok(); }
}`)

	code, stdout := runMain(t, "audit", "forms", "--process", "compras_entrada_documento",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d (WF003 é erro)\n%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	achados, _ := data["findings"].([]any)
	var wf3 []map[string]any
	for _, raw := range achados {
		if f, _ := raw.(map[string]any); f["rule"] == "WF003" {
			wf3 = append(wf3, f)
		}
	}
	if len(wf3) != 1 {
		t.Fatalf("esperava 1 achado WF003, veio %d: %+v", len(wf3), achados)
	}
	if arquivo, _ := wf3[0]["file"].(string); !strings.Contains(arquivo, "beforeStateEntry.js") {
		t.Errorf("achado no arquivo errado: %v", wf3[0])
	}
	if msg, _ := wf3[0]["message"].(string); !strings.Contains(msg, "166") {
		t.Errorf("mensagem sem o número: %v", wf3[0])
	}
}

// Script com todas as etapas certas não gera achado — a rede contra falso
// positivo, que foi o critério de calibração da regra.
func TestAuditProcessScriptSemAchado(t *testing.T) {
	stub := auditProcessStub(t)
	proj := auditProcessProject(t, stub.URL, `<form name="f"></form>`)
	escreveScript(t, proj, "compras_entrada_documento.beforeTaskSave.js",
		`function beforeTaskSave(colleagueId, nextSequenceId, userList) {
    var aprovar = 5;
    var inicio = 0;
    if (nextSequenceId == aprovar || nextSequenceId == inicio) { ok(); }
}`)

	code, stdout := runMain(t, "audit", "forms", "--process", "compras_entrada_documento",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitOK, stdout)
	}
	if strings.Contains(stdout, "WF003") {
		t.Errorf("nenhum WF003 esperado: %s", stdout)
	}
}

// Sem script com o prefixo do processo, a CLI DIZ que a regra ficou de fora.
// Silêncio aqui viraria "está tudo certo" — e o prefixo local pode diferir do
// processId do servidor (ROADMAP §1.7-A).
func TestAuditProcessSemScriptAvisa(t *testing.T) {
	stub := auditProcessStub(t)
	proj := auditProcessProject(t, stub.URL, `<form name="f"></form>`)

	_, stdout := runMain(t, "audit", "forms", "--process", "compras_entrada_documento",
		"--project", proj, "--server", "homolog")
	if !strings.Contains(stdout, "WF003") || !strings.Contains(stdout, "nenhum script local") {
		t.Errorf("faltou o aviso de que a WF003 não rodou: %s", stdout)
	}
}
