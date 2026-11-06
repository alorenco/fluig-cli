package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Cadastro de serviços (Painel de Controle > Serviços), ROADMAP §5.20.
//
// A API é a legada do ECM (fora dos swaggers), a mesma que o widget
// wcm_dataservice usa. Medida na homologação em 2026-11-06.
//
// ⚠️ O findByType devolve os segredos do serviço EM CLARO (oAuthVO.password,
// custom, consumerSecret, clientSecret, tokenAccess...). Por isso, o tipo
// Service não expõe nenhum desses campos. O DTO cru fica num campo não
// exportado e só serve ao round-trip do update, que precisa reenviar os
// segredos para não apagá-los.
const restServicesBase = "/ecm/api/rest/ecm/ecmservices/"

// Tipos de serviço (campo "type" do Fluig).
const (
	ServiceTypeSOAP = "soap"
	ServiceTypeREST = "rest"
	ServiceTypeJDBC = "jdbc"
)

var serviceTypeByCode = map[int]string{1: ServiceTypeSOAP, 3: ServiceTypeREST, 4: ServiceTypeJDBC}

// serviceTypeParam é o valor do filtro "types" do findByType.
var serviceTypeParam = map[string]string{ServiceTypeSOAP: "SOAP", ServiceTypeREST: "REST", ServiceTypeJDBC: "JDBC"}

// Tipos de autenticação de serviço REST (loadClients). A CLI só cria com os
// tipos de ServiceAuthWritable; os outros exigem segredos em vários campos.
const (
	ServiceAuthNone     = "none"
	ServiceAuthBasic    = "basic"
	ServiceAuthFluigAPI = "fluigAPI"
	ServiceAuthOAuth1   = "oauth1"
	ServiceAuthOAuth2   = "oauth2"
	ServiceAuthCustom   = "custom"
)

// ServiceAuthWritable lista os tipos de autenticação que a CLI grava.
var ServiceAuthWritable = []string{ServiceAuthNone, ServiceAuthBasic, ServiceAuthFluigAPI, ServiceAuthCustom, ServiceAuthOAuth1, ServiceAuthOAuth2}

// ErrServiceExists indica código de serviço repetido no create
// (HTTP 500 FDNDataServicePKException).
var ErrServiceExists = errors.New("já existe um serviço com este código")

// Service é um serviço cadastrado, SEM segredos.
type Service struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Type        string `json:"type"` // rest, soap, jdbc
	URL         string `json:"url"`  // URL base (REST: oAuthVO.domain)
	AuthType    string `json:"authType,omitempty"`
	TestPath    string `json:"testPath,omitempty"`
	Swagger     string `json:"swagger,omitempty"`
	Username    string `json:"username,omitempty"` // só basic; nunca a senha
	// CustomHeaders traz só os NOMES dos headers da autenticação custom. O
	// valor (o segredo) nunca sai do pacote.
	CustomHeaders []string `json:"customHeaders,omitempty"`
	// OAuthFields traz só os NOMES dos campos OAuth preenchidos.
	OAuthFields []string `json:"oauthFields,omitempty"`
	Internal    bool     `json:"internal"`
	LogRequests bool     `json:"logRequests"`

	oauthID int
	oauth   map[string]json.RawMessage // oAuthVO cru, com segredos: nunca serializar para saída
	item    map[string]json.RawMessage // item cru do findByType (corpo do updateLogActive)
}

// SecretEquals diz se o segredo guardado no servidor é igual ao informado,
// sem expor o valor: a senha na autenticação basic, o texto dos headers na
// custom. Outros tipos devolvem false.
func (s *Service) SecretEquals(secret string) bool {
	switch s.AuthType {
	case ServiceAuthBasic:
		return jsonString(s.oauth, "password") == secret
	case ServiceAuthCustom:
		cur := strings.ReplaceAll(jsonString(s.oauth, "custom"), "\r\n", "\n")
		return strings.TrimRight(cur, "\n") == strings.TrimRight(secret, "\n")
	case ServiceAuthOAuth1, ServiceAuthOAuth2:
		fields, err := ParseOAuthSecret(s.AuthType, secret)
		if err != nil {
			return false
		}
		for _, k := range serviceAuthFields[s.AuthType] {
			if jsonString(s.oauth, k) != fields[k] {
				return false
			}
		}
		return true
	}
	return false
}

