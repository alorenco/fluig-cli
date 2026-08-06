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
