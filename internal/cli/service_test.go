package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

type svcStub struct {
	mu        sync.Mutex
	created   map[string]any
	updated   map[string]any
	deleted   []string
	createErr bool
	logBody   map[string]any
	lastWrite map[string]any // corpo do último service/update (SOAP/JDBC)
}

func (s *svcStub) server(t *testing.T) *httptest.Server {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ecmservices_findByType.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	soapjdbc, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ecmservices_findByType_soapjdbc.json"))
	if err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/findByType", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			io.WriteString(w, `{"content":[]}`)
			return
		}
		// Junta REST + SOAP/JDBC numa resposta só.
		var a, b struct {
			Content []json.RawMessage `json:"content"`
		}
		_ = json.Unmarshal(fixture, &a)
		_ = json.Unmarshal(soapjdbc, &b)
		out, _ := json.Marshal(map[string]any{"content": append(a.Content, b.Content...)})
		w.Write(out)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/drivers", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":[{"name":"sqlserver","type":"SQLSERVER","available":true,"jdbcUrl":"jdbc:sqlserver://<host>:1433"},{"name":"oracle","type":"ORACLE","available":false,"jdbcUrl":"jdbc:oracle:thin:@<host>"}]}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/connection/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/AppDS") {
			io.WriteString(w, `{"content":"OK","message":null}`)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"content":"ERROR","message":{"message":"The TCP/IP connection to the host x has failed."}}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/service", func(w http.ResponseWriter, r *http.Request) {
		if s.createErr {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"content":"ERROR","message":{"message":"Já existe um serviço cadastrado com este código.","errorCode":"FDNDataServicePKException"}}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.created)
		_ = json.Unmarshal(b, &s.lastWrite)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/update", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.updated)
		_ = json.Unmarshal(b, &s.lastWrite)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/getDataServiceSummary/", func(w http.ResponseWriter, r *http.Request) {
		active := s.logBody != nil && s.logBody["logRequestActive"] == true
		io.WriteString(w, `{"content":{"summary":{"total":"3","successRate":"66.7","success":"2","failed":"1"},"service":{"logExpirationTime":"06/11/2026 04:37","logRequestActive":`+map[bool]string{true: "true", false: "false"}[active]+`}}}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/updateLogActive", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.logBody)
		s.mu.Unlock()
		io.WriteString(w, `{"content":{"logExpirationTime":"06/11/2026 04:37","logRequestActive":true}}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/downloadLog", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("PK\x03\x04zip-falso"))
	})
	mux.HandleFunc("/api/public/2.0/authorize/client/test", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("serviceCode") {
		case "jusbrasil":
			io.WriteString(w, `{"content":{"serviceCode":"jusbrasil","description":"jusbrasil:SUCCESS","httpStatusResult":200,"result":"{\"ok\":true}"},"message":{"message":"OK"}}`)
		case "APIInfoBip":
			io.WriteString(w, `{"content":{"serviceCode":"APIInfoBip","httpStatusResult":401,"result":"{\"erro\":\"token\"}"},"message":{"message":"OK"}}`)
		default:
			io.WriteString(w, `{"content":{"httpStatusResult":0,"result":"com.totvs.X: java.net.UnknownHostException: host.invalid\n\tat a.b.C(C.java:1)"},"message":{"message":"OK"}}`)
		}
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/delete", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.deleted = append(s.deleted, r.URL.Query().Get("id"))
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestServiceListTabela(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "service", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "Código", "Autenticação", "APIInfoBip", "REST", "basic", "https://op.digesto.com.br"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "REDACTED") {
		t.Errorf("tabela vaza segredo:\n%s", stdout)
	}
}

func TestServiceShowJSONSemSegredos(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "service", "show", "APIInfoBipWhatsApp", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, `"authType":"custom"`) || strings.Contains(stdout, "REDACTED") {
		t.Errorf("envelope inesperado: %s", stdout)
	}
}

func TestServiceShowNaoEncontrado(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, _ := runMain(t, "service", "show", "nao_existe", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, esperava %d", code, output.ExitNotFound)
	}
}

