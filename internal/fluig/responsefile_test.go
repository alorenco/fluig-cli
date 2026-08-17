package fluig

import (
	"net/http"
	"testing"
)

// responseFileName lê o nome do arquivo físico do Content-Disposition. O Fluig
// manda o nome SEM aspas e com acento em CP-1252, então o parse tolerante e a
// decodificação são obrigatórios (relato de 2026-08-17).
func TestResponseFileName(t *testing.T) {
	casos := []struct{ nome, header, quer string }{
		{"entre aspas", `attachment; filename="manual.pdf"`, "manual.pdf"},
		{"sem aspas e com espaço", "attachment; filename=Aditivo No. 9387.pdf", "Aditivo No. 9387.pdf"},
		{"acento em CP-1252", "attachment; filename=Renegocia\xe7\xe3o.pdf", "Renegociação.pdf"},
		{"RFC 2231 em UTF-8", `attachment; filename*=UTF-8''Renegocia%C3%A7%C3%A3o.pdf`, "Renegociação.pdf"},
		{"nome cru com barra", `attachment; filename="Nov/24."`, "Nov/24."},
		{"sem nome", "inline", ""},
		{"header ausente", "", ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			h := http.Header{}
			if c.header != "" {
				h.Set("Content-Disposition", c.header)
			}
			if got := responseFileName(h); got != c.quer {
				t.Errorf("= %q, quer %q", got, c.quer)
			}
		})
	}
}
