package devserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/project"
)

// Simulação de contexto de processo no preview de formulários.
//
// Formulário de processo depende do displayFields (events/displayFields.js),
// que roda NO SERVIDOR quando o portal renderiza o form numa solicitação: lê
// as variáveis de workflow (getValue("WKNumState"), "WKUser", …) e copia
// valores para os campos via form.setValue — o JS cliente do formulário lê
// esses campos para mostrar/esconder as seções de cada etapa. No preview cru
// o evento nunca roda e o formulário "some".
//
// O preview resolve executando o displayFields LOCAL **no navegador**, com a
// API server-side emulada em JS (shims):
//   - getValue(WK*) lê de um painel flutuante (persistido em localStorage);
//   - form.setValue/getValue/getFormMode/setEnabled operam no DOM;
//   - DatasetFactory server-side é um wrapper sobre o DatasetFactory cliente
//     da própria página (que já funciona no preview: mesma origem do proxy
//     autenticado), acrescentando getValue(row, col)/rowsCount ao resultado.
// Evento que use interop Java do Rhino (importClass etc.) falha no try/catch
// e o painel mostra o erro — o form fica como no preview cru.
//
// O painel descobre os valores plausíveis de WKNumState pela API REST de
// process-management (via /_dev/api/formsim/*): com o formulário vinculado
// (.fluigcli/forms.json), o processo é auto-detectado casando o documentId
// com o formId das versões (GET /v2/processes?expand=versions) e as etapas
// reais vêm de GET .../process-versions/{v}/states. Sem vínculo, o usuário
// escolhe o processo na lista — ou digita o número da etapa direto.

// --- injeção no HTML do preview ---

// displayFieldsFile é o único evento de formulário executado no render.
const displayFieldsFile = "displayFields.js"

// injectFormSim acrescenta ao HTML do preview o bootstrap da simulação (com o
// fonte do displayFields local embutido) e o runtime /_dev/formsim.js.
func (s *Server) injectFormSim(page []byte, folder, formDir string) []byte {
	var event any
	if b, err := os.ReadFile(filepath.Join(project.FormEventsDir(formDir), displayFieldsFile)); err == nil {
		event = string(b)
	}
	boot, err := json.Marshal(map[string]any{
		"folder":    folder,
		"event":     event, // nil sem events/displayFields.js
		"companyId": s.opts.CompanyID,
	})
	if err != nil {
		return page
	}
	// json.Marshal escapa < > & (<…) — seguro dentro de <script>.
	tag := "\n<script>window.__fluigcliFormSim=" + string(boot) + ";</script>\n" +
		"<script src=\"" + formSimJSPath + "\"></script>\n"
	out := string(page)
	if i := strings.LastIndex(strings.ToLower(out), "</body>"); i >= 0 {
		return []byte(out[:i] + tag + out[i:])
	}
	return []byte(out + tag)
}

// --- API local do painel (/_dev/api/formsim/*) ---

const (
	formSimJSPath  = "/_dev/formsim.js"
	formSimAPIPath = "/_dev/api/formsim/"
)

// formSimContext é a resposta de /_dev/api/formsim/context.
type formSimContext struct {
	UserCode  string                  `json:"userCode"`
	CompanyID int                     `json:"companyId"`
	Form      *formSimFormLink        `json:"form"`      // vínculo do forms.json (null sem vínculo)
	Processes []fluig.ProcessFormLink `json:"processes"` // processos cujo formId casa com o vínculo
}

type formSimFormLink struct {
	DocumentID int    `json:"documentId"`
	Name       string `json:"name"`
}

// formSimStates é a resposta de /_dev/api/formsim/states.
type formSimStates struct {
	ProcessID string               `json:"processId"`
	Version   int                  `json:"version"`
	States    []fluig.ProcessState `json:"states"`
}

// formSimCache evita repetir chamadas caras ao upstream durante a execução
// (expand=versions e states mudam raramente); force=1 no painel renova.
type formSimCache struct {
	mu        sync.Mutex
	contexts  map[string]*formSimContext
	states    map[string]*formSimStates
	processes []fluig.ProcessSummary
}

