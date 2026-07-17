// Package fluig implementa o cliente da plataforma TOTVS Fluig (login, sessão
// e APIs REST/SOAP). Este pacote não importa cobra nem faz I/O de terminal —
// é a fronteira que permite promovê-lo a biblioteca pública no futuro.
package fluig

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Options configura um Client. LogWriter recebe o log de requisições quando
// Verbose=true (a CLI passa o stderr); credenciais nunca são logadas.
type Options struct {
	BaseURL      string // ex.: https://fluig.empresa.com.br:8443
	Username     string
	Password     string
	CompanyID    int
	Timeout      time.Duration
	Verbose      bool
	LogWriter    io.Writer
	SessionCache SessionCache // opcional: reaproveita a sessão entre execuções
}

// SessionCache persiste os cookies de sessão entre execuções (implementação em
// internal/config). O pacote fluig só define o contrato — mantém-se agnóstico de
// onde/como o cache é gravado.
type SessionCache interface {
	Load(key string) []*http.Cookie
	Save(key string, cookies []*http.Cookie) error
}

// Client fala com um servidor Fluig mantendo a sessão em cookie jar.
type Client struct {
	opts       Options
	http       *http.Client
	base       *url.URL
	userCode   string // cache do userCode real (findUserByLogin)
	sessionKey string // host|usuário — chave do cache de sessão
	cache      SessionCache

	serverVersion *ServerVersion // cache da versão do produto (ServerVersion)
}

const defaultTimeout = 30 * time.Second

// sessionJars guarda os cookie jars por host+porta+usuário durante a execução,
// para reutilizar sessão entre múltiplos Clients no mesmo processo.
var (
	sessionJarsMu sync.Mutex
	sessionJars   = map[string]http.CookieJar{}
)

func sessionJar(key string) http.CookieJar {
	sessionJarsMu.Lock()
	defer sessionJarsMu.Unlock()
	if jar, ok := sessionJars[key]; ok {
		return jar
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err) // cookiejar.New(nil) nunca falha
	}
	sessionJars[key] = jar
	return jar
}

// SessionKeyFor devolve a chave do cache de sessão (host|usuário) de um servidor.
func SessionKeyFor(baseURL, username string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || base.Host == "" {
		return "", fmt.Errorf("URL base inválida: %q", baseURL)
	}
	return base.Host + "|" + username, nil
}

func NewClient(opts Options) (*Client, error) {
	base, err := url.Parse(strings.TrimRight(opts.BaseURL, "/"))
	if err != nil || base.Host == "" {
		return nil, fmt.Errorf("URL base inválida: %q", opts.BaseURL)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	transport := http.DefaultTransport
	if opts.Verbose && opts.LogWriter != nil {
		transport = &loggingTransport{base: transport, w: opts.LogWriter}
	}

	sessionKey := base.Host + "|" + opts.Username
	return &Client{
		opts:       opts,
		base:       base,
		sessionKey: sessionKey,
		cache:      opts.SessionCache,
		http: &http.Client{
			Jar:       sessionJar(sessionKey),
			Timeout:   opts.Timeout,
			Transport: transport,
		},
	}, nil
}

// url monta a URL absoluta de um caminho da plataforma.
func (c *Client) url(path string) string {
	return c.base.String() + path
}

// BaseURL devolve uma cópia da URL base do servidor.
func (c *Client) BaseURL() *url.URL {
	u := *c.base
	return &u
}

// SessionJar expõe o cookie jar vivo da sessão. Quem fala com o servidor por
// fora do Client (ex.: o proxy do `fluigcli dev`) compartilha a mesma sessão —
// inclusive as rotações de cookie feitas pelo servidor, que invalidariam uma
// cópia estática dos cookies (o jwt.token expira).
func (c *Client) SessionJar() http.CookieJar {
	return c.http.Jar
}

// SaveSession persiste os cookies atuais no cache de sessão (no-op sem cache).
// Processos longos (ex.: `fluigcli dev`) chamam ao encerrar, para que as
// rotações de cookie acumuladas sobrevivam à execução.
func (c *Client) SaveSession() {
	c.saveSession()
}

// cookies retorna os cookies de sessão atuais para a URL base.
func (c *Client) cookies() []*http.Cookie {
	return c.http.Jar.Cookies(c.base)
}

const maxBodyLog = 4 * 1024

// readBody lê o corpo com limite, garantindo o reaproveitamento da conexão.
func readBody(resp *http.Response, limit int64) (string, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return string(data), err
}

// loggingTransport loga método, URL, status e duração no stderr. Headers e
// corpos nunca são logados — cookies e j_password ficam mascarados por design.
type loggingTransport struct {
	base http.RoundTripper
	w    io.Writer
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	fmt.Fprintf(t.w, "fluig: → %s %s\n", req.Method, maskURL(req.URL))
	resp, err := t.base.RoundTrip(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Fprintf(t.w, "fluig: ← erro após %s: %v\n", elapsed, err)
		return resp, err
	}
	fmt.Fprintf(t.w, "fluig: ← %d em %s\n", resp.StatusCode, elapsed)
	return resp, nil
}

// maskURL remove credenciais eventualmente presentes em query strings.
func maskURL(u *url.URL) string {
	masked := *u
	q := masked.Query()
	changed := false
	for key := range q {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "senha") {
			q.Set(key, "***")
			changed = true
		}
	}
	if changed {
		masked.RawQuery = q.Encode()
	}
	return masked.String()
}
