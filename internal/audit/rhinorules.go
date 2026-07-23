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
	return out
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
