package devserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// Painel de logs do servidor: subtela /_dev/logs/ (molde do Dataset Lab) que
// acompanha o server.log ao vivo. Um poller por arquivo alimenta um hub SSE
// dedicado (molde do reload.go): o devserver consulta o fluigcliHelper por
// offset (fluig.ReadServerLog) enquanto houver navegador conectado e
// retransmite as linhas cruas — filtro de nível/palavra, pausa e colorização
// ficam no navegador (cada aba filtra por conta própria).

const (
	logPanelPath    = "/_dev/logs/"
	logFilesAPIPath = "/_dev/api/log/files"
	logStreamPath   = "/_dev/api/log/stream"
)

// logPollInterval é variável para os testes encurtarem a espera.
var logPollInterval = 2 * time.Second

const (
	logTailBacklog = 200 // entradas do tail inicial (semente do ring)
	logRingLines   = 400 // linhas guardadas para assinantes tardios
)

// logEvent é o payload de um evento SSE do stream de log.
type logEvent struct {
	File  string   `json:"file,omitempty"`  // primeiro evento da conexão
	Lines []string `json:"lines,omitempty"` // linhas novas (ou o backlog)
	Info  string   `json:"info,omitempty"`  // nota (ex.: rotação do arquivo)
	Error string   `json:"error,omitempty"` // falha (helper ausente, arquivo sumiu…)
}

// logHub é o poller + assinantes de UM arquivo de log. Vive enquanto houver
// assinante; o último unsubscribe derruba o poller e descarta o hub (a
// próxima visita recomeça com um tail fresco).
type logHub struct {
	file string
	srv  *Server

	mu     sync.Mutex
	subs   map[chan logEvent]struct{}
	ring   []string
	offset int64
	cancel context.CancelFunc
	closed bool
}

// logSubscribe registra um assinante do arquivo, criando o hub (e o poller)
// se for o primeiro. Devolve o backlog acumulado até aqui — a cópia é feita
// sob o mesmo lock do registro, então nada se perde entre backlog e eventos.
func (s *Server) logSubscribe(file string) (*logHub, chan logEvent, []string) {
	s.logMu.Lock()
	if s.logHubs == nil {
		s.logHubs = map[string]*logHub{}
	}
	h := s.logHubs[file]
	if h == nil {
		h = &logHub{file: file, srv: s, subs: map[chan logEvent]struct{}{}}
		s.logHubs[file] = h
	}
	s.logMu.Unlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan logEvent, 32)
	h.subs[ch] = struct{}{}
	backlog := append([]string(nil), h.ring...)
	if h.cancel == nil && !h.closed {
		base := s.runContext()
		ctx, cancel := context.WithCancel(base)
		h.cancel = cancel
		go h.poll(ctx)
	}
	return h, ch, backlog
}

// logUnsubscribe remove o assinante; o último derruba o poller e o hub.
func (s *Server) logUnsubscribe(h *logHub, ch chan logEvent) {
	h.mu.Lock()
	delete(h.subs, ch)
	last := len(h.subs) == 0
	if last && h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.mu.Unlock()
	if last {
		s.logMu.Lock()
		if s.logHubs[h.file] == h {
			delete(s.logHubs, h.file)
		}
		s.logMu.Unlock()
	}
}

// closeLogHubs encerra todos os streams (Shutdown do servidor).
func (s *Server) closeLogHubs() {
	s.logMu.Lock()
	hubs := make([]*logHub, 0, len(s.logHubs))
	for _, h := range s.logHubs {
		hubs = append(hubs, h)
	}
	s.logHubs = nil
	s.logMu.Unlock()
	for _, h := range hubs {
		h.mu.Lock()
		h.closed = true
		if h.cancel != nil {
			h.cancel()
			h.cancel = nil
		}
		for ch := range h.subs {
			close(ch)
			delete(h.subs, ch)
		}
		h.mu.Unlock()
	}
}

// runContext é a base dos pollers (o ctx do Run; Background nos testes, que
// servem o handler sem Run).
func (s *Server) runContext() context.Context {
	s.npmMu.Lock()
	defer s.npmMu.Unlock()
	if s.runCtx != nil {
		return s.runCtx
	}
	return context.Background()
}

// broadcast envia sem bloquear (assinante lento perde o evento — o ring
// não ajuda depois do backlog, mas o próximo poll segue normalmente).
func (h *logHub) broadcast(ev logEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(ev.Lines) > 0 {
		h.ring = append(h.ring, ev.Lines...)
		if len(h.ring) > logRingLines {
			h.ring = h.ring[len(h.ring)-logRingLines:]
		}
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// poll semeia o hub com um tail e segue lendo por offset até o cancel.
func (h *logHub) poll(ctx context.Context) {
	client := h.srv.opts.Client
	tail, err := client.TailServerLog(ctx, fluig.ServerLogTailOptions{File: h.file, Lines: logTailBacklog})
	if err != nil {
		h.broadcast(logEvent{Error: err.Error()})
		return
	}
	seed := make([]string, 0, len(tail.Entries))
	for _, entry := range tail.Entries {
		seed = append(seed, strings.Split(entry, "\n")...)
	}
	h.mu.Lock()
	h.offset = tail.Size
	h.mu.Unlock()
	if len(seed) > 0 {
		h.broadcast(logEvent{Lines: seed})
	}

	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		h.mu.Lock()
		offset := h.offset
		h.mu.Unlock()
		chunk, err := client.ReadServerLog(ctx, h.file, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			if failures == 1 {
				h.broadcast(logEvent{Error: err.Error()})
			}
			continue
		}
		failures = 0
		if chunk.Size < offset {
			h.mu.Lock()
			h.offset = 0
			h.mu.Unlock()
			h.broadcast(logEvent{Info: "arquivo rotacionado — recomeçando do início"})
			continue
		}
		h.mu.Lock()
		h.offset = chunk.To
		h.mu.Unlock()
		if chunk.Content == "" {
			continue
		}
		h.broadcast(logEvent{Lines: strings.Split(strings.TrimSuffix(chunk.Content, "\n"), "\n")})
	}
}

// handleLogPanel entrega a página do painel (dados via SSE + files API).
func (s *Server) handleLogPanel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(logPanelHTML))
}

// handleLogFilesAPI lista os arquivos do diretório de log (seletor do painel).
func (s *Server) handleLogFilesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Client == nil {
		simError(w, http.StatusServiceUnavailable, "dev server sem cliente autenticado — o painel de logs fica indisponível")
		return
	}
	files, err := s.opts.Client.ListServerLogs(r.Context())
	if err != nil {
		simError(w, http.StatusBadGateway, err.Error())
		return
	}
	if files == nil {
		files = []fluig.ServerLogFile{}
	}
	simJSON(w, map[string]any{"files": files})
}

// handleLogStream é o endpoint SSE do painel: backlog + linhas novas.
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	writeEv := func(ev logEvent) {
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
		fl.Flush()
	}
	file := strings.TrimSpace(r.URL.Query().Get("file"))
	if file == "" {
		file = fluig.DefaultServerLog
	}
	if s.opts.Client == nil {
		writeEv(logEvent{File: file, Error: "dev server sem cliente autenticado — o painel de logs fica indisponível"})
		return
	}

	h, ch, backlog := s.logSubscribe(file)
	defer s.logUnsubscribe(h, ch)
	writeEv(logEvent{File: file, Lines: backlog})

	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeEv(ev)
		case <-keepalive.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			fl.Flush()
		}
	}
}
