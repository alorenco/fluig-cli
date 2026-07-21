package devserver

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/project"
)

// Explorador de Processos: subtela /_dev/processes/ (molde do Dataset Lab)
// que reúne, só para leitura, tudo que o desenvolvedor precisa saber de um
// processo ao escrever formulário/scripts — códigos das etapas (WKNumState),
// quem atua em cada etapa (mecanismo + papel/grupo/campo), transições,
// regras de gateway, o formulário vinculado, os scripts (com presença local)
// e o diagrama SVG. Nenhum endpoint novo no servidor: agrega o export XML
// nativo (fluig.ProcessDetail) + ListForms + forms.json + scan local.

const (
	processExplorerPath = "/_dev/processes/"
	processAPIPath      = "/_dev/api/process/"
)

// procFormInfo descreve o formulário vinculado ao processo.
type procFormInfo struct {
	DocumentID int    `json:"documentId"`
	Name       string `json:"name,omitempty"`   // nome no servidor (ListForms)
	Folder     string `json:"folder,omitempty"` // pasta local vinculada (forms.json)
}

// procEventInfo é um evento do processo com a presença local anexada.
type procEventInfo struct {
	fluig.ProcessEventInfo
	LocalPath string `json:"localPath,omitempty"` // relativo à raiz; vazio = só no servidor
}

// procVersionInfo é uma versão para o seletor.
type procVersionInfo struct {
	Version int  `json:"version"`
	Active  bool `json:"active"`
}

// procDetailResp é a resposta agregada de /_dev/api/process/detail.
type procDetailResp struct {
	ID          string                     `json:"id"`
	Description string                     `json:"description"`
	Version     int                        `json:"version"`
	Author      string                     `json:"author,omitempty"`
	Active      bool                       `json:"active"`
	FormID      int                        `json:"formId"`
	Form        *procFormInfo              `json:"form,omitempty"`
	States      []fluig.ProcessStateDetail `json:"states"`
	Events      []procEventInfo            `json:"events"`
	Versions    []procVersionInfo          `json:"versions,omitempty"`
	DiagramSvg  string                     `json:"diagramSvg,omitempty"`
}

// handleProcessExplorer entrega a página (dados via /_dev/api/process/*).
func (s *Server) handleProcessExplorer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(processExplorerHTML))
}

// handleProcessAPI roteia as chamadas do explorador.
func (s *Server) handleProcessAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Client == nil {
		simError(w, http.StatusServiceUnavailable, "dev server sem cliente autenticado — o explorador de processos fica indisponível")
		return
	}
	force := r.URL.Query().Get("force") == "1"
	switch strings.TrimPrefix(r.URL.Path, processAPIPath) {
	case "list":
		s.serveProcessList(w, r, force)
	case "detail":
		s.serveProcessDetail(w, r, force)
	default:
		http.NotFound(w, r)
	}
}

// serveProcessList lista os processos (combobox), reusando o cache de execução
// do painel de simulação (s.sim.processes) — a mesma listagem.
func (s *Server) serveProcessList(w http.ResponseWriter, r *http.Request, force bool) {
	s.sim.mu.Lock()
	cached := s.sim.processes
	s.sim.mu.Unlock()
	if cached == nil || force {
		procs, err := s.opts.Client.ListProcesses(r.Context())
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao listar processos: "+err.Error())
			return
		}
		if procs == nil {
			procs = []fluig.ProcessSummary{}
		}
		s.sim.mu.Lock()
		s.sim.processes = procs
		s.sim.mu.Unlock()
		cached = procs
	}
	simJSON(w, map[string]any{"processes": cached})
}

// serveProcessDetail agrega o detalhamento de uma versão do processo. Cache
// por id|version (o export XML é a parte cara); a presença LOCAL de scripts é
// recomputada a cada request (scan barato) para refletir saves na hora.
func (s *Server) serveProcessDetail(w http.ResponseWriter, r *http.Request, force bool) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		simError(w, http.StatusBadRequest, "parâmetro id é obrigatório")
		return
	}
	version := 0
	if v := strings.TrimSpace(r.URL.Query().Get("version")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			simError(w, http.StatusBadRequest, "versão inválida: "+v)
			return
		}
		version = n
	}
	key := id + "|" + strconv.Itoa(version)

	s.sim.mu.Lock()
	if s.sim.procDetails == nil {
		s.sim.procDetails = map[string]*procDetailResp{}
	}
	cached := s.sim.procDetails[key]
	s.sim.mu.Unlock()

	if cached == nil || force {
		resp, err := s.buildProcessDetail(r, id, version)
		if err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				simError(w, http.StatusNotFound, "processo não encontrado: "+id)
				return
			}
			simError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.sim.mu.Lock()
		s.sim.procDetails[key] = resp
		s.sim.mu.Unlock()
		cached = resp
	}

	// Presença local dos scripts: recomputa sempre (reflete saves na hora).
	out := *cached
	out.Events = s.withLocalScripts(cached.ID, cached.Events)
	simJSON(w, out)
}

