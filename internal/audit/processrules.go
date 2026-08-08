package audit

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Regras WF* — cruzamento do formulário com o processo ao qual ele está
// vinculado (audit --process). Diferente das demais famílias, estas regras
// precisam de um dado do servidor (as sequences reais do processo); a CLI
// busca o processo e entrega a lista pronta — este pacote segue sem rede.
//
// Motivação (ROADMAP3 §4.12, feedback de 2026-08-03/04): um formulário
// declarava seções `activity-0/5/10…` e o processo tinha sequences
// `7, 9, 13…`. Nenhuma coincidia. O formulário não renderizava seção alguma e
// a validação nunca rodava — e NENHUM comando acusava: o audit passava (não é
// erro de sintaxe), o diff passava (local == servidor) e o request start não
// executa os eventos do formulário. Ninguém decora os ids do BPMN.

// ProcessActivity é uma etapa do processo na visão mínima das regras WF*:
// sequence (= WKNumState), nome e tipo. O chamador converte do ProcessDetail.
type ProcessActivity struct {
	Sequence int
	Name     string
	Kind     string // start|task|service|gateway|event|end|unknown
}

// activityClassRe casa as classes de seção por etapa (`activity-<N>`) no HTML.
// A convenção: cada seção carrega `activity-N` e o JS do formulário mostra
// `$(".activity-" + WKNumState)`.
var activityClassRe = regexp.MustCompile(`\bactivity-(\d+)\b`)

// CheckFormActivities cruza as classes `activity-N` do HTML do formulário com
// as etapas reais do processo.
//
//   - WF001 (erro): `activity-N` sem etapa de sequence N no processo — a seção
//     NUNCA renderiza. `activity-0` é sempre válido: é a convenção do
//     formulário de abertura (WKNumState = 0 antes do primeiro envio).
//   - WF002 (aviso): atividade HUMANA do processo sem seção `activity-N` no
//     HTML. Só é emitido quando o formulário usa a convenção (tem pelo menos
//     uma classe activity-*): formulário igual em todas as etapas é legítimo.
//
// rel é o caminho do HTML no relatório; processID entra nas mensagens.
func CheckFormActivities(rel string, content []byte, processID string, states []ProcessActivity) []Finding {
	valid := map[int]ProcessActivity{}
	for _, st := range states {
		valid[st.Sequence] = st
	}

	// Primeira linha de cada N distinto + contagem (30 repetições da mesma
	// classe errada são UM problema, não 30).
	firstLine := map[int]int{}
	count := map[int]int{}
	for i, line := range strings.Split(string(content), "\n") {
		for _, m := range activityClassRe.FindAllStringSubmatch(line, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			if _, seen := firstLine[n]; !seen {
				firstLine[n] = i + 1
			}
			count[n]++
		}
	}

	var out []Finding

	// WF001 — seção sem etapa correspondente.
	invalid := make([]int, 0, len(firstLine))
	for n := range firstLine {
		if n == 0 {
			continue // formulário de abertura (WKNumState = 0)
		}
		if _, ok := valid[n]; !ok {
			invalid = append(invalid, n)
		}
	}
	sort.Ints(invalid)
	suggestion := "etapas do processo: " + humanStatesSummary(states)
	for _, n := range invalid {
		msg := fmt.Sprintf("a seção activity-%d não corresponde a nenhuma etapa do processo %q — ela nunca renderiza", n, processID)
		if count[n] > 1 {
			msg += fmt.Sprintf(" (%d ocorrências)", count[n])
		}
		out = append(out, Finding{
			Rule: RuleActivityUnknown, Severity: SeverityError,
			File: rel, Line: firstLine[n],
			Message: msg, Suggestion: suggestion,
		})
	}

	// WF002 — atividade humana sem seção. Só quando o form adota a convenção.
	if len(firstLine) == 0 {
		return out
	}
	for _, st := range sortedBySequence(states) {
		if st.Kind != "task" {
			continue
		}
		if _, ok := firstLine[st.Sequence]; ok {
			continue
		}
		out = append(out, Finding{
			Rule: RuleActivityMissing, Severity: SeverityWarning,
			File: rel, Line: 1,
			Message: fmt.Sprintf("a atividade humana %d (%q) não tem seção activity-%d no formulário",
				st.Sequence, st.Name, st.Sequence),
			Suggestion: "se a etapa deve mostrar o formulário igual às demais, ignore; senão, adicione a seção",
		})
	}
	return out
}

