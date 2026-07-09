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
}

// Server é o dev server montado e pronto para rodar.
type Server struct {
	opts    Options
	mounts  *mountTable
	hub     *hub
	handler http.Handler
}

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
	}

	proxy := s.newProxy()
	mux := http.NewServeMux()
	mux.HandleFunc(reloadPath, s.handleReload)
	mux.HandleFunc("/_dev/forms/", s.handleFormPreview)
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
	_, _ = w.Write(injectReloadScript(b))
}

// serveFormsIndex lista os formulários locais com links de preview.
func (s *Server) serveFormsIndex(w http.ResponseWriter) {
	entries, _ := os.ReadDir(filepath.Join(s.opts.Root, project.FormsDirName))
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"pt-BR\"><head><meta charset=\"utf-8\">" +
		"<title>fluigcli dev — formulários</title></head><body>" +
		"<h1>Formulários do projeto</h1><p>Preview local (modo novo registro); " +
		"datasets e style guide vêm do servidor via proxy.</p><ul>")
	n := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fmt.Fprintf(&b, "<li><a href=\"/_dev/forms/%s/\">%s</a></li>",
			url.PathEscape(e.Name()), html.EscapeString(e.Name()))
		n++
	}
	b.WriteString("</ul>")
	if n == 0 {
		b.WriteString("<p>Nenhuma pasta em forms/ — baixe com <code>fluigcli form import</code>.</p>")
	}
	b.WriteString("</body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(injectReloadScript([]byte(b.String())))
}
