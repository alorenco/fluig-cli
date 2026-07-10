// Package devserver implementa o servidor de desenvolvimento do fluigcli:
// um proxy reverso autenticado do Fluig que serve do disco os arquivos de
// widget em edição (map-local), oferece preview de formulários locais e
// recarrega o navegador ao salvar (live reload via SSE).
//
// O pacote não importa cobra nem escreve no terminal — mensagens saem pelos
// callbacks Infof/Warnf, e a CLI faz a tradução para o Printer.
package devserver

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/project"
)

// Options configura o dev server.
type Options struct {
	Root     string         // raiz do projeto Fluig local
	Upstream *url.URL       // URL base do servidor Fluig
	Jar      http.CookieJar // sessão autenticada (compartilhada com o fluig.Client)
	Host     string         // endereço de escuta (padrão 127.0.0.1; ver ListensBeyondLoopback)
	Port     int            // porta local
	Debounce time.Duration  // espera após o salvamento antes de recarregar
	Infof    func(format string, args ...any)
	Warnf    func(format string, args ...any)

	// Simulação de processo no preview de formulários (formsim.go). Sem
	// Client o painel fica só com valores manuais (a API local responde 503).
	Client    *fluig.Client // cliente autenticado (processos, etapas, userCode)
	FormScope string        // chave do servidor no forms.json (Server.FormScopeKey)
	CompanyID int           // WKCompany simulado
}

// Server é o dev server montado e pronto para rodar.
type Server struct {
	opts    Options
	mounts  *mountTable
	hub     *hub
	handler http.Handler

	warnedMu sync.Mutex
	warned   map[string]bool // avisos já emitidos (warnOnce)

	theme formThemeProbe // detecção (única) do tema novo no servidor
	sim   formSimCache   // cache da API de simulação de formulários
}

// probeTimeout limita as sondagens que o dev server faz no upstream.
const probeTimeout = 15 * time.Second

const defaultDebounce = 500 * time.Millisecond

// New monta o dev server (sem abrir a porta ainda).
func New(opts Options) (*Server, error) {
	if opts.Root == "" || opts.Upstream == nil || opts.Jar == nil {
		return nil, errors.New("devserver: Root, Upstream e Jar são obrigatórios")
	}
	if opts.Debounce <= 0 {
		opts.Debounce = defaultDebounce
	}
	if opts.Infof == nil {
		opts.Infof = func(string, ...any) {}
	}
	if opts.Warnf == nil {
		opts.Warnf = func(string, ...any) {}
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	s := &Server{
		opts:   opts,
		mounts: newMountTable(opts.Root),
		hub:    newHub(),
		warned: map[string]bool{},
	}

	proxy := s.newProxy()
	mux := http.NewServeMux()
	mux.HandleFunc(reloadPath, s.handleReload)
	mux.HandleFunc("/_dev/forms/", s.handleFormPreview)
	mux.HandleFunc(formSimJSPath, s.handleFormSimJS)
	mux.HandleFunc(formSimAPIPath, s.handleFormSimAPI)
	mux.HandleFunc("/_dev/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/_dev/forms/", http.StatusFound)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if local, ok := s.mounts.resolve(r.URL.Path); ok {
				w.Header().Set("Cache-Control", "no-store")
				http.ServeFile(w, r, local)
				return
			}
		}
		proxy.ServeHTTP(w, r)
	})
	s.handler = mux
	return s, nil
}

// Addr é o endereço de escuta. O padrão é loopback: o proxy carrega a sessão
// autenticada do usuário. Outro endereço (ex.: IP de tailnet num servidor de
// desenvolvimento remoto) é escolha consciente de quem roda — a CLI avisa.
func (s *Server) Addr() string {
	return net.JoinHostPort(s.opts.Host, fmt.Sprint(s.opts.Port))
}

// URL é a origem que o navegador deve abrir.
func (s *Server) URL() string {
	return "http://" + s.Addr()
}

