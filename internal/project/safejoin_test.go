package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	base := filepath.FromSlash("/tmp/proj/datasets")

	// Casos seguros (ficam dentro de base).
	ok := []struct {
		elems []string
		want  string
	}{
		{[]string{"ds_clientes.js"}, filepath.FromSlash("/tmp/proj/datasets/ds_clientes.js")},
		{[]string{"sub", "a.js"}, filepath.FromSlash("/tmp/proj/datasets/sub/a.js")},
		{[]string{"Formulário de Compras.html"}, filepath.FromSlash("/tmp/proj/datasets/Formulário de Compras.html")},
	}
	for _, tc := range ok {
		got, err := SafeJoin(base, tc.elems...)
		if err != nil || got != tc.want {
			t.Errorf("SafeJoin(%v) = (%q, %v), quer (%q, nil)", tc.elems, got, err, tc.want)
		}
	}

	// Casos de ataque (traversal) — devem falhar.
	bad := [][]string{
		{"../evil.js"},
		{"../../etc/passwd"},
		{"a", "..", "..", "..", "evil"},
		{filepath.FromSlash("../../../home/ubuntu/.ssh/authorized_keys")},
		{"."},
		{".."},
	}
	for _, elems := range bad {
		if got, err := SafeJoin(base, elems...); err == nil {
			t.Errorf("SafeJoin(%v) deveria falhar (traversal), mas devolveu %q", elems, got)
		}
	}
}

// Garante que a mensagem de erro não vaza o base completo desnecessariamente.
func TestSafeJoinErrorMentionsSegment(t *testing.T) {
	_, err := SafeJoin("/tmp/x", "../y")
	if err == nil || !strings.Contains(err.Error(), "inseguro") {
		t.Errorf("erro inesperado: %v", err)
	}
}
