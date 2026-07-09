package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// server add dentro de um projeto grava a conexão no servers.json (sem
// identidade) e o usuário no servers.local.json, e garante o .gitignore.
func TestServerAddProjetoDivideArquivos(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	code, stdout := runMain(t, "server", "add", "--name", "homolog", "--host", "10.1.2.253",
		"--port", "8080", "--ssl=false", "--username", "alorenco", "--env", "hml",
		"--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d, stdout=%s", code, stdout)
	}

	// servers.json versionável não pode conter identidade.
	raw, err := os.ReadFile(filepath.Join(proj, ".fluigcli", "servers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(raw); strings.Contains(s, "alorenco") || strings.Contains(s, "\"username\"") {
		t.Errorf("servers.json versionável vazou identidade:\n%s", s)
	}

	// servers.local.json (git-ignorado) tem o usuário.
	var local config.LocalFile
	data, err := os.ReadFile(filepath.Join(proj, ".fluigcli", "servers.local.json"))
	if err != nil {
		t.Fatalf("servers.local.json ausente: %v", err)
	}
	if err := json.Unmarshal(data, &local); err != nil {
		t.Fatal(err)
	}
	if len(local.Identities) != 1 || local.Identities[0].Username != "alorenco" {
		t.Errorf("identidade local inesperada: %+v", local)
	}

	// .gitignore ganhou a entrada da identidade pessoal.
	gi, err := os.ReadFile(filepath.Join(proj, ".gitignore"))
	if err != nil || !strings.Contains(string(gi), ".fluigcli/servers.local.json") {
		t.Errorf(".gitignore deveria ignorar servers.local.json: %q, err=%v", gi, err)
	}
}

// ensureLocalGitignore é idempotente: não duplica a entrada e preserva o resto.
func TestEnsureLocalGitignoreIdempotente(t *testing.T) {
	proj := t.TempDir()
	giPath := filepath.Join(proj, ".gitignore")
	if err := os.WriteFile(giPath, []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := ensureLocalGitignore(proj); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(giPath)
	got := string(data)
	if n := strings.Count(got, ".fluigcli/servers.local.json"); n != 1 {
		t.Errorf("entrada duplicada (%d vezes):\n%s", n, got)
	}
	if !strings.Contains(got, "node_modules/") {
		t.Errorf("conteúdo anterior perdido:\n%s", got)
	}
}

// Sem identidade e em modo não-interativo sem FLUIGCLI_USERNAME, o auth falha
// com erro de uso orientando a definir o usuário.
func TestEnsureIdentitySemUsuarioNaoInterativo(t *testing.T) {
	app := &App{}
	server := &config.Server{Name: "homolog", Host: "h", Port: 80}
	err := app.ensureIdentity(server)
	if err == nil {
		t.Fatal("sem usuário e não-interativo deveria falhar")
	}
	if output.ExitCodeFor(err) != output.ExitUsage {
		t.Errorf("exit = %d, quer %d (uso)", output.ExitCodeFor(err), output.ExitUsage)
	}
	if !strings.Contains(err.Error(), config.EnvUsername) {
		t.Errorf("erro deveria citar %s: %q", config.EnvUsername, err.Error())
	}
}

// FLUIGCLI_USERNAME resolve a identidade quando não há overlay nem prompt.
func TestEnsureIdentityViaEnv(t *testing.T) {
	t.Setenv(config.EnvUsername, "ci-bot")
	app := &App{}
	server := &config.Server{Name: "homolog", Host: "h", Port: 80}
	if err := app.ensureIdentity(server); err != nil {
		t.Fatal(err)
	}
	if server.Username != "ci-bot" || server.UserCode != "ci-bot" {
		t.Errorf("identidade via env não aplicada: %+v", server)
	}
}
