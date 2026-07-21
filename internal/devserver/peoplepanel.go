package devserver

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// Subtela Pessoas: /_dev/people/ (molde do Dataset Lab / Explorador de
// Processos). Reúne, para o desenvolvedor, os usuários, grupos e papéis da
// plataforma — quem participa dos grupos/papéis que ele configura na
// atribuição dos processos — com um atalho para incluir/remover o PRÓPRIO
// usuário logado no dev de um grupo ou papel e um "Onde é usado?" que cruza
// grupo/papel com as etapas dos processos. Nenhum endpoint novo no Fluig:
// orquestra o cliente existente (ListAdminUsers, ListGroups/ListRoles,
// List*Users, Add/Remove*User, GetAdminUser) + o export XML já cacheado pelo
// explorador (procDetails). Só o usuário logado é escrito (opts.Username).

const (
	peoplePanelPath = "/_dev/people/"
	peopleAPIPath   = "/_dev/api/people/"
)

// peopleUsageHit é uma etapa de processo onde o grupo/papel atua.
type peopleUsageHit struct {
	ProcessID   string `json:"processId"`
	ProcessName string `json:"processName"`
	Sequence    int    `json:"sequence"` // WKNumState da etapa
	StateName   string `json:"stateName"`
	Mechanism   string `json:"mechanism,omitempty"`
}

// peopleUsageResp é o resultado do "Onde é usado?" (varredura dos processos).
type peopleUsageResp struct {
	Kind    string           `json:"kind"` // group | role
	Code    string           `json:"code"`
	Hits    []peopleUsageHit `json:"hits"`
	Scanned int              `json:"scanned"`          // processos varridos
	Failed  int              `json:"failed,omitempty"` // exports que falharam (pulados)
}

// handlePeoplePanel entrega a página (dados via /_dev/api/people/*).
func (s *Server) handlePeoplePanel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(peoplePanelHTML))
}

// handlePeopleAPI roteia as chamadas da subtela Pessoas.
func (s *Server) handlePeopleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Client == nil {
		simError(w, http.StatusServiceUnavailable, "dev server sem cliente autenticado — a subtela Pessoas fica indisponível")
		return
	}
	force := r.URL.Query().Get("force") == "1"
	switch strings.TrimPrefix(r.URL.Path, peopleAPIPath) {
	case "users":
		s.servePeopleUsers(w, r, force)
	case "groups":
		s.servePeopleGroups(w, r, force)
	case "roles":
		s.servePeopleRoles(w, r, force)
	case "members":
		s.servePeopleMembers(w, r, force)
	case "user":
		s.servePeopleUser(w, r)
	case "membership":
		s.servePeopleMembership(w, r)
	case "usage":
		s.servePeopleUsage(w, r, force)
	default:
		http.NotFound(w, r)
	}
}

// isAdminAuthErr detecta o 401 dos módulos /admin/api/v1 (sem privilégio
// administrativo). Espelha a detecção do status.go: o 401 chega como
// HTTPError ou como texto "Unauthorized" extraído do corpo HTML.
func isAdminAuthErr(err error) bool {
	var httpErr *fluig.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized {
		return true
	}
	return strings.Contains(err.Error(), "Unauthorized")
}

// peopleAPIError responde a falha de uma consulta admin. O 401 (falta de
// privilégio) vira um payload 200 {needsAdmin:true} — a página inteira depende
// de admin, então o banner explicativo é melhor que um erro seco por aba.
func peopleAPIError(w http.ResponseWriter, err error) {
	if isAdminAuthErr(err) {
		simJSON(w, map[string]any{"needsAdmin": true, "error": statusErrText(err)})
		return
	}
	simError(w, http.StatusBadGateway, err.Error())
}

// servePeopleUsers lista todos os usuários (inclui BLOCKED — o dev precisa ver
// membros inativos de um grupo). Cache de execução; force=1 renova.
func (s *Server) servePeopleUsers(w http.ResponseWriter, r *http.Request, force bool) {
	s.sim.mu.Lock()
	cached := s.sim.peopleUsers
	s.sim.mu.Unlock()
	if cached == nil || force {
		list, err := s.opts.Client.ListAdminUsers(r.Context(), fluig.AdminUserFilter{ShowInactive: true})
		if err != nil {
			peopleAPIError(w, err)
			return
		}
		if list == nil {
			list = []fluig.AdminUser{}
		}
		s.sim.mu.Lock()
		s.sim.peopleUsers = list
		s.sim.mu.Unlock()
		cached = list
	}
	simJSON(w, map[string]any{"users": cached, "me": s.opts.Username})
}

// servePeopleGroups lista todos os grupos. Cache de execução; force=1 renova.
func (s *Server) servePeopleGroups(w http.ResponseWriter, r *http.Request, force bool) {
	s.sim.mu.Lock()
	cached := s.sim.peopleGroups
	s.sim.mu.Unlock()
	if cached == nil || force {
		list, err := s.opts.Client.ListGroups(r.Context(), fluig.GroupFilter{})
		if err != nil {
			peopleAPIError(w, err)
			return
		}
		if list == nil {
			list = []fluig.Group{}
		}
		s.sim.mu.Lock()
		s.sim.peopleGroups = list
		s.sim.mu.Unlock()
		cached = list
	}
	simJSON(w, map[string]any{"groups": cached, "me": s.opts.Username})
}

