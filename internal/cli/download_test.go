package cli

import (
	"path/filepath"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func TestExtensionForMime(t *testing.T) {
	casos := map[string]string{
		"application/pdf":                                                   ".pdf",
		"application/pdf; charset=UTF-8":                                    ".pdf", // tolera o parâmetro do header
		"APPLICATION/PDF":                                                   ".pdf",
		"image/jpeg":                                                        ".jpg", // não ".jfif"
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
		"application/octet-stream":                                          "",
		"":                                                                  "",
		"tipo/inventado":                                                    "",
	}
	for mimeType, quer := range casos {
		if got := extensionForMime(mimeType); got != quer {
			t.Errorf("extensionForMime(%q) = %q, quer %q", mimeType, got, quer)
		}
	}
}

// O critério de "já tem extensão" precisa recusar o ponto decimal que aparece
// na descrição do documento ("Contrato 12.056").
func TestHasFileExtension(t *testing.T) {
	casos := map[string]bool{
		"manual.pdf":      true,
		"pacote.7z":       true,
		"Contrato 12.056": false,
		"Aditivo 9387":    false,
		"backup.tar.gz":   true,
		"nome.":           false,
		"nome.extensao":   false, // longa demais para ser extensão
	}
	for nome, quer := range casos {
		if got := hasFileExtension(nome); got != quer {
			t.Errorf("hasFileExtension(%q) = %v, quer %v", nome, got, quer)
		}
	}
}

func TestResolveDownloadName(t *testing.T) {
	casos := []struct {
		nome, tmpl, physical, description, mimeType, quer string
	}{
		{"nome físico ganha da descrição", "", "arquivo.pdf", "Descrição", "application/pdf", "arquivo.pdf"},
		{"sem nome físico, usa a descrição", "", "", "manual.pdf", "application/pdf", "manual.pdf"},
		{"barra da descrição vira _", "", "", "Nov/24.", "application/pdf", "Nov_24.pdf"},
		{"extensão vem do mime", "", "", "relatorio", "application/pdf", "relatorio.pdf"},
		{"mime genérico não inventa extensão", "", "", "dados", "application/octet-stream", "dados"},
		{"sem nome nenhum, usa o id", "", "", "", "application/pdf", "documento_42.pdf"},
		{"template com id e nome", "{id}_{name}{ext}", "manual.pdf", "", "application/pdf", "42_manual.pdf"},
		{"template com subpasta", "{id}/{fileName}", "manual.pdf", "", "application/pdf", "42/manual.pdf"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got, err := resolveDownloadName(c.tmpl, 42, c.physical, c.description, c.mimeType)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != filepath.ToSlash(c.quer) {
				t.Errorf("= %q, quer %q", got, c.quer)
			}
		})
	}
}

// Template inválido é USAGE_ERROR: gravar "{foo}" no nome seria pior.
func TestResolveDownloadNameTemplateInvalido(t *testing.T) {
	for _, tmpl := range []string{"{versao}", "{id", "  "} {
		_, err := resolveDownloadName(tmpl, 42, "manual.pdf", "", "application/pdf")
		if err == nil {
			t.Fatalf("template %q deveria ser recusado", tmpl)
		}
		if code := output.AsError(err).Code; code != output.CodeUsage {
			t.Errorf("template %q: código %s, quer %s", tmpl, code, output.CodeUsage)
		}
	}
}

func TestNameGuardUnique(t *testing.T) {
	g := nameGuard{}
	if got := g.unique("/tmp/a.pdf"); got != "/tmp/a.pdf" {
		t.Errorf("primeiro = %q", got)
	}
	if got := g.unique("/tmp/a.pdf"); got != "/tmp/a (2).pdf" {
		t.Errorf("segundo = %q, quer /tmp/a (2).pdf", got)
	}
	if got := g.unique("/tmp/a.pdf"); got != "/tmp/a (3).pdf" {
		t.Errorf("terceiro = %q, quer /tmp/a (3).pdf", got)
	}
	// A comparação ignora caixa (Windows e macOS tratam como o mesmo arquivo):
	// "/tmp/A.PDF" colide com os três nomes já usados e pega o sufixo 4.
	if got := g.unique("/tmp/A.PDF"); got != "/tmp/A (4).PDF" {
		t.Errorf("caixa diferente = %q, quer /tmp/A (4).PDF", got)
	}
	// Sem extensão o sufixo vai no fim.
	if g.unique("/tmp/b") != "/tmp/b" || g.unique("/tmp/b") != "/tmp/b (2)" {
		t.Error("sufixo sem extensão incorreto")
	}
}