// ServiceFilter filtra a listagem. Pattern é aplicado pelo servidor (trecho do
// código); Types vazio traz todos os tipos.
type ServiceFilter struct {
	Pattern string
	Types   []string
}

const servicePageSize = 100

// ListServices lista os serviços (GET findByType), paginando até o fim.
func (c *Client) ListServices(ctx context.Context, f ServiceFilter) ([]Service, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	var out []Service
	for offset := 0; ; offset += servicePageSize {
		q := url.Values{}
		q.Set("offset", strconv.Itoa(offset))
		q.Set("limit", strconv.Itoa(servicePageSize))
		if f.Pattern != "" {
			q.Set("pattern", f.Pattern)
		}
		for _, t := range f.Types {
			p, ok := serviceTypeParam[t]
			if !ok {
				return nil, fmt.Errorf("tipo de serviço inválido: %q", t)
			}
			q.Add("types", p)
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restServicesBase+"findByType")+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, &HTTPError{StatusCode: status, URL: "ecmservices/findByType", Body: truncate(string(body), 512)}
		}
		page, err := parseServiceList(body)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < servicePageSize {
			return out, nil
		}
	}
}

func parseServiceList(body []byte) ([]Service, error) {
	var rawItems struct {
		Content []map[string]json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(body, &rawItems)
	var resp struct {
		Content []struct {
			Name        string                     `json:"name"`
			Description string                     `json:"description"`
			Type        int                        `json:"type"`
			Internal    bool                       `json:"internal"`
			OAuth       map[string]json.RawMessage `json:"oAuthVO"`
			DataService struct {
				URL              string `json:"dataServiceURL"`
				ExtraParameters  string `json:"extraParameters"`
				LogRequestActive bool   `json:"logRequestActive"`
				OAuthID          int    `json:"oauthId"`
			} `json:"dataService"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de ecmservices/findByType: %w", err)
	}
	out := make([]Service, 0, len(resp.Content))
	for idx, it := range resp.Content {
		s := Service{
			Code:        it.Name,
			Description: it.Description,
			Type:        serviceTypeByCode[it.Type],
			URL:         it.DataService.URL,
			Swagger:     it.DataService.ExtraParameters,
			Internal:    it.Internal,
			LogRequests: it.DataService.LogRequestActive,
			oauthID:     it.DataService.OAuthID,
		}
		if idx < len(rawItems.Content) {
			s.item = rawItems.Content[idx]
		}
		if s.Type == "" {
			s.Type = strconv.Itoa(it.Type)
		}
		if s.Type == ServiceTypeREST && it.OAuth != nil {
			s.oauth = it.OAuth
			if d := jsonString(it.OAuth, "domain"); d != "" {
				s.URL = d
			}
			s.AuthType = jsonString(it.OAuth, "authenticationType")
			s.TestPath = jsonString(it.OAuth, "serviceTestUrl")
			switch s.AuthType {
			case ServiceAuthBasic:
				s.Username = jsonString(it.OAuth, "username")
			case ServiceAuthCustom:
				s.CustomHeaders = customHeaderNames(jsonString(it.OAuth, "custom"))
			case ServiceAuthOAuth1, ServiceAuthOAuth2:
				for _, k := range serviceAuthFields[s.AuthType] {
					if jsonString(it.OAuth, k) != "" {
						s.OAuthFields = append(s.OAuthFields, k)
					}
				}
			}
			var id int
			if raw, ok := it.OAuth["id"]; ok && json.Unmarshal(raw, &id) == nil && id != 0 {
				s.oauthID = id
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// GetService devolve o serviço de código exato. O Fluig não tem GET por
// código: a busca usa o pattern do findByType e filtra o nome exato.
func (c *Client) GetService(ctx context.Context, code string) (*Service, error) {
	list, err := c.ListServices(ctx, ServiceFilter{Pattern: code})
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Code == code {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("%w: serviço %q", ErrNotFound, code)
}

// ServiceInput descreve a criação ou a alteração de um serviço REST. Na
// alteração, campo nil mantém o valor atual.
type ServiceInput struct {
	Description *string
	URL         *string
	AuthType    *string
	TestPath    *string
	Swagger     *string
	Username    *string
	Password    *string           // só basic; vem de env/prompt, nunca de argumento
	Custom      *string           // só custom: "Nome: valor" por linha; vem de stdin/env
	OAuth       map[string]string // só oauth1/oauth2: campos de ParseOAuthSecret
}

// CreateService cria um serviço REST (POST ecmservices/service).
func (c *Client) CreateService(ctx context.Context, code string, in ServiceInput) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	auth := deref(in.AuthType, ServiceAuthNone)
	desc := deref(in.Description, code)
	oauth := map[string]any{
		"code":               code,
		"description":        desc,
		"authenticationType": auth,
		"domain":             deref(in.URL, ""),
		"serviceTestUrl":     deref(in.TestPath, ""),
		"custom":             "",
	}
	if auth == ServiceAuthCustom {
		oauth["custom"] = deref(in.Custom, "")
	}
	for k, v := range in.OAuth {
		oauth[k] = v
	}
	if auth == ServiceAuthBasic {
		pw := deref(in.Password, "")
		oauth["username"] = deref(in.Username, "")
		oauth["password"] = pw
		oauth["passwordConfirmation"] = pw
	}
	payload := map[string]any{
		"name":        code,
		"url":         deref(in.Swagger, ""),
		"description": desc,
		"type":        "3",
		"oAuthVO":     oauth,
	}
	return c.postServiceWrite(ctx, "service", payload)
}

// UpdateService altera um serviço REST (POST ecmservices/update). Reenvia o
// oAuthVO atual (com os segredos que o servidor devolveu) e troca só os
// campos informados — assim a alteração da URL não apaga a senha guardada.
func (c *Client) UpdateService(ctx context.Context, current *Service, in ServiceInput) error {
	if current.Type != ServiceTypeREST {
		return fmt.Errorf("só serviços REST podem ser alterados pela CLI (o serviço %q é %s)", current.Code, current.Type)
	}
	if current.oauthID == 0 {
		return fmt.Errorf("o servidor não devolveu o id do cliente OAuth do serviço %q", current.Code)
	}
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	oauth := make(map[string]any, len(current.oauth)+8)
	for k, v := range current.oauth {
		oauth[k] = v
	}
	desc := deref(in.Description, current.Description)
	oauth["id"] = current.oauthID
	oauth["code"] = current.Code
	oauth["description"] = desc
	set := func(key string, v *string) {
		if v != nil {
			oauth[key] = *v
		}
	}
	set("domain", in.URL)
	set("serviceTestUrl", in.TestPath)
	set("authenticationType", in.AuthType)
	// Troca de tipo: não deixa a credencial do tipo antigo guardada.
	if in.AuthType != nil && *in.AuthType != ServiceAuthBasic {
		for _, k := range []string{"username", "password", "passwordConfirmation"} {
			delete(oauth, k)
		}
	}
	if in.AuthType != nil && *in.AuthType != ServiceAuthCustom {
		oauth["custom"] = ""
	}
	set("custom", in.Custom)
	if in.AuthType != nil {
		for t, keys := range serviceAuthFields {
			if t == *in.AuthType || t == ServiceAuthBasic {
				continue
			}
			for _, k := range keys {
				if !slices.Contains(serviceAuthFields[*in.AuthType], k) {
					delete(oauth, k)
				}
			}
		}
	}
	for k, v := range in.OAuth {
		oauth[k] = v
	}
	set("username", in.Username)
	if in.Password != nil {
		oauth["password"] = *in.Password
		oauth["passwordConfirmation"] = *in.Password
	}
	payload := map[string]any{
		"name":        current.Code,
		"url":         deref(in.Swagger, current.Swagger),
		"description": desc,
		"type":        "3",
		"oAuthVO":     oauth,
	}
	return c.postServiceWrite(ctx, "update", payload)
}

// DeleteService exclui um serviço (DELETE ecmservices/delete?id=). O Fluig
// responde 500 EJBException genérico para código inexistente; por isso a
// existência é conferida antes.
func (c *Client) DeleteService(ctx context.Context, code string) error {
	if _, err := c.GetService(ctx, code); err != nil {
		return err
	}
	endpoint := c.url(restServicesBase+"delete") + "?id=" + url.QueryEscape(code)
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	return checkServiceResponse("ecmservices/delete", body, status)
}

func (c *Client) postServiceWrite(ctx context.Context, op string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restServicesBase+op), data)
	if err != nil {
		return err
	}
	return checkServiceResponse("ecmservices/"+op, body, status)
}

// checkServiceResponse interpreta o envelope {"content":"OK"|"ERROR",
// "message":{message,errorCode}}. O erro vem com HTTP 500.
func checkServiceResponse(op string, body []byte, status int) error {
	var parsed struct {
		Content any `json:"content"`
		Message *struct {
			Message   string `json:"message"`
			ErrorCode string `json:"errorCode"`
		} `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)
	if parsed.Message != nil && parsed.Message.Message != "" {
		msg := strings.TrimSpace(parsed.Message.Message)
		if parsed.Message.ErrorCode == "FDNDataServicePKException" {
			return fmt.Errorf("%w (resposta do servidor: %s)", ErrServiceExists, msg)
		}
		return fmt.Errorf("%w: %s", errServerRejected, msg)
	}
	if status < 200 || status >= 300 {
		return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
	}
	return nil
}

// ParseCustomHeaders valida o texto da autenticação custom e devolve o texto
// normalizado (LF, sem linhas vazias no fim) e os nomes dos headers. Regra:
// uma linha por header, no formato "Nome: valor"; nome no padrão de token
// HTTP; valor não vazio; sem linha vazia no meio. As mensagens de erro nunca
// citam o valor.
func ParseCustomHeaders(text string) (string, []string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return "", nil, errors.New("nenhum header informado; use uma linha \"Nome: valor\" por header")
	}
	lines := strings.Split(text, "\n")
	names := make([]string, 0, len(lines))
	for i, line := range lines {
		n := i + 1
		if strings.TrimSpace(line) == "" {
			return "", nil, fmt.Errorf("linha %d vazia; use uma linha \"Nome: valor\" por header, sem linhas vazias", n)
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return "", nil, fmt.Errorf("linha %d sem \":\"; o formato é \"Nome: valor\"", n)
		}
		if !validHeaderName(name) {
			return "", nil, fmt.Errorf("linha %d: nome de header inválido (sem espaços; letras, números e - _ . apenas)", n)
		}
		if strings.TrimSpace(value) == "" {
			return "", nil, fmt.Errorf("linha %d: header %q sem valor", n, name)
		}
		names = append(names, name)
	}
	return text, names, nil
}

// customHeaderNames extrai só os nomes do texto guardado no servidor, sem
// validar (o cadastro pode ter vindo da interface).
func customHeaderNames(text string) []string {
	var names []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if name, _, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(name) != "" {
			names = append(names, strings.TrimSpace(name))
		}
	}
	return names
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", r)
		if !ok {
			return false
		}
	}
	return true
}

