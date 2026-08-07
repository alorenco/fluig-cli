package audit

import (
	"fmt"
	"regexp"
	"strings"
)

// Regras FL* — validam chamadas às APIs de script do Fluig contra o catálogo
// do fluig.d.ts (apicatalog.go). Nascem como AVISO: o d.ts é incompleto por
// definição, então um achado pode ser typo (o caso comum) ou API que falta no
// fork — nesse caso a correção é completar o fluig.d.ts, não o código.

var (
	// Chamada objeto.membro( ou objeto.sub.membro( — só nos objetos indexados.
	apiCallRe = regexp.MustCompile(`\b(hAPI|FLUIGC|DatasetFactory|DatasetBuilder|docAPI|WCMAPI|fluigAPI|customHTML)\s*\.\s*(\w+)(?:\s*\.\s*(\w+))?\s*\(`)
	// getValue global (não precedido de ponto) com argumento WK*.
	wkGetValueRe = regexp.MustCompile(`(^|[^.\w])getValue\(\s*["'](WK\w+)["']`)
	// form.<método>( — só vale nos eventos de formulário, onde `form` é o
	// FormController (fora deles o nome é comum demais para inferir).
	formCallRe = regexp.MustCompile(`\bform\s*\.\s*(\w+)\s*\(`)
)

// apiFindings roda FL001/FL002/FL004 sobre uma linha de JS (arquivo .js ou
// <script> de markup, server-side ou client-side — as chamadas se auto-escopam
// pelo objeto usado).
func apiFindings(rel string, n int, line string) []Finding {
	cat := apiCatalog()
	var out []Finding
	for _, m := range apiCallRe.FindAllStringSubmatch(line, -1) {
		obj, member, sub := m[1], m[2], m[3]
		if sub != "" {
			// objeto.m2.m3( — se m2 é um sub-namespace conhecido (FLUIGC.message),
			// valida m3 nele; senão valida m2 no objeto raiz (encadeamento).
			if nested := obj + "." + member; cat.KnownObject(nested) {
				obj, member = nested, sub
			}
		}
		if !cat.KnownObject(obj) || cat.HasMember(obj, member) {
			continue
		}
		rule := RuleUnknownAPI
		if obj == "hAPI" {
			rule = RuleUnknownHAPI
		}
		out = append(out, Finding{
			Rule: rule, Severity: SeverityWarning, File: rel, Line: n,
			Message:    fmt.Sprintf("%s.%s(...) não existe na referência de APIs do Fluig (fluig.d.ts)", obj, member),
			Suggestion: apiSuggestion(cat.NearestMember(obj, member), obj),
		})
	}
	for _, m := range wkGetValueRe.FindAllStringSubmatch(line, -1) {
		name := m[2]
		if cat.HasWKVar(name) {
			continue
		}
		out = append(out, Finding{
			Rule: RuleUnknownWKVar, Severity: SeverityWarning, File: rel, Line: n,
			Message:    fmt.Sprintf("getValue(%q): variável não existe na referência (fluig.d.ts) — o Fluig devolve null em silêncio", name),
			Suggestion: apiSuggestion(cat.NearestWKVar(name), "getValue"),
		})
	}
	return out
}

// formEventFindings roda FL003 (métodos do FormController) sobre uma linha de
// evento de formulário.
func formEventFindings(rel string, n int, line string) []Finding {
	cat := apiCatalog()
	if !cat.KnownObject("form") {
		return nil
	}
	var out []Finding
	for _, m := range formCallRe.FindAllStringSubmatch(line, -1) {
		if cat.HasMember("form", m[1]) {
			continue
		}
		out = append(out, Finding{
			Rule: RuleUnknownFormAPI, Severity: SeverityWarning, File: rel, Line: n,
			Message:    fmt.Sprintf("form.%s(...) não existe no FormController (fluig.d.ts)", m[1]),
			Suggestion: apiSuggestion(cat.NearestMember("form", m[1]), "form"),
		})
	}
	return out
}

func apiSuggestion(nearest, obj string) string {
	if nearest != "" {
		return fmt.Sprintf("quis dizer %q?", nearest)
	}
	return fmt.Sprintf("confira a assinatura no reference/fluig.d.ts da skill (grep '%s') — se a API existe mesmo, ela falta no arquivo", obj)
}

