package audit

import (
	"fmt"
	"regexp"
	"strings"
)

// Regras RHINO* — footguns do motor de script do Fluig (Rhino), que a análise
// estática pega bem. Rodam SÓ em JS server-side (datasets, eventos globais,
// mecanismos, scripts de processo e eventos de formulário) — no navegador não
// existe java.lang.String, então não valem lá. Nascem como AVISO (heurística):
// o mantenedor pode elevar a erro via .fluigcli/audit.json.

// RHINO001 (footgun B1) — comparação estrita (=== / !==) entre um valor que é
// java.lang.String (retorno de método Java) e um literal de texto JS. No Rhino
// do Fluig, um java.lang.String NUNCA é === a uma primitiva JS: a comparação é
// sempre false, sem erro. Foi a causa real do falso-negativo do
// ds_fin_flan_fluig (constraints[i].getFieldName() === 'codcoligada' nunca
// casava — o parâmetro ficava nulo).

// javaStringMethods são métodos cujo retorno, em script server-side, é
// java.lang.String (não uma string JS). Lista curada: o fluig.d.ts declara
// vários como `string` (para o IntelliSense), então não dá para extraí-la do
// tipo — ela vem do comportamento real no Rhino. `getValue` fica DE FORA
// (ambíguo demais: todo objeto JS pode ter um). Limitação conhecida.
var javaStringMethods = []string{
	// Constraint (dataset/mecanismo) — o caso clássico do footgun.
	"getFieldName", "getInitialValue", "getFinalValue",
	// Colleague / usuário (SDK).
	"getColleagueName", "getColleagueId", "getMail", "getLogin",
	"getFullName", "getFirstName", "getLastName", "getEmail", "getCode",
	// Valor de campo do formulário (hAPI) e ResultSet/Map (código de banco).
	"getCardValue", "getString",
}

var (
	// Alternação dos métodos-gatilho (montada de javaStringMethods).
	javaMethodAlt = strings.Join(javaStringMethods, "|")

	// Literal de texto JS ('...' ou "..."). Sem tratar aspas escapadas — raro
	// numa comparação de igualdade.
	strLit = `'[^']*'|"[^"]*"`

	// Chamada de um método-gatilho, com encadeamento opcional de métodos de
	// string que PRESERVAM o java.lang.String (toLowerCase/trim/substring…):
	//   x.getFieldName()            e   x.getFieldName().toLowerCase().trim()
	javaCall = `(?:` + javaMethodAlt + `)\s*\([^()]*\)(?:\s*\.\s*\w+\s*\([^()]*\))*`

	// Comparação DIRETA: chamada Java  ===|!==  literal (e o espelho).
	directEqRe       = regexp.MustCompile(`\.\s*` + javaCall + `\s*(===|!==)\s*(?:` + strLit + `)`)
	directEqMirrorRe = regexp.MustCompile(`(?:` + strLit + `)\s*(===|!==)\s*(?:[\w)\]]\s*\.\s*)` + javaCall)

	// Declaração de variável local.
	declRe = regexp.MustCompile(`\b(?:var|let|const)\s+(\w+)\s*=\s*(.*)`)
	// Chamada de um método-gatilho em qualquer lugar da expressão.
	anyJavaCallRe = regexp.MustCompile(`\.\s*(?:` + javaMethodAlt + `)\s*\(`)

	// Comparação de uma variável (bareword) com literal (e o espelho).
	varEqRe       = regexp.MustCompile(`(^|[^.\w])(\w+)\s*(===|!==)\s*(?:` + strLit + `)`)
	varEqMirrorRe = regexp.MustCompile(`(?:` + strLit + `)\s*(===|!==)\s*(\w+)\b`)
)

