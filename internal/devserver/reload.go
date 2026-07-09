package devserver

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/alorenco/fluig-cli/internal/project"
)

// reloadPath é o endpoint SSE que o script injetado escuta.
const reloadPath = "/_dev/reload"

// hub distribui o sinal de reload para as conexões SSE abertas.
type hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func newHub() *hub {
	return &hub{subs: map[chan string]struct{}{}}
}

func (h *hub) subscribe() chan string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan string, 4)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
}

// broadcast envia sem bloquear: assinante com buffer cheio perde o evento
// (o próximo salvamento recarrega de qualquer forma).
func (h *hub) broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// closeAll encerra as conexões SSE (necessário para o Shutdown do servidor).
func (h *hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		close(ch)
		delete(h.subs, ch)
	}
}

// handleReload é o endpoint SSE.
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(": fluigcli dev\n\n"))
	fl.Flush()

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write([]byte("data: " + msg + "\n\n"))
			fl.Flush()
		case <-keepalive.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			fl.Flush()
		}
	}
}

// startWatcher observa forms/ e wcm/widget/ e dispara o reload (com debounce).
// Mudança em arquivo renderizado no servidor (view.ftl, .properties,
// application.info…) não recarrega — recarregar mentiria que a mudança
// apareceu; sai um aviso pedindo o deploy.
func (s *Server) startWatcher(ctx context.Context) (stop func(), err error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	watchedAny := false
	for _, dir := range []string{project.FormsDirName, project.WidgetsDir} {
		base := filepath.Join(s.opts.Root, dir)
		if addRecursive(w, base) == nil {
			watchedAny = true
		}
	}
	if !watchedAny {
		s.opts.Warnf("nenhuma pasta forms/ ou wcm/widget/ no projeto — live reload sem nada para observar")
	}

	go func() {
		var timer *time.Timer
		warned := map[string]time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename|fsnotify.Remove) == 0 {
					continue
				}
				inWidgets := within(s.opts.Root, project.WidgetsDir, ev.Name)
				if inWidgets {
					// Widget nova ou jboss-web.xml editado mudam o map-local.
					s.mounts.invalidate()
				}
				// Pasta nova entra na observação (sem recarregar: pasta vazia
				// não muda nada no navegador).
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = addRecursive(w, ev.Name)
					continue
				}
				if inWidgets {
					if m, ok := s.mounts.byViewFTL(ev.Name); ok {
						// view.ftl é rerenderizado localmente pelo proxy:
						// recarrega — e re-arma o aviso de FTL não suportado,
						// para o usuário saber se a edição destravou (ou não)
						// o render local.
						s.clearWarn("ftl:" + m.appCode)
					} else if code, serverSide := widgetServerSidePath(s.opts.Root, ev.Name); serverSide {
						// Avisa no máximo uma vez por widget a cada rajada.
						if time.Since(warned[code]) > 2*time.Second {
							warned[code] = time.Now()
							s.opts.Warnf("%s é renderizado no servidor — para ver essa mudança publique com: fluigcli widget export %s",
								relOrSelf(s.opts.Root, ev.Name), code)
						}
						continue
					}
				}
				if timer != nil {
					timer.Stop()
				}
				name := relOrSelf(s.opts.Root, ev.Name)
				timer = time.AfterFunc(s.opts.Debounce, func() {
					s.opts.Infof("mudança em %s — recarregando o navegador", name)
					s.hub.broadcast("reload")
				})
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return func() { _ = w.Close() }, nil
}

// addRecursive observa dir e todas as subpastas.
func addRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			return w.Add(p)
		}
		return nil
	})
}

// within informa se path está sob <root>/<sub>.
func within(root, sub, path string) bool {
	rel, err := filepath.Rel(filepath.Join(root, sub), path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// widgetServerSidePath identifica arquivos de widget que o navegador não
// carrega direto (viram parte do WAR renderizado no servidor): tudo que não
// está sob src/main/webapp/resources — view/edit.ftl, .properties,
// application.info, WEB-INF etc. Devolve o nome da pasta da widget.
func widgetServerSidePath(root, path string) (code string, serverSide bool) {
	rel, err := filepath.Rel(filepath.Join(root, project.WidgetsDir), path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return "", false // a própria pasta da widget
	}
	code = parts[0]
	staticPrefix := []string{"src", "main", "webapp", "resources"}
	for i, seg := range staticPrefix {
		if len(parts)-1 <= i || parts[1+i] != seg {
			return code, true
		}
	}
	return code, false
}

// relOrSelf encurta o caminho para exibição (relativo à raiz quando possível).
func relOrSelf(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}
