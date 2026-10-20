package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// RED: publicar processo novo a partir de XML base + scripts locais.
func TestWorkflowPublishNewCreatesProcess(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.beforeTaskSave.js": "function beforeTaskSave(){ /* novo */ }",
	})

	// XML base: fixture real com processId antigo.
	base, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export.xml"))
	if err != nil {
		t.Fatal(err)
	}
	xmlPath := filepath.Join(t.TempDir(), "base.xml")
	if err := os.WriteFile(xmlPath, base, 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
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
	if stub.releaseCalls != 1 {
		t.Errorf("release chamado %d vezes, quer 1", stub.releaseCalls)
	}
}

// RED: --new sem --file é erro de uso.
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

// --file sem --new é erro de uso.
func TestWorkflowPublishFileSemNew(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"Compras.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	base, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export.xml"))
	xmlPath := filepath.Join(t.TempDir(), "base.xml")
	os.WriteFile(xmlPath, base, 0o644)
	code, _ := runMain(t, "workflow", "publish", "Compras", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d (uso)", code, output.ExitUsage)
	}
}

// Processo já existente + --new: recusa sem alterar (não sobrescreve).
func TestWorkflowPublishNewJaExiste(t *testing.T) {
	stub := &workflowStub{version: 5}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"Compras.beforeTaskSave.js": "function beforeTaskSave(){}",
	})
	base, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export.xml"))
	xmlPath := filepath.Join(t.TempDir(), "base.xml")
	os.WriteFile(xmlPath, base, 0o644)
	code, _ := runMain(t, "workflow", "publish", "Compras", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code == output.ExitOK {
		t.Errorf("processo existente com --new deveria falhar, veio exit 0")
	}
	if stub.newImportedXML != nil || stub.importedXML != nil {
		t.Error("nada podia ter sido importado")
	}
}

// Evento local que não existe no XML base: falha antes do import.
func TestWorkflowPublishNewEventoInexistente(t *testing.T) {
	stub := &workflowStub{}
	proj := workflowProject(t, stub.server(t).URL)
	escrevePublishScripts(t, proj, map[string]string{
		"NovoProc.naoTem.js": "function naoTem(){}",
	})
	base, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export.xml"))
	xmlPath := filepath.Join(t.TempDir(), "base.xml")
	os.WriteFile(xmlPath, base, 0o644)
	code, _ := runMain(t, "workflow", "publish", "NovoProc", "--new", "--file", xmlPath,
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
	if stub.newImportedXML != nil {
		t.Error("nada podia ter sido importado")
	}
}
