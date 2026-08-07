package cli

// Testes do task assume (ROADMAP3 §4.5): SOAP takeProcessTask do
// ECMWorkflowEngineService (fora dos swaggers). O <result> de sucesso é a
// string "OK" (validado ao vivo) — a confirmação vem da releitura das tarefas.
// O task release foi DESCARTADO: não há API de devolução ao pool (o
// releaseProcess do WSDL libera VERSÃO de processo — ver DIARIO.md).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func TestTaskAssume(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "assume", "196530",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	// O envelope SOAP leva o userCode resolvido (não o login), o número da
	// solicitação e o threadSequence 0 (fluxo único).
	for _, want := range []string{"<userId>uc</userId>", "<processInstanceId>196530</processInstanceId>",
		"<threadSequence>0</threadSequence>"} {
		if !strings.Contains(stub.soapTaskBody, want) {
			t.Errorf("envelope sem %s:\n%s", want, stub.soapTaskBody)
		}
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["action"] != "assumed" || data["requestId"].(float64) != 196530 {
		t.Errorf("data inesperado: %+v", data)
	}
	// A confirmação relê as tarefas em aberto e anexa o assignee corrente.
	if data["assignee"] == nil || data["stateName"] != "Acompanhar Retornos" {
		t.Errorf("sem a confirmação do estado: %+v", data)
	}
}

func TestTaskAssumeThread(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "task", "assume", "196530", "--thread", "7",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stub.soapTaskBody, "<threadSequence>7</threadSequence>") {
		t.Errorf("--thread não chegou ao envelope:\n%s", stub.soapTaskBody)
	}
}

// Recusa de negócio (usuário fora do papel do pool): o result vem com texto e
// vira SERVER_ERROR (exit 5) com a mensagem do servidor.
func TestTaskAssumeRecusado(t *testing.T) {
	stub := &requestStub{takeRejects: true}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "assume", "196530",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "não pertence ao papel") {
		t.Errorf("mensagem da recusa não chegou: %+v", env.Error)
	}
}

func TestTaskAssumeNumeroInvalido(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "task", "assume", "abc", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
}
