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
)

// scanCSS roda as regras sobre um arquivo .css.
func scanCSS(rel string, content []byte, cat *Catalog) []Finding {
	var out []Finding
	for i, line := range splitLinesNoComments(string(content)) {
		n := i + 1
		out = append(out, checkLegacy(rel, n, line)...)
		if cssURLExtRe.MatchString(line) {
			out = append(out, externalFinding(rel, n, line, "recurso externo no CSS"))
		}
		out = append(out, colorFindings(rel, n, line, cat, "cor fixa %s no CSS")...)
	}
	return out
}

// scanMarkup roda as regras sobre HTML/FTL: recursos externos, cores em
// style= e em blocos <style>, e classes fs-* inexistentes.
func scanMarkup(rel string, content []byte, cat *Catalog) []Finding {
	var out []Finding
	inStyle := false
	for i, line := range strings.Split(string(content), "\n") {
		n := i + 1
		out = append(out, checkLegacy(rel, n, line)...)
		switch {
		case scriptSrcExtRe.MatchString(line):
			out = append(out, externalFinding(rel, n, line, "script externo"))
		case linkHrefExtRe.MatchString(line):
			out = append(out, externalFinding(rel, n, line, "recurso externo (link)"))
		}

		// Cores fixas: em style="..." sempre; no conteúdo da linha quando
		// dentro de <style> (CSS embutido no HTML).
		for _, m := range styleAttrRe.FindAllStringSubmatch(line, -1) {
			out = append(out, colorFindings(rel, n, m[1], cat, "cor fixa %s em style= inline")...)
		}
		wasInStyle := inStyle
		if styleOpenRe.MatchString(line) {
			inStyle = true
		}
		if styleCloseRe.MatchString(line) {
			inStyle = false
		}
		if wasInStyle {
			out = append(out, colorFindings(rel, n, line, cat, "cor fixa %s em <style> embutido")...)
			if cssURLExtRe.MatchString(line) {
				out = append(out, externalFinding(rel, n, line, "recurso externo no CSS embutido"))
			}
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
	add := func(display, hex string) {
		if _, dup := seen[display]; dup {
			return
		}
		seen[display] = struct{}{}
		out = append(out, Finding{
			Rule: RuleHardcodedHex, Severity: SeverityError, File: rel, Line: n,
			Message:    fmt.Sprintf(msgFmt, display) + " — quebra o tema fixo e o dark mode do 2.0",
			Suggestion: cat.SuggestColor(hex),
		})
	}
	for _, m := range hexColorRe.FindAllString(s, -1) {
		if hex, ok := normalizeHex(m); ok {
			add(m, hex)
		}
	}
	for _, m := range rgbColorRe.FindAllStringSubmatch(s, -1) {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		if r <= 255 && g <= 255 && b <= 255 {
			add(m[0]+")", rgbToHex(r, g, b))
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