// oauthRequired são os campos que o Fluig exige (medido em 2026-11-06,
// tirando um campo por vez: sem eles o servidor responde "Erro ao
// criar/atualizar cliente OAuth"). No oauth2, refreshToken e
// refreshTokenURL são opcionais; no oauth1, todos são obrigatórios.
var oauthRequired = map[string][]string{
	ServiceAuthOAuth1: {"consumerKey", "consumerSecret", "tokenAccess", "tokenSecret", "accessTokenURL", "requestTokenURL", "authorizationURL"},
	ServiceAuthOAuth2: {"clientId", "clientSecret", "tokenAccess"},
}

// ParseOAuthSecret valida o JSON dos segredos OAuth: objeto de textos, só os
// campos que a tela do Fluig usa para o tipo, com o par principal
// preenchido. As mensagens de erro nunca citam os valores.
func ParseOAuthSecret(auth, text string) (map[string]string, error) {
	allowed := serviceAuthFields[auth]
	if auth != ServiceAuthOAuth1 && auth != ServiceAuthOAuth2 {
		return nil, fmt.Errorf("tipo %q não é OAuth", auth)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil || raw == nil {
		return nil, fmt.Errorf("os campos OAuth precisam ser um objeto JSON, por exemplo {\"%s\":\"...\"}", allowed[0])
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if !slices.Contains(allowed, k) {
			return nil, fmt.Errorf("campo %q não existe no %s; use: %s", k, auth, strings.Join(allowed, ", "))
		}
		sv, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("o campo %q precisa ser texto", k)
		}
		out[k] = sv
	}
	for _, k := range oauthRequired[auth] {
		if strings.TrimSpace(out[k]) == "" {
			return nil, fmt.Errorf("o %s exige o campo %q", auth, k)
		}
	}
	return out, nil
}

