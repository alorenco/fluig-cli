package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// replacementStub simula os endpoints da substituição de usuário: REST v2
// user-replacements (list), findUserByLogin (resolução de login) e o SOAP
// ECMColleagueReplacementService (show). Fixtures reais sanitizadas.
type replacementStub struct {
	listQuery url.Values
	soapEmpty bool // quando true, o getReplacementsOfUser devolve o fault de result null
}

func (s *replacementStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})

	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		login := r.URL.Query().Get("login")
		if login == "fantasma" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		io.WriteString(w, `{"content":{"userCode":"`+login+`-code","fullName":"Nome `+login+`","email":"`+login+`@x.com"}}`)
	})

	mux.HandleFunc("/process-management/api/v2/user-replacements", func(w http.ResponseWriter, r *http.Request) {
		s.listQuery = r.URL.Query()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_user_replacements.json"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})

	mux.HandleFunc("/webdesk/ECMColleagueReplacementService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
		if s.soapEmpty {
			io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Server</faultcode><faultstring>Cannot write part result. RPC/Literal parts cannot be null. (WS-I BP R2211)</faultstring></soap:Fault></soap:Body></soap:Envelope>`)
			return
		}
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "soap_getReplacementsOfUser.xml"))
		if err != nil {
			t.Fatal(err)
		}
		w.Write(b)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// list: tabela (Titular/Substituto/Início/Fim) + --json + filtro por login.
func TestReplacementListTabela(t *testing.T) {
	stub := &replacementStub{}
	proj := adminUserProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "replacement", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Titular", "Substituto", "Início", "Fim", "aandrade", "bbarros", "2025-01-15", "2050-12-31"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "replacement", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	reps, _ := data["replacements"].([]any)
	if len(reps) != 2 {
		t.Errorf("esperava 2 substituições no json, veio %d", len(reps))
	}

	// filtro --user resolve login → userCode antes de consultar.
	code, _ = runMain(t, "replacement", "list", "--user", "aandrade", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--user exit=%d", code)
	}
	if got := stub.listQuery.Get("userCode"); got != "aandrade-code" {
		t.Errorf("--user não resolveu para userCode: userCode=%q", got)
	}

	// login inexistente no filtro → exit 4 (não filtro ignorado em silêncio).
	code, _ = runMain(t, "replacement", "list", "--user", "fantasma", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("--user inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// show: usa o SOAP (com flags Workflow/GED); vazio → mensagem informativa.
func TestReplacementShow(t *testing.T) {
	stub := &replacementStub{}
	proj := adminUserProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "replacement", "show", "aandrade", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Substituto", "Workflow", "GED", "sim", "não"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}

	stubEmpty := &replacementStub{soapEmpty: true}
	projEmpty := adminUserProject(t, stubEmpty.server(t).URL)
	code, stdout = runMain(t, "replacement", "show", "aandrade", "--project", projEmpty, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("show vazio exit=%d", code)
	}
	if !strings.Contains(stdout, "não tem substituições") {
		t.Errorf("show vazio sem mensagem:\n%s", stdout)
	}
}
