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

	// Produção exige a trava: sem --yes (e sem TTY) é bloqueada orientando o
	// --yes; com --yes passa da trava (e falha adiante, na autenticação
	// contra o host fake — o que prova que a guarda liberou).
	app := &App{}
	root := newRootCmd(app)
	root.SetArgs([]string{"dev", "--project", proj, "--server", "producao"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "PRODUÇÃO") {
		t.Errorf("produção sem --yes deveria pedir a confirmação: %v", err)
	}
	app = &App{}
	root = newRootCmd(app)
	root.SetArgs([]string{"dev", "--project", proj, "--server", "producao", "--yes"})
	err = root.Execute()
	if err == nil || strings.Contains(err.Error(), "PRODUÇÃO") {
		t.Errorf("com --yes a trava deveria liberar (falhando só na conexão): %v", err)
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