// FL006 — `getDataset(...).values` (ou `.getValue`) encadeado DIRETO no
// client-side (ROADMAP3 §4.11-D, caso 13.4 do feedback de 2026-08-03/04).
// Quando a chamada falha no navegador (sessão/JWT expirado, dataset fora do
// ar), `getDataset` devolve undefined e o formulário estoura com
// `TypeError: Cannot read properties of undefined (reading 'values')` — sem
// nenhuma pista para o usuário final. Só o encadeamento IMEDIATO é apontado:
// retorno guardado em variável (mesmo sem if) fica de fora — na dúvida, calar.
// No server-side a regra não roda: lá a falha vira exceção, não undefined.
var chainedGetDatasetRe = regexp.MustCompile(
	`\bgetDataset\s*\((?:[^()]|\([^()]*\))*\)\s*\.\s*(values|getValue)\b`)

// chainedDatasetLineFindings roda a FL006 numa linha de JS client-side (já sem
// comentário de linha; o chamador de arquivo .js mascara strings/comentários).
func chainedDatasetLineFindings(rel string, n int, line string) []Finding {
	var out []Finding
	for _, m := range chainedGetDatasetRe.FindAllStringSubmatch(line, -1) {
		out = append(out, Finding{
			Rule: RuleChainedGetDataset, Severity: SeverityWarning, File: rel, Line: n,
			Message: fmt.Sprintf("getDataset(...).%s encadeado — se a chamada falhar (sessão expirada, dataset fora do ar), "+
				"o retorno é undefined e o formulário quebra com TypeError, sem pista para o usuário", m[1]),
			Suggestion: "guarde o retorno e verifique: var ds = DatasetFactory.getDataset(...); if (ds && ds.values) { ... }",
		})
	}
	return out
}

// chainedDatasetFileFindings roda a FL006 num arquivo .js client-side inteiro
// (strings e comentários mascarados — código comentado não acusa).
func chainedDatasetFileFindings(rel string, content []byte) []Finding {
	var out []Finding
	for i, line := range strings.Split(string(maskSource(content)), "\n") {
		out = append(out, chainedDatasetLineFindings(rel, i+1, line)...)
	}
	return out
}

// isServerSideJS informa se o JS roda no servidor (Rhino): datasets, eventos
// globais, mecanismos, scripts de processo e eventos de formulário. Regras de
// navegador (SG007) não valem lá.
func isServerSideJS(rel string) bool {
	for _, prefix := range []string{"datasets/", "events/", "mechanisms/", "workflow/scripts/"} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return isFormEventJS(rel)
}

// isFormEventJS informa se o arquivo é um evento de formulário
// (forms/<Form>/events/*.js) — onde `form` é o FormController global.
func isFormEventJS(rel string) bool {
	return strings.HasPrefix(rel, "forms/") && strings.Contains(rel, "/events/")
}

// FL007 — caractere fora do CP-1252 em script server-side (ROADMAP3 §4.3).
// O Fluig guarda os scripts em colunas varchar com collation
// Latin1_General_CI_AS (CP-1252): na GRAVAÇÃO, qualquer caractere fora dessa
// página (→, ✓, emoji…) vira "?" de forma PERMANENTE — comprovado byte a byte
// na homologação (2026-08-07). O CP-1252 cobre a acentuação toda e a
// pontuação tipográfica (— – " " ' ' … •), então o aviso só dispara no que
// realmente se perde. Aviso (não erro): o "?" num comentário é cosmético; num
// literal de string é bug — quem decide é o autor.
func nonCP1252Findings(rel string, content []byte) []Finding {
	var out []Finding
	for i, line := range strings.Split(string(content), "\n") {
		for _, r := range line {
			if r == '\t' || CP1252Encodable(r) {
				continue
			}
			out = append(out, Finding{
				Rule: RuleNonCP1252, Severity: SeverityWarning, File: rel, Line: i + 1,
				Message: fmt.Sprintf("o caractere %q (U+%04X) não existe no CP-1252 — o banco do Fluig o grava como \"?\" (perda permanente)", r, r),
				Suggestion: "troque por um equivalente ASCII/CP-1252 (ex.: \"→\" → \"->\") ou aceite o \"?\" no servidor",
			})
			break // um aviso por linha basta — a correção é revisar a linha
		}
	}
	return out
}

// CP1252Encodable informa se a runa existe na página de código Windows-1252.
// Latin-1 (U+0000–U+00FF) entra quase todo: o CP-1252 substitui só a faixa de
// controle U+0080–U+009F pelos tipográficos abaixo.
func CP1252Encodable(r rune) bool {
	if r < 0x80 {
		return true
	}
	if r >= 0xA0 && r <= 0xFF {
		return true
	}
	switch r {
	case '€', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 'Ž',
		'‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 'ž', 'Ÿ':
		return true
	}
	return false
}
