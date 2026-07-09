package devserver

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// projRoot monta um projeto Fluig de teste com uma widget (context-root do
// jboss-web.xml ≠ nome da pasta), uma widget sem jboss-web.xml e um formulário.
func projRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("wcm/widget/minha_widget/src/main/webapp/WEB-INF/jboss-web.xml",
		`<?xml version="1.0"?><jboss-web><context-root>/painel</context-root></jboss-web>`)
	write("wcm/widget/minha_widget/src/main/webapp/resources/js/painel.js", "// js local\n")
	write("wcm/widget/minha_widget/src/main/resources/view.ftl", "<div>${instanceId}</div>")
	write("wcm/widget/sem_jboss/src/main/webapp/resources/css/estilo.css", "body{}\n")
	write("forms/Meu Form/Meu Form.html", "<html><body><h1>form</h1></body></html>")
	write("forms/Meu Form/utils.css", ".x{}\n")
	return root
}

func TestMapLocalResolve(t *testing.T) {
	mt := newMountTable(projRoot(t))

	// Context-root do jboss-web.xml vale mais que o nome da pasta.
	if p, ok := mt.resolve("/painel/resources/js/painel.js"); !ok || !strings.HasSuffix(p, "painel.js") {
		t.Errorf("resolve por context-root: ok=%v p=%q", ok, p)
	}
	if _, ok := mt.resolve("/minha_widget/resources/js/painel.js"); ok {
		t.Error("nome da pasta não deveria resolver quando o jboss-web.xml define outro context-root")
	}
	// Sem jboss-web.xml, vale o nome da pasta.
	if _, ok := mt.resolve("/sem_jboss/resources/css/estilo.css"); !ok {
		t.Error("resolve pelo nome da pasta falhou")
	}
	// Sufixo de locale cai para o arquivo base (padrão do portal).
	if p, ok := mt.resolve("/painel/resources/js/painel_pt_BR.js"); !ok || !strings.HasSuffix(p, "painel.js") {
		t.Errorf("fallback de locale: ok=%v p=%q", ok, p)
	}
	// Arquivo que só existe no servidor segue para o proxy.
	if _, ok := mt.resolve("/painel/resources/js/gerado_no_servidor.js"); ok {
		t.Error("arquivo inexistente localmente deveria ir para o proxy")
	}
	// Traversal não sai da pasta da widget.
	if _, ok := mt.resolve("/painel/../forms/Meu Form/Meu Form.html"); ok {
		t.Error("traversal deveria ser rejeitado")
	}
	// view.ftl não é servível (fora de src/main/webapp).
	if _, ok := mt.resolve("/painel/view.ftl"); ok {
		t.Error("view.ftl não deveria ser servido pelo map-local")
	}
}

func TestMapLocalInvalidate(t *testing.T) {
	root := projRoot(t)
	mt := newMountTable(root)
	if _, ok := mt.resolve("/nova/resources/js/app.js"); ok {
		t.Fatal("widget ainda não existe")
	}
	p := filepath.Join(root, "wcm", "widget", "nova", "src", "main", "webapp", "resources", "js")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "app.js"), []byte("//"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Sem invalidate, o cache antigo vale; com invalidate, a widget nova entra.
	mt.invalidate()
	if _, ok := mt.resolve("/nova/resources/js/app.js"); !ok {
		t.Error("widget nova deveria resolver após invalidate")
	}
}

func TestWidgetServerSidePath(t *testing.T) {
	root := projRoot(t)
	cases := []struct {
		rel        string
		code       string
		serverSide bool
	}{
		{"wcm/widget/minha_widget/src/main/resources/view.ftl", "minha_widget", true},
		{"wcm/widget/minha_widget/src/main/resources/minha_widget_pt_BR.properties", "minha_widget", true},
		{"wcm/widget/minha_widget/src/main/webapp/WEB-INF/web.xml", "minha_widget", true},
		{"wcm/widget/minha_widget/pom.xml", "minha_widget", true},
		{"wcm/widget/minha_widget/src/main/webapp/resources/js/painel.js", "minha_widget", false},
	}
	for _, c := range cases {
		code, ss := widgetServerSidePath(root, filepath.Join(root, filepath.FromSlash(c.rel)))
		if code != c.code || ss != c.serverSide {
			t.Errorf("%s: code=%q serverSide=%v; quer %q/%v", c.rel, code, ss, c.code, c.serverSide)
		}
	}
	if _, ss := widgetServerSidePath(root, filepath.Join(root, "forms", "x.html")); ss {
		t.Error("arquivo fora de wcm/widget não é server-side de widget")
	}
}

