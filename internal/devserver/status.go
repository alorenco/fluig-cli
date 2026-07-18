package devserver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// Painel de status do servidor no dashboard: /_dev/api/status reusa as mesmas
// consultas do `fluigcli server status` (versão do produto, estatísticas e
// monitores do /environment). Stats e monitores exigem admin (401 sem o
// privilégio) — a versão não; o payload degrada campo a campo em vez de
// falhar inteiro, e o dashboard mostra o que veio.

const statusAPIPath = "/_dev/api/status"

// statusCacheTTL segura as consultas ao /environment entre polls do painel
// (o dashboard pede a cada 60s; o TTL menor garante dado fresco a cada poll
// sem martelar o servidor em aberturas de página simultâneas).
const statusCacheTTL = 55 * time.Second

// serverStatusCache guarda a última resposta montada do status.
type serverStatusCache struct {
	mu      sync.Mutex
	at      time.Time
	payload map[string]any
}

// statusErrText traduz o erro das consultas admin para o painel: 401 vira a
// explicação de privilégio (o caso comum). O 401 do /environment pode chegar
// como HTTPError ou como texto "Unauthorized" extraído do corpo HTML (vira
// errServerRejected) — os dois formatos são reconhecidos.
func statusErrText(err error) string {
	var httpErr *fluig.HTTPError
	if (errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized) ||
		strings.Contains(err.Error(), "Unauthorized") {
		return "requer usuário com privilégio administrativo no servidor"
	}
	return err.Error()
}

// handleStatusAPI responde GET /_dev/api/status com a saúde do servidor
// conectado. Sem Client (execução sem sessão) devolve só a identificação.
func (s *Server) handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	s.status.mu.Lock()
	if s.status.payload != nil && time.Since(s.status.at) < statusCacheTTL {
		payload := s.status.payload
		s.status.mu.Unlock()
		simJSON(w, payload)
		return
	}
	s.status.mu.Unlock()

	payload := map[string]any{
		"server": map[string]any{
			"name": s.opts.ServerName,
			"env":  s.opts.ServerEnv,
			"url":  s.opts.Upstream.String(),
			"user": s.opts.Username,
		},
	}
	if s.opts.Client == nil {
		payload["unavailable"] = "dev server sem sessão autenticada"
		simJSON(w, payload)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()

	if ver, err := s.opts.Client.ServerVersion(ctx); err != nil {
		payload["versionError"] = statusErrText(err)
	} else {
		payload["version"] = ver.String()
	}
	// Estado do fluigcliHelper (instalado? qual versão?) — erro aqui não é
	// crítico para o painel, só omite o item.
	if helper, err := s.opts.Client.HelperStatus(ctx); err == nil {
		payload["helper"] = helper
	}
	if stats, err := s.opts.Client.ServerStatistics(ctx); err != nil {
		payload["statsError"] = statusErrText(err)
	} else {
		payload["stats"] = stats
	}
	if monitors, err := s.opts.Client.ServerMonitors(ctx); err != nil {
		payload["monitorsError"] = statusErrText(err)
	} else {
		payload["monitors"] = monitors
	}

	s.status.mu.Lock()
	s.status.payload = payload
	s.status.at = time.Now()
	s.status.mu.Unlock()
	simJSON(w, payload)
}
