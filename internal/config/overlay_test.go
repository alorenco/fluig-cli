package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readJSON lê e desserializa um arquivo JSON num destino, falhando o teste.
func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("json %s: %v", path, err)
	}
}

// Add em projeto separa conexão (servers.json, versionável) de identidade
// (servers.local.json, pessoal): o arquivo do time não pode conter username.
func TestAddProjetoSeparaIdentidade(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := NewStore(proj)

	in := Server{Name: "homolog", Host: "h", Port: 8080, Username: "alorenco", CompanyID: 1, Env: EnvHml}
	if err := st.Add(in, false); err != nil {
		t.Fatal(err)
	}

	var shared ServersFile
	readJSON(t, filepath.Join(proj, ".fluigcli", "servers.json"), &shared)
	if len(shared.Servers) != 1 {
		t.Fatalf("servers compartilhados = %d, quer 1", len(shared.Servers))
	}
	if s := shared.Servers[0]; s.Username != "" || s.UserCode != "" || s.ID != "" {
		t.Errorf("arquivo do time não pode conter identidade: %+v", s)
	}
	if shared.Servers[0].Host != "h" || shared.Servers[0].CompanyID != 1 {
		t.Errorf("conexão não persistida corretamente: %+v", shared.Servers[0])
	}

	var local LocalFile
	readJSON(t, filepath.Join(proj, ".fluigcli", "servers.local.json"), &local)
	if len(local.Identities) != 1 || local.Identities[0].Username != "alorenco" {
		t.Errorf("identidade não foi para o overlay local: %+v", local)
	}

	// List reconstrói o servidor efetivo com a identidade do overlay.
	servers, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Username != "alorenco" || servers[0].UserCode != "alorenco" {
		t.Errorf("List não aplicou o overlay de identidade: %+v", servers)
	}
}

// O padrão do projeto é pessoal: SetDefault grava no overlay local, nunca no
// servers.json versionável (senão o padrão de um vazaria para o time).
func TestSetDefaultProjetoVaiParaLocal(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := NewStore(proj)
	if err := st.Add(Server{Name: "homolog", Host: "h", Port: 80, Username: "u", CompanyID: 1}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.Add(Server{Name: "producao", Host: "p", Port: 443, SSL: true, Username: "u", CompanyID: 1}, false); err != nil {
		t.Fatal(err)
	}

	path, err := st.SetDefault("producao", false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "servers.local.json" {
		t.Errorf("padrão do projeto deveria ir para servers.local.json, foi para %s", path)
	}

	var shared ServersFile
	readJSON(t, filepath.Join(proj, ".fluigcli", "servers.json"), &shared)
	if shared.Default != "" {
		t.Errorf("servers.json versionável não pode carregar o padrão pessoal: %q", shared.Default)
	}
	if def, _ := st.DefaultName(); def != "producao" {
		t.Errorf("DefaultName = %q, quer producao", def)
	}
}

// Remove limpa a identidade e o padrão órfãos do overlay local.
func TestRemoveLimpaOverlayLocal(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := NewStore(proj)
	if err := st.Add(Server{Name: "homolog", Host: "h", Port: 80, Username: "u", CompanyID: 1}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetDefault("homolog", false); err != nil {
		t.Fatal(err)
	}

	removed, err := st.Remove("homolog")
	if err != nil {
		t.Fatal(err)
	}
	// O servidor removido volta com identidade (para o chamador limpar o keyring).
	if removed.Username != "u" || removed.KeyringKey() == "" {
		t.Errorf("removed deveria ter identidade para o keyring: %+v", removed)
	}

	var local LocalFile
	readJSON(t, filepath.Join(proj, ".fluigcli", "servers.local.json"), &local)
	if local.Default != "" || len(local.Identities) != 0 {
		t.Errorf("overlay local deveria ficar limpo após remover: %+v", local)
	}
}

// A identidade pode ter usuários diferentes por servidor no mesmo projeto.
func TestSetIdentityPorServidor(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := NewStore(proj)
	if err := st.Add(Server{Name: "homolog", Host: "h", Port: 80, CompanyID: 1}, false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIdentity("homolog", "maria", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIdentity("homolog", "joao", ""); err != nil { // sobrescreve
		t.Fatal(err)
	}
	got, err := st.Get("homolog")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "joao" {
		t.Errorf("username = %q, quer joao (última identidade vence)", got.Username)
	}
}

// KeyringKey deriva de baseURL+usuário e é vazia sem usuário resolvido.
func TestKeyringKey(t *testing.T) {
	s := Server{Name: "x", Host: "fluig.acme", Port: 443, SSL: true, Username: "ana"}
	if got, want := s.KeyringKey(), "https://fluig.acme|ana"; got != want {
		t.Errorf("KeyringKey = %q, quer %q", got, want)
	}
	if (&Server{Host: "h"}).KeyringKey() != "" {
		t.Error("KeyringKey sem usuário deveria ser vazia")
	}
}
