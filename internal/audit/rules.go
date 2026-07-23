package audit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Detecções compartilhadas. O legado casa "fluig-style-guide.min.css" mas NÃO
// o atual "fluig-style-guide-flat.min.css" (o -flat quebra o substring).
const legacyCSSName = "fluig-style-guide.min.css"

var (
	hexColorRe = regexp.MustCompile(`#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b`)
	rgbColorRe = regexp.MustCompile(`rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})`)
	extHostRe  = regexp.MustCompile(`(?:https?:)?//([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)

	cssURLExtRe    = regexp.MustCompile(`(?i)(?:@import\s+(?:url\()?|url\()\s*['"]?(?:https?:)?//`)
	scriptSrcExtRe = regexp.MustCompile(`(?i)<script[^>]*\bsrc\s*=\s*["'](?:https?:)?//`)
	linkHrefExtRe  = regexp.MustCompile(`(?i)<link[^>]*\bhref\s*=\s*["'](?:https?:)?//`)
	styleAttrRe    = regexp.MustCompile(`(?i)style\s*=\s*("[^"]*"|'[^']*')`)
	classAttrRe    = regexp.MustCompile(`(?i)class\s*=\s*("[^"]*"|'[^']*')`)
	styleOpenRe    = regexp.MustCompile(`(?i)<style\b`)
	styleCloseRe   = regexp.MustCompile(`(?i)</style>`)
	scriptOpenRe   = regexp.MustCompile(`(?i)<script\b`)
	scriptCloseRe  = regexp.MustCompile(`(?i)</script>`)
	// alert/confirm/prompt nativos — aceita window. na frente, mas não outros
	// prefixos com ponto (FLUIGC.message.alert não conta).
	nativeDialogRe = regexp.MustCompile(`(^|[^.\w])((?:window\.)?(?:alert|confirm|prompt))\s*\(`)
	selectorClass  = regexp.MustCompile(`\.(-?[a-zA-Z][a-zA-Z0-9_-]+)`)
)

// scanCSS roda as regras sobre um arquivo .css.
func scanCSS(rel string, content []byte, cat *Catalog) []Finding {
	clean := splitLinesNoComments(string(content))
	var out []Finding
	for i, line := range clean {
		n := i + 1
		out = append(out, checkLegacy(rel, n, line)...)
		if cssURLExtRe.MatchString(line) {
			out = append(out, externalFinding(rel, n, line, "recurso externo no CSS"))
		}
		out = append(out, colorFindings(rel, n, line, cat, "cor fixa %s no CSS")...)
	}
	out = append(out, importantFindings(rel, strings.Join(clean, "\n"), cat)...)
	return out
}

// importantFindings acha !important em regras cujo seletor usa classe do
// style guide — é briga direta com o tema fixo. !important em classe própria
// do projeto não é apontado.
func importantFindings(rel, css string, cat *Catalog) []Finding {
	var out []Finding
	var stack []string // seletores abertos (inclui @media etc.)
	var pending strings.Builder
	line := 1
	flush := func() string {
		s := pending.String()
		pending.Reset()
		return s
	}
	checkDecl := func(decl string, declLine int) {
		if !strings.Contains(decl, "!important") {
			return
		}
		for _, sel := range stack {
			for _, m := range selectorClass.FindAllStringSubmatch(sel, -1) {
				if cat.HasClass(m[1]) {
					out = append(out, Finding{
						Rule: RuleImportant, Severity: SeverityWarning, File: rel, Line: declLine,
						Message:    fmt.Sprintf("!important sobre a classe %q do style guide — briga com o tema em vez de compor com ele", m[1]),
						Suggestion: "prefira uma classe própria (ou as utilitárias fs-*) e deixe o tema mandar",
					})
					return
				}
			}
		}
	}
	for i := 0; i < len(css); i++ {
		switch c := css[i]; c {
		case '\n':
			line++
			pending.WriteByte(' ')
		case '{':
			stack = append(stack, flush())
		case '}':
			checkDecl(flush(), line)
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case ';':
			checkDecl(flush(), line)
		default:
			pending.WriteByte(c)
		}
	}
	return out
}

// scanJS roda as regras sobre um arquivo .js. As regras de navegador (SG007)
// só valem no client-side; as FL* de API valem nos dois lados (a chamada se
// auto-escopa pelo objeto usado) e form.* só nos eventos de formulário, onde
// `form` é garantidamente o FormController (Rhino).
func scanJS(rel string, content []byte) []Finding {
	serverSide := isServerSideJS(rel)
	formEvent := isFormEventJS(rel)
	var out []Finding
	for i, line := range strings.Split(string(content), "\n") {
		n := i + 1
		if !serverSide {
			out = append(out, nativeDialogFindings(rel, n, line)...)
		}
		out = append(out, apiFindings(rel, n, line)...)
		if formEvent {
			out = append(out, formEventFindings(rel, n, line)...)
		}
	}
	if serverSide {
		// Footguns do Rhino (RHINO*) — só no JS que roda no servidor.
		out = append(out, rhinoFindings(rel, content)...)
	}
	return out
}

func nativeDialogFindings(rel string, n int, line string) []Finding {
	var out []Finding
	for _, m := range nativeDialogRe.FindAllStringSubmatch(line, -1) {
		out = append(out, Finding{
			Rule: RuleNativeDialog, Severity: SeverityWarning, File: rel, Line: n,
			Message:    fmt.Sprintf("diálogo nativo do navegador (%s) — fora do padrão visual do Fluig", m[2]),
			Suggestion: "use FLUIGC.toast / FLUIGC.message.confirm / FLUIGC.message.alert (style guide)",
		})
	}
	return out
}

// scanMarkup roda as regras sobre HTML/FTL: recursos externos, cores em
// style= e em blocos <style>, e classes fs-* inexistentes.
func scanMarkup(rel string, content []byte, cat *Catalog) []Finding {
	var out []Finding
	inStyle, inScript := false, false
	for i, line := range strings.Split(string(content), "\n") {
		n := i + 1
		out = append(out, checkLegacy(rel, n, line)...)
		switch {
		case scriptSrcExtRe.MatchString(line):
			out = append(out, externalFinding(rel, n, line, "script externo"))
		case linkHrefExtRe.MatchString(line):
			out = append(out, externalFinding(rel, n, line, "recurso externo (link)"))
		}

		// Estilo inline: o atributo em si é aviso (SG005); cores dentro dele
		// são erro (SG003). Cores também valem no CSS embutido em <style>.
		for _, m := range styleAttrRe.FindAllStringSubmatch(line, -1) {
			out = append(out, Finding{
				Rule: RuleInlineStyle, Severity: SeverityWarning, File: rel, Line: n,
				Message:    "estilo inline (style=) — escapa do tema e do CSS do formulário/widget",
				Suggestion: "mova a regra para o CSS próprio (classe) ou use as utilitárias fs-*",
			})
			out = append(out, colorFindings(rel, n, m[1], cat, "cor fixa %s em style= inline")...)
		}
		wasInStyle, wasInScript := inStyle, inScript
		if styleOpenRe.MatchString(line) {
			inStyle = true
		}
		if styleCloseRe.MatchString(line) {
			inStyle = false
		}
		if scriptOpenRe.MatchString(line) && !scriptSrcExtRe.MatchString(line) {
			inScript = true
		}
		if scriptCloseRe.MatchString(line) {
			inScript = false
		}
		if wasInStyle {
			out = append(out, colorFindings(rel, n, line, cat, "cor fixa %s em <style> embutido")...)
			if cssURLExtRe.MatchString(line) {
				out = append(out, externalFinding(rel, n, line, "recurso externo no CSS embutido"))
			}
		}
		if wasInScript {
			out = append(out, nativeDialogFindings(rel, n, line)...)
			out = append(out, apiFindings(rel, n, line)...)
		}

		// Classes fs-* inexistentes (typos) — ignora interpolações.
		for _, m := range classAttrRe.FindAllStringSubmatch(line, -1) {
			for _, cls := range strings.Fields(strings.Trim(m[1], `"'`)) {
				if !strings.HasPrefix(cls, "fs-") || strings.ContainsAny(cls, "${}") {
					continue
				}
				if cat.HasClass(cls) {
					continue
				}
				sug := "confira o catálogo em style-guide/css.html"
				if near := cat.NearestClass(cls); near != "" {
					sug = fmt.Sprintf("quis dizer %q?", near)
				}
				out = append(out, Finding{
					Rule: RuleUnknownClass, Severity: SeverityWarning, File: rel, Line: n,
					Message:    fmt.Sprintf("classe %q não existe no style guide do servidor", cls),
					Suggestion: sug,
				})
			}
		}
	}
	return out
}

func checkLegacy(rel string, n int, line string) []Finding {
	if !strings.Contains(line, legacyCSSName) {
		return nil
	}
	return []Finding{{
		Rule: RuleLegacyCSS, Severity: SeverityWarning, File: rel, Line: n,
		Message:    "referência ao CSS legado do style guide (descontinuado no Fluig 2.0)",
		Suggestion: "troque para style-guide/css/fluig-style-guide-flat.min.css — no render da solicitação o servidor reescreve sozinho, mas fora dele o arquivo é 404",
		Fix:        "fluig-style-guide-flat.min.css",
		fixOld:     legacyCSSName,
	}}
}

func externalFinding(rel string, n int, line, what string) Finding {
	host := ""
	if m := extHostRe.FindStringSubmatch(line); m != nil {
		host = " (" + m[1] + ")"
	}
	return Finding{
		Rule: RuleExternalRes, Severity: SeverityError, File: rel, Line: n,
		Message:    what + host + " — visual fora do tema e dependência de internet/versão congelada",
		Suggestion: "sirva o recurso do próprio WAR/servidor (nos templates SPA a dependência vem por npm e entra no bundle)",
	}
}

// colorFindings acha cores fixas (hex e rgb/rgba) num trecho, uma vez por
// valor por linha, com a sugestão de variável do catálogo.
func colorFindings(rel string, n int, s string, cat *Catalog, msgFmt string) []Finding {
	seen := map[string]struct{}{}
	var out []Finding
	add := func(display, hex string, fixable bool) {
		if _, dup := seen[display]; dup {
			return
		}
		seen[display] = struct{}{}
		f := Finding{
			Rule: RuleHardcodedHex, Severity: SeverityError, File: rel, Line: n,
			Message:    fmt.Sprintf(msgFmt, display) + " — quebra o tema fixo e o dark mode do 2.0",
			Suggestion: cat.SuggestColor(hex),
		}
		// Correção determinística só quando o valor bate EXATO com uma
		// variável do tema (mesmo render no light) e o token é hex literal.
		if v, exact := cat.ExactVar(hex); exact && fixable {
			f.Fix = "var(" + v + ")"
			f.fixOld = display
		}
		out = append(out, f)
	}
	for _, m := range hexColorRe.FindAllString(s, -1) {
		if hex, ok := normalizeHex(m); ok {
			add(m, hex, true)
		}
	}
	for _, m := range rgbColorRe.FindAllStringSubmatch(s, -1) {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		if r <= 255 && g <= 255 && b <= 255 {
			// rgb() fica sem --fix: o casamento textual exato do trecho não é
			// garantido (espaçamento/alfa) — a sugestão orienta o manual.
			add(m[0]+")", rgbToHex(r, g, b), false)
		}
	}
	return out
}

// splitLinesNoComments divide o CSS em linhas com os comentários /* */
// apagados (preservando a contagem de linhas).
func splitLinesNoComments(css string) []string {
	var b strings.Builder
	b.Grow(len(css))
	inComment := false
	for i := 0; i < len(css); i++ {
		c := css[i]
		if inComment {
			if c == '*' && i+1 < len(css) && css[i+1] == '/' {
				inComment = false
				i++
			} else if c == '\n' {
				b.WriteByte('\n')
			}
			continue
		}
		if c == '/' && i+1 < len(css) && css[i+1] == '*' {
			inComment = true
			i++
			continue
		}
		b.WriteByte(c)
	}
	return strings.Split(b.String(), "\n")
}
