package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Páginas do portal (ROADMAP §5.21). Medido na homologação em 2026-11-06.
//
// Quase tudo usa a API oficial /page-management/api/v2. A exceção é a
// CRIAÇÃO: o POST /v2/pages/create responde IllegalArgumentException ou
// NullPointerException para qualquer corpo testado, e a tela do Fluig usa a
// rota legada /portal/api/rest/wcm/page/create. As preferências de widget
// também só existem na API legada (setpreferences/findWidgetDataByInstanceId).
//
// Modelo de versões (medido):
//   - página nova nasce em DRAFT (versão 1);
//   - pôr widget exige rascunho aberto (sem rascunho = 404
//     PageVersionDraftNotFoundException); POST /draft abre um;
//   - o GET /application mostra sempre a versão PUBLICADA;
//   - publicar recria as instâncias: o instanceId MUDA; as preferências
//     são copiadas;
//   - DELETE de widget age direto na versão publicada (sem rascunho);
//   - o PUT da página (permissões) abre um rascunho sozinho;
//   - excluir exige arquivar antes (DELETE /archive).
const (
	restPagesBase  = "/page-management/api/v2/pages"
	legacyPageBase = "/portal/api/rest/wcm/page/"
)

// Status de página.
const (
	PageStatusDraft     = "DRAFT"
	PageStatusPublished = "PUBLISHED"
	PageStatusArchived  = "ARCHIVED"
)

// Tipos de permissão de página.
const (
	PagePermView   = "pageView"
	PagePermUpdate = "pageUpdate"
	PagePermRemove = "pageRemove"
)

var pagePermDescription = map[string]string{
	PagePermView:   "permission.page.view",
	PagePermUpdate: "permission.page.update",
	PagePermRemove: "permission.page.remove",
}

var (
	// ErrPageExists indica código de página repetido no create
	// (WCMPageDuplicatedException).
	ErrPageExists = errors.New("já existe uma página com este código")
	// ErrPageNoDraft indica operação que exige rascunho aberto.
	ErrPageNoDraft = errors.New("a página não tem rascunho aberto")
	// ErrPageDraftExists indica rascunho já aberto (PageDraftCreateException).
	ErrPageDraftExists = errors.New("a página já tem um rascunho aberto")
	// ErrPageNotPublished indica página sem versão publicada (o GET de
	// widgets responde 500 EJBException).
	ErrPageNotPublished = errors.New("a página ainda não tem versão publicada")
	// ErrPageConcurrentEdit indica rascunho aberto por OUTRO usuário
	// (ConcurrentPageEditException). A tela do Fluig resolve com
	// PUT /change-edit-user (TakeOverPageDraft) e repete a operação.
	ErrPageConcurrentEdit = errors.New("outro usuário está editando o rascunho da página")
)

// Page é uma página do portal.
type Page struct {
	ID            int    `json:"id"`
	Code          string `json:"code"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Layout        string `json:"layout,omitempty"`
	Status        string `json:"status"`
	Authorization string `json:"authorization"`
	Version       int    `json:"version"`
	Internal      bool   `json:"internal"`
	ParentCode    string `json:"parentCode,omitempty"`
	// EditorLogin/EditorName: último usuário que editou a página (com
	// rascunho aberto, quem está editando o rascunho).
	EditorLogin string `json:"editorLogin,omitempty"`
	EditorName  string `json:"editorName,omitempty"`

	raw map[string]json.RawMessage
}

// PageWidget é uma instância de widget na versão publicada de uma página.
type PageWidget struct {
	InstanceID  int               `json:"instanceId"`
	Widget      string            `json:"widget"`
	Slot        string            `json:"slot"`
	Order       int               `json:"order"`
	Preferences map[string]string `json:"preferences,omitempty"`
}

// PagePermission é uma permissão de página. Só um de Group/Role/User vem
// preenchido.
type PagePermission struct {
	Permission string `json:"permission"`
	Group      string `json:"group,omitempty"`
	Role       string `json:"role,omitempty"`
	User       string `json:"user,omitempty"`
}

// pageAPIError lê o envelope de erro do v2 ({"code","message"}).
type pageAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func pageError(op string, status int, body []byte) error {
	var e pageAPIError
	_ = json.Unmarshal(body, &e)
	msg := strings.TrimSpace(e.Message)
	switch e.Code {
	case "WCMPageNotFoundException", "WCMComponentNotFound", "WCMWidgetNotFoundException":
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case "PageVersionDraftNotFoundException":
		return ErrPageNoDraft
	case "PageDraftCreateException":
		return ErrPageDraftExists
	case "ConcurrentPageEditException":
		return ErrPageConcurrentEdit
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: %s", ErrNotFound, op)
	}
	if msg != "" && !strings.HasPrefix(msg, "Oops") {
		return fmt.Errorf("%w: %s", errServerRejected, msg)
	}
	return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
}

// ListPages lista as páginas visíveis ao usuário. Sem includeInternal, só as
// páginas customizadas (internal=false).
func (c *Client) ListPages(ctx context.Context, includeInternal bool) ([]Page, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	var out []Page
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("pageSize", "100")
		q.Set("expand", "layout")
		if !includeInternal {
			q.Set("internal", "false")
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restPagesBase)+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, pageError("page-management/v2/pages", status, body)
		}
		var resp struct {
			Items   []map[string]json.RawMessage `json:"items"`
			HasNext bool                         `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/pages: %w", err)
		}
		for _, raw := range resp.Items {
			out = append(out, pageFromRaw(raw))
		}
		if !resp.HasNext || len(resp.Items) == 0 {
			return out, nil
		}
	}
}

