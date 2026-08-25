package cli

// Testes do `request move --manager` (ROADMAP §4.14, superado em 2026-08-25):
// SOAP saveAndSendTask com managerMode=true, destino resolvido pelo diagrama
// (export da versão) quando a etapa só tem uma saída.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func decodeEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é um envelope JSON: %v\n%s", err, stdout)
	}
	return env
}

// Atividade automática travada com UMA saída no diagrama (19→37): o destino é
// resolvido sozinho e o envelope SOAP vai em modo gestor.
func TestRequestMoveManagerDestinoAutomatico(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196534", "--manager", "--yes", "--comment", "reexecução",
		"--project", proj, "--server", "homolog", "--json")
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	for _, want := range []string{
		"<managerMode>true</managerMode>", "<choosedState>37</choosedState>", "<completeTask>true</completeTask>",
		"<processInstanceId>196534</processInstanceId>", "<threadSequence>0</threadSequence>",
		"<comments>reexecução</comments>", "<userId>uc</userId>", "<password></password>",
	} {
		if !strings.Contains(stub.soapMoveBody, want) {
			t.Errorf("envelope sem %s:\n%s", want, stub.soapMoveBody)
		}
	}
	if stub.moveBody != nil {
		t.Errorf("--manager não deve chamar a REST /move: %v", stub.moveBody)
	}
	env := decodeEnvelope(t, stdout)
	res := env["data"].(map[string]any)["result"].(map[string]any)
	if res["targetState"].(float64) != 37 || res["nextState"].(float64) != 17 {
		t.Errorf("targetState/nextState: %v", res)
	}
	if res["nextAssignee"] != "[Pool:Role:cabine_fiscal]" || res["processLink"].(float64) != 83 {
		t.Errorf("nextAssignee/processLink: %v", res)
	}
}

// Etapa com DUAS saídas (5→17, 5→26): exit 2 com data.options[] para escolher.
func TestRequestMoveManagerDestinoAmbiguo(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196535", "--manager", "--yes",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitUsage, stdout)
	}
	if stub.soapMoveBody != "" {
		t.Errorf("não deveria ter chamado o SOAP: %s", stub.soapMoveBody)
	}
	env := decodeEnvelope(t, stdout)
	opts := env["data"].(map[string]any)["options"].([]any)
	if len(opts) != 2 {
		t.Fatalf("options: %v", opts)
	}
	got := []float64{opts[0].(map[string]any)["targetState"].(float64), opts[1].(map[string]any)["targetState"].(float64)}
	if got[0] != 17 || got[1] != 26 {
		t.Errorf("destinos: %v", got)
	}
}

// --target-state explícito dispensa o diagrama e vai direto no choosedState.
func TestRequestMoveManagerTargetExplicito(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196535", "--manager", "--yes", "--target-state", "26", "--thread", "1",
		"--project", proj, "--server", "homolog", "--json")
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if !strings.Contains(stub.soapMoveBody, "<choosedState>26</choosedState>") ||
		!strings.Contains(stub.soapMoveBody, "<threadSequence>1</threadSequence>") {
		t.Errorf("envelope: %s", stub.soapMoveBody)
	}
}

// Escrita em tarefa de outro responsável: exige confirmação (--yes em
// não-interativo) e recusa campos de formulário.
func TestRequestMoveManagerGuardas(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196534", "--manager",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitUsage || stub.soapMoveBody != "" {
		t.Fatalf("sem --yes: exit=%d soap=%q\n%s", code, stub.soapMoveBody, stdout)
	}
	code, stdout = runMain(t, "request", "move", "196534", "--manager", "--yes", "--field", "a=b",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitUsage || stub.soapMoveBody != "" {
		t.Fatalf("com --field: exit=%d soap=%q\n%s", code, stub.soapMoveBody, stdout)
	}
}

// Recusa de negócio do motor (par ERROR) vira erro de servidor, não sucesso.
func TestRequestMoveManagerRecusaDoServidor(t *testing.T) {
	stub := &requestStub{managerRejects: true}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196534", "--manager", "--yes",
		"--project", proj, "--server", "homolog", "--json")
	if code == 0 {
		t.Fatalf("recusa deveria falhar\n%s", stdout)
	}
	env := decodeEnvelope(t, stdout)
	msg := env["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "Tarefa não encontrada") {
		t.Errorf("mensagem: %s", msg)
	}
}