// servePeopleRoles lista todos os papéis. Cache de execução; force=1 renova.
func (s *Server) servePeopleRoles(w http.ResponseWriter, r *http.Request, force bool) {
	s.sim.mu.Lock()
	cached := s.sim.peopleRoles
	s.sim.mu.Unlock()
	if cached == nil || force {
		list, err := s.opts.Client.ListRoles(r.Context(), fluig.RoleFilter{})
		if err != nil {
			peopleAPIError(w, err)
			return
		}
		if list == nil {
			list = []fluig.Role{}
		}
		s.sim.mu.Lock()
		s.sim.peopleRoles = list
		s.sim.mu.Unlock()
		cached = list
	}
	simJSON(w, map[string]any{"roles": cached, "me": s.opts.Username})
}

// peopleMemberKey normaliza a chave do cache de membros/uso.
func peopleMemberKey(kind, code string) string { return kind + "|" + code }

// servePeopleMembers lista os membros de um grupo ou papel. Cache por
// kind|code (invalidado na escrita e no clear-caches).
func (s *Server) servePeopleMembers(w http.ResponseWriter, r *http.Request, force bool) {
	kind, code, ok := peopleKindCode(w, r)
	if !ok {
		return
	}
	key := peopleMemberKey(kind, code)
	s.sim.mu.Lock()
	if s.sim.peopleMembers == nil {
		s.sim.peopleMembers = map[string][]fluig.AdminUser{}
	}
	cached, has := s.sim.peopleMembers[key]
	s.sim.mu.Unlock()
	if !has || force {
		var (
			list []fluig.AdminUser
			err  error
		)
		if kind == "group" {
			list, err = s.opts.Client.ListGroupUsers(r.Context(), code, 0)
		} else {
			list, err = s.opts.Client.ListRoleUsers(r.Context(), code, 0)
		}
		if err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				simError(w, http.StatusNotFound, err.Error())
				return
			}
			peopleAPIError(w, err)
			return
		}
		if list == nil {
			list = []fluig.AdminUser{}
		}
		s.sim.mu.Lock()
		s.sim.peopleMembers[key] = list
		s.sim.mu.Unlock()
		cached = list
	}
	simJSON(w, map[string]any{"kind": kind, "code": code, "members": cached, "me": s.opts.Username})
}

// servePeopleUser carrega um usuário com grupos e papéis (visão reversa).
func (s *Server) servePeopleUser(w http.ResponseWriter, r *http.Request) {
	login := strings.TrimSpace(r.URL.Query().Get("login"))
	if login == "" {
		simError(w, http.StatusBadRequest, "parâmetro login é obrigatório")
		return
	}
	u, err := s.opts.Client.GetAdminUser(r.Context(), login)
	if err != nil {
		if errors.Is(err, fluig.ErrNotFound) {
			simError(w, http.StatusNotFound, err.Error())
			return
		}
		peopleAPIError(w, err)
		return
	}
	simJSON(w, map[string]any{"user": u, "me": s.opts.Username})
}

// peopleMembershipReq é o corpo do POST /membership.
type peopleMembershipReq struct {
	Kind   string `json:"kind"`   // group | role
	Code   string `json:"code"`   // código do grupo/papel
	Action string `json:"action"` // add | remove
}

// servePeopleMembership inclui/remove o usuário LOGADO no dev (opts.Username)
// de um grupo ou papel. Só o próprio usuário é escrito — a superfície é
// mínima de propósito (o CRUD completo fica na CLI). Confirm simples em
// qualquer ambiente (decisão do mantenedor); a trava de produção não se aplica.
func (s *Server) servePeopleMembership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		simError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var req peopleMembershipReq
	if !decodeDeployBody(w, r, &req) {
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	code := strings.TrimSpace(req.Code)
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if kind != "group" && kind != "role" {
		simError(w, http.StatusBadRequest, "kind inválido: use group ou role")
		return
	}
	if code == "" {
		simError(w, http.StatusBadRequest, "informe o código do grupo/papel")
		return
	}
	if action != "add" && action != "remove" {
		simError(w, http.StatusBadRequest, "action inválida: use add ou remove")
		return
	}
	login := strings.TrimSpace(s.opts.Username)
	if login == "" {
		simError(w, http.StatusBadRequest, "não sei qual é o seu usuário — a identidade do servidor não foi resolvida (cadastre com server add ou defina FLUIGCLI_USERNAME)")
		return
	}

	ctx := r.Context()
	var err error
	switch {
	case kind == "group" && action == "add":
		err = s.opts.Client.AddGroupUser(ctx, code, login)
	case kind == "group" && action == "remove":
		err = s.opts.Client.RemoveGroupUser(ctx, code, login)
	case kind == "role" && action == "add":
		err = s.opts.Client.AddRoleUser(ctx, code, login)
	case kind == "role" && action == "remove":
		err = s.opts.Client.RemoveRoleUser(ctx, code, login)
	}
	if err != nil {
		if errors.Is(err, fluig.ErrNotFound) {
			simError(w, http.StatusNotFound, err.Error())
			return
		}
		peopleAPIError(w, err)
		return
	}
	// Invalida o cache de membros (a lista mudou). O "me" das listagens de
	// usuário não muda de estado; o cache de uso não depende de membership.
	s.sim.mu.Lock()
	if s.sim.peopleMembers != nil {
		delete(s.sim.peopleMembers, peopleMemberKey(kind, code))
	}
	s.sim.mu.Unlock()
	verbo := "incluído em"
	if action == "remove" {
		verbo = "removido de"
	}
	s.opts.Infof("pessoas: %s %s %q %s", login, map[string]string{"group": "grupo", "role": "papel"}[kind], code, verbo)
	simJSON(w, map[string]any{"ok": true, "kind": kind, "code": code, "action": action, "login": login})
}

