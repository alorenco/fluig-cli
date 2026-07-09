package cli

import (
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

func TestDevGuards(t *testing.T) {
	proj := projWithServers(t, doisServidores()...) // hml + producao (prod)

	// --json é recusado antes de qualquer coisa.
	code, _ := runMain(t, "dev", "--json", "--project", proj, "--server", "hml")
	if code != output.ExitUsage {
		t.Errorf("--json: exit = %d, quer %d", code, output.ExitUsage)
	}

	// Produção é recusada sem exceção.
	app := &App{}
	root := newRootCmd(app)
	root.SetArgs([]string{"dev", "--project", proj, "--server", "producao", "--yes"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "PRODUÇÃO") {
		t.Errorf("produção deveria ser recusada mesmo com --yes: %v", err)
	}

	// Servidor sem env marcado também é recusado, com dica do server update.
	semEnv := projWithServers(t, config.Server{ID: "9", Name: "solto", Host: "h", Port: 80, Username: "u", CompanyID: 1})
	app = &App{}
	root = newRootCmd(app)
	root.SetArgs([]string{"dev", "--project", semEnv, "--server", "solto"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "server update") {
		t.Errorf("sem env deveria orientar o server update: %v", err)
	}
}
