package devserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/fluig/soap"
	"github.com/alorenco/fluig-cli/internal/project"
)

// Publicação de formulário pela barra do preview (🚀): a mesma semântica do
// `fluigcli form export` — atualizar cria nova versão (ou mantém), criar
// exige pasta GED/dataset/persistência/descritor — só que dirigida pelo
// diálogo do navegador. O servidor alvo pode ser QUALQUER um cadastrado
// (decisão do mantenedor, 2026-07-11), inclusive produção mediante
// confirmação digitada do nome; a credencial vem de sessão em cache/keyring/
// env ou da senha digitada no diálogo (trafega só até o dev server local).

// DeployServerInfo descreve um servidor cadastrado para o diálogo.
type DeployServerInfo struct {
	Name    string `json:"name"`
	Env     string `json:"env"` // dev | hml | prod | ""
	URL     string `json:"url"`
	Default bool   `json:"default"` // padrão do projeto/global
	Current bool   `json:"current"` // o servidor da sessão do dev
}

// ErrDeployNeedsPassword sinaliza credencial indisponível para o servidor
// alvo — o diálogo deve pedir a senha e reenviar.
var ErrDeployNeedsPassword = errors.New("senha necessária para o servidor")

// deployConn é uma conexão autenticada com um servidor alvo (cacheada por
// execução: a senha digitada não fica guardada, a sessão sim).
type deployConn struct {
	client *fluig.Client
	scope  string // FormScopeKey do servidor (bucket do forms.json)
}

// deployClient resolve o cliente do servidor alvo: o da sessão do dev quando
// é o servidor conectado; senão a factory da CLI (sessão/keyring/env/senha).
func (s *Server) deployClient(ctx context.Context, name, password string) (*fluig.Client, string, error) {
	for _, info := range s.opts.DeployServers {
		if info.Name == name && info.Current && s.opts.Client != nil {
			return s.opts.Client, s.opts.FormScope, nil
		}
	}
	s.sim.mu.Lock()
	if s.deploys == nil {
		s.deploys = map[string]deployConn{}
	}
	cached, ok := s.deploys[name]
	s.sim.mu.Unlock()
	if ok && password == "" {
		return cached.client, cached.scope, nil
	}
	if s.opts.DeployConnect == nil {
		return nil, "", errors.New("publicação em outros servidores indisponível nesta execução")
	}
	client, scope, err := s.opts.DeployConnect(ctx, name, password)
	if err != nil {
		return nil, "", err
	}
	s.sim.mu.Lock()
	s.deploys[name] = deployConn{client: client, scope: scope}
	s.sim.mu.Unlock()
	return client, scope, nil
}

func (s *Server) serveDeployServers(w http.ResponseWriter) {
	servers := s.opts.DeployServers
	if servers == nil {
		servers = []DeployServerInfo{}
	}
	simJSON(w, map[string]any{"servers": servers})
}

// deployAuthReq é o corpo comum dos POSTs do diálogo (a senha só trafega do
// navegador ao dev server local e não é logada nem persistida).
type deployAuthReq struct {
	Server   string `json:"server"`
	Password string `json:"password"`
	Folder   string `json:"folder"`
}

// deployFormInfo é um formulário do servidor para o seletor (dataset e campo
// descritor entram nos defaults do diálogo de atualização — padrão Fluig).
type deployFormInfo struct {
	DocumentID      int    `json:"documentId"`
	Name            string `json:"name"`
	DatasetName     string `json:"datasetName"`
	CardDescription string `json:"cardDescription"`
}

// serveDeployFolders navega as pastas do GED do servidor alvo: parentId 0 =
// raízes, senão as subpastas (SOAP ECMFolderService, validado ao vivo).
func (s *Server) serveDeployFolders(w http.ResponseWriter, r *http.Request) {
	var req struct {
		deployAuthReq
		ParentID int `json:"parentId"`
	}
	if !decodeDeployBody(w, r, &req) {
		return
	}
	client, _, err := s.deployClient(r.Context(), req.Server, req.Password)
	if err != nil {
		deployAuthError(w, err)
		return
	}
	folders, err := client.ListGEDFolders(r.Context(), req.ParentID)
	if err != nil {
		simError(w, http.StatusBadGateway, "falha ao listar as pastas do GED: "+err.Error())
		return
	}
	if folders == nil {
		folders = []fluig.GEDFolder{}
	}
	simJSON(w, map[string]any{"folders": folders})
}

