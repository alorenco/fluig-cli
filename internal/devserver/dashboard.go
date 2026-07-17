package devserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alorenco/fluig-cli/internal/project"
)

// Dashboard do dev server (rota / exata): acessos rápidos (formulários,
// portal, widgets do disco), o watch integrado (publicar ao salvar, por tipo
// de artefato — ponte com a CLI) e configurações leves (live reload, caches).

// WatchStatus é o estado do watch integrado, exposto no dashboard.
type WatchStatus struct {
	Available bool     `json:"available"` // há pastas observáveis e ponte ativa
	Enabled   bool     `json:"enabled"`
	Types     []string `json:"types"`  // tipos ligados (dataset|event|mechanism|form|workflow)
	Recent    []string `json:"recent"` // últimas publicações (mensagens do watch)
}

// WatchBridge é a ponte com o watch integrado, implementada pela CLI (o
// devserver não importa cobra/terminal).
type WatchBridge interface {
	Status() WatchStatus
	Set(enabled bool, types []string) error
}

// watchTypeLabels descreve os tipos publicáveis, na ordem de exibição —
// exatamente a cobertura do `fluigcli watch`.
var watchTypeLabels = []struct{ ID, Label string }{
	{"dataset", "Datasets"},
	{"event", "Eventos globais"},
	{"mechanism", "Mecanismos"},
	{"form", "Formulários"},
	{"workflow", "Scripts de processo"},
}

// reloadEnabled/reloadDebounce leem os controles dinâmicos do live reload.
func (s *Server) reloadEnabled() bool {
	s.dashMu.Lock()
	defer s.dashMu.Unlock()
	return !s.reloadOff
}

func (s *Server) reloadDebounceNow() time.Duration {
	s.dashMu.Lock()
	defer s.dashMu.Unlock()
	return s.debounceNow
}

// handleDash devolve os dados do dashboard.
func (s *Server) handleDash(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	formsCount := 0
	if entries, err := os.ReadDir(filepath.Join(s.opts.Root, project.FormsDirName)); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				formsCount++
			}
		}
	}
	types := make([]map[string]string, 0, len(watchTypeLabels))
	for _, t := range watchTypeLabels {
		types = append(types, map[string]string{"id": t.ID, "label": t.Label})
	}
	var watch any
	if s.opts.Watch != nil {
		watch = s.opts.Watch.Status()
	}
	// Widgets SPA: só o acionável — bundle desatualizado e o estado do npm
	// watch (ligável pelo dashboard). Sem widget SPA o card não aparece.
	npmOn := s.npmWatchOn()
	spa := []map[string]any{}
	for _, wgt := range project.FindSPAWidgets(s.opts.Root) {
		npm := s.npmStateOf(wgt.Code)
		if npm == "" {
			if npmOn {
				npm = "aguardando spawn"
			} else {
				npm = "parado"
			}
		}
		spa = append(spa, map[string]any{
			"code":  wgt.Code,
			"stale": project.StaleBundle(wgt),
			"npm":   npm,
		})
	}
	s.dashMu.Lock()
	reload := map[string]any{"enabled": !s.reloadOff, "debounceMs": s.debounceNow.Milliseconds()}
	s.dashMu.Unlock()

	companyID := s.opts.CompanyID
	if companyID <= 0 {
		companyID = 1
	}
	simJSON(w, map[string]any{
		"server": map[string]any{
			"name":      s.opts.ServerName,
			"env":       s.opts.ServerEnv,
			"url":       s.opts.Upstream.String(),
			"user":      s.opts.Username,
			"companyId": s.opts.CompanyID,
		},
		"uptimeSeconds": int(time.Since(s.startedAt).Seconds()),
		"portalPath":    "/portal/p/" + strconv.Itoa(companyID) + "/home",
		"formsCount":    formsCount,
		"spa":           spa,
		"npmWatch":      npmOn,
		"watch":         watch,
		"watchTypes":    types,
		"reload":        reload,
	})
}

// handleDashWatch liga/desliga o watch integrado e escolhe os tipos.
func (s *Server) handleDashWatch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if s.opts.Watch == nil {
		simError(w, http.StatusServiceUnavailable, "watch indisponível nesta execução")
		return
	}
	var req struct {
		Enabled bool     `json:"enabled"`
		Types   []string `json:"types"`
	}
	if !decodeDeployBody(w, r, &req) {
		return
	}
	valid := map[string]bool{}
	for _, t := range watchTypeLabels {
		valid[t.ID] = true
	}
	for _, t := range req.Types {
		if !valid[t] {
			simError(w, http.StatusBadRequest, "tipo de artefato desconhecido: "+t)
			return
		}
	}
	if err := s.opts.Watch.Set(req.Enabled, req.Types); err != nil {
		simError(w, http.StatusInternalServerError, err.Error())
		return
	}
	simJSON(w, s.opts.Watch.Status())
}

// handleDashNpmWatch liga/desliga o `npm run watch` das widgets SPA sem
// reiniciar o dev (pedido do mantenedor, 2026-07-17).
func (s *Server) handleDashNpmWatch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		simError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeDeployBody(w, r, &req) {
		return
	}
	if err := s.SetNpmWatch(req.Enabled); err != nil {
		simError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	simJSON(w, map[string]any{"enabled": s.npmWatchOn()})
}

// handleDashReload pausa/retoma o live reload e ajusta o debounce.
func (s *Server) handleDashReload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	var req struct {
		Enabled    bool  `json:"enabled"`
		DebounceMs int64 `json:"debounceMs"`
	}
	if !decodeDeployBody(w, r, &req) {
		return
	}
	if req.DebounceMs < 50 || req.DebounceMs > 10000 {
		simError(w, http.StatusBadRequest, "debounce fora da faixa (50–10000 ms)")
		return
	}
	s.dashMu.Lock()
	s.reloadOff = !req.Enabled
	s.debounceNow = time.Duration(req.DebounceMs) * time.Millisecond
	reload := map[string]any{"enabled": !s.reloadOff, "debounceMs": s.debounceNow.Milliseconds()}
	s.dashMu.Unlock()
	s.opts.Infof("live reload %s (debounce %dms)", map[bool]string{true: "ativo", false: "pausado"}[req.Enabled], req.DebounceMs)
	simJSON(w, reload)
}

// handleDashClearCaches zera os caches do painel de simulação e as conexões
// de publicação (a sessão do próprio dev fica).
func (s *Server) handleDashClearCaches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodPost {
		simError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	s.sim.mu.Lock()
	s.sim.contexts = nil
	s.sim.states = nil
	s.sim.processes = nil
	s.sim.users = nil
	s.sim.datasets = nil
	s.deploys = nil
	s.sim.mu.Unlock()
	s.invalidateProjectAudit()
	s.status.mu.Lock()
	s.status.payload = nil
	s.status.mu.Unlock()
	s.opts.Infof("caches do painel de simulação limpos")
	simJSON(w, map[string]any{"ok": true})
}

// serveDashboard entrega a página do dashboard (dados via /_dev/api/dash).
func (s *Server) serveDashboard(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(dashboardHTML))
}
