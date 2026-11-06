package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
var ServiceAuthWritable = []string{ServiceAuthNone, ServiceAuthBasic, ServiceAuthFluigAPI}

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
	Internal    bool   `json:"internal"`
	LogRequests bool   `json:"logRequests"`

	oauthID int
	oauth   map[string]json.RawMessage // oAuthVO cru, com segredos: nunca serializar para saída
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
	for _, it := range resp.Content {
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
			if s.AuthType == ServiceAuthBasic {
				s.Username = jsonString(it.OAuth, "username")
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
	Password    *string // só basic; vem de env/prompt, nunca de argumento
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
	// Troca para um tipo sem senha: não deixa a credencial antiga guardada.
	if in.AuthType != nil && *in.AuthType != ServiceAuthBasic {
		for _, k := range []string{"username", "password", "passwordConfirmation"} {
			delete(oauth, k)
		}
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

func deref(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}