func (s *Server) serveDeployForms(w http.ResponseWriter, r *http.Request) {
	var req deployAuthReq
	if !decodeDeployBody(w, r, &req) {
		return
	}
	ctx := r.Context()
	client, scope, err := s.deployClient(ctx, req.Server, req.Password)
	if err != nil {
		deployAuthError(w, err)
		return
	}
	pub, err := client.ResolveUserCode(ctx)
	if err != nil {
		simError(w, http.StatusBadGateway, "não consegui resolver o usuário no servidor alvo: "+err.Error())
		return
	}
	forms, err := client.ListForms(ctx, pub)
	if err != nil {
		simError(w, http.StatusBadGateway, "falha ao listar os formulários do servidor: "+err.Error())
		return
	}
	out := make([]deployFormInfo, 0, len(forms))
	for _, f := range forms {
		out = append(out, deployFormInfo{
			DocumentID: f.DocumentID, Name: f.Description,
			DatasetName: f.DatasetName, CardDescription: f.CardDescription,
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })

	linked := 0
	if m, err := project.LoadFormMap(s.opts.Root, scope); err == nil {
		if link, ok := m.ByFolder(req.Folder); ok {
			linked = link.DocumentID
		}
	}
	// Datasets para o campo da criação (sugestão via datalist) — falha aqui
	// não impede o diálogo.
	var datasets []string
	if ds, err := client.ListDatasets(ctx); err == nil {
		for _, d := range ds {
			datasets = append(datasets, d.ID)
		}
		sort.Strings(datasets)
	}
	simJSON(w, map[string]any{"forms": out, "linkedDocumentId": linked, "datasets": datasets})
}

// deployReq é o pedido de publicação.
type deployReq struct {
	deployAuthReq
	DocumentID  int    `json:"documentId"`  // >0 = atualizar esse form
	VersionMode string `json:"versionMode"` // keep (default do diálogo) | new
	Confirm     string `json:"confirm"`     // nome digitado (exigido em prod)
	// Overrides na atualização (padrão Fluig: dataset e campo descritor
	// aparecem e podem ser ajustados); vazios = preserva o que está no
	// servidor, como o form export.
	DatasetName     string `json:"datasetName"`
	CardDescription string `json:"cardDescription"` // campo descritor
	Create          *struct {
		Name            string `json:"name"`
		DatasetName     string `json:"datasetName"`
		CardDescription string `json:"cardDescription"`
		PersistenceType string `json:"persistenceType"` // db (default) | single
		ParentID        int    `json:"parentId"`
	} `json:"create"`
}

func (s *Server) serveDeploy(w http.ResponseWriter, r *http.Request) {
	var req deployReq
	if !decodeDeployBody(w, r, &req) {
		return
	}
	// Servidor precisa estar na lista oferecida (e produção exige o nome
	// digitado, como a trava do CLI).
	var target *DeployServerInfo
	for i := range s.opts.DeployServers {
		if s.opts.DeployServers[i].Name == req.Server {
			target = &s.opts.DeployServers[i]
			break
		}
	}
	if target == nil {
		simError(w, http.StatusBadRequest, "servidor desconhecido: "+req.Server)
		return
	}
	if target.Env == "prod" && req.Confirm != target.Name {
		simError(w, http.StatusBadRequest,
			"o servidor "+target.Name+" é de PRODUÇÃO — digite o nome exato dele no campo de confirmação para publicar")
		return
	}
	formDir, err := project.SafeJoin(filepath.Join(s.opts.Root, project.FormsDirName), req.Folder)
	if err != nil {
		simError(w, http.StatusBadRequest, "pasta inválida")
		return
	}
	if st, err := os.Stat(formDir); err != nil || !st.IsDir() {
		simError(w, http.StatusBadRequest, "pasta de formulário não encontrada: "+req.Folder)
		return
	}
	upload, err := buildFormUpload(formDir)
	if err != nil {
		simError(w, http.StatusBadRequest, "não consegui ler a pasta do formulário: "+err.Error())
		return
	}
	if len(upload.Files) == 0 {
		simError(w, http.StatusBadRequest, "a pasta não tem arquivos para enviar")
		return
	}

	ctx := r.Context()
	client, scope, err := s.deployClient(ctx, req.Server, req.Password)
	if err != nil {
		deployAuthError(w, err)
		return
	}
	pub, err := client.ResolveUserCode(ctx)
	if err != nil {
		simError(w, http.StatusBadGateway, "não consegui resolver o usuário no servidor alvo: "+err.Error())
		return
	}
	fmap, err := project.LoadFormMap(s.opts.Root, scope)
	if err != nil {
		simError(w, http.StatusInternalServerError, "falha ao ler .fluigcli/forms.json: "+err.Error())
		return
	}
	names := make([]string, 0, len(upload.Files))
	for _, ff := range upload.Files {
		names = append(names, ff.Name)
	}

	var (
		action, formName string
		docID            int
	)
	if req.DocumentID > 0 {
		// Atualização: as informações do formulário vêm do servidor, como no
		// `form export` (nome/descritor/dataset preservados).
		forms, err := client.ListForms(ctx, pub)
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao listar os formulários do servidor: "+err.Error())
			return
		}
		var existing *fluig.Form
		for i := range forms {
			if forms[i].DocumentID == req.DocumentID {
				existing = &forms[i]
				break
			}
		}
		if existing == nil {
			simError(w, http.StatusNotFound, "o formulário escolhido não existe mais no servidor — recarregue a lista")
			return
		}
		versionOption := fluig.VersionNew
		if req.VersionMode == "keep" || req.VersionMode == "" {
			versionOption = fluig.VersionKeep // padrão Fluig: manter a atual
		}
		descriptor := strings.TrimSpace(req.CardDescription)
		if descriptor == "" {
			descriptor = existing.CardDescription
		}
		dataset := strings.TrimSpace(req.DatasetName)
		if dataset == "" {
			dataset = existing.DatasetName
		}
		upload.PrincipalFile = fluig.ChoosePrincipalFile(names, req.Folder, existing.Description)
		// Assinatura do UpdateForm: o parâmetro "name" vira o descriptionField
		// (campo descritor) do SOAP e "cardDescription" vira o título — ordem
		// validada na homologação desde a Fase 4.
		res, err := client.UpdateForm(ctx, pub, existing.DocumentID, descriptor, existing.Description, dataset, versionOption, upload)
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao atualizar o formulário: "+err.Error())
			return
		}
		docID = deployDocumentIDOf(res, existing.DocumentID)
		action, formName = "updated", existing.Description
		fmap.Upsert(project.FormLink{Folder: req.Folder, DocumentID: docID, Name: formName, DatasetName: dataset})
	} else {
		c := req.Create
		if c == nil {
			simError(w, http.StatusBadRequest, "escolha um formulário existente ou preencha os dados da criação")
			return
		}
		formName = strings.TrimSpace(c.Name)
		if formName == "" {
			formName = req.Folder
		}
		if strings.TrimSpace(c.DatasetName) == "" {
			simError(w, http.StatusBadRequest, "o nome do dataset é obrigatório para criar um formulário")
			return
		}
		if c.ParentID <= 0 {
			simError(w, http.StatusBadRequest, "o id da pasta do GED é obrigatório para criar um formulário")
			return
		}
		persist := fluig.PersistenceDB
		if c.PersistenceType == "single" {
			persist = fluig.PersistenceSingle
		}
		card := strings.TrimSpace(c.CardDescription)
		if card == "" {
			card = formName
		}
		upload.PrincipalFile = fluig.ChoosePrincipalFile(names, req.Folder, formName)
		res, err := client.CreateForm(ctx, pub, formName, card, c.DatasetName, c.ParentID, persist, upload)
		if err != nil {
			simError(w, http.StatusBadGateway, "falha ao criar o formulário: "+err.Error())
			return
		}
		docID = deployDocumentIDOf(res, 0)
		action = "created"
		if docID != 0 {
			fmap.Upsert(project.FormLink{Folder: req.Folder, DocumentID: docID, Name: formName, DatasetName: c.DatasetName})
		}
	}
	if err := fmap.Save(); err != nil {
		s.opts.Warnf("formulário publicado, mas não consegui gravar o vínculo em forms.json: %v", err)
	}
	// O contexto do painel (processo detectado etc.) pode ter mudado.
	s.sim.mu.Lock()
	delete(s.sim.contexts, req.Folder)
	s.sim.mu.Unlock()

	s.opts.Infof("formulário %q publicado em %q (%s, documentId %d)", formName, req.Server, action, docID)
	simJSON(w, map[string]any{"action": action, "documentId": docID, "name": formName, "server": req.Server})
}

