package cli

// Testes do comando audit (linter do Style Guide — local, sem servidor).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func auditProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	form := filepath.Join(root, "forms", "F")
	if err := os.MkdirAll(form, 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<link rel="stylesheet" href="/style-guide/css/fluig-style-guide.min.css">
<form name="f"><div style="color:#fff">x</div></form>`
	if err := os.WriteFile(filepath.Join(form, "F.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Com achado de erro, o default (--fail-on error) reprova com exit 1 e o
// envelope ok=false PRESERVA os findings no data.
func TestAuditReprovaComEnvelope(t *testing.T) {
	proj := auditProject(t)
	code, stdout := runMain(t, "audit", "--project", proj, "--json")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quero %d\n%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é JSON: %v\n%s", err, stdout)
	}
	if env.OK || env.Error == nil || env.Error.Code != output.CodeAuditFailed {
		t.Errorf("envelope: ok=%v error=%+v", env.OK, env.Error)
	}
	data, _ := env.Data.(map[string]any)
	findings, _ := data["findings"].([]any)
	if len(findings) != 3 { // SG001 legado + SG005 inline + SG003 cor no style=
		t.Errorf("findings=%d, quero 3: %v", len(findings), findings)
	}
	counts, _ := data["counts"].(map[string]any)
	if counts["error"] != float64(1) || counts["warning"] != float64(2) {
		t.Errorf("counts: %v", counts)
	}
}

// --fail-on none só relata (exit 0, ok=true); --fail-on warning pega aviso.
func TestAuditFailOn(t *testing.T) {
	proj := auditProject(t)
	code, stdout := runMain(t, "audit", "--fail-on", "none", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("fail-on none: exit=%d\n%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || !env.OK {
		t.Fatalf("envelope: err=%v ok=%v", err, env.OK)
	}

	// Sem os erros (só o aviso do CSS legado), error não reprova; warning sim.
	os.WriteFile(filepath.Join(proj, "forms", "F", "F.html"),
		[]byte(`<link href="/style-guide/css/fluig-style-guide.min.css">`), 0o644)
	if code, _ := runMain(t, "audit", "--project", proj, "--json"); code != output.ExitOK {
		t.Errorf("só aviso com fail-on error: exit=%d, quero 0", code)
	}
	if code, _ := runMain(t, "audit", "--fail-on", "warning", "--project", proj, "--json"); code != output.ExitGeneric {
		t.Errorf("só aviso com fail-on warning: exit=%d, quero 1", code)
	}
	if code, _ := runMain(t, "audit", "--fail-on", "banana", "--project", proj, "--json"); code != output.ExitUsage {
		t.Errorf("fail-on inválido: exit=%d, quero 2", code)
	}
}

// Projeto limpo: exit 0 e mensagem de sucesso.
func TestAuditLimpo(t *testing.T) {
	proj := t.TempDir()
	form := filepath.Join(proj, "forms", "Ok")
	os.MkdirAll(form, 0o755)
	os.WriteFile(filepath.Join(form, "Ok.html"),
		[]byte(`<form name="ok"><div class="fs-bg-white form-group">tudo certo</div></form>`), 0o644)
	code, stdout := runMain(t, "audit", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["scanned"] != float64(1) {
		t.Errorf("scanned: %v", data["scanned"])
	}
}

// Modo humano: tabela com a regra e o local (padrão de listagem).
func TestAuditTabela(t *testing.T) {
	proj := auditProject(t)
	code, stdout := runMain(t, "audit", "--fail-on", "none", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, frag := range []string{"SG001", "SG003", "forms/F/F.html:", "Problema"} {
		if !strings.Contains(stdout, frag) {
			t.Errorf("tabela sem %q:\n%s", frag, stdout)
		}
	}
}

// --fix aplica as correções determinísticas e reaudita para o relatório.
func TestAuditFix(t *testing.T) {
	proj := auditProject(t)
	code, stdout := runMain(t, "audit", "--fix", "--fail-on", "none", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["fixed"] != float64(2) { // legado + #fff
		t.Errorf("fixed: %v", data["fixed"])
	}
	got, _ := os.ReadFile(filepath.Join(proj, "forms", "F", "F.html"))
	if !strings.Contains(string(got), "fluig-style-guide-flat.min.css") ||
		!strings.Contains(string(got), "var(--fs-color-neutral-light-00)") {
		t.Errorf("arquivo não corrigido:\n%s", got)
	}
	// O relatório pós-fix não traz mais os corrigidos (sobra o SG005 inline).
	findings, _ := data["findings"].([]any)
	for _, raw := range findings {
		f, _ := raw.(map[string]any)
		if f["rule"] == "SG001" || f["rule"] == "SG003" {
			t.Errorf("achado corrigido ainda no relatório: %v", f)
		}
	}
}

// Baseline (ROADMAP §5.5): --save-baseline grava o retrato de hoje; depois só
// achado NOVO reprova, e o antigo continua no relatório, marcado.
func TestAuditBaseline(t *testing.T) {
	proj := auditProject(t)

	code, stdout := runMain(t, "audit", "--save-baseline", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("--save-baseline: exit=%d\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(proj, ".fluigcli", "audit-baseline.json")); err != nil {
		t.Fatalf("baseline não gravado: %v", err)
	}

	// Mesma árvore: os 3 achados viram dívida conhecida e o audit aprova.
	code, stdout = runMain(t, "audit", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("com baseline: exit=%d, quero 0\n%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	findings, _ := data["findings"].([]any)
	if len(findings) != 3 {
		t.Errorf("o baseline não pode ESCONDER achado do relatório: %d", len(findings))
	}
	for _, f := range findings {
		if m, _ := f.(map[string]any); m["baseline"] != true {
			t.Errorf("achado antigo sem a marca baseline: %v", m)
		}
	}
	b, _ := data["baseline"].(map[string]any)
	if b["known"] != float64(3) || b["new"] != float64(0) {
		t.Errorf("resumo do baseline: %v", b)
	}

	// --no-baseline volta a reprovar tudo.
	if code, _ := runMain(t, "audit", "--no-baseline", "--project", proj, "--json"); code != output.ExitGeneric {
		t.Errorf("--no-baseline: exit=%d, quero %d", code, output.ExitGeneric)
	}

	// Achado NOVO num arquivo já coberto pelo baseline reprova.
	novo := "<script src=\"https://cdn.example.com/x.js\"></script>\n<form name=\"f\"></form>"
	if err := os.WriteFile(filepath.Join(proj, "forms", "F", "G.html"), []byte(novo), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout = runMain(t, "audit", "--project", proj, "--json")
	if code != output.ExitGeneric {
		t.Fatalf("achado novo: exit=%d, quero %d\n%s", code, output.ExitGeneric, stdout)
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || !strings.Contains(env.Error.Message, "baseline") {
		t.Errorf("a mensagem devia dizer quantos achados do baseline não contam: %+v", env.Error)
	}
	data, _ = env.Data.(map[string]any)
	b, _ = data["baseline"].(map[string]any)
	if b["known"] != float64(3) || b["new"] != float64(1) {
		t.Errorf("resumo do baseline com achado novo: %v", b)
	}

	// Dívida quitada: o arquivo antigo some e o audit avisa para regravar.
	os.Remove(filepath.Join(proj, "forms", "F", "F.html"))
	code, stdout = runMain(t, "audit", "--project", proj)
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "dívida quitada") {
		t.Errorf("faltou avisar da dívida quitada:\n%s", stdout)
	}
}

// Exceções via .fluigcli/audit.json.
func TestAuditIgnoreConfig(t *testing.T) {
	proj := auditProject(t)
	os.MkdirAll(filepath.Join(proj, ".fluigcli"), 0o755)
	os.WriteFile(filepath.Join(proj, ".fluigcli", "audit.json"),
		[]byte(`{"ignore":["forms/"]}`), 0o644)
	code, stdout := runMain(t, "audit", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	// Config quebrada = erro de uso.
	os.WriteFile(filepath.Join(proj, ".fluigcli", "audit.json"), []byte(`{nao é json`), 0o644)
	if code, _ := runMain(t, "audit", "--project", proj, "--json"); code != output.ExitUsage {
		t.Errorf("config inválida: exit=%d, quero 2", code)
	}
}
