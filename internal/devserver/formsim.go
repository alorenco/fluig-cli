package devserver

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
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

// displayFieldsFile é o evento executado no render; validateFormFile é o que
// o Fluig roda ao salvar/movimentar — o painel simula os dois gatilhos.
const (
	displayFieldsFile = "displayFields.js"
	validateFormFile  = "validateForm.js"
)

// formWdkJSPath é a máquina real de tabelas pai×filho (wdkAddChild etc.) que
// o render 2.0 injeta no fim da página do formulário — validado na
// homologação em 2026-07-10 buscando o render real via streamcontrol.
const formWdkJSPath = "/ecm_resources/resources/assets/forms/wdkdetail.js"

// serverHasWdkDetail sonda (uma vez) a máquina wdkdetail.js no upstream.
func (s *Server) serverHasWdkDetail() bool {
	s.wdk.once.Do(func() {
		client := &http.Client{Jar: s.opts.Jar, Timeout: probeTimeout}
		resp, err := client.Get(s.opts.Upstream.String() + formWdkJSPath)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
		s.wdk.ok = resp.StatusCode == http.StatusOK
	})
	return s.wdk.ok
}

// formWdkProbe guarda o resultado (único) da sonda do wdkdetail.js.
type formWdkProbe struct {
	once sync.Once
	ok   bool
}

// serveFormScreenShell é a moldura do modo de tela: um iframe com a largura
// do dispositivo apontando para o preview (?framed=<modo>). O iframe tem
// viewport próprio, então o grid responsivo quebra linha de verdade.
func (s *Server) serveFormScreenShell(w http.ResponseWriter, folder, mode string) {
	width, height, label := 375, 812, "Celular"
	if mode == "tablet" {
		width, height, label = 768, 1024, "Tablet"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, formScreenShell,
		html.EscapeString(folder), label, width, height,
		width, height,
		html.EscapeString(folder), label, width, height,
		html.EscapeString((&url.URL{Path: "./", RawQuery: "framed=" + mode}).String()))
}

// formScreenShell é a página da moldura (self-contained; a barra do preview
// continua DENTRO do iframe, controlando o modo via window.top).
const formScreenShell = `<!doctype html><html><head><meta charset="utf-8">
<title>%s · %s %dx%d</title>
<style>
  body{margin:0;min-height:100vh;background:#39434d;display:flex;flex-direction:column;
    align-items:center;font:13px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
  .frame{margin:18px 0 28px;width:%dpx;max-width:96vw;height:min(%dpx,88vh);
    background:#fff;border-radius:14px;border:6px solid #1d2b36;
    box-shadow:0 14px 48px rgba(0,0,0,.45);overflow:hidden}
  .frame iframe{width:100%%;height:100%%;border:0}
  .top{color:#cdd6de;padding:12px 16px 0;display:flex;gap:14px;align-items:center}
  .top a{color:#8fd4e8}
</style></head><body>
<div class="top"><strong>%s</strong> · %s %dx%d <a href="./">sair do modo tela</a></div>
<div class="frame"><iframe src="%s"></iframe></div>
</body></html>`

