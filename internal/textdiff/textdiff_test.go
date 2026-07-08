package textdiff

import (
	"fmt"
	"strings"
	"testing"
)

func TestUnifiedIguais(t *testing.T) {
	if got := Unified("a", "b", "x\ny\n", "x\ny\n"); got != "" {
		t.Errorf("textos iguais deveriam dar diff vazio, veio:\n%s", got)
	}
}

func TestUnifiedMudancaSimples(t *testing.T) {
	a := "l1\nl2\nl3\n"
	b := "l1\nlX\nl3\n"
	got := Unified("servidor:ds", "local:ds.js", a, b)
	want := "--- servidor:ds\n+++ local:ds.js\n@@ -1,3 +1,3 @@\n l1\n-l2\n+lX\n l3\n"
	if got != want {
		t.Errorf("diff =\n%q\nquer:\n%q", got, want)
	}
}

func TestUnifiedDoisHunks(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&a, "linha%d\n", i)
		if i == 2 || i == 27 {
			fmt.Fprintf(&b, "linha%d mudada\n", i)
		} else {
			fmt.Fprintf(&b, "linha%d\n", i)
		}
	}
	got := Unified("a", "b", a.String(), b.String())
	if strings.Count(got, "@@") != 4 { // 2 hunks × marcador duplo por cabeçalho @@
		t.Errorf("esperava 2 hunks separados, veio:\n%s", got)
	}
	if !strings.Contains(got, "-linha2\n+linha2 mudada\n") || !strings.Contains(got, "-linha27\n+linha27 mudada\n") {
		t.Errorf("hunks deveriam trocar as linhas 2 e 27:\n%s", got)
	}
}

func TestUnifiedAdicaoERemocao(t *testing.T) {
	got := Unified("a", "b", "x\n", "x\nnova\n")
	if !strings.Contains(got, "+nova") {
		t.Errorf("adição não apareceu:\n%s", got)
	}
	got = Unified("a", "b", "x\nvelha\n", "x\n")
	if !strings.Contains(got, "-velha") {
		t.Errorf("remoção não apareceu:\n%s", got)
	}
}

func TestUnifiedArquivoGrandeNaoExplode(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 10000; i++ {
		a.WriteString("mesma\n")
		b.WriteString("outra\n")
	}
	got := Unified("a", "b", a.String(), b.String())
	if !strings.Contains(got, "-mesma") || !strings.Contains(got, "+outra") {
		t.Errorf("fallback de arquivo grande deveria emitir bloco único")
	}
}