func pageFromRaw(raw map[string]json.RawMessage) Page {
	p := Page{raw: raw}
	_ = json.Unmarshal(raw["id"], &p.ID)
	_ = json.Unmarshal(raw["version"], &p.Version)
	_ = json.Unmarshal(raw["internal"], &p.Internal)
	p.Code = jsonString(raw, "code")
	p.Title = jsonString(raw, "title")
	p.Description = jsonString(raw, "description")
	p.Status = jsonString(raw, "status")
	p.Authorization = jsonString(raw, "authorization")
	p.ParentCode = jsonString(raw, "parentCode")
	if u, ok := raw["user"]; ok {
		var usr struct {
			Login    string `json:"login"`
			FullName string `json:"fullName"`
		}
		_ = json.Unmarshal(u, &usr)
		p.EditorLogin, p.EditorName = usr.Login, usr.FullName
	}
	if l, ok := raw["layout"]; ok {
		var lay struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(l, &lay)
		p.Layout = lay.Code
	}
	return p
}

// GetPage carrega uma página pelo código (com o layout).
func (c *Client) GetPage(ctx context.Context, code string) (*Page, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restPagesBase+"/"+url.PathEscape(code))+"?expand=layout&expand=user", nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		// O GET de página inexistente responde 404 com corpo vazio.
		return nil, fmt.Errorf("%w: página %q", ErrNotFound, code)
	}
	if status != http.StatusOK {
		return nil, pageError("page-management/v2/pages/{code}", status, body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/pages/{code}: %w", err)
	}
	p := pageFromRaw(raw)
	return &p, nil
}

// PageWidgets lista as widgets da versão PUBLICADA da página. Página que
// nunca foi publicada devolve ErrPageNotPublished.
func (c *Client) PageWidgets(ctx context.Context, code string) ([]PageWidget, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restPagesBase+"/"+url.PathEscape(code)+"/application"), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		var e pageAPIError
		_ = json.Unmarshal(body, &e)
		if status == http.StatusInternalServerError && e.Code == "EJBException" {
			return nil, ErrPageNotPublished
		}
		return nil, pageError("page-management/v2/pages/{code}/application", status, body)
	}
	var resp struct {
		Widgets []struct {
			ID    int    `json:"id"`
			Slot  string `json:"slot"`
			Order int    `json:"order"`
			Code  string `json:"code"`
		} `json:"widgets"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/pages/{code}/application: %w", err)
	}
	out := make([]PageWidget, 0, len(resp.Widgets))
	for _, w := range resp.Widgets {
		out = append(out, PageWidget{InstanceID: w.ID, Widget: w.Code, Slot: w.Slot, Order: w.Order})
	}
	return out, nil
}

// PagePermissions lista as permissões da página.
func (c *Client) PagePermissions(ctx context.Context, code string) ([]PagePermission, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restPagesBase+"/"+url.PathEscape(code)+"/permission"), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, pageError("page-management/v2/pages/{code}/permission", status, body)
	}
	var resp struct {
		Items []PagePermission `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/pages/{code}/permission: %w", err)
	}
	return resp.Items, nil
}

