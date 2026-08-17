package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeFileName(t *testing.T) {
	casos := []struct{ nome, entrada, quer string }{
		{"nome simples", "manual.pdf", "manual.pdf"},
		{
			"barra na descrição (caso real de 2026-08-17)",
			"Aditivo de Renegociação de Contrato - 9387 - Cancelado em Nov/24.",
			"Aditivo de Renegociação de Contrato - 9387 - Cancelado em Nov_24",
		},
		{"barra invertida e dois-pontos", `C:\pasta\arquivo.txt`, "C__pasta_arquivo.txt"},
		{"reservados do Windows", `a*b?c"d<e>f|g.txt`, "a_b_c_d_e_f_g.txt"},
		{"controle é descartado", "linha\nquebrada.txt", "linhaquebrada.txt"},
		{"espaço e ponto no fim", "relatorio.  ", "relatorio"},
		{"só pontos", "...", ""},
		{"vazio", "", ""},
		{"nome reservado do Windows", "NUL.txt", "_NUL.txt"},
		{"nome reservado sem extensão", "con", "_con"},
		{"traversal vira nome comum", "../../etc/passwd", ".._.._etc_passwd"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := SafeFileName(c.entrada); got != c.quer {
				t.Errorf("SafeFileName(%q) = %q, quer %q", c.entrada, got, c.quer)
			}
		})
	}
}

// O resultado nunca escapa do diretório de destino nem cria subpasta: é isso
// que faz o os.WriteFile funcionar sem MkdirAll do nome.
func TestSafeFileNameNuncaTemSeparador(t *testing.T) {
	for _, entrada := range []string{"a/b", `a\b`, "../x", "/etc/passwd", "Nov/24."} {
		got := SafeFileName(entrada)
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("SafeFileName(%q) = %q, ainda tem separador", entrada, got)
		}
		if got == "" {
			continue
		}
		if _, err := SafeJoin(t.TempDir(), got); err != nil {
			t.Errorf("SafeJoin recusou %q (de %q): %v", got, entrada, err)
		}
	}
}

// Nome muito longo é cortado no limite de bytes, sem partir caractere acentuado
// e mantendo a extensão.
func TestSafeFileNameCortaNomeLongo(t *testing.T) {
	longo := strings.Repeat("ç", 300) + ".pdf"
	got := SafeFileName(longo)
	if len(got) > maxFileNameBytes {
		t.Errorf("nome com %d bytes, limite é %d", len(got), maxFileNameBytes)
	}
	if filepath.Ext(got) != ".pdf" {
		t.Errorf("extensão perdida: %q", got)
	}
	for _, r := range got {
		if r == '\uFFFD' {
			t.Fatalf("corte partiu um caractere multibyte: %q", got)
		}
	}
}
