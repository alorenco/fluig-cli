package devserver

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Render local de view.ftl — só para desenvolvimento.
//
// O portal renderiza o view.ftl no servidor e embute a saída na página, num
// envelope estável por instância:
//
//	<div id="_instance_4347_" appcode="ramais" class="widget-custom-inside">
//	  <div id="wcm_widget_4347" class="wcm_widget">
//	    <div class="wcm_corpo_widget_single">
//	      <div id="..." class="... wcm-widget-class ...">   ← saída do view.ftl
//
// O proxy acha cada envelope cujo appcode é uma widget do projeto, renderiza o
// view.ftl local e troca o bloco da saída (o div com wcm-widget-class, que o
// padrão SuperWidget exige) — o markup passa a vir do disco, como o JS/CSS.
//
// O renderizador é deliberadamente mínimo: substitui ${instanceId} e remove
// comentários <#-- -->. FreeMarker de verdade (outras ${…}, <#if>, <@macro>)
// não é suportado — nesse caso a widget mantém o render do servidor e sai um
// aviso (uma vez por widget), preservando a fidelidade.

var (
	ftlComment = regexp.MustCompile(`(?s)<#--.*?-->`)
	// instanceTag casa o envelope de instância de widget na página do portal.
	instanceTag = regexp.MustCompile(`<div[^>]*\bid="_instance_(\d+)_"[^>]*>`)
	appcodeAttr = regexp.MustCompile(`\bappcode="([^"]*)"`)
)

// renderViewFTL renderiza um view.ftl para desenvolvimento. Erro = FTL usa
// recursos além de ${instanceId}/comentários (o chamador cai para o servidor).
func renderViewFTL(path, instanceID string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := ftlComment.ReplaceAllString(string(b), "")
	s = strings.ReplaceAll(s, "${instanceId}", instanceID)
	for _, marker := range []string{"${", "<#", "<@"} {
		if strings.Contains(s, marker) {
			return "", fmt.Errorf("view.ftl usa FreeMarker além de ${instanceId} (%q) — mantido o render do servidor", marker)
		}
	}
	return s, nil
}

// rewriteWidgetBlocks troca, no HTML do portal, a saída renderizada do
// view.ftl de cada widget do projeto pela versão local. Blocos de widgets de
// fora do projeto (ou com FTL não suportado) ficam intactos.
func (s *Server) rewriteWidgetBlocks(page string) string {
	locs := instanceTag.FindAllStringSubmatchIndex(page, -1)
	if len(locs) == 0 {
		return page
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		tag := page[loc[0]:loc[1]]
		instanceID := page[loc[2]:loc[3]]
		am := appcodeAttr.FindStringSubmatch(tag)
		if am == nil {
			continue
		}
		m, ok := s.mounts.byAppCode(am[1])
		if !ok {
			continue
		}
		rendered, err := renderViewFTL(m.viewFTL, instanceID)
		if err != nil {
			s.warnOnce("ftl:"+m.appCode, "widget %s: %v", m.appCode, err)
			continue
		}
		// A saída do FTL é o div marcado com wcm-widget-class dentro do
		// envelope; o envelope termina no próximo _instance_ (ou no fim).
		blockEnd := len(page)
		for _, nxt := range locs {
			if nxt[0] > loc[1] {
				blockEnd = nxt[0]
				break
			}
		}
		start, end := findWidgetOutput(page, loc[1], blockEnd)
		if start < 0 || start < last {
			continue
		}
		b.WriteString(page[last:start])
		b.WriteString(rendered)
		last = end
	}
	if last == 0 {
		return page
	}
	b.WriteString(page[last:])
	return b.String()
}

// findWidgetOutput localiza [start,end) do div raiz da saída do view.ftl —
// o primeiro div com wcm-widget-class após from — fechando por contagem de
// profundidade. Devolve start=-1 se o marcador não existe no intervalo.
func findWidgetOutput(page string, from, until int) (start, end int) {
	region := page[from:until]
	mark := strings.Index(region, "wcm-widget-class")
	if mark < 0 {
		return -1, -1
	}
	start = strings.LastIndex(region[:mark], "<div")
	if start < 0 {
		return -1, -1
	}
	depth := 0
	for i := start; i < len(region); {
		switch {
		case strings.HasPrefix(region[i:], "<div"):
			depth++
			i += 4
		case strings.HasPrefix(region[i:], "</div>"):
			depth--
			i += 6
			if depth == 0 {
				return from + start, from + i
			}
		default:
			i++
		}
	}
	return -1, -1 // div não fecha no envelope: melhor não mexer
}

// warnOnce emite cada aviso uma única vez por chave (evita spam a cada página).
func (s *Server) warnOnce(key, format string, args ...any) {
	s.warnedMu.Lock()
	seen := s.warned[key]
	s.warned[key] = true
	s.warnedMu.Unlock()
	if !seen {
		s.opts.Warnf(format, args...)
	}
}

// clearWarn re-arma um aviso do warnOnce (usado quando o arquivo muda).
func (s *Server) clearWarn(key string) {
	s.warnedMu.Lock()
	delete(s.warned, key)
	s.warnedMu.Unlock()
}