func TestServiceCreate(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	// O GET de confirmação depois do create usa a fixture: cria um código que existe nela.
	code, stdout := runMain(t, "service", "create", "jusbrasil", "--url", "https://op.digesto.com.br",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	o := stub.created["oAuthVO"].(map[string]any)
	if o["authenticationType"] != "none" || o["domain"] != "https://op.digesto.com.br" {
		t.Errorf("create inesperado: %+v", o)
	}
}

func TestServiceCreateValidacoes(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	cases := map[string][]string{
		"sem url":           {"service", "create", "x"},
		"url sem esquema":   {"service", "create", "x", "--url", "api.example.com"},
		"auth não gravável": {"service", "create", "x", "--url", "https://x", "--auth", "oauth2"},
		"basic sem usuário": {"service", "create", "x", "--url", "https://x", "--auth", "basic"},
		"usuário sem basic": {"service", "create", "x", "--url", "https://x", "--username", "jsilva"},
		// Sem FLUIGCLI_SERVICE_PASSWORD e sem terminal: não há de onde ler a senha.
		"basic sem senha": {"service", "create", "x", "--url", "https://x", "--auth", "basic", "--username", "jsilva"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			args = append(args, "--project", proj, "--server", "homolog", "--json")
			if code, out := runMain(t, args...); code != output.ExitUsage {
				t.Errorf("exit=%d, esperava %d\n%s", code, output.ExitUsage, out)
			}
		})
	}
	if stub.created != nil {
		t.Errorf("validação falhou e o create foi enviado: %+v", stub.created)
	}
}