func deref(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

// ServiceTestResult é o resultado do teste de um serviço REST.
type ServiceTestResult struct {
	Code       string `json:"code"`
	HTTPStatus int    `json:"httpStatus"` // 0 = a chamada não saiu
	Body       string `json:"body"`
	Error      string `json:"error,omitempty"`
}

// TestService chama o caminho de teste cadastrado no serviço, pelo próprio
// Fluig (GET /api/public/2.0/authorize/client/test, a rota do botão
// "Testar" do Painel de Controle; medido em 2026-11-06). O Fluig aplica a
// autenticação do cadastro (inclusive os headers custom). Sem caminho de
// teste, a chamada vai para a raiz da URL base. A rota não aceita outro
// caminho por parâmetro.
func (c *Client) TestService(ctx context.Context, code string) (*ServiceTestResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url("/api/public/2.0/authorize/client/test") + "?serviceCode=" + url.QueryEscape(code)
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "authorize/client/test", Body: truncate(string(body), 512)}
	}
	var resp struct {
		Content struct {
			HTTPStatusResult int    `json:"httpStatusResult"`
			Result           string `json:"result"`
			Description      string `json:"description"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de authorize/client/test: %w", err)
	}
	r := &ServiceTestResult{Code: code, HTTPStatus: resp.Content.HTTPStatusResult, Body: resp.Content.Result}
	if r.HTTPStatus == 0 {
		// Falha antes da chamada: o result traz um stack trace Java; fica só a
		// primeira linha, sem o nome da classe da exceção.
		first, _, _ := strings.Cut(r.Body, "\n")
		if _, msg, ok := strings.Cut(first, ": "); ok {
			first = msg
		}
		r.Error, r.Body = strings.TrimSpace(first), ""
	}
	return r, nil
}

// ServiceLogSummary é o resumo do log de requisições de um serviço.
type ServiceLogSummary struct {
	Code        string  `json:"code"`
	Active      bool    `json:"active"`
	ExpiresAt   string  `json:"expiresAt,omitempty"` // "DD/MM/AAAA HH:MM", horário do servidor
	Total       int     `json:"total"`
	Success     int     `json:"success"`
	Failed      int     `json:"failed"`
	SuccessRate float64 `json:"successRate"`
}

// GetServiceLogSummary lê o resumo do log (GET getDataServiceSummary).
func (c *Client) GetServiceLogSummary(ctx context.Context, code string) (*ServiceLogSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restServicesBase+"getDataServiceSummary/"+url.PathEscape(code)), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, checkServiceResponse("ecmservices/getDataServiceSummary", body, status)
	}
	var resp struct {
		Content struct {
			Summary struct {
				Total       string `json:"total"`
				SuccessRate string `json:"successRate"`
				Success     string `json:"success"`
				Failed      string `json:"failed"`
			} `json:"summary"`
			Service struct {
				LogExpirationTime string `json:"logExpirationTime"`
				LogRequestActive  bool   `json:"logRequestActive"`
			} `json:"service"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("resposta inesperada de getDataServiceSummary: %w", err)
	}
	sm := resp.Content.Summary
	out := &ServiceLogSummary{Code: code, Active: resp.Content.Service.LogRequestActive, ExpiresAt: resp.Content.Service.LogExpirationTime}
	out.Total, _ = strconv.Atoi(sm.Total)
	out.Success, _ = strconv.Atoi(sm.Success)
	out.Failed, _ = strconv.Atoi(sm.Failed)
	out.SuccessRate, _ = strconv.ParseFloat(sm.SuccessRate, 64)
	return out, nil
}