// rhinoFindings roda as regras RHINO* sobre um arquivo JS server-side inteiro
// (precisa de visão do arquivo para o rastreio de variáveis).
func rhinoFindings(rel string, content []byte) []Finding {
	lines := strings.Split(string(content), "\n")
	javaVars := collectJavaStringVars(lines)

	var out []Finding
	seen := map[string]bool{} // dedup por linha+trecho
	add := func(n int, key, msg, sug string) {
		k := fmt.Sprintf("%d|%s", n, key)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Finding{
			Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: rel, Line: n,
			Message: msg, Suggestion: sug,
		})
	}

	for i, raw := range lines {
		n := i + 1
		line := stripLineComment(raw)

		// (a) comparação direta com a chamada Java.
		if m := directEqRe.FindStringSubmatch(line); m != nil {
			add(n, "direct:"+m[0], directMsg(m[1]), directSug())
		}
		if m := directEqMirrorRe.FindStringSubmatch(line); m != nil {
			add(n, "direct:"+m[0], directMsg(m[1]), directSug())
		}

		// (b) comparação de uma variável que sabemos ser java.lang.String.
		for _, m := range varEqRe.FindAllStringSubmatch(line, -1) {
			name, op := m[2], m[3]
			if src, ok := javaVars[name]; ok {
				add(n, "var:"+name+":"+op, varMsg(name, src, op), directSug())
			}
		}
		for _, m := range varEqMirrorRe.FindAllStringSubmatch(line, -1) {
			op, name := m[1], m[2]
			if src, ok := javaVars[name]; ok {
				add(n, "var:"+name+":"+op, varMsg(name, src, op), directSug())
			}
		}
	}

	// RHINO002 (footgun B3) — sintaxe ES6+ que o Rhino do Fluig não aceita.
	out = append(out, rhinoES6Findings(rel, lines)...)
	// RHINO003 (footgun B2) — const declarado no corpo de um laço.
	out = append(out, rhinoConstInLoopFindings(rel, content)...)
	return out
}

// RHINO003 (footgun B2) — `const` declarado dentro do corpo de um laço
// (for/while/do). No Rhino do Fluig, o `const` NÃO é reinicializado a cada
// iteração: ele congela o valor da primeira, em silêncio (sem erro). Validado
// na homologação com dataset-canário (2026-07-22): `for (var i…){ const x =
// i*10; }` produziu `0;0;` em vez de `0;10;`. Fix: trocar por `let`. Severidade
// error (bug de dados invisível).
//
// A detecção precisa da estrutura de blocos (um `const` só é footgun quando o
// escopo mais próximo que o encerra é um laço, não uma função). Por isso não é
// regex por linha: um scanner mascara o código-fonte inteiro e mantém uma pilha
// de escopos. Casos tratados de propósito:
//   - `const` numa função aninhada DENTRO do laço NÃO é apontado (a função cria
//     escopo novo a cada chamada — o `const` reinicializa).
//   - `const` no cabeçalho de `for (const x of …)` NÃO é apontado (fica dentro
//     dos parênteses, antes do corpo).
//   - `const` num bloco simples (`if`, `{}`) dentro do laço É apontado (o bloco
//     é reexecutado a cada iteração).
type scopeKind int

const (
	scopeOther scopeKind = iota
	scopeLoop
	scopeFunc
)

func rhinoConstInLoopFindings(rel string, content []byte) []Finding {
	src := maskSource(content)
	var (
		out       []Finding
		stack     []scopeKind
		pending   = scopeOther // tipo do próximo bloco `{` a abrir
		expect    = scopeOther // cabeçalho aguardado (loop/func) até o `(` fechar
		inHeader  bool         // dentro dos parênteses de cabeçalho de for/while/function
		hdrDepth  int
		line      = 1
		seenLines = map[int]bool{}
	)
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '\n' {
			line++
			continue
		}
		if inHeader {
			switch c {
			case '(':
				hdrDepth++
			case ')':
				if hdrDepth--; hdrDepth == 0 {
					inHeader = false
					pending = expect
					expect = scopeOther
				}
			}
			continue
		}
		if isWordByte(c) && (i == 0 || !isWordByte(src[i-1])) {
			word := readWord(src, i)
			switch word {
			case "for", "while":
				expect = scopeLoop
			case "function":
				expect = scopeFunc
			case "do":
				pending = scopeLoop
			case "const":
				if enclosingLoop(stack) && !seenLines[line] {
					seenLines[line] = true
					out = append(out, Finding{
						Rule: RuleConstInLoop, Severity: SeverityError, File: rel, Line: line,
						Message:    "`const` declarado no corpo de um laço — o Rhino do Fluig congela o valor da 1ª iteração (não reinicializa a cada volta), em silêncio",
						Suggestion: "troque por `let` — ou mova a declaração para fora do laço se o valor não muda",
					})
				}
			}
			i += len(word) - 1
			continue
		}
		switch c {
		case '(':
			if expect != scopeOther {
				inHeader = true
				hdrDepth = 1
			}
		case '{':
			stack = append(stack, pending)
			pending = scopeOther
		case '}':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case '=':
			if i+1 < len(src) && src[i+1] == '>' { // arrow function: corpo é escopo de função
				pending = scopeFunc
				i++
			}
		}
	}
	return out
}