// ListensBeyondLoopback informa se o endereço de escuta é alcançável por
// outras máquinas (qualquer coisa que não seja loopback) — quem acessa a
// porta age no Fluig com a sessão do usuário, então isso merece aviso.
func (s *Server) ListensBeyondLoopback() bool {
	host := s.opts.Host
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

// Mounts descreve os map-locals ativos (para a CLI listar na largada).
func (s *Server) Mounts() []string {
	var out []string
	for _, m := range s.mounts.snapshot() {
		out = append(out, m.contextRoot+" → "+relOrSelf(s.opts.Root, m.dir))
	}
	sort.Strings(out)
	return out
}

// Run abre a porta e serve até o contexto ser cancelado (Ctrl+C).
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("não consegui abrir %s: %w", s.Addr(), err)
	}
	stopWatch, err := s.startWatcher(ctx)
	if err != nil {
		_ = ln.Close()
		return err
	}
	defer stopWatch()

	srv := &http.Server{Handler: s.handler}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		s.hub.closeAll() // derruba as conexões SSE para o Shutdown concluir
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		close(done)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-done
	return nil
}

// handleFormPreview serve o preview local de formulários em /_dev/forms/...
//
//	/_dev/forms/            → índice com os formulários do projeto
//	/_dev/forms/<pasta>/    → HTML principal da pasta (regra do export)
//	/_dev/forms/<pasta>/<f> → demais arquivos da pasta (css/js/imagens)
//
// Como a origem é a mesma do proxy, os caminhos absolutos que os formulários
// usam (/style-guide, /portal, /webdesk/vcXMLRPC.js, …) resolvem no servidor
// real com a sessão injetada — DatasetFactory funciona com dados reais.
func (s *Server) handleFormPreview(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/_dev/forms/")
	if rest == "" {
		s.serveFormsIndex(w)
		return
	}
	segs := strings.Split(strings.Trim(rest, "/"), "/")
	folder := segs[0]
	formDir, err := project.SafeJoin(filepath.Join(s.opts.Root, project.FormsDirName), folder)
	if err != nil {
		http.Error(w, "caminho inválido", http.StatusBadRequest)
		return
	}
	if st, err := os.Stat(formDir); err != nil || !st.IsDir() {
		http.NotFound(w, r)
		return
	}
	// /_dev/forms/<pasta> (sem barra): redireciona para os refs relativos resolverem.
	if len(segs) == 1 && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}
	if len(segs) == 1 {
		s.serveFormMain(w, r, formDir, folder)
		return
	}
	file, err := project.SafeJoin(formDir, segs[1:]...)
	if err != nil {
		http.Error(w, "caminho inválido", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, file)
}

