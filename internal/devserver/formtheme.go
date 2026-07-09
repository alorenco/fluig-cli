package devserver

import (
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

// formThemeInject espelha o bloco que o Fluig 2.0 acrescenta ao <head> (sem o
// script de tema dark, que depende do portal pai e é no-op no preview).
const formThemeInject = "\n<!-- fluigcli dev: emulação do render de formulários do Fluig 2.0 -->\n" +
	"<script src='/ecm_resources/resources/assets/forms/forms.js'></script>\n" +
	"<link type='text/css' rel='stylesheet' href='" + formThemeFlatCSS + "'/>\n" +
	"<link type='text/css' rel='stylesheet' href='/style-guide/css/animalia-icons.min.css'/>\n" +
	"<link type='text/css' rel='stylesheet' href='/style-guide/css/fluig-icons.min.css'/>\n"

// formThemeProbe descobre (uma vez) se o servidor tem o tema novo.
type formThemeProbe struct {
	once sync.Once
	v2   bool
}

// serverHasNewTheme sonda o CSS flat no upstream com a sessão do proxy.
func (s *Server) serverHasNewTheme() bool {
	s.theme.once.Do(func() {
		client := &http.Client{Jar: s.opts.Jar, Timeout: probeTimeout}
		resp, err := client.Get(s.opts.Upstream.String() + formThemeFlatCSS)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
		s.theme.v2 = resp.StatusCode == http.StatusOK
		if s.theme.v2 {
			s.opts.Infof("preview de formulários emulando o render do Fluig 2.0 (tema flat + forms.js injetados)")
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
		out = out[:i] + formThemeInject + out[i:]
	} else {
		out = formThemeInject + out
	}
	return []byte(out)
}