// enclosingLoop informa se o escopo mais próximo que encerra a posição é um
// laço (varre a pilha de dentro para fora; blocos "other" são transparentes, um
// escopo de função corta a busca — dentro dele o const é reinicializado).
func enclosingLoop(stack []scopeKind) bool {
	for k := len(stack) - 1; k >= 0; k-- {
		switch stack[k] {
		case scopeLoop:
			return true
		case scopeFunc:
			return false
		}
	}
	return false
}

// maskSource troca por espaço o conteúdo de strings ('...'/"..."/`...`, incl.
// interpolação de template) e de comentários (`//` e `/* */`) do código inteiro,
// preservando quebras de linha e comprimento. Assim o scanner de blocos não vê
// chaves, parênteses nem palavras-chave dentro de texto ou comentário.
func maskSource(content []byte) []byte {
	b := append([]byte(nil), content...)
	const (
		normal = iota
		lineComment
		blockComment
		inString
	)
	state := normal
	var quote byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch state {
		case normal:
			switch {
			case c == '/' && i+1 < len(b) && b[i+1] == '/':
				state = lineComment
				b[i] = ' '
			case c == '/' && i+1 < len(b) && b[i+1] == '*':
				state = blockComment
				b[i] = ' '
			case c == '\'' || c == '"' || c == '`':
				state = inString
				quote = c // preserva o delimitador
			}
		case lineComment:
			if c == '\n' {
				state = normal
			} else {
				b[i] = ' '
			}
		case blockComment:
			if c == '*' && i+1 < len(b) && b[i+1] == '/' {
				b[i], b[i+1] = ' ', ' '
				i++
				state = normal
			} else if c != '\n' {
				b[i] = ' '
			}
		case inString:
			switch {
			case c == '\\' && i+1 < len(b):
				b[i] = ' '
				if b[i+1] != '\n' {
					b[i+1] = ' '
				}
				i++
			case c == quote:
				state = normal // preserva o delimitador de fechamento
			case c != '\n':
				b[i] = ' '
			}
		}
	}
	return b
}