// serveFormMain serve o HTML principal do formulário com o script de live
// reload injetado.
func (s *Server) serveFormMain(w http.ResponseWriter, r *http.Request, formDir, folder string) {
	fc, err := project.ReadFormFolder(formDir)
	if err != nil {
		http.Error(w, "não consegui ler a pasta do formulário", http.StatusInternalServerError)
		return
	}
	var names []string
	for _, f := range fc.Files {
		names = append(names, filepath.Base(f))
	}
	main := fluig.ChoosePrincipalFile(names, folder)
	if main == "" {
		http.Error(w, "a pasta não tem arquivo HTML", http.StatusNotFound)
		return
	}
	b, err := os.ReadFile(filepath.Join(formDir, main))
	if err != nil {
		http.Error(w, "não consegui ler o HTML principal", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	page := s.injectFormSim(s.applyFormTheme(b), folder, formDir)
	_, _ = w.Write(injectReloadScript(page))
}

// formsIndexCSS estiliza o índice de formulários. Self-contained de propósito
// (nada do style guide do servidor): o índice é da CLI, não do Fluig, e assim
// funciona igual em qualquer versão do servidor.
const formsIndexCSS = `
:root{--bg:#f4f6f8;--card:#fff;--txt:#1d2b36;--sub:#5a6b7b;--line:#e3e8ee;
  --accent:#0c9abe;--accent-txt:#fff;--shadow:0 1px 2px rgba(16,36,54,.08)}
@media(prefers-color-scheme:dark){:root{--bg:#12181f;--card:#1b232d;
  --txt:#e6edf3;--sub:#93a4b4;--line:#2b3742;--shadow:0 1px 2px rgba(0,0,0,.4)}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--txt);
  font:15px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
header{padding:28px 32px 20px;border-bottom:1px solid var(--line)}
header h1{margin:0;font-size:22px;font-weight:650}
header h1 small{color:var(--accent);font-weight:650}
header p{margin:6px 0 0;color:var(--sub);font-size:13.5px}
main{max-width:1080px;margin:0 auto;padding:24px 32px 48px}
.bar{display:flex;gap:12px;align-items:center;margin-bottom:20px}
.bar input{flex:1;max-width:420px;padding:9px 14px;border:1px solid var(--line);
  border-radius:8px;background:var(--card);color:var(--txt);font-size:14px;outline:none}
.bar input:focus{border-color:var(--accent)}
.bar .count{color:var(--sub);font-size:13px}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:14px}
a.card{display:block;background:var(--card);border:1px solid var(--line);
  border-radius:10px;padding:16px 18px;text-decoration:none;color:var(--txt);
  box-shadow:var(--shadow);transition:transform .08s,border-color .08s}
a.card:hover{transform:translateY(-2px);border-color:var(--accent)}
a.card .name{font-weight:600;font-size:14.5px;word-break:break-word}
a.card .meta{margin-top:6px;color:var(--sub);font-size:12.5px;display:flex;
  gap:8px;flex-wrap:wrap}
.badge{background:color-mix(in srgb,var(--accent) 12%,transparent);
  color:var(--accent);border-radius:5px;padding:1px 7px;font-size:11.5px;font-weight:600}
.empty{color:var(--sub);padding:40px 0;text-align:center}
.empty code{background:var(--card);border:1px solid var(--line);
  border-radius:5px;padding:2px 7px}`

// formsIndexJS filtra os cards pelo campo de busca.
const formsIndexJS = `document.getElementById("q").addEventListener("input",function(){
  var q=this.value.toLowerCase(),n=0;
  document.querySelectorAll("a.card").forEach(function(c){
    var hit=c.dataset.name.indexOf(q)>=0;c.style.display=hit?"":"none";if(hit)n++;});
  document.getElementById("count").textContent=n+" formulário(s)";});
document.getElementById("q").focus();`

// serveFormsIndex lista os formulários locais com links de preview.
func (s *Server) serveFormsIndex(w http.ResponseWriter) {
	entries, _ := os.ReadDir(filepath.Join(s.opts.Root, project.FormsDirName))
	type formCard struct{ name, files, events string }
	var cards []formCard
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		card := formCard{name: e.Name()}
		if fc, err := project.ReadFormFolder(filepath.Join(s.opts.Root, project.FormsDirName, e.Name())); err == nil {
			card.files = fmt.Sprintf("%d arquivo(s)", len(fc.Files))
			if n := len(fc.EventFiles); n > 0 {
				card.events = fmt.Sprintf("%d evento(s)", n)
			}
		}
		cards = append(cards, card)
	}
	sort.Slice(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].name) < strings.ToLower(cards[j].name)
	})

	var b strings.Builder
	fmt.Fprintf(&b, "<!DOCTYPE html><html lang=\"pt-BR\"><head><meta charset=\"utf-8\">"+
		"<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">"+
		"<title>fluigcli dev — formulários</title><style>%s</style></head><body>"+
		"<header><h1><small>fluigcli dev</small> · Formulários do projeto</h1>"+
		"<p>Preview local em modo novo registro — datasets, style guide e APIs vêm do servidor via proxy; salvar um arquivo recarrega o navegador.</p></header><main>", formsIndexCSS)
	if len(cards) == 0 {
		b.WriteString("<p class=\"empty\">Nenhuma pasta em <code>forms/</code> — baixe do servidor com <code>fluigcli form import</code>.</p>")
	} else {
		fmt.Fprintf(&b, "<div class=\"bar\"><input id=\"q\" type=\"search\" placeholder=\"Filtrar formulários…\">"+
			"<span class=\"count\" id=\"count\">%d formulário(s)</span></div><div class=\"grid\">", len(cards))
		for _, c := range cards {
			esc := html.EscapeString(c.name)
			fmt.Fprintf(&b, "<a class=\"card\" data-name=\"%s\" href=\"/_dev/forms/%s/\"><div class=\"name\">%s</div><div class=\"meta\">",
				html.EscapeString(strings.ToLower(c.name)), url.PathEscape(c.name), esc)
			if c.files != "" {
				fmt.Fprintf(&b, "<span>%s</span>", c.files)
			}
			if c.events != "" {
				fmt.Fprintf(&b, "<span class=\"badge\">%s</span>", c.events)
			}
			b.WriteString("</div></a>")
		}
		fmt.Fprintf(&b, "</div><script>%s</script>", formsIndexJS)
	}
	b.WriteString("</main></body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(injectReloadScript([]byte(b.String())))
}
