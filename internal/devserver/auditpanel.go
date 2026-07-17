package devserver

// Auditoria de Style Guide no preview de formulários: /_dev/api/audit roda o
// linter (internal/audit) na pasta do formulário exibido e o botão 🎨 da
// barra mostra o resultado — como o preview recarrega a cada salvamento, a
// auditoria reexecuta junto. Local e read-only (nada vai ao servidor).

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/alorenco/fluig-cli/internal/audit"
	"github.com/alorenco/fluig-cli/internal/project"
)

const (
	auditAPIPath        = "/_dev/api/audit"
	auditProjectAPIPath = "/_dev/api/audit/project"
)

// auditState guarda o catálogo embutido (carregado uma única vez) e o cache
// do resumo do projeto para o dashboard — invalidado a cada salvamento em
// forms//wcm/widget (fsnotify) e no clear-caches.
type auditState struct {
	once sync.Once
	cat  *audit.Catalog
	err  error

	projMu sync.Mutex
	proj   map[string]any // resumo do projeto (nil = recalcular)
}

func (s *Server) auditCatalog() (*audit.Catalog, error) {
	s.audit.once.Do(func() {
		s.audit.cat, s.audit.err = audit.Embedded()
	})
	return s.audit.cat, s.audit.err
}

// invalidateProjectAudit descarta o resumo cacheado (próximo GET recalcula).
func (s *Server) invalidateProjectAudit() {
	s.audit.projMu.Lock()
	s.audit.proj = nil
	s.audit.projMu.Unlock()
}

// handleAuditProjectAPI responde GET /_dev/api/audit/project com o resumo do
// linter no projeto inteiro (forms/ + wcm/widget/, os alvos default do
// `fluigcli audit`): contagens, top regras violadas e o total varrido.
func (s *Server) handleAuditProjectAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	s.audit.projMu.Lock()
	cached := s.audit.proj
	s.audit.projMu.Unlock()
	if cached != nil {
		simJSON(w, cached)
		return
	}

	cat, err := s.auditCatalog()
	if err != nil {
		simError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg, err := audit.LoadConfig(s.opts.Root)
	if err != nil {
		simError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := audit.Run(s.opts.Root, nil, cat, cfg)
	if err != nil {
		simError(w, http.StatusInternalServerError, err.Error())
		return
	}
	errCount, warnCount := 0, 0
	type ruleAgg struct {
		Rule     string `json:"rule"`
		Severity string `json:"severity"`
		Count    int    `json:"count"`
		Title    string `json:"title"` // hint da regra (audit.RuleTitles)
	}
	byRule := map[string]*ruleAgg{}
	for _, f := range res.Findings {
		if f.Severity == audit.SeverityError {
			errCount++
		} else {
			warnCount++
		}
		agg := byRule[f.Rule]
		if agg == nil {
			agg = &ruleAgg{Rule: f.Rule, Severity: string(f.Severity), Title: audit.RuleTitles[f.Rule]}
			byRule[f.Rule] = agg
		}
		agg.Count++
	}
	rules := make([]ruleAgg, 0, len(byRule))
	for _, agg := range byRule {
		rules = append(rules, *agg)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Count != rules[j].Count {
			return rules[i].Count > rules[j].Count
		}
		return rules[i].Rule < rules[j].Rule
	})
	payload := map[string]any{
		"counts":  map[string]int{"error": errCount, "warning": warnCount},
		"rules":   rules,
		"scanned": res.Scanned,
		"ignored": len(res.Ignored),
	}
	s.audit.projMu.Lock()
	s.audit.proj = payload
	s.audit.projMu.Unlock()
	simJSON(w, payload)
}

// handleAuditAPI responde GET /_dev/api/audit?form=<pasta> com os achados do
// linter para aquele formulário: {findings, counts, scanned}.
func (s *Server) handleAuditAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	folder := r.URL.Query().Get("form")
	if folder == "" {
		simError(w, http.StatusBadRequest, "parâmetro form obrigatório")
		return
	}
	// O nome vem do navegador — confina em forms/ (anti-traversal).
	dir, err := project.SafeJoin(filepath.Join(s.opts.Root, project.FormsDirName), folder)
	if err != nil {
		simError(w, http.StatusBadRequest, err.Error())
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		simError(w, http.StatusNotFound, "formulário não encontrado: "+folder)
		return
	}
	cat, err := s.auditCatalog()
	if err != nil {
		simError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg, err := audit.LoadConfig(s.opts.Root)
	if err != nil {
		simError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := audit.Run(s.opts.Root, []string{dir}, cat, cfg)
	if err != nil {
		simError(w, http.StatusInternalServerError, err.Error())
		return
	}
	errCount, warnCount := 0, 0
	for _, f := range res.Findings {
		if f.Severity == audit.SeverityError {
			errCount++
		} else {
			warnCount++
		}
	}
	findings := res.Findings
	if findings == nil {
		findings = []audit.Finding{}
	}
	simJSON(w, map[string]any{
		"findings": findings,
		"counts":   map[string]int{"error": errCount, "warning": warnCount},
		"scanned":  res.Scanned,
	})
}
