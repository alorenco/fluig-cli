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

// --- request cancel (ROADMAP3 §4.6) — SOAP cancelInstance, mesma semântica
// de result do takeProcessTask ("OK" = sucesso). ---

func TestRequestCancel(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "cancel", "196533", "--comment", "teste encerrado",
		"--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"<processInstanceId>196533</processInstanceId>",
		"<userId>uc</userId>", "<cancelText>teste encerrado</cancelText>"} {
		if !strings.Contains(stub.soapTaskBody, want) {
			t.Errorf("envelope sem %s:\n%s", want, stub.soapTaskBody)
		}
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	first, _ := results[0].(map[string]any)
	// A confirmação relê o status — CANCELED prova o efeito, não o "OK".
	if first["success"] != true || first["status"] != "CANCELED" {
		t.Errorf("resultado inesperado: %+v", first)
	}
}

// Sem --yes em modo não-interativo, NADA é cancelado (exit 2).
func TestRequestCancelExigeYes(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, _ := runMain(t, "request", "cancel", "196533", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Fatalf("exit=%d, quer %d", code, output.ExitUsage)
	}
	if strings.Contains(stub.soapTaskBody, "cancelInstance") {
		t.Error("o envelope não poderia ter sido enviado sem confirmação")
	}
}

// Lote com um item recusado → exit 6 com results[] por item.
func TestRequestCancelParcial(t *testing.T) {
	stub := &requestStub{takeRejects: true} // o stub recusa todos
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "cancel", "196533", "196534",
		"--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitPartial, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("results deveria ter os 2 itens (sucesso E falha): %+v", results)
	}
}

// --- §4.17: --assignee em atividade de POOL — a recusa "Usuário selecionado
// não encontrado" ganha a explicação certa. ---

// No move, a CLI consulta os candidatos reais e NOMEIA o pool.
func TestRequestMoveAssigneePoolExplicado(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "move", "196526", "--movement", "15",
		"--target-state", "21", "--assignee", "pessoa_em_pool",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	for _, want := range []string{"usa POOL", "Sucesso do Cliente", "Pool:Role:sucesso_cliente",
		"Omita --assignee", "task assume 196526"} {
		if !strings.Contains(env.Error.Message, want) {
			t.Errorf("mensagem sem %q: %s", want, env.Error.Message)
		}
	}
}

// No start a solicitação ainda não existe — a dica sai genérica, mas sai.
func TestRequestStartAssigneePoolExplicado(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "request", "start", "compras_requisicao_abastecimento",
		"--target-state", "21", "--assignee", "pessoa_em_pool",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	for _, want := range []string{"causa comum é a atividade destino usar POOL", "omita --assignee",
		"request assignees"} {
		if !strings.Contains(env.Error.Message, want) {
			t.Errorf("mensagem sem %q: %s", want, env.Error.Message)
		}
	}
}

// Recusa DIFERENTE (não é o texto do pool) não é tocada.
func TestRequestMoveOutraRecusaNaoEnriquece(t *testing.T) {
	stub := &requestStub{}
	proj := requestProject(t, stub.server(t).URL)
	// o stub /quebrado/ devolve o throw de evento no start
	code, stdout := runMain(t, "request", "start", "quebrado", "--assignee", "alguem",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if strings.Contains(env.Error.Message, "POOL") {
		t.Errorf("recusa de outro tipo não pode ganhar a dica de pool: %s", env.Error.Message)
	}
}
