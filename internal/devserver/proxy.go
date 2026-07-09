package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
)

// originKey guarda no contexto da requisição a origem local (http://127.0.0.1:porta)
// vista pelo navegador, para a reescrita do corpo em ModifyResponse.
type originKey struct{}

// newProxy monta o proxy reverso autenticado. A sessão mora no jar do proxy
// (compartilhado com o fluig.Client): o cookie do navegador é descartado, os
// cookies da sessão são injetados na ida e os Set-Cookie do servidor voltam
// para o jar — o navegador nunca vê credenciais do Fluig.
func (s *Server) newProxy() *httputil.ReverseProxy {
	upstream := s.opts.Upstream
	jar := s.opts.Jar
	return &httputil.ReverseProxy{
		// FlushInterval negativo repassa respostas em streaming sem segurar
		// buffer (long-polling de notificações do portal).
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.Host = upstream.Host
			pr.Out.Header.Del("Cookie")
			for _, c := range jar.Cookies(upstream) {
				pr.Out.AddCookie(c)
			}
			// Sem Accept-Encoding explícito o transport negocia gzip e entrega
			// o corpo já descomprimido — necessário para reescrever o HTML.
			pr.Out.Header.Del("Accept-Encoding")
			origin := "http://" + pr.In.Host
			pr.Out = pr.Out.WithContext(context.WithValue(pr.Out.Context(), originKey{}, origin))
		},
		ModifyResponse: func(resp *http.Response) error {
			origin, _ := resp.Request.Context().Value(originKey{}).(string)
			// Set-Cookie fica no jar do proxy, fora do navegador.
			if len(resp.Header.Values("Set-Cookie")) > 0 {
				jar.SetCookies(upstream, resp.Cookies())
				resp.Header.Del("Set-Cookie")
			}
			// Redirect absoluto para o host real → origem local.
			if loc := resp.Header.Get("Location"); origin != "" && strings.HasPrefix(loc, upstream.String()) {
				resp.Header.Set("Location", origin+strings.TrimPrefix(loc, upstream.String()))
			}
			if origin == "" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
				return nil
			}
			// HTML: URLs absolutas do host real (WCMAPI.serverURL, links, logo)
			// viram a origem local; o markup das widgets do projeto é
			// rerenderizado do view.ftl local (ver ftl.go); e o script de live
			// reload entra no fim.
			b, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return err
			}
			body := strings.ReplaceAll(string(b), upstream.String(), origin)
			body = s.rewriteWidgetBlocks(body)
			out := injectReloadScript([]byte(body))
			resp.Body = io.NopCloser(strings.NewReader(string(out)))
			resp.ContentLength = int64(len(out))
			resp.Header.Set("Content-Length", fmt.Sprint(len(out)))
			resp.Header.Del("Content-Encoding")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.opts.Warnf("proxy: erro em %s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, "fluigcli dev: o servidor Fluig não respondeu", http.StatusBadGateway)
		},
	}
}

// reloadScript é injetado em toda resposta HTML: escuta o SSE do dev server e
// recarrega a página quando um arquivo observado muda.
const reloadScript = `<script id="fluigcli-dev-reload">(function(){` +
	`var es=new EventSource("` + reloadPath + `");` +
	`es.onmessage=function(e){if(e.data==="reload")location.reload();};` +
	`console.log("fluigcli dev: live reload ativo");})();</script>`

// injectReloadScript insere o script antes do </body> (ou anexa ao final).
func injectReloadScript(page []byte) []byte {
	s := string(page)
	if i := strings.LastIndex(strings.ToLower(s), "</body>"); i >= 0 {
		return []byte(s[:i] + reloadScript + s[i:])
	}
	return []byte(s + reloadScript)
}