// isWordByte informa se o byte pode compor um identificador JS (ASCII).
func isWordByte(c byte) bool {
	return c == '_' || c == '$' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// readWord devolve o identificador que começa em i (bytes de palavra ASCII).
func readWord(b []byte, i int) string {
	j := i
	for j < len(b) && isWordByte(b[j]) {
		j++
	}
	return string(b[i:j])
}

// RHINO002 (footgun B3) — sintaxe ES6+ que o Rhino do Fluig (Voyager 2) não
// aceita: dá SyntaxError no deploy/execução, o pior lugar para descobrir (o
// dataset "some" ou erra sem mensagem clara). Validado empiricamente na
// homologação com dataset-canário (2026-07-22). O audit trata todo JS
// server-side como Rhino do Voyager 2 (heurística default — não há fonte de
// versão offline). Os recursos SUPORTADOS (template literal, let/const, arrow,
// for...of, destructuring, rest param, Map/Set, Array.includes/find,
// String.padStart) NÃO são apontados. Severidade error: é SyntaxError certo,
// não heurística de tipo como o RHINO001. Limitações conhecidas (indistinguíveis
// de recurso suportado por análise textual, ficam de fora para não gerar
// falso-positivo): propriedade shorthand `{ valor }` (parece bloco/destructuring)
// e spread solitário `[...a]` (igual a rest de destructuring `[...a] = x`).
var (
	// class Foo | class { | class extends — NÃO casa .classList, myclass etc.
	rhinoClassRe = regexp.MustCompile(`(^|[^.\w$])class\b(\s+[A-Za-z_$][\w$]*|\s*\{|\s+extends\b)`)
	// import/export como palavra-chave (módulos ES6). module.exports fica de fora
	// (é `exports`, com s, e vem depois de ponto).
	rhinoModuleRe = regexp.MustCompile(`(^|[^.\w$])(import|export)\b`)
	// async function | async foo( | async ( — funções assíncronas (ES2017).
	rhinoAsyncRe = regexp.MustCompile(`(^|[^.\w$])async\s+(function\b|[A-Za-z_$][\w$]*\s*\(|\()`)
	// await <expr> — NÃO casa .await.
	rhinoAwaitRe = regexp.MustCompile(`(^|[^.\w$])await\s`)
	// spread `...x` seguido de vírgula: inequívoco (rest de param/destructuring é
	// sempre o ÚLTIMO, nunca seguido de vírgula). Casa [...a, 3], f(...a, b).
	rhinoSpreadRe = regexp.MustCompile(`\.\.\.[A-Za-z_$][\w$.\[\]]*\s*,`)
	// propriedade computada `{ [k]: }` / `, [k]:` num objeto literal.
	rhinoComputedRe = regexp.MustCompile(`[{,]\s*\[[^\]\[]+\]\s*:`)
	// parâmetros de function/arrow (para checar valor default).
	rhinoFuncParamsRe  = regexp.MustCompile(`function\b[^(){};]*\(([^()]*)\)`)
	rhinoArrowParamsRe = regexp.MustCompile(`\(([^()]*)\)\s*=>`)
)

type es6Rule struct {
	re  *regexp.Regexp
	key string
	msg string
	sug string
}

func rhinoES6Findings(rel string, lines []string) []Finding {
	var out []Finding
	seen := map[string]bool{} // dedup por linha+construção
	add := func(n int, kind, msg, sug string) {
		k := fmt.Sprintf("%d|%s", n, kind)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Finding{
			Rule: RuleRhinoES6, Severity: SeverityError, File: rel, Line: n,
			Message: msg, Suggestion: sug,
		})
	}
	rules := []es6Rule{
		{rhinoClassRe, "class", "declaração `class` (ES6)", "no Rhino não há `class` — use função construtora + prototype (ES5)"},
		{rhinoModuleRe, "module", "`import`/`export` (módulos ES6)", "o script server-side não tem módulos — use as APIs globais (DatasetFactory, hAPI…) direto"},
		{rhinoAsyncRe, "async", "`async` (ES2017)", "o script server-side é síncrono — remova `async` e trate o retorno direto"},
		{rhinoAwaitRe, "await", "`await` (ES2017)", "o script server-side é síncrono — remova `await` e use o retorno direto"},
		{rhinoSpreadRe, "spread", "spread `...` em array/chamada (ES6)", "use `.concat(...)` ou `.apply(null, arr)` (ES5); rest de parâmetro em função é suportado"},
		{rhinoComputedRe, "computed", "propriedade computada `[expr]:` em objeto literal (ES6)", "crie o objeto e atribua depois: `obj[expr] = valor;`"},
	}
	const es6Suffix = " — o Rhino do Fluig (Voyager 2) não aceita; dá SyntaxError no deploy"
	for i, raw := range lines {
		n := i + 1
		line := maskStrings(raw)
		for _, r := range rules {
			if r.re.MatchString(line) {
				add(n, r.key, r.msg+es6Suffix, r.sug)
			}
		}
		// Parâmetro com valor default (ES6): `=` "solto" dentro de uma lista de
		// parâmetros de function/arrow (exclui ==, ===, !=, >=, <=, =>).
		for _, re := range []*regexp.Regexp{rhinoFuncParamsRe, rhinoArrowParamsRe} {
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				if hasBareAssign(m[1]) {
					add(n, "default", "parâmetro de função com valor default (ES6)"+es6Suffix,
						"atribua no corpo: `if (y == null) { y = 10; }`")
				}
			}
		}
	}
	return out
}

