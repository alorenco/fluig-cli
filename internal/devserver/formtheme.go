package devserver

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Emulação do render de formulários do Fluig 2.0.
//
// Ao servir um formulário (iframe do streamcontrol numa solicitação), o Fluig
// 2.0 transforma o HTML — validado na homologação diffando o fonte local com
// o render real (`/webdesk/streamcontrol/<formId>/...`):
//
//  1. reescreve as referências ao style guide legado (descontinuado na 2.0,
//     responde 404) para o tema novo: fluig-style-guide.min.css →
//     fluig-style-guide-flat.min.css;
//  2. injeta no fim do <head> o runtime de formulários
//     (/ecm_resources/resources/assets/forms/forms.js) e os CSS do tema novo
//     (flat + animalia-icons + fluig-icons) — é a "sobreposição forçada" que
//     faz formulários antigos renderizarem certo na 2.0.
//
// O preview replica as duas transformações para ficar fiel ao que o usuário
// vê no portal. (O servidor também vincula valores de card no <body> — fora
// do escopo do preview, que equivale ao modo "novo registro".)
//
// A emulação é condicionada ao servidor ter o tema novo (probe único no CSS
// flat): num Fluig 1.x o legado existe, nada é reescrito e o preview cru já é
// fiel.

// formThemeFlatCSS é o CSS do tema novo — a existência dele no servidor é o
// detector de Fluig 2.0.
const formThemeFlatCSS = "/style-guide/css/fluig-style-guide-flat.min.css"

// legacyFormCSS são os caminhos descontinuados na 2.0 que o servidor reescreve
// para o tema novo ao renderizar o formulário.
var legacyFormCSS = []string{
	"/style-guide/css/fluig-style-guide.min.css",
	"/portal/resources/style-guide/css/fluig-style-guide-flat.min.css",
	"/portal/resources/style-guide/css/fluig-style-guide.min.css",
}

// formThemeAssets são os assets que o render 2.0 acrescenta ao <head> (sem o
// script de tema dark, que depende do portal pai e é no-op no preview). Cada
// um é sondado individualmente: há servidores 2.0 em que parte deles responde
// 500 (observado na homologação com animalia-icons/fluig-icons) — injetar um
// asset quebrado só suja o console do preview.
var formThemeAssets = []struct{ tag, path string }{
	{"<script src='%s'></script>", "/ecm_resources/resources/assets/forms/forms.js"},
	{"<link type='text/css' rel='stylesheet' href='%s'/>", formThemeFlatCSS},
	{"<link type='text/css' rel='stylesheet' href='%s'/>", "/style-guide/css/animalia-icons.min.css"},
	{"<link type='text/css' rel='stylesheet' href='%s'/>", "/style-guide/css/fluig-icons.min.css"},
}

// formThemeProbe descobre (uma vez) se o servidor tem o tema novo e monta o
// bloco de injeção só com os assets que respondem 200.
type formThemeProbe struct {
	once   sync.Once
	v2     bool
	inject string
}

// serverHasNewTheme sonda o CSS flat (detector do 2.0) e os demais assets do
// tema no upstream, com a sessão do proxy.
func (s *Server) serverHasNewTheme() bool {
	s.theme.once.Do(func() {
		client := &http.Client{Jar: s.opts.Jar, Timeout: probeTimeout}
		ok := func(path string) bool {
			resp, err := client.Get(s.opts.Upstream.String() + path)
			if err != nil {
				return false
			}
			_ = resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		}
		if !ok(formThemeFlatCSS) {
			return
		}
		s.theme.v2 = true
		var b strings.Builder
		b.WriteString("\n<!-- fluigcli dev: emulação do render de formulários do Fluig 2.0 -->\n")
		var skipped []string
		for _, a := range formThemeAssets {
			if a.path == formThemeFlatCSS || ok(a.path) {
				fmt.Fprintf(&b, a.tag+"\n", a.path)
			} else {
				skipped = append(skipped, a.path)
			}
		}
		s.theme.inject = b.String()
		s.opts.Infof("preview de formulários emulando o render do Fluig 2.0 (tema flat + forms.js injetados)")
		if len(skipped) > 0 {
			s.opts.Warnf("preview: assets do tema indisponíveis no servidor e não injetados: %s", strings.Join(skipped, ", "))
		}
	})
	return s.theme.v2
}

// applyFormTheme aplica ao HTML do preview as transformações do render 2.0.
func (s *Server) applyFormTheme(page []byte) []byte {
	if !s.serverHasNewTheme() {
		return page
	}
	out := string(page)
	for _, legacy := range legacyFormCSS {
		out = strings.ReplaceAll(out, legacy, formThemeFlatCSS)
	}
	if i := strings.LastIndex(strings.ToLower(out), "</head>"); i >= 0 {
		out = out[:i] + s.theme.inject + out[i:]
	} else {
		out = s.theme.inject + out
	}
	return []byte(out)
}
