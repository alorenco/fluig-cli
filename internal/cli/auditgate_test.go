package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// Pré-checagem do audit nos publish (ROADMAP2 §3.13): o §3.2 entregou só o
// `dataset export`, e a inconsistência era o problema — o mesmo erro de script
// passava batido nos outros comandos.

// escreve grava um arquivo dentro do projeto, criando as pastas.
func escreve(t *testing.T, proj, rel, conteudo string) string {
	t.Helper()
	path := filepath.Join(proj, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// event export: erro de audit barra o arquivo e a lista do servidor NÃO é
// regravada (o save do Fluig substitui o conjunto inteiro).
func TestEventExportBloqueadoPeloAudit(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)
	file := escreve(t, proj, "events/meuEventoNovo.js", dsConstEmLaco)

	code, stdout := runMain(t, "event", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d (audit reprovado); stdout=%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || env.Error.Code != output.CodeAuditFailed {
		t.Fatalf("esperava %s, veio %+v", output.CodeAuditFailed, env.Error)
	}
	if !strings.Contains(env.Error.Message, "RHINO003") {
		t.Errorf("a mensagem não cita a regra: %q", env.Error.Message)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.saved) != 0 {
		t.Errorf("saveEventList foi chamado apesar do erro de audit (%d eventos)", len(stub.saved))
	}
}

// event export --no-audit: publica como antes.
func TestEventExportNoAuditPublica(t *testing.T) {
	stub := &eventStub{}
	proj := eventProject(t, stub.server(t).URL)
	file := escreve(t, proj, "events/meuEventoNovo.js", dsConstEmLaco)

	code, stdout := runMain(t, "event", "export", file, "--no-audit", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d com --no-audit; stdout=%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.saved) == 0 {
		t.Error("nada foi salvo com --no-audit")
	}
}

// mechanism export: mesma regra do dataset (lote, por arquivo).
func TestMechanismExportBloqueadoPeloAudit(t *testing.T) {
	stub := &mechStub{}
	proj := mechProject(t, stub.server(t).URL)
	file := escreve(t, proj, "mechanisms/mec_novo.js", dsConstEmLaco)

	code, stdout := runMain(t, "mechanism", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || env.Error.Code != output.CodeAuditFailed {
		t.Errorf("esperava %s, veio %+v", output.CodeAuditFailed, env.Error)
	}
}

// workflow publish é ATÔMICO: um script reprovado aborta tudo e nada é
// importado no servidor (não existe versão pela metade).
func TestWorkflowPublishAbortaPeloAudit(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escreve(t, proj, "workflow/scripts/Compras.beforeTaskSave.js", dsConstEmLaco)

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || env.Error.Code != output.CodeAuditFailed {
		t.Fatalf("esperava %s, veio %+v", output.CodeAuditFailed, env.Error)
	}
	if len(stub.importedXML) != 0 {
		t.Errorf("o processo foi importado apesar do erro de audit")
	}
	if stub.releaseCalls != 0 {
		t.Errorf("release chamado %d vezes; deveria ser 0", stub.releaseCalls)
	}
}

// Dois scripts reprovados: a mensagem diz quantos e que nada foi publicado.
func TestWorkflowPublishAbortaCitandoTodos(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escreve(t, proj, "workflow/scripts/Compras.beforeTaskSave.js", dsConstEmLaco)
	escreve(t, proj, "workflow/scripts/Compras.afterProcessFinish.js", dsConstEmLaco)

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d; stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "nada foi publicado") {
		t.Errorf("mensagem sem o veredicto do lote: %+v", env.Error)
	}
}

// form export: só as regras de RUNTIME barram. Uma cor fixa (SG003, nível erro)
// não pode impedir a publicação de um formulário legado.
func TestFormExportSGNaoBarra(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	formDir := filepath.Join(proj, "forms", "Formulario de Teste")
	escreve(t, proj, "forms/Formulario de Teste/Formulario de Teste.html",
		`<html><body><p style="color:#ff0000">legado</p></body></html>`)

	code, stdout := runMain(t, "form", "export", formDir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d: cor fixa não pode barrar o form export; stdout=%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.updateBody == "" {
		t.Error("o formulário não foi enviado")
	}
}

// form export: erro de RUNTIME (RHINO003 num evento) barra o envio.
func TestFormExportRhinoBarra(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	formDir := filepath.Join(proj, "forms", "Formulario de Teste")
	escreve(t, proj, "forms/Formulario de Teste/Formulario de Teste.html", "<html>ok</html>")
	escreve(t, proj, "forms/Formulario de Teste/events/onLoad.js", dsConstEmLaco)

	code, stdout := runMain(t, "form", "export", formDir, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d, quer %d (audit reprovado); stdout=%s", code, output.ExitGeneric, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "RHINO003") {
		t.Errorf("esperava RHINO003 barrando: %+v", env.Error)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.updateBody != "" {
		t.Error("o formulário foi enviado apesar do erro de runtime")
	}
}

// O recorte de regras é por prefixo e vale para as duas pontas.
func TestAuditGateOptsBloqueia(t *testing.T) {
	todas := auditGateOpts{}
	runtime := auditGateOpts{regras: regrasDeRuntime}
	casos := []struct {
		regra          string
		todas, runtime bool
	}{
		{"RHINO003", true, true},
		{"FL002", true, true},
		{"SG002", true, false},
		{"SG003", true, false},
	}
	for _, c := range casos {
		if got := todas.bloqueia(c.regra); got != c.todas {
			t.Errorf("todas.bloqueia(%s)=%v, quer %v", c.regra, got, c.todas)
		}
		if got := runtime.bloqueia(c.regra); got != c.runtime {
			t.Errorf("runtime.bloqueia(%s)=%v, quer %v", c.regra, got, c.runtime)
		}
	}
}

// Alvo vazio não pode virar "audite o projeto inteiro": o `audit.Run` com lista
// vazia cai nas pastas convencionais. Num plano de deploy só com passos de
// formulário, o gate de scripts recebia [] e varria tudo — 474 avisos de
// arquivos que o plano nem toca (medido ao vivo em 2026-07-29).
func TestAuditGateListaVaziaNaoAuditaProjeto(t *testing.T) {
	proj := t.TempDir()
	// Um dataset com erro no projeto, fora de qualquer lote.
	if err := os.MkdirAll(filepath.Join(proj, "datasets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "datasets", "ds_ruim.js"), []byte(dsConstEmLaco), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{Project: proj}
	p := output.NewPrinter(true, "teste")

	gate := app.auditBeforePublish(p, nil, auditGateOpts{})
	if gate.ran {
		t.Error("o gate rodou com lista vazia — varreria o projeto inteiro")
	}
	if len(gate.findings) != 0 || gate.warnings != 0 {
		t.Errorf("achados de arquivos fora do lote: %d findings, %d avisos", len(gate.findings), gate.warnings)
	}
}