// maskStrings troca o conteúdo de literais de string ('...', "...", `...`) e de
// comentários de linha (//) por espaços, preservando o comprimento. Assim as
// regras de sintaxe RHINO002 não casam palavras-chave dentro de texto (ex.:
// `var s = "class x"`). Não trata comentário de bloco `/* */`.
func maskStrings(line string) string {
	b := []byte(line)
	var quote byte // 0 = fora de string; senão o delimitador ativo
	for i := 0; i < len(b); i++ {
		c := b[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(b) {
				b[i], b[i+1] = ' ', ' '
				i++
				continue
			}
			if c == quote {
				quote = 0 // preserva o delimitador de fechamento
				continue
			}
			b[i] = ' '
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c // preserva o delimitador de abertura
		case '/':
			if i+1 < len(b) && b[i+1] == '/' {
				for j := i; j < len(b); j++ {
					b[j] = ' '
				}
				return string(b)
			}
		}
	}
	return string(b)
}

// hasBareAssign informa se há um `=` de atribuição no trecho — descartando os
// operadores de comparação (==, ===, !=, >=, <=) e a seta de arrow (=>).
func hasBareAssign(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '=' {
			continue
		}
		var prev, next byte
		if i > 0 {
			prev = s[i-1]
		}
		if i+1 < len(s) {
			next = s[i+1]
		}
		if prev == '=' || prev == '!' || prev == '<' || prev == '>' {
			continue
		}
		if next == '=' || next == '>' {
			continue
		}
		return true
	}
	return false
}

// collectJavaStringVars faz um rastreio leve, intra-arquivo, das variáveis
// locais que recebem um java.lang.String. Uma variável é "java" quando alguma
// atribuição vem de um método-gatilho SEM coerção (sem String(...) e sem `+`,
// que forçam string JS) — e nunca recebeu uma atribuição comprovadamente
// segura (conservador: uma reatribuição segura tira a variável do conjunto).
func collectJavaStringVars(lines []string) map[string]string {
	hasJava := map[string]string{} // nome -> método de origem
	hasSafe := map[string]bool{}
	for _, raw := range lines {
		m := declRe.FindStringSubmatch(stripLineComment(raw))
		if m == nil {
			continue
		}
		name, rhs := m[1], strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[2]), ";"))
		call := anyJavaCallRe.FindString(rhs)
		coerced := strings.Contains(rhs, "String(") || strings.Contains(rhs, "+")
		if call != "" && !coerced {
			method := strings.Trim(strings.TrimPrefix(strings.TrimSpace(call), "."), " (")
			if _, ok := hasJava[name]; !ok {
				hasJava[name] = method
			}
		} else {
			hasSafe[name] = true
		}
	}
	out := map[string]string{}
	for name, src := range hasJava {
		if !hasSafe[name] {
			out[name] = src
		}
	}
	return out
}

func directMsg(op string) string {
	return fmt.Sprintf("comparação estrita (%s) entre retorno de método Java (java.lang.String) e literal de texto — no Rhino do Fluig é SEMPRE %s", op, alwaysResult(op))
}

func varMsg(name, src, op string) string {
	return fmt.Sprintf("a variável %q recebe um java.lang.String (de .%s(...)) e é comparada com %s a um literal — no Rhino do Fluig isso é SEMPRE %s", name, src, op, alwaysResult(op))
}

// alwaysResult: === com java.lang.String é sempre false; !== é sempre true.
func alwaysResult(op string) string {
	if op == "!==" {
		return "true"
	}
	return "false"
}

func directSug() string {
	return "converta antes de comparar (String(x.getFieldName()) === 'y') ou use igualdade solta (==)"
}

// stripLineComment corta um comentário de linha `//` fora de string. Simples:
// não trata `//` dentro de '...'/"..." nem de regex — suficiente para as
// comparações que a regra examina.
func stripLineComment(line string) string {
	inS, inD := false, false
	for i := 0; i+1 < len(line); i++ {
		switch line[i] {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case '/':
			if !inS && !inD && line[i+1] == '/' {
				return line[:i]
			}
		}
	}
	return line
}
