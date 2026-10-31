package testrun

import (
	"encoding/json"
	"strconv"
)

// Sondas do --self-check (SPEC-TEST-D §6). O esperado é o comportamento
// MEDIDO no servidor Fluig Voyager 2: o canário ES6 de 2026-07-22 e o da
// String Java de 2026-07-30 (projeto Fluig), mais os defeitos confirmados no
// Gate D e no canário D0.4-b (2026-10-31). Um motor que diverge aqui dá falso positivo no teste
// de unidade.
type selfProbe struct {
	Name, Src, Want string
}

var selfProbes = []selfProbe{
	{"let/const", "(function(){ let a = 1; const b = 2; return a + b; })()", "3"},
	{"template literal", "(function(){ var x = 2; return `v=${x}`; })()", "v=2"},
	{"arrow function", "[1, 2].map(x => x * 2).join(',')", "2,4"},
	{"for…of", "(function(){ var s = 0; for (var v of [1, 2, 3]) s += v; return s; })()", "6"},
	{"destructuring", "(function(){ var {p} = {p: 1}; var [m] = [2]; return p + m; })()", "3"},
	{"parâmetro rest", "(function(...r){ return r.length; })(1, 2, 3)", "3"},
	{"generator", "(function*(){ yield 1; })().next().value", "1"},
	{"Promise existe", "typeof Promise", "function"},
	{"Map/Set", "new Map().set('a', 1).get('a') + new Set([1, 1]).size", "2"},
	{"Array.includes/find", "[1, 2].includes(2) + ',' + [1, 2].find(x => x > 1)", "true,2"},
	{"padStart/startsWith/repeat", "'5'.padStart(3, '0') + 'a'.repeat(2) + 'ab'.startsWith('a')", "005aatrue"},
	{"Object.assign", "Object.assign({}, {z: 1}).z", "1"},
	// Medida no canário D0.4-b (2026-10-31): o shorthand SOZINHO funciona no
	// servidor; o computed não (sonda abaixo, junto com o shorthand).
	{"shorthand sozinho", "(function(){ var valor = 1; return ({ valor }).valor; })()", "1"},
	{"SyntaxError: parâmetro default", "(function(x, y = 10){ return y; })(1)", "E"},
	{"SyntaxError: spread em array", "(function(){ var a = [1]; return [...a, 3].length; })()", "E"},
	{"SyntaxError: shorthand + computed", "(function(){ var valor = 1, chave = 'c'; return { valor, [chave]: 1 }; })()", "E"},
	{"SyntaxError: class", "(function(){ class A {} return 1; })()", "E"},
	{"defeito: const no corpo de laço", "(function(){ var o = ''; for (var i = 0; i < 3; i++) { const x = i; o += x; } return o; })()", "000"},
	{"defeito: const homônimo em blocos irmãos", "(function(){ if (true) { const y = 1; } else { const y = 2; } return 1; })()", "E"},
	{"defeito: String Java vazia é truthy", "(function(){ var s = new java.lang.String(''); return s ? 'truthy' : 'falsy'; })()", "truthy"},
	{"defeito: String Java === texto", "(function(){ var s = new java.lang.String('a'); return (s === 'a') + ',' + (s == 'a') + ',' + typeof s; })()", "false,true,object"},
}

const selfProbeScript = "function __fluigcliProbe(src) {\n" +
	"    try { return String(eval(src)); } catch (e) { return 'E'; }\n" +
	"}\n"

// SelfCheckFile monta o arquivo de teste em memória do --self-check.
func SelfCheckFile() *UnitFile {
	f := &UnitFile{Path: "(self-check)", Description: "perfil do Rhino do servidor Fluig Voyager 2"}
	for _, p := range selfProbes {
		arg, _ := json.Marshal(p.Src)
		f.Cases = append(f.Cases, &UnitCase{
			Name:    p.Name,
			Scripts: []ScriptRef{{Name: "fluigcli-self-check.js", Source: selfProbeScript}},
			Call:    "__fluigcliProbe",
			Args:    []json.RawMessage{arg},
			Expect:  &UnitExpect{Return: json.RawMessage(strconv.Quote(p.Want)), HasReturn: true},
		})
	}
	return f
}