// decodeDeployBody decodifica o corpo JSON dos POSTs do diálogo.
func decodeDeployBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		simError(w, http.StatusMethodNotAllowed, "use POST")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		simError(w, http.StatusBadRequest, "corpo inválido: "+err.Error())
		return false
	}
	return true
}

// deployAuthError traduz a falha de conexão: sem credencial vira 401 com
// needsPassword (o diálogo mostra o campo de senha).
func deployAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrDeployNeedsPassword) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":         "sem credencial salva para este servidor — informe a senha",
			"needsPassword": true,
		})
		return
	}
	simError(w, http.StatusBadGateway, "falha ao conectar no servidor alvo: "+err.Error())
}

// buildFormUpload lê a pasta local no formato do upload (mesma leitura do
// `form export`).
func buildFormUpload(folder string) (fluig.FormUpload, error) {
	fc, err := project.ReadFormFolder(folder)
	if err != nil {
		return fluig.FormUpload{}, err
	}
	var up fluig.FormUpload
	for _, path := range fc.Files {
		content, err := os.ReadFile(path)
		if err != nil {
			return fluig.FormUpload{}, err
		}
		up.Files = append(up.Files, fluig.FormFile{Name: filepath.Base(path), Content: content})
	}
	for _, path := range fc.EventFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			return fluig.FormUpload{}, err
		}
		up.Events = append(up.Events, fluig.FormEvent{ID: project.ArtifactName(path), Code: string(content)})
	}
	return up, nil
}

// deployDocumentIDOf extrai o documentId do resultado SOAP (fallback quando o
// servidor não devolve).
func deployDocumentIDOf(res *soap.WriteResult, fallback int) int {
	if res != nil && res.DocumentID != 0 {
		return res.DocumentID
	}
	return fallback
}