func (s *Server) handleFormSimJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(formSimJS))
}

func (s *Server) handleFormSimAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Client == nil {
		simError(w, http.StatusServiceUnavailable, "dev server sem cliente autenticado — simulação só com valores manuais")
		return
	}
	force := r.URL.Query().Get("force") == "1"
	switch strings.TrimPrefix(r.URL.Path, formSimAPIPath) {
	case "context":
		s.serveFormSimContext(w, r, force)
	case "processes":
		s.serveFormSimProcesses(w, r, force)
	case "states":
		s.serveFormSimStates(w, r, force)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveFormSimContext(w http.ResponseWriter, r *http.Request, force bool) {
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		simError(w, http.StatusBadRequest, "parâmetro folder é obrigatório")
		return
	}
	s.sim.mu.Lock()
	if s.sim.contexts == nil {
		s.sim.contexts = map[string]*formSimContext{}
	}
	cached := s.sim.contexts[folder]
	s.sim.mu.Unlock()
	if cached != nil && !force {
		simJSON(w, cached)
		return
	}

	ctx := r.Context()
	out := &formSimContext{CompanyID: s.opts.CompanyID}
	if code, err := s.opts.Client.ResolveUserCode(ctx); err == nil {
		out.UserCode = code
	} else {
		s.warnOnce("formsim:usercode", "simulação: não consegui resolver o userCode do usuário: %v", err)
	}
	// Vínculo local pasta↔formulário (forms.json) → processos com o mesmo formId.
	if s.opts.FormScope != "" {
		if m, err := project.LoadFormMap(s.opts.Root, s.opts.FormScope); err == nil {
			if link, ok := m.ByFolder(folder); ok {
				out.Form = &formSimFormLink{DocumentID: link.DocumentID, Name: link.Name}
				procs, err := s.opts.Client.FindProcessesByFormID(ctx, link.DocumentID)
				if err != nil {
					simError(w, http.StatusBadGateway, "falha ao procurar processos do formulário: "+err.Error())
					return
				}
				out.Processes = procs
			}
		}
	}
	s.sim.mu.Lock()
	s.sim.contexts[folder] = out
	s.sim.mu.Unlock()
	simJSON(w, out)
}

func (s *Server) serveFormSimProcesses(w http.ResponseWriter, r *http.Request, force bool) {
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
	simJSON(w, cached)
}

func (s *Server) serveFormSimStates(w http.ResponseWriter, r *http.Request, force bool) {
	processID := r.URL.Query().Get("process")
	if processID == "" {
		simError(w, http.StatusBadRequest, "parâmetro process é obrigatório")
		return
	}
	version, _ := strconv.Atoi(r.URL.Query().Get("version"))
	key := processID + "|" + strconv.Itoa(version)
	s.sim.mu.Lock()
	if s.sim.states == nil {
		s.sim.states = map[string]*formSimStates{}
	}
	cached := s.sim.states[key]
	s.sim.mu.Unlock()
	if cached != nil && !force {
		simJSON(w, cached)
		return
	}

	ctx := r.Context()
	if version <= 0 {
		// Sem versão (processo escolhido manualmente): usa a corrente, pelo
		// SOAP nativo já validado (getWorkFlowProcessVersion).
		v, err := s.opts.Client.WorkflowVersion(ctx, processID)
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao descobrir a versão do processo: "+err.Error())
			return
		}
		version = v
	}
	states, err := s.opts.Client.ProcessStates(ctx, processID, version)
	if err != nil {
		simError(w, http.StatusBadGateway, "falha ao listar as etapas do processo: "+err.Error())
		return
	}
	if states == nil {
		states = []fluig.ProcessState{}
	}
	out := &formSimStates{ProcessID: processID, Version: version, States: states}
	s.sim.mu.Lock()
	s.sim.states[key] = out
	s.sim.mu.Unlock()
	simJSON(w, out)
}

func simJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func simError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