// SetServiceLog liga (por hours horas, de 1 a 24) ou desliga o log de
// requisições do serviço (POST updateLogActive com o item da listagem, como
// a tela faz). Devolve a hora de expiração informada pelo servidor.
func (c *Client) SetServiceLog(ctx context.Context, s *Service, active bool, hours int) (string, error) {
	if s.item == nil {
		return "", fmt.Errorf("o serviço %q veio sem o item da listagem", s.Code)
	}
	if err := c.EnsureSession(ctx); err != nil {
		return "", err
	}
	item := make(map[string]any, len(s.item)+2)
	for k, v := range s.item {
		item[k] = v
	}
	item["logRequestActive"] = active
	item["expirationTime"] = hours
	data, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(restServicesBase+"updateLogActive"), data)
	if err != nil {
		return "", err
	}
	if err := checkServiceResponse("ecmservices/updateLogActive", body, status); err != nil {
		return "", err
	}
	var resp struct {
		Content struct {
			LogExpirationTime string `json:"logExpirationTime"`
		} `json:"content"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Content.LogExpirationTime, nil
}

// DownloadServiceLog baixa o log de requisições de TODOS os serviços com log
// ligado (GET downloadLog → zip com um .log). ⚠️ O log traz as mensagens
// completas, inclusive os headers enviados (o segredo da autenticação
// custom). Quem chama grava em arquivo, nunca no terminal.
func (c *Client) DownloadServiceLog(ctx context.Context) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doRaw(ctx, http.MethodGet, c.url(restServicesBase+"downloadLog"), nil, "", "*/*")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "ecmservices/downloadLog", Body: truncate(string(body), 512)}
	}
	return body, nil
}

// serviceAuthFields são os campos do oAuthVO que a tela do Fluig envia para
// cada tipo de autenticação (dataservicewizard). A cópia entre servidores
// envia só estes: id, tenantId e datas da origem ficam de fora.
var serviceAuthFields = map[string][]string{
	ServiceAuthBasic:  {"username", "password", "passwordConfirmation"},
	ServiceAuthOAuth1: {"consumerKey", "consumerSecret", "tokenAccess", "tokenSecret", "accessTokenURL", "requestTokenURL", "authorizationURL"},
	ServiceAuthOAuth2: {"clientId", "clientSecret", "refreshToken", "tokenAccess", "refreshTokenURL"},
}

// CopyServiceTo cria ou alinha, no servidor de c, o serviço REST src (lido
// de OUTRO servidor), com o código code. Copia a URL, a descrição, o
// caminho de teste, o swagger, o tipo de autenticação e os segredos de
// qualquer tipo (basic, custom, OAuth 1, OAuth 2), sem expor os valores.
// Devolve "created" ou "updated".
func (c *Client) CopyServiceTo(ctx context.Context, src *Service, code string) (string, error) {
	if src.Type != ServiceTypeREST {
		return "", fmt.Errorf("só serviços REST podem ser copiados (o serviço %q é %s)", src.Code, src.Type)
	}
	if src.oauth == nil {
		return "", fmt.Errorf("o serviço %q veio sem os dados de autenticação", src.Code)
	}
	cur, err := c.GetService(ctx, code)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	oauth := map[string]any{
		"code":               code,
		"description":        src.Description,
		"authenticationType": src.AuthType,
		"domain":             src.URL,
		"serviceTestUrl":     src.TestPath,
		"custom":             jsonString(src.oauth, "custom"),
	}
	for _, k := range serviceAuthFields[src.AuthType] {
		if v, ok := src.oauth[k]; ok {
			oauth[k] = v
		}
	}
	payload := map[string]any{
		"name":        code,
		"url":         src.Swagger,
		"description": src.Description,
		"type":        "3",
		"oAuthVO":     oauth,
	}
	if cur == nil {
		if err := c.EnsureSession(ctx); err != nil {
			return "", err
		}
		return "created", c.postServiceWrite(ctx, "service", payload)
	}
	if cur.Type != ServiceTypeREST {
		return "", fmt.Errorf("o serviço %q no destino é %s; a cópia só trata REST", code, cur.Type)
	}
	if cur.oauthID == 0 {
		return "", fmt.Errorf("o destino não devolveu o id do cliente OAuth do serviço %q", code)
	}
	oauth["id"] = cur.oauthID
	return "updated", c.postServiceWrite(ctx, "update", payload)
}