// servePeopleUsage cruza um grupo/papel com as etapas dos processos ("Onde é
// usado?"). Varre o export XML de cada processo (cacheado, compartilhado com o
// explorador) e casa Assignment.Kind/Value. On-demand (botão); um export que
// falha é pulado (contado em Failed), sem abortar a varredura. Cache por
// kind|code.
func (s *Server) servePeopleUsage(w http.ResponseWriter, r *http.Request, force bool) {
	kind, code, ok := peopleKindCode(w, r)
	if !ok {
		return
	}
	key := peopleMemberKey(kind, code)
	s.sim.mu.Lock()
	if s.sim.peopleUsage == nil {
		s.sim.peopleUsage = map[string]*peopleUsageResp{}
	}
	cached := s.sim.peopleUsage[key]
	s.sim.mu.Unlock()
	if cached != nil && !force {
		simJSON(w, cached)
		return
	}

	ctx := r.Context()
	procs, err := s.opts.Client.ListProcesses(ctx)
	if err != nil {
		peopleAPIError(w, err)
		return
	}
	out := &peopleUsageResp{Kind: kind, Code: code, Hits: []peopleUsageHit{}}
	for _, p := range procs {
		d, err := s.ensureProcessDetail(ctx, p.ID, 0)
		if err != nil {
			out.Failed++
			s.warnOnce("people:usage:"+p.ID, "pessoas: não consegui detalhar o processo %s no \"onde é usado?\": %v", p.ID, err)
			continue
		}
		out.Scanned++
		for _, st := range d.States {
			a := st.Assignment
			if a == nil || a.Kind != kind || !strings.EqualFold(a.Value, code) {
				continue
			}
			out.Hits = append(out.Hits, peopleUsageHit{
				ProcessID:   d.ID,
				ProcessName: d.Description,
				Sequence:    st.Sequence,
				StateName:   st.Name,
				Mechanism:   a.Mechanism,
			})
		}
	}
	sort.SliceStable(out.Hits, func(i, j int) bool {
		if out.Hits[i].ProcessID != out.Hits[j].ProcessID {
			return out.Hits[i].ProcessID < out.Hits[j].ProcessID
		}
		return out.Hits[i].Sequence < out.Hits[j].Sequence
	})

	s.sim.mu.Lock()
	s.sim.peopleUsage[key] = out
	s.sim.mu.Unlock()
	simJSON(w, out)
}

// peopleKindCode lê e valida os parâmetros kind (group|role) e code.
func peopleKindCode(w http.ResponseWriter, r *http.Request) (kind, code string, ok bool) {
	kind = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	code = strings.TrimSpace(r.URL.Query().Get("code"))
	if kind != "group" && kind != "role" {
		simError(w, http.StatusBadRequest, "parâmetro kind inválido: use group ou role")
		return "", "", false
	}
	if code == "" {
		simError(w, http.StatusBadRequest, "parâmetro code é obrigatório")
		return "", "", false
	}
	return kind, code, true
}

// ensureProcessDetail devolve o detalhe do processo do cache (procDetails,
// compartilhado com o Explorador de Processos) ou o constrói e cacheia. Usa a
// mesma convenção de chave id|version do serveProcessDetail.
func (s *Server) ensureProcessDetail(ctx context.Context, id string, version int) (*procDetailResp, error) {
	key := id + "|" + strconv.Itoa(version)
	s.sim.mu.Lock()
	if s.sim.procDetails == nil {
		s.sim.procDetails = map[string]*procDetailResp{}
	}
	cached := s.sim.procDetails[key]
	s.sim.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	resp, err := s.buildProcessDetail(ctx, id, version)
	if err != nil {
		return nil, err
	}
	s.sim.mu.Lock()
	s.sim.procDetails[key] = resp
	s.sim.mu.Unlock()
	return resp, nil
}