func TestInjectReloadScript(t *testing.T) {
	out := string(injectReloadScript([]byte("<html><BODY>x</BODY></html>")))
	if !strings.Contains(out, "fluigcli-dev-reload") || !strings.HasSuffix(out, "</BODY></html>") {
		t.Errorf("injeção antes do </BODY> falhou: %q", out)
	}
	out = string(injectReloadScript([]byte("sem body")))
	if !strings.HasPrefix(out, "sem body") || !strings.Contains(out, "fluigcli-dev-reload") {
		t.Errorf("injeção por anexação falhou: %q", out)
	}
}

// newTestServer sobe o dev server (handler direto, sem porta fixa) contra um
// upstream fake, devolvendo também o jar da sessão.
func newTestServer(t *testing.T, upstream *httptest.Server) (*httptest.Server, *Server, http.CookieJar) {
	t.Helper()
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(u, []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "sessao-cli", Path: "/"}})
	s, err := New(Options{Root: projRoot(t), Upstream: u, Jar: jar, Port: 0, Debounce: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts, s, jar
}

func TestProxyReescreveEInjeta(t *testing.T) {
	var gotCookie, gotBrowserCookie, gotEncoding string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		gotBrowserCookie = ""
		if c, err := r.Cookie("do_navegador"); err == nil {
			gotBrowserCookie = c.Value
		}
		gotEncoding = r.Header.Get("Accept-Encoding")
		switch r.URL.Path {
		case "/portal/home":
			http.SetCookie(w, &http.Cookie{Name: "jwt.token", Value: "rotacionado", Path: "/"})
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<html><body><script>var serverURL = "`+
				"http://"+r.Host+`";</script></body></html>`)
		case "/redir":
			w.Header().Set("Location", "http://"+r.Host+"/portal/destino")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ts, _, jar := newTestServer(t, upstream)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/portal/home", nil)
	req.AddCookie(&http.Cookie{Name: "do_navegador", Value: "não-deve-passar"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(gotCookie, "JSESSIONIDSSO=sessao-cli") {
		t.Errorf("sessão da CLI não foi injetada: Cookie=%q", gotCookie)
	}
	if gotBrowserCookie != "" {
		t.Errorf("cookie do navegador vazou para o upstream: %q", gotBrowserCookie)
	}
	if gotEncoding != "gzip" {
		t.Errorf("Accept-Encoding deveria ser o gzip transparente do transport, veio %q", gotEncoding)
	}
	if len(resp.Header.Values("Set-Cookie")) != 0 {
		t.Error("Set-Cookie do Fluig não pode chegar ao navegador")
	}
	upURL, _ := url.Parse(upstream.URL)
	rotated := false
	for _, c := range jar.Cookies(upURL) {
		if c.Name == "jwt.token" && c.Value == "rotacionado" {
			rotated = true
		}
	}
	if !rotated {
		t.Error("Set-Cookie do upstream deveria rotacionar o jar do proxy")
	}
	s := string(body)
	if strings.Contains(s, upstream.URL) {
		t.Errorf("host real vazou no HTML: %s", s)
	}
	if !strings.Contains(s, `var serverURL = "`+ts.URL+`"`) {
		t.Errorf("serverURL deveria apontar para a origem local: %s", s)
	}
	if !strings.Contains(s, "fluigcli-dev-reload") {
		t.Error("script de live reload não foi injetado")
	}

	// Location absoluto reescrito para a origem local.
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp2, err := noRedirect.Get(ts.URL + "/redir")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if loc := resp2.Header.Get("Location"); loc != ts.URL+"/portal/destino" {
		t.Errorf("Location = %q, quer %q", loc, ts.URL+"/portal/destino")
	}
}

func TestMapLocalNoHandler(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "do servidor")
	}))
	defer upstream.Close()
	ts, _, _ := newTestServer(t, upstream)

	// Arquivo local vence, com cache desligado e query string ignorada.
	resp, err := http.Get(ts.URL + "/painel/resources/js/painel_pt_BR.js?v=123")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "// js local\n" {
		t.Errorf("map-local não serviu o arquivo do disco: %q", body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, quer no-store", cc)
	}
	// Sem arquivo local → upstream responde.
	resp2, err := http.Get(ts.URL + "/painel/resources/js/i18n_gerado.js")
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if !strings.Contains(string(body2), "do servidor") {
		t.Errorf("fallback para o upstream falhou: %q", body2)
	}
}

