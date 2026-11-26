package fluig

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// warUploadStub simula o servidor nos três cenários do ROADMAP §5.24:
// Fluig 2.0 e 1.8.2 (REST de upload existe) e 1.8.1 (a REST responde 500 com
// NotFoundException e só o servlet ApplicationUpload publica).
type warUploadStub struct {
	version     string // valor de /api/public/wcm/version
	restStatus  int    // 0 = 200 {}
	restBody    string
	servletResp []servletReply // respostas em ordem; a última se repete

	restCalls    int
	servletCalls []map[string]string // campos de cada envio ao servlet
	servletFile  []string            // nome do arquivo de cada envio
}

type servletReply struct {
	status int
	body   string
}

func (s *warUploadStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc(serverVersionPath, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"value":"TOTVS Fluig Plataforma - `+s.version+`"}`)
	})
	mux.HandleFunc(currentUserPath, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"id":5,"login":"jsilva"},"message":null}`)
	})
	mux.HandleFunc("/portal/p/1/home", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/portal/p/01/home", http.StatusFound)
	})
	mux.HandleFunc("/portal/p/01/home", func(w http.ResponseWriter, r *http.Request) {
		t.Error("o redirecionamento do tenant não devia ser seguido")
	})
	mux.HandleFunc(widgetUploadPath, func(w http.ResponseWriter, r *http.Request) {
		s.restCalls++
		if s.restStatus != 0 {
			w.WriteHeader(s.restStatus)
			io.WriteString(w, s.restBody)
			return
		}
		io.WriteString(w, `{}`)
	})
	mux.HandleFunc(applicationUploadPath, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("multipart inválido: %v", err)
		}
		fields := map[string]string{}
		for k, v := range r.MultipartForm.Value {
			fields[k] = v[0]
		}
		name := ""
		if _, h, err := r.FormFile(applicationUploadFileField); err == nil {
			name = h.Filename
		}
		s.servletCalls = append(s.servletCalls, fields)
		s.servletFile = append(s.servletFile, name)
		reply := s.servletResp[len(s.servletResp)-1]
		if i := len(s.servletCalls) - 1; i < len(s.servletResp) {
			reply = s.servletResp[i]
		}
		w.WriteHeader(reply.status)
		io.WriteString(w, reply.body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func warClient(t *testing.T, url string, mode WARUploadMode) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-war-" + t.Name(), Password: "p", CompanyID: 1, WARUpload: mode})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// restNotFound181 é o corpo do 500 que o FDNExceptionMapper devolve para rota
// inexistente (mesmo formato medido na homologação em outras rotas).
const restNotFound181 = `{"code":"NotFoundException","message":"","detailedMessage":"","helpUrl":null,"details":[]}`

func question() servletReply {
	return servletReply{200, "<script>\nparent.WCMUploadComplete('O componente já existe. Deseja substituir?','Question');\n</script>"}
}

func TestUploadWARAuto20UsaSoREST(t *testing.T) {
	stub := &warUploadStub{version: "Voyager 2.0.0-260707"}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	if err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK")); err != nil {
		t.Fatal(err)
	}
	if stub.restCalls != 1 || len(stub.servletCalls) != 0 {
		t.Errorf("rest=%d servlet=%d, quer 1 e 0", stub.restCalls, len(stub.servletCalls))
	}
}

// No 2.0, um 500 da REST é erro real: a CLI NÃO tenta o servlet.
func TestUploadWARAuto20ErroRESTNaoCaiNoServlet(t *testing.T) {
	stub := &warUploadStub{version: "Voyager 2.0.0-260707", restStatus: 500, restBody: restNotFound181}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK"))
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != 500 {
		t.Fatalf("quer HTTPError 500, veio %v", err)
	}
	if len(stub.servletCalls) != 0 {
		t.Errorf("o servlet não devia ser chamado no 2.0")
	}
}

func TestUploadWARAuto182RESTFunciona(t *testing.T) {
	stub := &warUploadStub{version: "Crystal Mist 1.8.2-260707"}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	if err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK")); err != nil {
		t.Fatal(err)
	}
	if stub.restCalls != 1 || len(stub.servletCalls) != 0 {
		t.Errorf("rest=%d servlet=%d, quer 1 e 0", stub.restCalls, len(stub.servletCalls))
	}
}

