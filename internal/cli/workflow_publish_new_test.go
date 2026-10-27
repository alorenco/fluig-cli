package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func writeBaseXML(t *testing.T, name string) string {
	t.Helper()
	base, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	xmlPath := filepath.Join(t.TempDir(), "base.xml")
	if err := os.WriteFile(xmlPath, base, 0o644); err != nil {
		t.Fatal(err)
	}
	return xmlPath
}

func decodePublish(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope JSON inválido: %v\n%s", err, stdout)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data não é objeto: %T\n%s", env.Data, stdout)
	}
	return data
}

func TestWorkflowPublishNewCreatesProcess(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){ /* novo */ }",
	})
	xmlPath := writeBaseXML(t, "rest_process_export.xml")

	code, stdout := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	data := decodePublish(t, stdout)
	if data["processId"] != "NovoProc" {
		t.Errorf("processId=%v, quer NovoProc", data["processId"])
	}
	if stub.newImportCalls != 1 {
		t.Errorf("import novo chamado %d vezes, quer 1", stub.newImportCalls)
	}
	if stub.importedXML != nil {
		t.Error("publish --new não podia usar o import de versão (processo existente)")
	}
	if !strings.Contains(stub.newImportQuery, "processId=NovoProc") {
		t.Errorf("query do import novo sem processId=NovoProc: %q", stub.newImportQuery)
	}
	if !strings.Contains(string(stub.newImportedXML), "/* novo */") {
		t.Error("XML importado não contém o script local")
	}
	// O XML base traz o processId antigo da fixture; o publish --new precisa
	// reescrever para o alvo.
	if !strings.Contains(string(stub.newImportedXML), "<processId>NovoProc</processId>") {
		t.Error("XML importado não teve o processId reescrito para NovoProc")
	}
	if strings.Contains(string(stub.newImportedXML), "zz_fluigcli_test_pub") {
		t.Error("XML importado ainda tem o processId antigo da fixture")
	}
}

func TestWorkflowPublishNewSemFile(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	code, _ := runMain(t, "workflow", "publish", "NovoProc", "--new",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d (uso)", code, output.ExitUsage)
	}
	if stub.importedXML != nil || stub.newImportedXML != nil {
		t.Error("nada podia ter sido importado")
	}
}

func TestWorkflowPublishFileSemNew(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"Compras.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	xmlPath := writeBaseXML(t, "rest_process_export.xml")
	for _, extra := range [][]string{{"--file", xmlPath}, {"--form-id", "123"}, {"--description", "X"}} {
		args := append([]string{"workflow", "publish", "Compras"}, extra...)
		args = append(args, "--json", "--project", proj, "--server", "homolog")
		code, _ := runMain(t, args...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d (uso)", extra, code, output.ExitUsage)
		}
	}
}

// Processo já existente: o servidor recusa o import novo com HTTP 400 e a CLI
// traduz numa mensagem clara (exit 5), sem liberar nada.
func TestWorkflowPublishNewJaExiste(t *testing.T) {
	stub := &workflowStub{newImportConflict: true}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"Compras.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	xmlPath := writeBaseXML(t, "rest_process_export.xml")

	code, stdout := runMain(t, "workflow", "publish", "Compras", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope JSON inválido: %v", err)
	}
	if env.Error == nil || !strings.Contains(env.Error.Message, "já existe") {
		t.Errorf("mensagem deveria dizer que o processo já existe: %+v", env.Error)
	}
	// A mensagem da CLI mantém o texto do servidor.
	if env.Error == nil || !strings.Contains(env.Error.Message, "Ja existe um Processo") {
		t.Errorf("mensagem deveria manter o texto do servidor: %+v", env.Error)
	}
	if stub.newImportCalls != 0 {
		t.Error("import recusado não podia contar como chamado")
	}
	if stub.releaseCalls != 0 {
		t.Error("nada podia ter sido liberado")
	}
}

// Evento local que não existe no XML base: falha antes do import.
func TestWorkflowPublishNewEventoInexistente(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.naoTem.js": "function naoTem(){}",
	})
	xmlPath := writeBaseXML(t, "rest_process_export.xml")
	code, _ := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
	if stub.newImportedXML != nil {
		t.Error("nada podia ter sido importado")
	}
}

// formId do XML base ausente no destino: recusa com exit 4 antes do import.
func TestWorkflowPublishNewFormIDMissing(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	// A fixture full traz formId 263801, que não existe no stub.
	xmlPath := writeBaseXML(t, "rest_process_export_full.xml")
	code, stdout := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitNotFound, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope JSON inválido: %v", err)
	}
	if env.Error == nil || !strings.Contains(env.Error.Message, "--form-id") {
		t.Errorf("mensagem deveria orientar o --form-id: %+v", env.Error)
	}
	if stub.newImportedXML != nil {
		t.Error("nada podia ter sido importado")
	}
}

// --form-id reescreve o valor e passa na checagem quando o doc é formulário.
func TestWorkflowPublishNewFormIDFlag(t *testing.T) {
	stub := &workflowStub{existingDocs: map[int]string{999: "CardIndex"}}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){ /* novo */ }",
	})
	xmlPath := writeBaseXML(t, "rest_process_export_full.xml")
	code, stdout := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--form-id", "999", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(string(stub.newImportedXML), "<formId>999</formId>") {
		t.Error("XML importado não teve o formId reescrito para 999")
	}
	if stub.releaseCalls != 1 {
		t.Errorf("release chamado %d vezes, quer 1", stub.releaseCalls)
	}
}

// formId de pasta (não formulário): recusa com exit 4 antes do import.
func TestWorkflowPublishNewFormIDNotForm(t *testing.T) {
	stub := &workflowStub{existingDocs: map[int]string{555: "Folder"}}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	xmlPath := writeBaseXML(t, "rest_process_export_full.xml")
	code, stdout := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--form-id", "555", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitNotFound, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope JSON inválido: %v", err)
	}
	if env.Error == nil || !strings.Contains(env.Error.Message, "não é um formulário") {
		t.Errorf("mensagem deveria dizer que não é formulário: %+v", env.Error)
	}
	if stub.newImportCalls != 0 {
		t.Error("nada podia ter sido importado")
	}
}

// --description troca o nome do processo novo.
func TestWorkflowPublishNewDescription(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){ /* novo */ }",
	})
	xmlPath := writeBaseXML(t, "rest_process_export.xml")
	code, stdout := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--description", "Novo Processo", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(string(stub.newImportedXML), "<processDescription>Novo Processo</processDescription>") {
		t.Error("XML importado não teve a descrição reescrita")
	}
}
