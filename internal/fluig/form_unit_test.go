package fluig

import "testing"

func TestChoosePrincipalFile(t *testing.T) {
	cases := []struct {
		name       string
		files      []string
		candidates []string
		want       string
	}{
		{"único html", []string{"form.js", "pagina.html", "estilo.css"}, []string{"qualquer"}, "pagina.html"},
		{"nenhum html", []string{"form.js", "estilo.css"}, nil, ""},
		{"vários html casa pasta", []string{"a.html", "frm_x.html"}, []string{"frm_x"}, "frm_x.html"},
		{"vários html casa nome do form", []string{"a.html", "b.html"}, []string{"nao", "b"}, "b.html"},
		{"vários html sem casar → primeiro", []string{"a.html", "b.html"}, []string{"nada"}, "a.html"},
		{"htm também conta", []string{"index.htm"}, nil, "index.htm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ChoosePrincipalFile(tc.files, tc.candidates...); got != tc.want {
				t.Errorf("ChoosePrincipalFile(%v, %v) = %q, quer %q", tc.files, tc.candidates, got, tc.want)
			}
		})
	}
}
