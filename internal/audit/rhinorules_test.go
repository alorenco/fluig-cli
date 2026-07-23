package audit

import "testing"

func rhinoCount(t *testing.T, src string) int {
	t.Helper()
	return len(rhinoFindings("datasets/ds_x.js", []byte(src)))
}

func TestRhinoB1Direto(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"chamada direta ===", `if (constraints[i].getFieldName() === 'codcoligada') {}`, 1},
		{"chamada direta encadeada", `if (c.getFieldName().toLowerCase() === 'x') {}`, 1},
		{"!== também", `if (c.getInitialValue() !== 'S') {}`, 1},
		{"espelho literal à esquerda", `if ('x' === c.getFieldName()) {}`, 1},
		{"getString de ResultSet", `var v = 0; if (rs.getString('col') === 'A') {}`, 1},
		{"igualdade solta não conta", `if (c.getFieldName() == 'codcoligada') {}`, 0},
		{"getValue fica de fora (ambíguo)", `if (ds.getValue(0, 'c') === 'A') {}`, 0},
		{"comparação com número não conta", `if (c.getFinalValue() === 3) {}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rhinoCount(t, tc.src); got != tc.want {
				t.Errorf("%s: got %d achados, quer %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestRhinoB1Variavel(t *testing.T) {
	// Caso-guia (ds_fin_flan_fluig PRÉ-correção): variável recebe o
	// java.lang.String e é comparada com ===.
	preFix := `
function createDataset(fields, constraints, sortFields) {
  for (var i = 0; i < constraints.length; i++) {
    var campo = constraints[i].getFieldName().toLowerCase();
    if (campo === 'codcoligada') { return 1; }
  }
}`
	if got := rhinoCount(t, preFix); got != 1 {
		t.Fatalf("pré-correção: got %d, quer 1", got)
	}

	// Caso-guia CORRIGIDO (o do arquivo real): String(...) coage para JS.
	postFix := `
function createDataset(fields, constraints, sortFields) {
  for (var i = 0; i < constraints.length; i++) {
    var campo = String(constraints[i].getFieldName()).toLowerCase();
    if (campo === 'codcoligada') { return 1; }
  }
}`
	if got := rhinoCount(t, postFix); got != 0 {
		t.Fatalf("pós-correção (String wrap): got %d, quer 0", got)
	}

	// Concatenação também coage (string JS).
	concat := `
var s = rs.getString('c') + '';
if (s === 'A') {}`
	if got := rhinoCount(t, concat); got != 0 {
		t.Errorf("concatenação: got %d, quer 0", got)
	}

	// Reatribuição segura tira a variável do conjunto (conservador).
	reassign := `
var campo = c.getFieldName();
var campo = 'literal';
if (campo === 'x') {}`
	if got := rhinoCount(t, reassign); got != 0 {
		t.Errorf("reatribuição segura: got %d, quer 0", got)
	}

	// Variável comum (não-java) comparada com literal: nada.
	comum := `
var acao = 'UPSERT';
if (acao === 'UPSERT') {}`
	if got := rhinoCount(t, comum); got != 0 {
		t.Errorf("variável comum: got %d, quer 0", got)
	}
}

// A regra só vale no JS server-side (Rhino). No navegador não existe
// java.lang.String — scanJS não deve rodar RHINO* fora do server-side.
func TestRhinoB1SoServerSide(t *testing.T) {
	src := []byte(`if (c.getFieldName() === 'x') {}`)

	server := scanJS("datasets/ds_x.js", src)
	if n := countRule(server, RuleJavaStrictEq); n != 1 {
		t.Errorf("dataset (server-side): got %d RHINO001, quer 1", n)
	}

	client := scanJS("wcm/widget/w/src/main/webapp/resources/js/app.js", src)
	if n := countRule(client, RuleJavaStrictEq); n != 0 {
		t.Errorf("widget (client-side): got %d RHINO001, quer 0", n)
	}
}

func countRule(fs []Finding, rule string) int {
	n := 0
	for _, f := range fs {
		if f.Rule == rule {
			n++
		}
	}
	return n
}
