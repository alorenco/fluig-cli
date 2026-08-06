package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRoot(t *testing.T) {
	t.Run("acha pela pasta .fluigcli num ancestral", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, root, ".fluigcli")
		nested := mkdir(t, root, "src/qualquer/coisa")
		if got := FindRoot(nested); got != root {
			t.Errorf("FindRoot(%q) = %q, quer %q", nested, got, root)
		}
	})

	t.Run("acha pela pasta convencional", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, root, "datasets")
		if got := FindRoot(root); got != root {
			t.Errorf("FindRoot = %q, quer %q", root, root)
		}
	})

	t.Run("arquivo com nome de pasta convencional não conta", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "datasets"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := FindRoot(dir); got == dir {
			t.Errorf("arquivo regular não deveria caracterizar raiz de projeto")
		}
	})

	t.Run("sem projeto retorna vazio", func(t *testing.T) {
		if got := FindRoot(t.TempDir()); got != "" {
			t.Errorf("FindRoot = %q, quer \"\"", got)
		}
	})

	// Regressão (2026-08-06): toda pasta de formulário tem uma subpasta
	// events/, que também é pasta convencional de projeto. A descoberta parava
	// na pasta do formulário e a CLI procurava o servers.json lá.
	t.Run("pasta de formulário não é raiz (tem events/ dentro)", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, root, ".fluigcli")
		form := mkdir(t, root, "forms/frm_notificacoes")
		mkdir(t, root, "forms/frm_notificacoes/events")
		if got := FindRoot(form); got != root {
			t.Errorf("FindRoot(%q) = %q, quer a raiz %q", form, got, root)
		}
	})

	t.Run("de dentro da events/ do formulário também acha a raiz", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, root, ".fluigcli")
		events := mkdir(t, root, "forms/frm_notificacoes/events")
		if got := FindRoot(events); got != root {
			t.Errorf("FindRoot(%q) = %q, quer a raiz %q", events, got, root)
		}
	})

	// Sem .fluigcli/ (projeto recém-clonado, antes do server add) a heurística
	// das pastas convencionais é a única pista — e ela também não pode parar na
	// pasta do formulário.
	t.Run("projeto sem .fluigcli: pasta de formulário ainda não é raiz", func(t *testing.T) {
		root := t.TempDir()
		form := mkdir(t, root, "forms/frm_notificacoes")
		mkdir(t, root, "forms/frm_notificacoes/events")
		mkdir(t, root, "datasets")
		if got := FindRoot(form); got != root {
			t.Errorf("FindRoot(%q) = %q, quer a raiz %q", form, got, root)
		}
	})

	// O marcador forte vence a heurística: pasta convencional solta no meio do
	// caminho não sequestra a descoberta.
	t.Run(".fluigcli do projeto vence pasta convencional intermediária", func(t *testing.T) {
		root := t.TempDir()
		mkdir(t, root, ".fluigcli")
		fundo := mkdir(t, root, "sub/wcm/a/b")
		if got := FindRoot(fundo); got != root {
			t.Errorf("FindRoot(%q) = %q, quer a raiz %q", fundo, got, root)
		}
	})
}

func mkdir(t *testing.T, root string, rel string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