func TestServiceCreateBasicSenhaDaEnv(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	t.Setenv(envServicePassword, "s3nha")
	code, out := runMain(t, "service", "create", "APIInfoBip", "--url", "https://x", "--auth", "basic", "--username", "jsilva",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	o := stub.created["oAuthVO"].(map[string]any)
	if o["password"] != "s3nha" || o["username"] != "jsilva" {
		t.Errorf("basic sem credencial: %+v", o)
	}
	if strings.Contains(out, "s3nha") {
		t.Errorf("saída vaza a senha: %s", out)
	}
}

func TestServiceCreateDuplicado(t *testing.T) {
	stub := &svcStub{createErr: true}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, "service", "create", "jusbrasil", "--url", "https://x", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitServer || !strings.Contains(out, "service update jusbrasil") {
		t.Errorf("exit=%d\n%s", code, out)
	}
}

func TestServiceUpdatePreservaSenha(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, "service", "update", "APIInfoBip", "--url", "https://novo.example.com",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	o := stub.updated["oAuthVO"].(map[string]any)
	if o["domain"] != "https://novo.example.com" || o["password"] != "REDACTED" {
		t.Errorf("update perdeu a senha guardada: %+v", o)
	}
}

func TestServiceUpdateSemAlteracao(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, _ := runMain(t, "service", "update", "jusbrasil", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, esperava %d", code, output.ExitUsage)
	}
}

func TestServiceDelete(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, _ := runMain(t, "service", "delete", "jusbrasil", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitUsage {
		t.Errorf("sem --yes: exit=%d, esperava %d", code, output.ExitUsage)
	}
	code, out := runMain(t, "service", "delete", "jusbrasil", "nao_existe", "--yes", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, esperava %d\n%s", code, output.ExitPartial, out)
	}
	if len(stub.deleted) != 1 || stub.deleted[0] != "jusbrasil" {
		t.Errorf("deletes inesperados: %v", stub.deleted)
	}
}

func TestServiceCreateCustomPorStdin(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	old := serviceStdin
	serviceStdin = strings.NewReader("Authorization: Bearer zz-falso\n")
	t.Cleanup(func() { serviceStdin = old })
	code, out, errOut := runMainStderr(t, "service", "create", "APIInfoBipWhatsApp", "--url", "https://x", "--auth", "custom",
		"--custom-headers-stdin", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if o := stub.created["oAuthVO"].(map[string]any); o["custom"] != "Authorization: Bearer zz-falso" {
		t.Errorf("custom não enviado: %+v", o)
	}
	if strings.Contains(out+errOut, "zz-falso") {
		t.Errorf("saída vaza o valor do header: %s %s", out, errOut)
	}
	if !strings.Contains(out, `"customHeaders":["Authorization"]`) {
		t.Errorf("envelope sem os nomes dos headers: %s", out)
	}
}

func TestServiceCreateCustomPorEnv(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	t.Setenv(envServiceCustomHeaders, "X-Api-Key: zz-falso")
	code, out := runMain(t, "service", "create", "APIInfoBipWhatsApp", "--url", "https://x", "--auth", "custom",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if o := stub.created["oAuthVO"].(map[string]any); o["custom"] != "X-Api-Key: zz-falso" {
		t.Errorf("custom não enviado: %+v", o)
	}
}

func TestServiceCustomValidacoes(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	old := serviceStdin
	t.Cleanup(func() { serviceStdin = old })
	cases := map[string]struct {
		stdin string
		args  []string
	}{
		"custom sem headers":     {"", []string{"service", "create", "x", "--url", "https://x", "--auth", "custom"}},
		"header inválido":        {"Authorization Bearer zz", []string{"service", "create", "x", "--url", "https://x", "--auth", "custom", "--custom-headers-stdin"}},
		"stdin sem custom":       {"A: b", []string{"service", "create", "x", "--url", "https://x", "--custom-headers-stdin"}},
		"dois leitores de stdin": {"A: b", []string{"service", "create", "x", "--url", "https://x", "--auth", "custom", "--custom-headers-stdin", "--password-stdin"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			serviceStdin = strings.NewReader(c.stdin)
			args := append(c.args, "--project", proj, "--server", "homolog", "--json")
			code, out := runMain(t, args...)
			if code != output.ExitUsage {
				t.Errorf("exit=%d, esperava %d\n%s", code, output.ExitUsage, out)
			}
			if strings.Contains(out, "Bearer zz") {
				t.Errorf("mensagem cita o valor: %s", out)
			}
		})
	}
	if stub.created != nil {
		t.Errorf("validação falhou e o create foi enviado: %+v", stub.created)
	}
}

func TestServiceUpdateCustomPreservaHeaders(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, "service", "update", "APIInfoBipWhatsApp", "--url", "https://novo", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if o := stub.updated["oAuthVO"].(map[string]any); o["custom"] != "Authorization: REDACTED" {
		t.Errorf("update perdeu os headers guardados: %+v", o)
	}
}

func TestServiceShowCustomMostraSoNomes(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, "service", "show", "APIInfoBipWhatsApp", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(out, "Headers custom: Authorization (valores ocultos)") || strings.Contains(out, "REDACTED") {
		t.Errorf("show inesperado:\n%s", out)
	}
}

func TestServiceTest(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, "service", "test", "jusbrasil", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK || !strings.Contains(out, `"httpStatus":200`) {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	code, out = runMain(t, "service", "test", "APIInfoBip", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitServer || !strings.Contains(out, `"httpStatus":401`) {
		t.Errorf("401: exit=%d\n%s", code, out)
	}
	code, out = runMain(t, "service", "test", "FluigRest", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitServer || !strings.Contains(out, "UnknownHostException") || strings.Contains(out, "C.java") {
		t.Errorf("status 0: exit=%d\n%s", code, out)
	}
	if code, _ := runMain(t, "service", "test", "nao_existe", "--project", proj, "--server", "homolog", "--json"); code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d", code)
	}
}

func TestServiceLog(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, "service", "log", "jusbrasil", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK || !strings.Contains(out, `"total":3`) || !strings.Contains(out, `"active":false`) {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	code, out = runMain(t, "service", "log", "jusbrasil", "--enable", "--hours", "5", "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK || stub.logBody["logRequestActive"] != true || stub.logBody["expirationTime"] != float64(5) || stub.logBody["name"] != "jusbrasil" {
		t.Fatalf("exit=%d corpo=%v\n%s", code, stub.logBody, out)
	}
	for name, args := range map[string][]string{
		"horas demais":    {"service", "log", "jusbrasil", "--enable", "--hours", "25"},
		"os dois":         {"service", "log", "jusbrasil", "--enable", "--disable"},
		"horas sem ligar": {"service", "log", "jusbrasil", "--hours", "2"},
		"sem código":      {"service", "log"},
	} {
		if code, _ := runMain(t, append(args, "--project", proj, "--server", "homolog", "--json")...); code != output.ExitUsage {
			t.Errorf("%s: exit=%d", name, code)
		}
	}
	path := filepath.Join(t.TempDir(), "log.zip")
	if code, out := runMain(t, "service", "log", "--download", "-o", path, "--project", proj, "--server", "homolog", "--json"); code != output.ExitOK {
		t.Fatalf("download: exit=%d\n%s", code, out)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("arquivo do log: %v %v", info, err)
	}
}

func TestServiceCreateOAuthPorStdin(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	old := serviceStdin
	t.Cleanup(func() { serviceStdin = old })
	serviceStdin = strings.NewReader(`{"clientId":"cid","clientSecret":"zz-segredo","tokenAccess":"tok"}`)
	code, out, errOut := runMainStderr(t, "service", "create", "jusbrasil", "--url", "https://x", "--auth", "oauth2", "--oauth-stdin",
		"--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if o := stub.created["oAuthVO"].(map[string]any); o["clientSecret"] != "zz-segredo" {
		t.Errorf("oauth não enviado: %+v", o)
	}
	if strings.Contains(out+errOut, "zz-segredo") {
		t.Errorf("saída vaza o segredo: %s %s", out, errOut)
	}
	// Sem os campos: recusa antes de gravar.
	stub.created = nil
	serviceStdin = strings.NewReader("")
	if code, _ := runMain(t, "service", "create", "x", "--url", "https://x", "--auth", "oauth1", "--project", proj, "--server", "homolog", "--json"); code != output.ExitUsage || stub.created != nil {
		t.Errorf("oauth1 sem campos: exit=%d created=%v", code, stub.created)
	}
}

func TestServiceSOAPeJDBC(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	run := func(args ...string) (int, string) {
		return runMain(t, append(args, "--project", proj, "--server", "homolog", "--json")...)
	}
	// show do SOAP basic e do JDBC: nada de senha.
	code, out := run("service", "show", "wsDataServer")
	if code != output.ExitOK || !strings.Contains(out, `"soapPackage":"com.empresa.ws"`) || strings.Contains(out, "REDACTED") {
		t.Fatalf("show soap: exit=%d\n%s", code, out)
	}
	code, out = run("service", "show", "AppDS")
	if code != output.ExitOK || !strings.Contains(out, `"driver":"sqlserver"`) {
		t.Fatalf("show jdbc: exit=%d\n%s", code, out)
	}
	// Update do SOAP basic sem senha nova: reenvia a guardada.
	if code, out := run("service", "update", "wsDataServer", "--description", "novo"); code != output.ExitOK {
		t.Fatalf("update soap: exit=%d\n%s", code, out)
	}
	if stub.lastWrite["password"] != "REDACTED" || stub.lastWrite["type"] != "1" || stub.lastWrite["soapWsPackage"] != "com.empresa.ws" {
		t.Errorf("update soap perdeu campos: %+v", stub.lastWrite)
	}
	// Update do JDBC sem senha: recusa (o Fluig não devolve a senha).
	stub.lastWrite = nil
	if code, _ := run("service", "update", "AppDS", "--description", "novo"); code != output.ExitUsage || stub.lastWrite != nil {
		t.Errorf("update jdbc sem senha: exit=%d write=%v", code, stub.lastWrite)
	}
	t.Setenv(envServicePassword, "s3nha")
	if code, out := run("service", "update", "AppDS", "--description", "novo"); code != output.ExitOK {
		t.Fatalf("update jdbc: exit=%d\n%s", code, out)
	}
	if stub.lastWrite["password"] != "s3nha" || stub.lastWrite["fileName"] != "SQLSERVER" || stub.lastWrite["type"] != "4" {
		t.Errorf("update jdbc inesperado: %+v", stub.lastWrite)
	}
	// Create JDBC: driver indisponível e flag de REST são recusados.
	if code, _ := run("service", "create", "x", "--type", "jdbc", "--url", "jdbc:x", "--driver", "oracle", "--username", "u"); code != output.ExitUsage {
		t.Errorf("driver indisponível: exit=%d", code)
	}
	if code, _ := run("service", "create", "x", "--type", "soap", "--url", "http://x?wsdl", "--test-path", "/x"); code != output.ExitUsage {
		t.Errorf("flag de REST no SOAP: exit=%d", code)
	}
	// Teste de conexão JDBC.
	if code, out := run("service", "test", "AppDS"); code != output.ExitOK || !strings.Contains(out, `"connected":true`) {
		t.Errorf("test jdbc: exit=%d\n%s", code, out)
	}
	if code, _ := run("service", "test", "wsDataServer"); code != output.ExitUsage {
		t.Errorf("test soap: exit=%d", code)
	}
	if code, out := run("service", "drivers"); code != output.ExitOK || strings.Contains(out, "ORACLE") {
		t.Errorf("drivers: exit=%d\n%s", code, out)
	}
}