func TestFormPreview(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	ts, _, _ := newTestServer(t, upstream)

	// Índice lista o formulário (com escape no link).
	resp, _ := http.Get(ts.URL + "/_dev/forms/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Meu Form") {
		t.Errorf("índice não lista o formulário: %s", body)
	}

	// HTML principal com o script injetado.
	resp, _ = http.Get(ts.URL + "/_dev/forms/Meu%20Form/")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<h1>form</h1>") ||
		!strings.Contains(string(body), "fluigcli-dev-reload") {
		t.Errorf("preview do form: status=%d body=%s", resp.StatusCode, body)
	}

	// Arquivo auxiliar da pasta (referência relativa).
	resp, _ = http.Get(ts.URL + "/_dev/forms/Meu%20Form/utils.css")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != ".x{}\n" {
		t.Errorf("arquivo auxiliar: %q", body)
	}

	// Pasta inexistente → 404; traversal → 400.
	resp, _ = http.Get(ts.URL + "/_dev/forms/NaoExiste/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("pasta inexistente: status=%d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/_dev/forms/", nil)
	req.URL.Path = "/_dev/forms/../../segredo"
	req.URL.RawPath = ""
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("traversal no preview: status=%d", resp.StatusCode)
		}
	}
}

func TestListenAddr(t *testing.T) {
	u, _ := url.Parse("http://fluig:8080")
	jar, _ := cookiejar.New(nil)
	newS := func(host string) *Server {
		t.Helper()
		s, err := New(Options{Root: t.TempDir(), Upstream: u, Jar: jar, Host: host, Port: 8787})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	// Padrão: loopback.
	s := newS("")
	if s.Addr() != "127.0.0.1:8787" || s.ListensBeyondLoopback() {
		t.Errorf("padrão: addr=%q beyond=%v", s.Addr(), s.ListensBeyondLoopback())
	}
	for host, beyond := range map[string]bool{
		"127.0.0.1": false, "localhost": false, "::1": false,
		"100.101.102.103": true, // IP de tailnet
		"0.0.0.0":         true,
		"minha-maquina":   true, // hostname não-loopback: melhor avisar
	} {
		if got := newS(host).ListensBeyondLoopback(); got != beyond {
			t.Errorf("ListensBeyondLoopback(%q) = %v, quer %v", host, got, beyond)
		}
	}
	if addr := newS("::1").Addr(); addr != "[::1]:8787" {
		t.Errorf("IPv6 addr = %q", addr)
	}
}

func TestHub(t *testing.T) {
	h := newHub()
	a, b := h.subscribe(), h.subscribe()
	h.broadcast("reload")
	for _, ch := range []chan string{a, b} {
		select {
		case msg := <-ch:
			if msg != "reload" {
				t.Errorf("msg = %q", msg)
			}
		default:
			t.Error("assinante não recebeu o broadcast")
		}
	}
	h.unsubscribe(a)
	h.closeAll()
	if _, ok := <-b; ok {
		t.Error("closeAll deveria fechar os canais")
	}
}