// Cenário da issue #8: 1.8.1 sem a REST. A CLI cai no servlet com os campos
// do formulário da Central de Componentes.
func TestUploadWARAuto181CaiNoServlet(t *testing.T) {
	stub := &warUploadStub{
		version: "Silver Mist 1.8.1-230912", restStatus: 500, restBody: restNotFound181,
		servletResp: []servletReply{{200, readFixture(t, "applicationupload_info.html")}},
	}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	if err := c.UploadWidgetWAR(context.Background(), "painel.war", []byte("PK fake")); err != nil {
		t.Fatal(err)
	}
	if stub.restCalls != 1 || len(stub.servletCalls) != 1 {
		t.Fatalf("rest=%d servlet=%d, quer 1 e 1", stub.restCalls, len(stub.servletCalls))
	}
	want := map[string]string{"forceDeploy": "false", "tenantId": "1", "tenantCode": "01", "userId": "5"}
	for k, v := range want {
		if got := stub.servletCalls[0][k]; got != v {
			t.Errorf("campo %s = %q, quer %q", k, got, v)
		}
	}
	if stub.servletFile[0] != "painel.war" {
		t.Errorf("arquivo = %q, quer painel.war", stub.servletFile[0])
	}
}

// Versão desconhecida e REST com 404 também caem no servlet.
func TestUploadWARAutoVersaoDesconhecida404(t *testing.T) {
	stub := &warUploadStub{
		version: "", restStatus: 404, restBody: "",
		servletResp: []servletReply{{200, readFixture(t, "applicationupload_info.html")}},
	}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	if err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK")); err != nil {
		t.Fatal(err)
	}
	if len(stub.servletCalls) != 1 {
		t.Errorf("servlet=%d, quer 1", len(stub.servletCalls))
	}
}

// Erro que não é de rota (400 da REST) não cai no servlet.
func TestUploadWARAuto181ErroDeNegocioNaoCai(t *testing.T) {
	stub := &warUploadStub{version: "1.8.1", restStatus: 400, restBody: `{"message":"x"}`}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	if err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK")); err == nil {
		t.Fatal("quer erro")
	}
	if len(stub.servletCalls) != 0 {
		t.Errorf("o servlet não devia ser chamado")
	}
}

func TestUploadWARAuto181AmbosFalham(t *testing.T) {
	stub := &warUploadStub{
		version: "1.8.1", restStatus: 500, restBody: restNotFound181,
		servletResp: []servletReply{{400, readFixture(t, "applicationupload_arquivo_invalido.json")}},
	}
	c := warClient(t, stub.server(t).URL, WARUploadAuto)
	err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("lixo"))
	if !errors.Is(err, errServerRejected) {
		t.Fatalf("quer errServerRejected, veio %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, `Provider "jar" not found`) || strings.Contains(msg, "<JavaError>") {
		t.Errorf("mensagem sem o texto limpo do servlet: %s", msg)
	}
	if !strings.Contains(msg, "REST também falhou") {
		t.Errorf("mensagem sem o erro da REST: %s", msg)
	}
}

// Question = componente já existe: reenvia com forceDeploy=true.
func TestUploadWARServletQuestionReenviaComForce(t *testing.T) {
	stub := &warUploadStub{
		version:     "1.8.1",
		servletResp: []servletReply{question(), {200, readFixture(t, "applicationupload_info.html")}},
	}
	c := warClient(t, stub.server(t).URL, WARUploadServlet)
	if err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK")); err != nil {
		t.Fatal(err)
	}
	if stub.restCalls != 0 {
		t.Errorf("modo servlet não devia chamar a REST")
	}
	if len(stub.servletCalls) != 2 {
		t.Fatalf("servlet=%d, quer 2", len(stub.servletCalls))
	}
	if stub.servletCalls[0]["forceDeploy"] != "false" || stub.servletCalls[1]["forceDeploy"] != "true" {
		t.Errorf("forceDeploy = %q, %q; quer false, true",
			stub.servletCalls[0]["forceDeploy"], stub.servletCalls[1]["forceDeploy"])
	}
}

func TestUploadWARServletQuestionDuasVezes(t *testing.T) {
	stub := &warUploadStub{version: "1.8.1", servletResp: []servletReply{question()}}
	c := warClient(t, stub.server(t).URL, WARUploadServlet)
	err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK"))
	if !errors.Is(err, errServerRejected) || len(stub.servletCalls) != 2 {
		t.Fatalf("quer errServerRejected após 2 envios; veio %v com %d envios", err, len(stub.servletCalls))
	}
}