// DefaultPagePermissions devolve as permissões que a tela do Fluig propõe
// para uma página nova (medido: GET /permission de código inexistente):
// visualização para o papel "user" e edição/remoção para o usuário logado.
func (c *Client) DefaultPagePermissions() []PagePermission {
	return []PagePermission{
		{Permission: PagePermView, Role: "user"},
		{Permission: PagePermUpdate, User: c.opts.Username},
		{Permission: PagePermRemove, User: c.opts.Username},
	}
}

// PageCreateInput descreve uma página nova.
type PageCreateInput struct {
	Code        string
	Title       string
	Description string
	Layout      string // código do layout
	Public      bool
	Permissions []PagePermission
}

// CreatePage cria uma página em rascunho pela rota legada (a mesma da tela).
func (c *Client) CreatePage(ctx context.Context, in PageCreateInput) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	auth := "1" // AUTHENTICATED (GET /portal/api/rest/wcm/page/authorization)
	if in.Public {
		auth = "0"
	}
	items := make([]map[string]any, 0, len(in.Permissions))
	for _, p := range in.Permissions {
		items = append(items, map[string]any{
			"permissionId":          nil,
			"permission":            p.Permission,
			"permissionDescription": pagePermDescription[p.Permission],
			"groupCode":             p.Group,
			"login":                 p.User,
			"roleCode":              p.Role,
		})
	}
	payload := map[string]any{
		"config": map[string]any{"validateFields": []map[string]string{{"key": "description"}}},
		"formData": map[string]any{
			"pageKey": "", "page": "", "layout": in.Layout, "hidePathIconPage": "",
			"iconFont": "flaticon-page-default", "title": in.Title, "description": in.Description,
			"code": in.Code, "isInternalFriendlyURL": false, "friendlyURL": "", "auth": auth,
			"parent-code": "", "parent": "", "enabledChildren": false, "enableChildLevel": false,
			"menuGroup": false, "enablePathParam": false, "enableMobileApp": false,
			"selectedItens": items,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(legacyPageBase+"create"), data)
	if err != nil {
		return err
	}
	return checkLegacyPageResponse("wcm/page/create", body, status)
}

