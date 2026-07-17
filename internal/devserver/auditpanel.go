package devserver

// Auditoria de Style Guide no preview de formulários: /_dev/api/audit roda o
// linter (internal/audit) na pasta do formulário exibido e o botão 🎨 da
// barra mostra o resultado — como o preview recarrega a cada salvamento, a
// auditoria reexecuta junto. Local e read-only (nada vai ao servidor).

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/alorenco/fluig-cli/internal/audit"
	"github.com/alorenco/fluig-cli/internal/project"
)

const auditAPIPath = "/_dev/api/audit"

// auditState guarda o catálogo embutido, carregado uma única vez.
type auditState struct {
	once sync.Once
	cat  *audit.Catalog
	err  error
}

func (s *Server) auditCatalog() (*audit.Catalog, error) {
	s.audit.once.Do(func() {
		s.audit.cat, s.audit.err = audit.Embedded()
	})
	return s.audit.cat, s.audit.err
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
