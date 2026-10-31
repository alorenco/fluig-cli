package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tests/unit/ e os *.unit.json são do test unit: o test run não os lê.
func TestDefaultSuitesIgnoraUnit(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{
		"tests/compras/compras.json",
		"tests/compras/fixtures/card.json",
		"tests/compras/regras.unit.json",
		"tests/unit/pagamentos.unit.json",
		"tests/unit/outra.json",
	} {
		full := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("{}"), 0o644)
	}
	got, err := DefaultSuites(root)
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, g := range got {
		r, _ := filepath.Rel(root, g)
		rel = append(rel, filepath.ToSlash(r))
	}
	if strings.Join(rel, ",") != "tests/compras/compras.json" {
		t.Errorf("suítes: %v", rel)
	}
}