// --- WF003: constante de etapa nos scripts × sequences reais ---
//
// Motivação (ROADMAP §4.12-b): o script de processo compara a etapa corrente
// com um número — quase sempre por uma constante declarada no topo da função:
//
//	function beforeStateEntry(sequenceId) {
//	    var cancelaState = 166;
//	    ...
//	    if (sequenceId == cancelaState) { atualizaMovimento("Cancela"); }
//
// Se 166 não é sequence de nenhuma etapa, o ramo NUNCA executa. Não há erro,
// nem no deploy nem em runtime: a condição só dá `false` para sempre. É o
// mesmo defeito silencioso do WF001, do outro lado do processo.
//
// Os dois casos reais que a calibração achou (projeto do mantenedor, 201
// scripts, 2026-08-08): uma constante defasada de versão anterior e uma
// constante copiada de outro processo, onde o "Início" tem outro número.

// eventSeqParam mapeia o evento de processo para o parâmetro que carrega a
// sequence da etapa. Só os eventos que recebem etapa entram aqui — nos demais
// (afterTaskCreate(colleagueId), afterProcessFinish(processId)…) nenhum
// parâmetro é etapa, e confundir daria falso positivo.
//
// Fonte das assinaturas: o catálogo de `internal/scaffold` (o mesmo do
// `workflow new-script`). O teste TestEventoDeEtapaBateComOCatalogo tranca os
// dois lados.
var eventSeqParam = map[string]string{
	"afterStateEntry":         "sequenceId",
	"afterStateLeave":         "sequenceId",
	"beforeStateEntry":        "sequenceId",
	"beforeStateLeave":        "sequenceId",
	"afterTaskComplete":       "nextSequenceId",
	"afterTaskSave":           "nextSequenceId",
	"beforeTaskComplete":      "nextSequenceId",
	"beforeTaskSave":          "nextSequenceId",
	"validateAvailableStates": "iCurrentState",
}

var (
	fnDeclRe = regexp.MustCompile(`\bfunction\s+(\w+)\s*\(([^)]*)\)`)
	// Declaração `var X = <resto>`; o resto vai até `;` ou fim de linha.
	varDeclRe = regexp.MustCompile(`\b(?:var|let|const)\s+([A-Za-z_$][\w$]*)\s*=\s*([^;\n]+)`)
	wkNumRe   = regexp.MustCompile(`getValue\s*\(\s*['"]WKNumState['"]\s*\)`)
	intLitRe  = regexp.MustCompile(`^\d{1,6}$`)
	// Comparação entre dois operandos simples (identificador ou número).
	// Literal de texto fica de fora de propósito: a calibração no projeto real
	// não achou NENHUMA comparação de etapa com `"17"`, e incluí-la só
	// aumentaria a superfície de falso positivo.
	stateCmpRe = regexp.MustCompile(`([A-Za-z_$][\w$]*|\d{1,6})\s*(===|!==|==|!=)\s*([A-Za-z_$][\w$]*|\d{1,6})`)
)