// buildProcessDetail busca e monta o detalhamento (parte cara, cacheável).
func (s *Server) buildProcessDetail(r *http.Request, id string, version int) (*procDetailResp, error) {
	ctx := r.Context()
	client := s.opts.Client
	d, err := client.ProcessDetail(ctx, id, version)
	if err != nil {
		return nil, err
	}
	// O export não carrega o id na raiz de forma confiável em todos os
	// campos — usamos o solicitado quando o parse vier vazio.
	if d.ID == "" {
		d.ID = id
	}

	resp := &procDetailResp{
		ID:          d.ID,
		Description: d.Description,
		Version:     d.Version,
		Author:      d.Author,
		Active:      d.Active,
		FormID:      d.FormID,
		States:      d.States,
		DiagramSvg:  d.DiagramSVG,
	}
	for _, ev := range d.Events {
		resp.Events = append(resp.Events, procEventInfo{ProcessEventInfo: ev})
	}

	// Versões (seletor). Best-effort: uma falha aqui não derruba o detalhe.
	if versions, err := client.ProcessVersions(ctx, id); err == nil {
		for _, v := range versions {
			resp.Versions = append(resp.Versions, procVersionInfo{Version: v.Version, Active: v.Active})
		}
		sort.SliceStable(resp.Versions, func(i, j int) bool { return resp.Versions[i].Version > resp.Versions[j].Version })
	} else {
		s.warnOnce("procexplorer:versions:"+id, "explorador de processos: não consegui listar versões de %s: %v", id, err)
	}

	// Formulário vinculado: nome no servidor + pasta local.
	if d.FormID > 0 {
		resp.Form = s.resolveProcessForm(ctx, d.FormID)
	}

	// Atribuição por usuário: resolve o userCode → nome (dataset colleague).
	s.enrichUserAssignments(ctx, resp.States)
	return resp, nil
}

// enrichUserAssignments preenche Assignment.Name das etapas atribuídas a um
// usuário/colaborador específico, resolvendo o userCode pelo dataset
// colleague. Best-effort: sem a lista, os chips caem no código (com aviso).
func (s *Server) enrichUserAssignments(ctx context.Context, states []fluig.ProcessStateDetail) {
	need := false
	for i := range states {
		if a := states[i].Assignment; a != nil && (a.Kind == "user" || a.Kind == "colleague") && a.Value != "" {
			need = true
			break
		}
	}
	if !need {
		return
	}
	users, err := s.ensureUsers(ctx)
	if err != nil {
		s.warnOnce("procexplorer:users", "explorador de processos: não consegui resolver nomes de usuário: %v", err)
		return
	}
	byCode := make(map[string]string, len(users))
	for _, u := range users {
		byCode[u.Code] = u.Name
	}
	for i := range states {
		a := states[i].Assignment
		if a != nil && (a.Kind == "user" || a.Kind == "colleague") {
			if name := byCode[a.Value]; name != "" {
				a.Name = name
			}
		}
	}
}

// resolveProcessForm descobre o nome do formulário no servidor (ListForms,
// cacheado) e a pasta local vinculada (forms.json do escopo). Best-effort:
// campos que não resolvem ficam vazios — o formId sozinho já é útil.
func (s *Server) resolveProcessForm(ctx context.Context, formID int) *procFormInfo {
	info := &procFormInfo{DocumentID: formID}
	if forms, err := s.processForms(ctx); err == nil {
		for _, f := range forms {
			if f.DocumentID == formID {
				info.Name = f.Description
				break
			}
		}
	} else {
		s.warnOnce("procexplorer:forms", "explorador de processos: não consegui listar formulários: %v", err)
	}
	if s.opts.FormScope != "" {
		if m, err := project.LoadFormMap(s.opts.Root, s.opts.FormScope); err == nil {
			if link, ok := m.ByDocumentID(formID); ok {
				info.Folder = link.Folder
			}
		}
	}
	return info
}

// processForms lista os formulários do servidor (nome ↔ documentId), com
// cache de execução (a listagem muda pouco; clear-caches renova).
func (s *Server) processForms(ctx context.Context) ([]fluig.Form, error) {
	s.sim.mu.Lock()
	cached := s.sim.formsList
	s.sim.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	code, err := s.opts.Client.ResolveUserCode(ctx)
	if err != nil {
		return nil, err
	}
	forms, err := s.opts.Client.ListForms(ctx, code)
	if err != nil {
		return nil, err
	}
	if forms == nil {
		forms = []fluig.Form{}
	}
	s.sim.mu.Lock()
	s.sim.formsList = forms
	s.sim.mu.Unlock()
	return forms, nil
}

// withLocalScripts anexa o caminho local de cada evento (workflow/scripts).
func (s *Server) withLocalScripts(processID string, events []procEventInfo) []procEventInfo {
	scripts, err := project.FindProcessScripts(s.opts.Root, processID)
	if err != nil || len(scripts) == 0 {
		return events
	}
	byEvent := map[string]string{}
	for _, sc := range scripts {
		if rel, err := filepath.Rel(s.opts.Root, sc.Path); err == nil {
			byEvent[sc.Event] = rel
		} else {
			byEvent[sc.Event] = sc.Path
		}
	}
	out := make([]procEventInfo, len(events))
	for i, ev := range events {
		ev.LocalPath = byEvent[ev.Event]
		out[i] = ev
	}
	return out
}