// injectFormSim acrescenta ao HTML do preview o bootstrap da simulação (com o
// fonte do displayFields local embutido) e o runtime /_dev/formsim.js. Para
// formulários com tabela pai×filho (tablename=), injeta antes a máquina REAL
// do servidor (wdkdetail.js) — o runtime marca as linhas-modelo e semeia o
// WdksetNewId; sem a máquina (Fluig sem o arquivo), o runtime emula.
// screen ≠ "" indica que o preview está dentro da moldura de dispositivo.
func (s *Server) injectFormSim(page []byte, folder, formDir string, screen string) []byte {
	readEvent := func(name string) any {
		if b, err := os.ReadFile(filepath.Join(project.FormEventsDir(formDir), name)); err == nil {
			return string(b)
		}
		return nil
	}
	boot, err := json.Marshal(map[string]any{
		"folder":    folder,
		"event":     readEvent(displayFieldsFile), // nil sem events/displayFields.js
		"validate":  readEvent(validateFormFile),  // nil sem events/validateForm.js
		"companyId": s.opts.CompanyID,
		"screen":    screen, // ""|phone|tablet — estado do modo de tela
	})
	if err != nil {
		return page
	}
	out := string(page)
	wdkTag := ""
	if strings.Contains(strings.ToLower(out), "tablename=") && s.serverHasWdkDetail() {
		wdkTag = "<script src=\"" + formWdkJSPath + "\"></script>\n"
	}
	// json.Marshal escapa < > & (<…) — seguro dentro de <script>.
	tag := "\n" + wdkTag +
		"<script>window.__fluigcliFormSim=" + string(boot) + ";</script>\n" +
		"<script src=\"" + formSimJSPath + "\"></script>\n"
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

// formSimUser é um usuário para o seletor de WKUser do painel (dataset
// colleague — validado ao vivo em 2026-07-11: colunas colleagueId/
// colleagueName/active, active como string "true"/"false").
type formSimUser struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// formSimCache evita repetir chamadas caras ao upstream durante a execução
// (expand=versions, states e a lista de usuários mudam raramente); force=1
// no painel renova.
type formSimCache struct {
	mu          sync.Mutex
	contexts    map[string]*formSimContext
	states      map[string]*formSimStates
	processes   []fluig.ProcessSummary
	users       []formSimUser
	datasets    []fluig.DatasetSummary     // lista do dataset lab (datasetlab.go)
	formsList   []fluig.Form               // lista de formulários (explorador de processos)
	procDetails map[string]*procDetailResp // detalhe por id|version (explorador de processos)
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
	case "users":
		s.serveFormSimUsers(w, r, force)
	case "deploy/servers":
		s.serveDeployServers(w)
	case "deploy/forms":
		s.serveDeployForms(w, r)
	case "deploy/folders":
		s.serveDeployFolders(w, r)
	case "deploy":
		s.serveDeploy(w, r)
	default:
		http.NotFound(w, r)
	}
}

// serveFormSimUsers lista os usuários (dataset colleague) para o seletor de
// WKUser — uma consulta por execução, ordenada por nome no servidor.
func (s *Server) serveFormSimUsers(w http.ResponseWriter, r *http.Request, force bool) {
	s.sim.mu.Lock()
	cached := s.sim.users
	s.sim.mu.Unlock()
	if cached == nil || force {
		users, err := s.loadColleagueUsers(r.Context())
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao listar usuários (dataset colleague): "+err.Error())
			return
		}
		s.sim.mu.Lock()
		s.sim.users = users
		s.sim.mu.Unlock()
		cached = users
	}
	simJSON(w, cached)
}

// loadColleagueUsers consulta o dataset colleague (uma vez) e devolve os
// usuários (código/nome/ativo), ordenados por nome no servidor.
func (s *Server) loadColleagueUsers(ctx context.Context) ([]formSimUser, error) {
	res, err := s.opts.Client.QueryDataset(ctx, "colleague", fluig.DatasetQuery{
		Fields:  []string{"colleagueId", "colleagueName", "active"},
		OrderBy: "colleagueName",
	})
	if err != nil {
		return nil, err
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	users := make([]formSimUser, 0, len(res.Rows))
	for _, row := range res.Rows {
		code := str(row["colleagueId"])
		if code == "" {
			continue
		}
		users = append(users, formSimUser{
			Code:   code,
			Name:   str(row["colleagueName"]),
			Active: strings.EqualFold(str(row["active"]), "true"),
		})
	}
	return users, nil
}

// ensureUsers devolve a lista de usuários com cache de execução (reusa o
// mesmo s.sim.users do painel de simulação). Não força renovação.
func (s *Server) ensureUsers(ctx context.Context) ([]formSimUser, error) {
	s.sim.mu.Lock()
	cached := s.sim.users
	s.sim.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	users, err := s.loadColleagueUsers(ctx)
	if err != nil {
		return nil, err
	}
	s.sim.mu.Lock()
	s.sim.users = users
	s.sim.mu.Unlock()
	return users, nil
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