// CheckProcessScriptStates cruza os números comparados com a etapa corrente,
// num script de processo, com as sequences reais do processo.
//
//   - WF003 (erro): o número não corresponde a etapa nenhuma — o ramo nunca
//     executa. A sequence 0 é ignorada: é a convenção de "sem etapa" (mesma
//     regra do activity-0 no WF001).
//
// A etapa corrente é reconhecida por duas fontes, as duas medidas no projeto
// real: o parâmetro de sequence do evento (ver eventSeqParam) e a variável que
// recebe `getValue("WKNumState")`, com ou sem `parseInt`.
func CheckProcessScriptStates(rel string, content []byte, processID string, states []ProcessActivity) []Finding {
	valid := map[int]bool{}
	for _, st := range states {
		valid[st.Sequence] = true
	}
	// Sem etapas não há o que cruzar (processo vazio ou não resolvido).
	if len(valid) == 0 {
		return nil
	}

	semComentario := string(maskComments(content))
	// Para as comparações, também sem o conteúdo das strings: `"a == 5"` não
	// é código. maskSource preserva os deslocamentos, então a linha continua
	// batendo.
	codigo := string(maskSource(content))

	estado := map[string]bool{}
	for _, m := range fnDeclRe.FindAllStringSubmatch(semComentario, -1) {
		param, ok := eventSeqParam[m[1]]
		if !ok {
			continue
		}
		for _, p := range strings.Split(m[2], ",") {
			if strings.TrimSpace(p) == param {
				estado[param] = true
			}
		}
	}

	// Constantes numéricas e variáveis de etapa. Uma variável que recebe
	// qualquer outra coisa em algum ponto sai do conjunto (conservador: o
	// mesmo critério do collectJavaStringVars).
	constNum := map[string]int{}
	sujo := map[string]bool{}
	for _, m := range varDeclRe.FindAllStringSubmatch(semComentario, -1) {
		nome, rhs := m[1], strings.TrimSpace(m[2])
		switch {
		case wkNumRe.MatchString(rhs):
			estado[nome] = true
		case intLitRe.MatchString(rhs):
			if anterior, visto := constNum[nome]; visto && anterior != atoiSeguro(rhs) {
				sujo[nome] = true
			}
			constNum[nome] = atoiSeguro(rhs)
		default:
			sujo[nome] = true
		}
	}

	// Um nome não pode ser etapa e constante ao mesmo tempo.
	for nome := range estado {
		delete(constNum, nome)
	}

	var out []Finding
	visto := map[string]bool{}
	for _, loc := range stateCmpRe.FindAllStringSubmatchIndex(codigo, -1) {
		esquerda := codigo[loc[2]:loc[3]]
		op := codigo[loc[4]:loc[5]]
		direita := codigo[loc[6]:loc[7]]

		var alvo string
		switch {
		case estado[esquerda]:
			alvo = direita
		case estado[direita]:
			alvo = esquerda
		default:
			continue
		}

		valor, origem := 0, ""
		switch {
		case intLitRe.MatchString(alvo):
			valor, origem = atoiSeguro(alvo), "o número"
		case !sujo[alvo] && constNum[alvo] != 0:
			valor, origem = constNum[alvo], "a constante "+alvo
		default:
			continue
		}
		if valor == 0 || valid[valor] {
			continue
		}
		// Uma constante errada costuma ser comparada em vários pontos. O
		// problema é um só — reportar a primeira ocorrência.
		chave := alvo + "\x00" + strconv.Itoa(valor)
		if visto[chave] {
			continue
		}
		visto[chave] = true

		linha := strings.Count(codigo[:loc[0]], "\n") + 1
		out = append(out, Finding{
			Rule: RuleStateConstUnknown, Severity: SeverityError,
			File: rel, Line: linha,
			Message: fmt.Sprintf("%s vale %d, que não é etapa do processo %q — a comparação %s %s %s nunca é verdadeira",
				origem, valor, processID, esquerda, op, direita),
			Suggestion: "etapas do processo: " + allStatesSummary(states),
		})
	}
	return out
}

// atoiSeguro converte o que o intLitRe já validou como inteiro.
func atoiSeguro(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// maskComments apaga comentários preservando o conteúdo das strings e os
// deslocamentos. Diferente do maskSource, que também apaga as strings — aqui o
// texto de `getValue("WKNumState")` precisa sobreviver.
func maskComments(content []byte) []byte {
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
				quote = c
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
				i++
			case c == quote:
				state = normal
			}
		}
	}
	return b
}

// allStatesSummary lista TODAS as etapas com nome. Diferente do WF001, aqui
// gateway e atividade automática importam: o script compara com elas também.
func allStatesSummary(states []ProcessActivity) string {
	var parts []string
	for _, st := range sortedBySequence(states) {
		nome := st.Name
		if nome == "" {
			nome = st.Kind
		}
		parts = append(parts, fmt.Sprintf("%d (%s)", st.Sequence, nome))
	}
	if len(parts) == 0 {
		return "o processo não tem etapa"
	}
	return strings.Join(parts, ", ")
}

// humanStatesSummary resume as etapas que importam para quem escreve o
// formulário: as humanas (com nome) e o início.
func humanStatesSummary(states []ProcessActivity) string {
	var parts []string
	for _, st := range sortedBySequence(states) {
		switch st.Kind {
		case "start":
			parts = append(parts, fmt.Sprintf("%d (início; a abertura usa activity-0)", st.Sequence))
		case "task":
			parts = append(parts, fmt.Sprintf("%d (%s)", st.Sequence, st.Name))
		}
	}
	if len(parts) == 0 {
		return "o processo não tem atividade humana"
	}
	return strings.Join(parts, ", ")
}

func sortedBySequence(states []ProcessActivity) []ProcessActivity {
	out := append([]ProcessActivity(nil), states...)
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}