// checkLegacyPageResponse interpreta as respostas da API legada de página:
// sucesso = 200 "{success: true}"; erro = {"content":"ERROR","message":{...}}
// ou {"success":false,"message":"..."}.
func checkLegacyPageResponse(op string, body []byte, status int) error {
	var env struct {
		Content any `json:"content"`
		Message *struct {
			Message   string `json:"message"`
			ErrorCode string `json:"errorCode"`
		} `json:"message"`
	}
	if json.Unmarshal(body, &env) == nil && env.Message != nil && env.Message.Message != "" {
		if env.Message.ErrorCode == "WCMPageDuplicatedException" {
			return fmt.Errorf("%w (resposta do servidor: %s)", ErrPageExists, env.Message.Message)
		}
		return fmt.Errorf("%w: %s", errServerRejected, env.Message.Message)
	}
	var flat struct {
		Success *bool  `json:"success"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &flat) == nil && flat.Success != nil && !*flat.Success {
		return fmt.Errorf("%w: %s", errServerRejected, flat.Message)
	}
	if status < 200 || status >= 300 {
		return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
	}
	return nil
}

// CreatePageDraft abre um rascunho a partir da versão publicada.
func (c *Client) CreatePageDraft(ctx context.Context, code string) error {
	return c.pageNoContent(ctx, http.MethodPost, "/"+url.PathEscape(code)+"/draft", nil, "v2/pages/{code}/draft")
}

// DraftHeldByOther diz se a página tem rascunho aberto por outro usuário
// (a mesma regra da tela: status DRAFT e editor diferente do logado).
func (c *Client) DraftHeldByOther(p *Page) bool {
	return p.Status == PageStatusDraft && p.EditorLogin != "" && !strings.EqualFold(p.EditorLogin, c.opts.Username)
}

// TakeOverPageDraft faz do usuário logado o editor do rascunho aberto
// (PUT /change-edit-user; medido: 204). É o "Confirmar" da tela quando o
// rascunho é de outro usuário.
func (c *Client) TakeOverPageDraft(ctx context.Context, code string) error {
	return c.pageNoContent(ctx, http.MethodPut, "/"+url.PathEscape(code)+"/change-edit-user", nil, "v2/pages/{code}/change-edit-user")
}

// DiscardPageDraft descarta o rascunho aberto.
func (c *Client) DiscardPageDraft(ctx context.Context, code string) error {
	return c.pageNoContent(ctx, http.MethodDelete, "/"+url.PathEscape(code)+"/draft", nil, "v2/pages/{code}/draft")
}

// PublishPage publica o rascunho aberto.
func (c *Client) PublishPage(ctx context.Context, code, description string) error {
	data, _ := json.Marshal(map[string]string{"description": description})
	return c.pageNoContent(ctx, http.MethodPost, "/"+url.PathEscape(code)+"/publish", data, "v2/pages/{code}/publish")
}

// RemovePageWidget remove uma instância da versão publicada.
func (c *Client) RemovePageWidget(ctx context.Context, code string, instanceID int) error {
	return c.pageNoContent(ctx, http.MethodDelete, "/"+url.PathEscape(code)+"/application/"+strconv.Itoa(instanceID), nil, "v2/pages/{code}/application/{instanceId}")
}

// DeletePage arquiva e exclui a página (o Fluig recusa excluir página não
// arquivada: WCMPageUnarchivedException).
func (c *Client) DeletePage(ctx context.Context, code string) error {
	p, err := c.GetPage(ctx, code)
	if err != nil {
		return err
	}
	if p.Status != PageStatusArchived {
		if err := c.pageNoContent(ctx, http.MethodDelete, "/"+url.PathEscape(code)+"/archive", nil, "v2/pages/{code}/archive"); err != nil {
			return err
		}
	}
	return c.pageNoContent(ctx, http.MethodDelete, "/"+url.PathEscape(code), nil, "v2/pages/{code}")
}

func (c *Client) pageNoContent(ctx context.Context, method, path string, data []byte, op string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, method, c.url(restPagesBase+path), data)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return pageError(op, status, body)
	}
	return nil
}

// AddPageWidget põe uma widget num slot do rascunho aberto. O Fluig NÃO
// confere o slot (aceita qualquer nome); quem chama deve validar antes.
func (c *Client) AddPageWidget(ctx context.Context, code, slot, widget string) (*PageWidget, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	data, _ := json.Marshal(map[string]string{"slot": slot, "widgetCode": widget})
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restPagesBase+"/"+url.PathEscape(code)+"/application"), data)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, pageError("v2/pages/{code}/application", status, body)
	}
	var w struct {
		ID    int    `json:"id"`
		Slot  string `json:"slot"`
		Order int    `json:"order"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/pages/{code}/application: %w", err)
	}
	return &PageWidget{InstanceID: w.ID, Widget: w.Code, Slot: w.Slot, Order: w.Order}, nil
}

// UpdatePagePermissions troca as permissões da página (PUT v2). O Fluig
// abre um rascunho; a mudança vale depois do publish. O corpo repete os
// campos atuais da página, como a tela de configuração faz (medido: sem eles
// o PUT responde 400 "” must not be null").
func (c *Client) UpdatePagePermissions(ctx context.Context, p *Page, perms []PagePermission) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	b := func(k string) bool {
		var v bool
		_ = json.Unmarshal(p.raw[k], &v)
		return v
	}
	parent := 0
	_ = json.Unmarshal(p.raw["parent"], &parent)
	friendly := jsonString(p.raw, "friendlyUrl")
	iconFont := jsonString(p.raw, "iconFont")
	if iconFont == "" {
		iconFont = "flaticon-page-default"
	}
	resource := "page." + strconv.Itoa(p.ID)
	items := make([]map[string]string, 0, len(perms))
	for _, pp := range perms {
		m := map[string]string{"permission": pp.Permission, "description": pagePermDescription[pp.Permission], "resource": resource}
		switch {
		case pp.Group != "":
			m["group"] = pp.Group
		case pp.Role != "":
			m["role"] = pp.Role
		default:
			m["user"] = pp.User
		}
		items = append(items, m)
	}
	payload := map[string]any{
		"title":               p.Title,
		"iconFont":            iconFont,
		"description":         p.Description,
		"authorization":       p.Authorization,
		"friendlyURL":         strings.TrimPrefix(friendly, "/"),
		"internalFriendlyURL": friendly == "" || strings.HasPrefix(friendly, "/"),
		"parent":              parent,
		"enabledChildren":     b("enabledChildren"),
		"disableChildLevel":   b("disableChildLevel"),
		"menuGroup":           b("menuGroup"),
		"enabledPathParam":    b("enabledPathParam"),
		"pageIconBase64":      "",
		"pageIcon":            "",
		"permissions":         items,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodPut, c.url(restPagesBase+"/"+url.PathEscape(p.Code)), data)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return pageError("v2/pages/{code}", status, body)
	}
	return nil
}

