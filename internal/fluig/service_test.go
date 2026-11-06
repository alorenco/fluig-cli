package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type serviceStub struct {
	mu        sync.Mutex
	queries   []string
	created   map[string]any
	updated   map[string]any
	deleted   []string
	createErr string // corpo de erro do create (HTTP 500)
}

func (s *serviceStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/findByType", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.queries = append(s.queries, r.URL.RawQuery)
		s.mu.Unlock()
		if r.URL.Query().Get("offset") != "0" {
			io.WriteString(w, `{"content":[],"message":null}`)
			return
		}
		w.Write(testdata(t, "ecmservices_findByType.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/service", func(w http.ResponseWriter, r *http.Request) {
		if s.createErr != "" {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, s.createErr)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &s.created)
		io.WriteString(w, `{"content":"OK","message":null}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/update", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &s.updated)
		io.WriteString(w, `{"content":"OK","message":null}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("delete com método %s", r.Method)
		}
		s.deleted = append(s.deleted, r.URL.Query().Get("id"))
		io.WriteString(w, `{"content":"OK","message":null}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func serviceClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-svc-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func strp(s string) *string { return &s }

func TestListServicesSemSegredos(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	list, err := c.ListServices(context.Background(), ServiceFilter{Types: []string{ServiceTypeREST}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("esperava 4 serviços, veio %d", len(list))
	}
	byCode := map[string]Service{}
	for _, s := range list {
		byCode[s.Code] = s
	}
	basic := byCode["APIInfoBip"]
	if basic.Type != ServiceTypeREST || basic.AuthType != ServiceAuthBasic || basic.URL != "https://api.infobip.com/" || basic.Username != "jsilva" {
		t.Errorf("serviço basic inesperado: %+v", basic)
	}
	if byCode["jusbrasil"].AuthType != ServiceAuthNone || byCode["FluigRest"].AuthType != ServiceAuthFluigAPI {
		t.Errorf("tipos de autenticação inesperados: %+v", byCode)
	}
	// O JSON de saída nunca carrega segredo (a fixture marca todos com REDACTED).
	out, _ := json.Marshal(list)
	for _, bad := range []string{"REDACTED", "password", "\"custom\":", "oAuthVO"} {
		if strings.Contains(string(out), bad) {
			t.Errorf("saída do Service vaza %q: %s", bad, out)
		}
	}
	if !strings.Contains(stub.queries[0], "types=REST") {
		t.Errorf("filtro de tipo não enviado: %q", stub.queries[0])
	}
}

func TestListServicesTipoInvalido(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	if _, err := c.ListServices(context.Background(), ServiceFilter{Types: []string{"ftp"}}); err == nil {
		t.Fatal("esperava erro para tipo inválido")
	}
}

func TestGetServiceExatoENaoEncontrado(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	s, err := c.GetService(context.Background(), "jusbrasil")
	if err != nil || s.URL != "https://op.digesto.com.br" {
		t.Fatalf("GetService: %+v, %v", s, err)
	}
	if !strings.Contains(stub.queries[0], "pattern=jusbrasil") {
		t.Errorf("GetService sem pattern: %q", stub.queries[0])
	}
	// "APIInfoBip" também casa com "APIInfoBipWhatsApp" no pattern: o filtro
	// final é pelo nome exato.
	s, err = c.GetService(context.Background(), "APIInfoBip")
	if err != nil || s.Code != "APIInfoBip" {
		t.Fatalf("match exato falhou: %+v, %v", s, err)
	}
	if _, err := c.GetService(context.Background(), "nao_existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
}

func TestCreateServiceCorpo(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	err := c.CreateService(context.Background(), "sabia_agentos", ServiceInput{
		URL: strp("https://agentos.example.com"), TestPath: strp("/health"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stub.created["name"] != "sabia_agentos" || stub.created["type"] != "3" || stub.created["description"] != "sabia_agentos" {
		t.Errorf("corpo inesperado: %+v", stub.created)
	}
	o := stub.created["oAuthVO"].(map[string]any)
	if o["code"] != "sabia_agentos" || o["authenticationType"] != "none" || o["domain"] != "https://agentos.example.com" || o["serviceTestUrl"] != "/health" {
		t.Errorf("oAuthVO inesperado: %+v", o)
	}
	if _, ok := o["password"]; ok {
		t.Errorf("auth none não deve mandar senha: %+v", o)
	}
}

func TestCreateServiceBasicMandaSenha(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	err := c.CreateService(context.Background(), "svc", ServiceInput{
		URL: strp("https://x"), AuthType: strp(ServiceAuthBasic), Username: strp("jsilva"), Password: strp("s3nha"),
	})
	if err != nil {
		t.Fatal(err)
	}
	o := stub.created["oAuthVO"].(map[string]any)
	if o["username"] != "jsilva" || o["password"] != "s3nha" || o["passwordConfirmation"] != "s3nha" {
		t.Errorf("basic sem credencial completa: %+v", o)
	}
}

func TestCreateServiceDuplicado(t *testing.T) {
	stub := &serviceStub{createErr: `{"content":"ERROR","message":{"message":"Já existe um serviço cadastrado com este código.","detail":"Já existe um serviço cadastrado com este código.","type":"ERROR","param":null,"errorCode":"FDNDataServicePKException"}}`}
	c := serviceClient(t, stub.server(t).URL)
	err := c.CreateService(context.Background(), "jusbrasil", ServiceInput{URL: strp("https://x")})
	if !errors.Is(err, ErrServiceExists) {
		t.Fatalf("esperava ErrServiceExists, veio %v", err)
	}
}

// O update reenvia o oAuthVO atual: a troca da URL preserva a senha e o
// segredo custom guardados no servidor.
func TestUpdateServicePreservaSegredos(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	ctx := context.Background()
	cur, err := c.GetService(ctx, "APIInfoBip")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateService(ctx, cur, ServiceInput{URL: strp("https://novo.example.com")}); err != nil {
		t.Fatal(err)
	}
	o := stub.updated["oAuthVO"].(map[string]any)
	if o["domain"] != "https://novo.example.com" || o["password"] != "REDACTED" || o["username"] != "jsilva" {
		t.Errorf("update perdeu campos: %+v", o)
	}
	if o["id"] != float64(2) || stub.updated["name"] != "APIInfoBip" {
		t.Errorf("update sem id/nome: %+v", stub.updated)
	}
}

func TestUpdateServiceTrocaParaNoneLimpaSenha(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	ctx := context.Background()
	cur, _ := c.GetService(ctx, "APIInfoBip")
	if err := c.UpdateService(ctx, cur, ServiceInput{AuthType: strp(ServiceAuthNone)}); err != nil {
		t.Fatal(err)
	}
	o := stub.updated["oAuthVO"].(map[string]any)
	for _, k := range []string{"username", "password", "passwordConfirmation"} {
		if _, ok := o[k]; ok {
			t.Errorf("troca para none manteve %q", k)
		}
	}
}

func TestDeleteServiceConfereExistencia(t *testing.T) {
	stub := &serviceStub{}
	c := serviceClient(t, stub.server(t).URL)
	ctx := context.Background()
	if err := c.DeleteService(ctx, "jusbrasil"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteService(ctx, "nao_existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
	if len(stub.deleted) != 1 || stub.deleted[0] != "jusbrasil" {
		t.Errorf("deletes inesperados: %v", stub.deleted)
	}
}
