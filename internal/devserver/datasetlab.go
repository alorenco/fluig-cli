package devserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// Dataset Lab: subtela web do dev server para montar e executar consultas de
// dataset e ver o resultado numa tabela — o ciclo "criei/editei um dataset,
// quero testar a consulta na hora". Só leitura; reusa fluig.QueryDataset
// (dataset-handle/search já validado) e fluig.ListDatasets — nenhum endpoint
// novo. Mesma mecânica do painel de simulação (formsim.go): a página é
// self-contained (datasetlabhtml.go) e conversa com /_dev/api/dataset/*.

const (
	datasetLabPath = "/_dev/datasets/"
	datasetAPIPath = "/_dev/api/dataset/"
)

// handleDatasetLab entrega a página do lab (dados via /_dev/api/dataset/*).
func (s *Server) handleDatasetLab(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(datasetLabHTML))
}

// handleDatasetAPI roteia as chamadas da API do lab. Sem cliente autenticado a
// consulta não existe (mesma guarda do formsim).
func (s *Server) handleDatasetAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Client == nil {
		simError(w, http.StatusServiceUnavailable, "dev server sem cliente autenticado — a consulta de datasets fica indisponível")
		return
	}
	force := r.URL.Query().Get("force") == "1"
	switch strings.TrimPrefix(r.URL.Path, datasetAPIPath) {
	case "list":
		s.serveDatasetList(w, r, force)
	case "fields":
		s.serveDatasetFields(w, r)
	case "query":
		s.serveDatasetQuery(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveDatasetList lista os datasets do servidor (select do lab), com cache de
// execução — a listagem muda pouco; force=1 renova (botão ↻ do painel).
func (s *Server) serveDatasetList(w http.ResponseWriter, r *http.Request, force bool) {
	s.sim.mu.Lock()
	cached := s.sim.datasets
	s.sim.mu.Unlock()
	if cached == nil || force {
		list, err := s.opts.Client.ListDatasets(r.Context())
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao listar datasets: "+err.Error())
			return
		}
		if list == nil {
			list = []fluig.DatasetSummary{}
		}
		s.sim.mu.Lock()
		s.sim.datasets = list
		s.sim.mu.Unlock()
		cached = list
	}
	simJSON(w, map[string]any{"datasets": cached})
}

// serveDatasetFields descobre as colunas de um dataset por uma consulta-sonda
// (limit 1) — popula os seletores de campos/ordenação/filtros do painel. É
// best-effort: datasets que exigem uma constraint obrigatória (ex.: sqlLimit)
// falham na sonda; devolvemos columns vazio + probeError e o usuário monta a
// consulta manualmente (as colunas se revelam no primeiro resultado real).
func (s *Server) serveDatasetFields(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		simError(w, http.StatusBadRequest, "parâmetro id é obrigatório")
		return
	}
	res, err := s.opts.Client.QueryDataset(r.Context(), id, fluig.DatasetQuery{Limit: 1})
	if err != nil {
		simJSON(w, map[string]any{"columns": []string{}, "probeError": err.Error()})
		return
	}
	cols := res.Columns
	if cols == nil {
		cols = []string{}
	}
	simJSON(w, map[string]any{"columns": cols})
}

// datasetQueryReq é o corpo do POST /_dev/api/dataset/query.
type datasetQueryReq struct {
	ID          string                 `json:"id"`
	Fields      []string               `json:"fields"`
	Constraints []datasetLabConstraint `json:"constraints"`
	OrderBy     string                 `json:"orderBy"`
	Limit       int                    `json:"limit"`
}

// datasetLabConstraint espelha fluig.DatasetConstraint no corpo JSON.
type datasetLabConstraint struct {
	Field   string `json:"field"`
	Initial string `json:"initial"`
	Final   string `json:"final"`
	Type    string `json:"type"` // MUST | MUST_NOT | SHOULD (vazio = MUST)
	Like    bool   `json:"like"`
}

// validConstraintTypes são os tipos aceitos pelo dataset-handle/search.
var validConstraintTypes = map[string]bool{"": true, "MUST": true, "MUST_NOT": true, "SHOULD": true}

// serveDatasetQuery executa a consulta e devolve colunas + linhas (com a
// semântica de null preservada), o tempo gasto e se o limite foi atingido.
func (s *Server) serveDatasetQuery(w http.ResponseWriter, r *http.Request) {
	var req datasetQueryReq
	if !decodeDeployBody(w, r, &req) {
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		simError(w, http.StatusBadRequest, "informe o dataset a consultar")
		return
	}
	cons := make([]fluig.DatasetConstraint, 0, len(req.Constraints))
	for _, c := range req.Constraints {
		if strings.TrimSpace(c.Field) == "" {
			continue // linha de filtro em branco: ignora
		}
		typ := strings.ToUpper(strings.TrimSpace(c.Type))
		if !validConstraintTypes[typ] {
			simError(w, http.StatusBadRequest, "tipo de filtro inválido: "+c.Type+" (use Must, MustNot ou Should)")
			return
		}
		cons = append(cons, fluig.DatasetConstraint{
			Field:   c.Field,
			Initial: c.Initial,
			Final:   c.Final,
			Type:    typ,
			Like:    c.Like,
		})
	}

	start := time.Now()
	res, err := s.opts.Client.QueryDataset(r.Context(), req.ID, fluig.DatasetQuery{
		Fields:      req.Fields,
		Constraints: cons,
		OrderBy:     strings.TrimSpace(req.OrderBy),
		Limit:       req.Limit,
	})
	elapsed := time.Since(start)
	if err != nil {
		simError(w, http.StatusBadGateway, err.Error())
		return
	}
	rows := res.Rows
	if rows == nil {
		rows = []map[string]*string{}
	}
	cols := res.Columns
	if cols == nil {
		cols = []string{}
	}
	simJSON(w, map[string]any{
		"columns":    cols,
		"rows":       rows, // valor nil → JSON null (célula ausente)
		"count":      len(rows),
		"durationMs": elapsed.Milliseconds(),
		// Aviso suave: com limite N e exatamente N linhas, pode haver mais.
		"truncated": req.Limit > 0 && len(rows) >= req.Limit,
		"limit":     req.Limit,
	})
}
