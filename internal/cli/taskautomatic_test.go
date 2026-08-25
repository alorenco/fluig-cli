package cli

// Testes do `task list --automatic` (ROADMAP §4.14, superado em 2026-08-25):
// solicitações paradas em atividade automática (assignee System:Auto), de
// todos os usuários, filtradas no cliente.

import (
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func TestTaskListAutomatic(t *testing.T) {
	stub := &taskStub{withAutomatic: true}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--automatic", "--process", "Compras", "--project", proj, "--server", "homolog", "--json")
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if stub.query.Get("assignee") != "" {
		t.Errorf("--automatic não deve filtrar responsável no servidor: %v", stub.query)
	}
	if stub.query.Get("processId") != "Compras" {
		t.Errorf("--process deve ir ao servidor: %v", stub.query)
	}
	env := decodeEnvelope(t, stdout)
	tasks := env["data"].(map[string]any)["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("quer só a System:Auto, veio %d: %v", len(tasks), tasks)
	}
	if tasks[0].(map[string]any)["requestId"].(float64) != 228691 {
		t.Errorf("tarefa: %v", tasks[0])
	}
}

func TestTaskListAutomaticTabelaEVazio(t *testing.T) {
	stub := &taskStub{withAutomatic: true}
	proj := taskProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "task", "list", "--automatic", "--project", proj, "--server", "homolog")
	if code != 0 || !strings.Contains(stdout, "Anexar Relatórios") || strings.Contains(stdout, "Aguardar Nota") {
		t.Fatalf("tabela: exit=%d\n%s", code, stdout)
	}
	stub = &taskStub{}
	proj = taskProject(t, stub.server(t).URL)
	code, out, stderr := runMainStderr(t, "task", "list", "--automatic", "--project", proj, "--server", "homolog")
	if code != 0 || !strings.Contains(out+stderr, "Nenhuma solicitação parada em atividade automática") {
		t.Fatalf("vazio: exit=%d\n%s%s", code, out, stderr)
	}
}

func TestTaskListAutomaticNaoCombina(t *testing.T) {
	stub := &taskStub{}
	proj := taskProject(t, stub.server(t).URL)
	for _, extra := range [][]string{{"--assignee", "jsilva"}, {"--group", "TI"}, {"--role", "x"}} {
		args := append([]string{"task", "list", "--automatic"}, extra...)
		args = append(args, "--project", proj, "--server", "homolog", "--json")
		if code, stdout := runMain(t, args...); code != output.ExitUsage {
			t.Errorf("%v: exit=%d\n%s", extra, code, stdout)
		}
	}
}
