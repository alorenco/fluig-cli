package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/browser"
	"github.com/alorenco/fluig-cli/internal/output"
)

const suiteUI = `{"process":"compras_entrada_documento","defaults":{"card":"fixtures/card.json"},"scenarios":[
 {"name":"tela","steps":[
   {"start":true,"expect":{"state":45}},
   {"assume":true},
   {"ui":{"open":"task"},"expect":{"enabled":["decisao"]}}
 ]}]}`

// Suíte com passo ui e sem navegador: exit 7 ANTES de abrir solicitação.
func TestTestRunUISemNavegador(t *testing.T) {
	old := findBrowser
	t.Cleanup(func() { findBrowser = old })
	chamado := 0
	findBrowser = func(string) (string, error) {
		chamado++
		return "", fmt.Errorf("%w: nada instalado", browser.ErrNotFound)
	}
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", suiteUI)
	code, stdout := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	env, _ := envelope(t, stdout)
	if code != output.ExitMissingHelper || env.Error == nil || env.Error.Code != output.CodeBrowserNotFound ||
		!strings.Contains(env.Error.Message, "--browser") {
		t.Fatalf("exit=%d %+v", code, env.Error)
	}
	if stub.moves != nil || stub.canceled {
		t.Error("nada pode ser escrito sem navegador")
	}

	// O --dry-run valida o passo ui sem procurar navegador.
	chamado = 0
	code, _ = runMain(t, "test", "run", "--dry-run", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || chamado != 0 {
		t.Errorf("dry-run: exit=%d, busca do navegador=%d", code, chamado)
	}

	// Suíte sem passo ui não procura navegador.
	proj = testProject(t, stub.server(t).URL, "hml", suiteCompras)
	runMain(t, "test", "run", "--dry-run", "--project", proj, "--server", "homolog")
	if chamado != 0 {
		t.Error("suíte sem ui não procura navegador")
	}
}