func TestUploadWARServletErroEWarning(t *testing.T) {
	for _, kind := range []string{"Error", "Warning"} {
		t.Run(kind, func(t *testing.T) {
			stub := &warUploadStub{version: "1.8.1", servletResp: []servletReply{
				{200, "<script>parent.WCMUploadComplete('Falha no deploy d\\'aqui','" + kind + "');</script>"},
			}}
			c := warClient(t, stub.server(t).URL, WARUploadServlet)
			err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK"))
			if !errors.Is(err, errServerRejected) || !strings.Contains(err.Error(), "Falha no deploy d'aqui") {
				t.Errorf("quer errServerRejected com a mensagem, veio %v", err)
			}
		})
	}
}

func TestUploadWARModoRESTNaoCai(t *testing.T) {
	stub := &warUploadStub{version: "1.8.1", restStatus: 500, restBody: restNotFound181}
	c := warClient(t, stub.server(t).URL, WARUploadREST)
	if err := c.UploadWidgetWAR(context.Background(), "x.war", []byte("PK")); err == nil {
		t.Fatal("quer erro")
	}
	if len(stub.servletCalls) != 0 {
		t.Errorf("modo rest não devia chamar o servlet")
	}
}

func TestParseApplicationUpload(t *testing.T) {
	kind, msg, err := parseApplicationUpload(200, readFixture(t, "applicationupload_info.html"))
	if err != nil || kind != "Info" || !strings.HasPrefix(msg, "Seu componente foi enviado com sucesso") {
		t.Errorf("info: kind=%q msg=%q err=%v", kind, msg, err)
	}
	// Página de login (sessão perdida) com 200: não pode virar sucesso.
	if _, _, err := parseApplicationUpload(200, "<html><form action=\"login\"></form></html>"); err == nil {
		t.Error("página sem WCMUploadComplete devia dar erro")
	}
	// Não-2xx sem JSON vira HTTPError.
	_, _, err = parseApplicationUpload(502, "Bad Gateway")
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != 502 {
		t.Errorf("quer HTTPError 502, veio %v", err)
	}
}

func TestUnescapeJSString(t *testing.T) {
	cases := map[string]string{
		`simples`:            "simples",
		`d\'água`:            "d'água",
		`linha\nnova`:        "linha\nnova",
		`barra \\ e \/`:      `barra \ e /`,
		`é acento`:           "é acento",
		`fim com barra \`:    `fim com barra \`,
		`\uZZZZ inválido`:    `\uZZZZ inválido`,
		`aspas \"duplas\"`:   `aspas "duplas"`,
		`tab\taqui`:          "tab\taqui",
		`retorno\r`:          "retorno\r",
		`outro \x escape`:    "outro x escape",
		`curto \u00`:         `curto \u00`,
		`unicode ABC`:        "unicode ABC",
		`várias \' \' \'`:    "várias ' ' '",
		`nada a fazer aqui.`: "nada a fazer aqui.",
	}
	for in, want := range cases {
		if got := unescapeJSString(in); got != want {
			t.Errorf("unescapeJSString(%q) = %q, quer %q", in, got, want)
		}
	}
}

func TestTenantCodeFromLocation(t *testing.T) {
	cases := map[string]string{
		"http://h:8080/portal/p/01/home": "01",
		"/portal/p/empresa/home":         "empresa",
		"/portal/home":                   "",
		"/portal/p/api/servlet/ping":     "",
		"/portal/login":                  "",
		"":                               "",
	}
	for in, want := range cases {
		if got := tenantCodeFromLocation(in); got != want {
			t.Errorf("tenantCodeFromLocation(%q) = %q, quer %q", in, got, want)
		}
	}
}

func TestParseWARUploadMode(t *testing.T) {
	for in, want := range map[string]WARUploadMode{"": WARUploadAuto, "AUTO": WARUploadAuto, "rest": WARUploadREST, " Servlet ": WARUploadServlet} {
		got, err := ParseWARUploadMode(in)
		if err != nil || got != want {
			t.Errorf("ParseWARUploadMode(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseWARUploadMode("ftp"); err == nil {
		t.Error("valor inválido devia dar erro")
	}
}
