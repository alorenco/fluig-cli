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
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/findByType", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			io.WriteString(w, `{"content":[]}`)
			return
		}
		w.Write(fixture)
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
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/ecmservices/update", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.updated)
		s.mu.Unlock()
		io.WriteString(w, `{"content":"OK"}`)
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
