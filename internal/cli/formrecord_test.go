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

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// formRecordStub simula o CRUD de cards do ecm-forms (fixture do E2E real).
type formRecordStub struct {
	listQuery  url.Values
	createBody string
	updateBody string
	deleted    []string
}

func (s *formRecordStub) server(t *testing.T) *httptest.Server {
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cardFind := `{"cardId":1111282,"version":2000,"companyId":1,"parentDocumentId":1111281,"activeVersion":true,` +
		`"values":[{"fieldId":"nome","value":"Registro de Teste"},{"fieldId":"quantidade","value":"99"}],` +
		`"children":[{"values":[{"fieldId":"item","value":"linha 1"}]}]}`
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm-forms/api/v2/cardindex/1111281/cards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			s.createBody = string(b)
			io.WriteString(w, cardFind)
			return
		}
		s.listQuery = r.URL.Query()
		w.Write(readTD("rest_form_cards.json"))
	})
	mux.HandleFunc("/ecm-forms/api/v2/cardindex/1111281/cards/", func(w http.ResponseWriter, r *http.Request) {
		card := strings.TrimPrefix(r.URL.Path, "/ecm-forms/api/v2/cardindex/1111281/cards/")
		if card != "1111282" {
			http.Error(w, `{"code":"NotFound","message":"registro não encontrado"}`, http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			s.updateBody = string(b)
			io.WriteString(w, cardFind)
		case http.MethodDelete:
			s.deleted = append(s.deleted, card)
			w.WriteHeader(http.StatusNoContent)
		default:
			io.WriteString(w, cardFind)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func formRecordProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "fr-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

// list: tabela com colunas escolhidas e $filter repassado cru.
func TestFormRecordsList(t *testing.T) {
	stub := &formRecordStub{}
	proj := formRecordProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "records", "list", "1111281",
		"--fields", "nome,quantidade", "--filter", "quantidade eq '99'",
		"--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Card", "Versão", "nome", "quantidade", "1111282", "Registro de Teste", "99"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if stub.listQuery.Get("$filter") != "quantidade eq '99'" {
		t.Errorf("$filter não repassado: %v", stub.listQuery)
	}

	code, stdout = runMain(t, "form", "records", "list", "1111281", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	records, _ := data["records"].([]any)
	first, _ := records[0].(map[string]any)
	values, _ := first["values"].(map[string]any)
	if len(records) != 1 || values["nome"] != "Registro de Teste" || values["anonymization_date"] != "" {
		t.Errorf("records inesperado: %+v", records)
	}
}

// show: campos ordenados + linhas filhas.
func TestFormRecordsShow(t *testing.T) {
	stub := &formRecordStub{}
	proj := formRecordProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "records", "show", "1111281", "1111282", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Registro 1111282 do formulário 1111281 (versão 2000)",
		"nome = Registro de Teste", "quantidade = 99", "filho 1:", "item = linha 1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, _ = runMain(t, "form", "records", "show", "1111281", "999", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("registro inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// create/update: corpo {values:[{fieldId,value}]} ordenado; sem valores = exit 2.
func TestFormRecordsCreateUpdate(t *testing.T) {
	stub := &formRecordStub{}
	proj := formRecordProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "records", "create", "1111281",
		"--field", "quantidade=42", "--field", "nome=Registro de Teste",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("create exit=%d stdout=%s", code, stdout)
	}
	// ordem estável (alfabética) no corpo
	want := `{"values":[{"fieldId":"nome","value":"Registro de Teste"},{"fieldId":"quantidade","value":"42"}]}`
	if stub.createBody != want {
		t.Errorf("corpo do create:\n got %s\nwant %s", stub.createBody, want)
	}

	code, _ = runMain(t, "form", "records", "update", "1111281", "1111282",
		"--field", "quantidade=99", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("update exit=%d", code)
	}
	if !strings.Contains(stub.updateBody, `"fieldId":"quantidade","value":"99"`) {
		t.Errorf("corpo do update inesperado: %s", stub.updateBody)
	}

	code, _ = runMain(t, "form", "records", "create", "1111281", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("create sem valores: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// delete: confirmação obrigatória; exclui em lote.
func TestFormRecordsDelete(t *testing.T) {
	stub := &formRecordStub{}
	proj := formRecordProject(t, stub.server(t).URL)
	code, _ := runMain(t, "form", "records", "delete", "1111281", "1111282", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --yes: exit=%d, quer %d", code, output.ExitUsage)
	}
	code, _ = runMain(t, "form", "records", "delete", "1111281", "1111282", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("delete exit=%d", code)
	}
	if len(stub.deleted) != 1 || stub.deleted[0] != "1111282" {
		t.Errorf("deletes inesperados: %v", stub.deleted)
	}
}
