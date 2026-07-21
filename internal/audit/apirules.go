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