// WidgetPreferences lê as preferências de uma instância (API legada).
func (c *Client) WidgetPreferences(ctx context.Context, instanceID int) (map[string]string, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(legacyPageBase+"findWidgetDataByInstanceId/"+strconv.Itoa(instanceID)) + "?rows=500&page=1"
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "wcm/page/findWidgetDataByInstanceId", Body: truncate(string(body), 512)}
	}
	var resp struct {
		Invdata []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"invdata"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de findWidgetDataByInstanceId: %w", err)
	}
	out := make(map[string]string, len(resp.Invdata))
	for _, kv := range resp.Invdata {
		out[kv.Key] = kv.Value
	}
	return out, nil
}

// SetWidgetPreferences SUBSTITUI as preferências de uma instância (API
// legada). Medido: a gravação troca o mapa inteiro; chaves omitidas somem.
func (c *Client) SetWidgetPreferences(ctx context.Context, instanceID int, prefs map[string]string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	data, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(legacyPageBase+"application/setpreferences/"+strconv.Itoa(instanceID)), data)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return &HTTPError{StatusCode: status, URL: "wcm/page/application/setpreferences", Body: truncate(string(body), 512)}
	}
	return nil
}

var (
	slotIDRe   = regexp.MustCompile(`id="(Slot[A-Za-z0-9_]+)"`)
	htmlTitleR = regexp.MustCompile(`(?s)<title>(.*?)</title>`)
)

// LayoutSlots descobre os slots de um layout. A API não expõe os slots; a
// CLI renderiza uma página PUBLICADA que usa o layout e lê os elementos
// id="Slot...". Devolve nil (sem erro) quando nenhuma página publicada do
// layout pode ser renderizada.
func (c *Client) LayoutSlots(ctx context.Context, layout string) ([]string, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(layoutsPath+"/"+url.PathEscape(layout)+"/pages")+"?pageSize=50", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, pageError("v2/layouts/{code}/pages", status, body)
	}
	var resp struct {
		Items []struct {
			Code   string `json:"code"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/layouts/{code}/pages: %w", err)
	}
	tried := 0
	for _, it := range resp.Items {
		if it.Status != PageStatusPublished || tried >= 3 {
			continue
		}
		tried++
		slots, ok := c.renderedSlots(ctx, it.Code, it.Title)
		if ok {
			return slots, nil
		}
	}
	return nil, nil
}

// renderedSlots renderiza a página e extrai os slots. ok=false quando o
// portal devolveu a página de erro (o título não traz o título da página).
func (c *Client) renderedSlots(ctx context.Context, code, title string) ([]string, bool) {
	endpoint := c.url("/portal/p/" + strconv.Itoa(c.opts.CompanyID) + "/" + url.PathEscape(code))
	body, status, err := c.doRaw(ctx, http.MethodGet, endpoint, nil, "", "text/html")
	if err != nil || status != http.StatusOK {
		return nil, false
	}
	m := htmlTitleR.FindSubmatch(body)
	if m == nil || !strings.Contains(html.UnescapeString(string(m[1])), title) {
		return nil, false
	}
	seen := map[string]bool{}
	var slots []string
	for _, s := range slotIDRe.FindAllSubmatch(body, -1) {
		name := string(s[1])
		if !seen[name] {
			seen[name] = true
			slots = append(slots, name)
		}
	}
	sort.Strings(slots)
	return slots, len(slots) > 0
}
