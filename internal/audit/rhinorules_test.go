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

// rhinoES6Count conta só os achados RHINO002 (ES6 não suportado) num trecho.
func rhinoES6Count(t *testing.T, src string) int {
	t.Helper()
	return countRule(rhinoFindings("datasets/ds_x.js", []byte(src)), RuleRhinoES6)
}

func TestRhinoB3NaoSuportado(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		// class
		{"class declaração", `class Pedido { }`, 1},
		{"class extends", `class Pedido extends Base {}`, 1},
		{"class anônima", `var C = class { };`, 1},
		// import/export
		{"import", `import x from 'y';`, 1},
		{"export const", `export const A = 1;`, 1},
		{"export default", `export default foo;`, 1},
		// async/await
		{"async function", `async function f() {}`, 1},
		{"async arrow", `var f = async () => 1;`, 1},
		{"await", `var x = await p;`, 1},
		// parâmetro default
		{"default em function", `function f(x, y = 10) {}`, 1},
		{"default em arrow", `var f = (a, b = 2) => a + b;`, 1},
		{"default com string", `function f(nome = "joão") {}`, 1},
		// spread
		{"spread em array com vírgula", `var a = [...b, 3];`, 1},
		{"spread em chamada com vírgula", `f(...args, extra);`, 1},
		// propriedade computada
		{"propriedade computada", `var o = { [chave]: 1 };`, 1},
		{"computada após vírgula", `var o = { a: 1, [k]: 2 };`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rhinoES6Count(t, tc.src); got != tc.want {
				t.Errorf("%s: got %d achados RHINO002, quer %d", tc.name, got, tc.want)
			}
		})
	}
}

// Recursos que o Rhino do Voyager 2 SUPORTA não podem virar achado (falso-
// positivo destrói a confiança na regra).
func TestRhinoB3Suportado(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"template literal", "var s = `oi ${nome}`;"},
		{"let/const", `let a = 1; const b = 2;`},
		{"arrow simples", `var f = (a, b) => a + b;`},
		{"for...of", `for (var x of lista) { log(x); }`},
		{"destructuring", `var { a, b } = obj; var [c, d] = arr;`},
		{"rest param em função", `function f(a, ...args) { return args; }`},
		{"rest em destructuring", `var [a, ...resto] = arr;`},
		{"Map/Set", `var m = new Map(); var s = new Set();`},
		{"Array.includes/find", `if (arr.includes(3)) {} arr.find(function (x) { return x; });`},
		{"String.padStart", `var z = String(n).padStart(2, '0');`},
		{"comparação não é default", `function f(a) { if (a == 1) return; }`},
		{"class dentro de string", `var msg = "use class aqui";`},
		{"classList não é class", `el.classList.add('x');`},
		{"module.exports não é export", `module.exports = foo;`},
		{"await como propriedade", `var v = obj.await;`},
		{"index de array não é computada", `var v = obj[chave];`},
		{"objeto com array-valor", `var o = { lista: [1, 2] };`},
		{"rest não é spread (sem vírgula depois)", `var a = [...b];`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rhinoES6Count(t, tc.src); got != 0 {
				t.Errorf("%s: got %d achados RHINO002, quer 0", tc.name, got)
			}
		})
	}
}

// rhinoConstCount conta só os achados RHINO003 (const em laço) num trecho.
func rhinoConstCount(t *testing.T, src string) int {
	t.Helper()
	return countRule(rhinoFindings("datasets/ds_x.js", []byte(src)), RuleConstInLoop)
}

func TestRhinoB2ConstEmLaco(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		// Caso-guia validado na homolog (dataset-canário).
		{"for com const no corpo", `for (var i = 0; i < 2; i++) { const x = i * 10; }`, 1},
		{"while com const", `while (cond) { const y = calc(); use(y); }`, 1},
		{"do-while com const", `do { const z = 1; } while (cond);`, 1},
		{"const em bloco (if) dentro do laço", `for (;;) { if (a) { const w = 1; } }`, 1},
		{"for-of com const no corpo", `for (var it of lista) { const v = it.id; }`, 1},
		{"dois const no mesmo laço, linhas diferentes",
			"for (var i = 0; i < 2; i++) {\n  const a = i;\n  const b = i + 1;\n}", 2},
		{"laço aninhado", `for (var i=0;i<2;i++){ for (var j=0;j<2;j++){ const k = j; } }`, 1},
		// Negativos.
		{"let no laço não conta", `for (var i = 0; i < 2; i++) { let x = i; }`, 0},
		{"var no laço não conta", `for (var i = 0; i < 2; i++) { var x = i; }`, 0},
		{"const fora do laço não conta", `const TAXA = 0.1; for (var i=0;i<2;i++){ use(TAXA); }`, 0},
		{"const em função aninhada no laço não conta",
			`for (var i=0;i<2;i++){ arr.forEach(function (x) { const y = x; }); }`, 0},
		{"const em arrow aninhada no laço não conta",
			`for (var i=0;i<2;i++){ arr.map(function(x){ return x; }); const OK = 1; use(OK); }`, 1}, // OK está no laço, não na função
		{"const no header de for-of não conta", `for (const item of lista) { use(item); }`, 0},
		{"const no header de for-in não conta", `for (const k in obj) { use(k); }`, 0},
		{"const dentro de string não conta", `for (var i=0;i<2;i++){ log("const x = 1"); }`, 0},
		{"const em comentário não conta", "for (var i=0;i<2;i++){ // const x = 1\n  use(i); }", 0},
		{"const em função que contém laço conta pelo laço interno",
			`function f() { for (var i=0;i<2;i++){ const x = i; } }`, 1},
		{"constants não é const", `for (var i=0;i<2;i++){ var constants = obj[i]; }`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rhinoConstCount(t, tc.src); got != tc.want {
				t.Errorf("%s: got %d achados RHINO003, quer %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestRhinoB2SoServerSide(t *testing.T) {
	src := []byte(`for (var i = 0; i < 2; i++) { const x = i; }`)

	server := scanJS("datasets/ds_x.js", src)
	if n := countRule(server, RuleConstInLoop); n != 1 {
		t.Errorf("dataset (server-side): got %d RHINO003, quer 1", n)
	}

	client := scanJS("wcm/widget/w/src/main/webapp/resources/js/app.js", src)
	if n := countRule(client, RuleConstInLoop); n != 0 {
		t.Errorf("widget (client-side): got %d RHINO003, quer 0", n)
	}
}

// Como o RHINO001, o B3 só vale no JS server-side (Rhino). No navegador ES6 é
// normal — scanJS não deve rodar RHINO002 fora do server-side.
func TestRhinoB3SoServerSide(t *testing.T) {
	src := []byte(`class Foo {} import x from 'y';`)

	server := scanJS("datasets/ds_x.js", src)
	if n := countRule(server, RuleRhinoES6); n != 2 {
		t.Errorf("dataset (server-side): got %d RHINO002, quer 2", n)
	}

	client := scanJS("wcm/widget/w/src/main/webapp/resources/js/app.js", src)
	if n := countRule(client, RuleRhinoES6); n != 0 {
		t.Errorf("widget (client-side): got %d RHINO002, quer 0", n)
	}
}
