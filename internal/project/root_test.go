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
}

func mkdir(t *testing.T, root string, rel string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
