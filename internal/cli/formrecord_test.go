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
	showQuery  url.Values
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
	// Card com tabelas filhas (fixture da produção): como o servidor real, só
	// devolve as linhas filhas quando vem expand=children.
	mux.HandleFunc("/ecm-forms/api/v2/cardindex/8000001/cards/900001", func(w http.ResponseWriter, r *http.Request) {
		s.showQuery = r.URL.Query()
		body := readTD("rest_form_card_children.json")
		if r.URL.Query().Get("expand") != "children" {
			var card map[string]any
			if err := json.Unmarshal(body, &card); err != nil {
				t.Fatal(err)
			}
			card["children"] = []any{}
			body, _ = json.Marshal(card)
		}
		w.Write(body)
	})
	// Card sem tabela filha: o servidor responde children vazio mesmo com o expand.
	mux.HandleFunc("/ecm-forms/api/v2/cardindex/8000001/cards/900002", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"cardId":900002,"version":1000,"parentDocumentId":8000001,"activeVersion":true,`+
			`"values":[{"fieldId":"nome","value":"Sem filhas"}],"children":[]}`)
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

// show: campos ordenados + linha filha sem metadados (o card do stub antigo não
// tem tableId/rowId — caso degradado que precisa continuar legível).
func TestFormRecordsShow(t *testing.T) {
	stub := &formRecordStub{}
	proj := formRecordProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "records", "show", "1111281", "1111282", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Registro 1111282 do formulário 1111281 (versão 2000)",
		"nome = Registro de Teste", "quantidade = 99", "(tabela sem nome)", "linha ?:", "item = linha 1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, _ = runMain(t, "form", "records", "show", "1111281", "999", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("registro inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// show com tabelas filhas (ROADMAP §2.10-A): expand=children por padrão,
// agrupamento por tabela, sufixo ___<rowId> removido e campos de controle fora
// do modo humano — mas presentes no --json.
func TestFormRecordsShowChildren(t *testing.T) {
	stub := &formRecordStub{}
	proj := formRecordProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "form", "records", "show", "8000001", "900001", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.showQuery.Get("expand") != "children" {
		t.Errorf("expand=children não enviado por padrão: %v", stub.showQuery)
	}
	for _, want := range []string{
		"Tabela filha", "tabelaItens", "tabelaTributos", // resumo em tabela
		"linha 1:", "itemProdutoServico = Cadeira de escritório", // sufixo ___1 removido
		"linha 88:", "trbCodTrb = ICMS",
		"parcelaVencimento = 10/08/2026", // campo de outra tabela na mesma linha
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
	// No trecho das filhas, campos de controle e nomes ainda sufixados não
	// aparecem (os campos do PAI seguem impressos como antes, inclusive os
	// anonymization_* que o servidor acrescenta).
	childBlock := stdout[strings.Index(stdout, "Tabela filha"):]
	for _, unwanted := range []string{"masterid =", "anonymization_date =", "companyid =", "tableid =", "itemProdutoServico___1"} {
		if strings.Contains(childBlock, unwanted) {
			t.Errorf("linhas filhas com %q:\n%s", unwanted, childBlock)
		}
	}

	// --json: linhas com tableId/rowId e TODOS os campos, inclusive os de controle.
	code, stdout = runMain(t, "form", "records", "show", "8000001", "900001", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	record, _ := data["record"].(map[string]any)
	children, _ := record["children"].([]any)
	if len(children) != 4 {
		t.Fatalf("children no --json: %d, quer 4", len(children))
	}
	first, _ := children[0].(map[string]any)
	values, _ := first["values"].(map[string]any)
	if first["tableId"] != "tabelaItens" || first["rowId"] != "1" {
		t.Errorf("metadados da linha: %+v", first)
	}
	if values["itemProdutoServico"] != "Cadeira de escritório" {
		t.Errorf("campo sem sufixo no --json: %+v", values)
	}
	if values["masterid"] != "900001" {
		t.Errorf("--json deveria manter os campos de controle: %+v", values)
	}

	// --no-children: sem o expand, e a saída diz que não consultou.
	code, stdout = runMain(t, "form", "records", "show", "8000001", "900001", "--no-children", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("--no-children exit=%d stdout=%s", code, stdout)
	}
	if stub.showQuery.Get("expand") != "" {
		t.Errorf("--no-children não deveria enviar expand: %v", stub.showQuery)
	}
	if !strings.Contains(stdout, "não consultadas") || strings.Contains(stdout, "tabelaItens") {
		t.Errorf("--no-children: saída inesperada:\n%s", stdout)
	}

	// Registro sem tabela filha: a mensagem diz que não há linhas, e não que
	// deixamos de consultar.
	code, stdout = runMain(t, "form", "records", "show", "8000001", "900002", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("card sem filhas: exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "não tem linhas de tabela filha") {
		t.Errorf("card sem filhas: saída inesperada:\n%s", stdout)
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
