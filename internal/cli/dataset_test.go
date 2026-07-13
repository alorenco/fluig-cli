package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

type hostPort struct {
	host string
	port int
}

func mustParseHostPort(t *testing.T, raw string) hostPort {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return hostPort{host: u.Hostname(), port: port}
}

// fluigDatasetStub simula login/ping + endpoints de dataset para os testes da CLI.
type fluigDatasetStub struct {
	editedImpl string
	created    bool

	toggled      []string // POSTs em enable/disable: "enable/<id>"...
	restoreQuery url.Values
	hasDraft     bool
}

func (s *fluigDatasetStub) server(t *testing.T) *httptest.Server {
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/webdesk/ECMDatasetService", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("SOAPAction") {
		case "findAllFormulariesDatasets":
			w.Write(readTD("soap_findAllDatasets.xml"))
		default:
			http.Error(w, "op?", 500)
		}
	})
	// REST v2: listagem paginada + consulta de valores.
	restCalls := 0
	mux.HandleFunc("/dataset/api/v2/datasets", func(w http.ResponseWriter, r *http.Request) {
		restCalls++
		if restCalls == 1 {
			w.Write(readTD("rest_datasets_page1.json"))
			return
		}
		w.Write(readTD("rest_datasets_page2.json"))
	})
	// REST v2: enable/disable, histórico e restore.
	toggle := func(op string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id := strings.TrimPrefix(r.URL.Path, "/dataset/api/v2/datasets/"+op+"/")
			if id == "nao_existe" {
				http.Error(w, `{"code":"DatasetNotFoundException","message":"dataset não encontrado"}`, http.StatusNotFound)
				return
			}
			s.toggled = append(s.toggled, op+"/"+id)
		}
	}
	mux.HandleFunc("/dataset/api/v2/datasets/enable/", toggle("enable"))
	mux.HandleFunc("/dataset/api/v2/datasets/disable/", toggle("disable"))
	mux.HandleFunc("/dataset/api/v2/dataset-history", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") == "zz_fluigcli_test_hist" {
			if s.restoreQuery == nil {
				w.Write(readTD("rest_dataset_history.json"))
				return
			}
			// Após um restore o histórico ganha a versão nova (o POST responde
			// corpo vazio — comportamento real; a CLI descobre a versão aqui).
			var hist struct {
				Items   []map[string]any `json:"items"`
				HasNext bool             `json:"hasNext"`
			}
			json.Unmarshal(readTD("rest_dataset_history.json"), &hist)
			hist.Items = append(hist.Items, map[string]any{
				"id": 11971, "tenantId": 1, "userTenantId": 7, "userName": "Ana Andrade",
				"datasetId": "zz_fluigcli_test_hist", "datasetDescription": "fluigcli teste history",
				"datasetImpl": "function createDataset(){}", "version": 3,
				"status": "PUBLISHED", "updateTime": 1783975000000,
			})
			b, _ := json.Marshal(hist)
			w.Write(b)
			return
		}
		io.WriteString(w, `{"items":[],"hasNext":false}`)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-history/restore/validation", func(w http.ResponseWriter, r *http.Request) {
		// Formato real da homolog (2026-07-13).
		io.WriteString(w, `{"datasetId":"`+r.URL.Query().Get("datasetId")+`","datasetDescription":"fluigcli teste history","draft":`+strconv.FormatBool(s.hasDraft)+`}`)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-history/restore", func(w http.ResponseWriter, r *http.Request) {
		s.restoreQuery = r.URL.Query()
		// Corpo VAZIO, como a homolog real (o swagger promete DatasetHistory,
		// mas não vem nada — validado 2026-07-13).
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") == "nao_existe" {
			io.WriteString(w, `{"columns":null,"values":null}`)
			return
		}
		w.Write(readTD("rest_dataset_handle.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/loadDataset", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") == "ds_exemplo" {
			w.Write(readTD("loadDataset.json"))
			return
		}
		// Dataset inexistente: o Fluig responde 500 (não 404) — quirk real.
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/createDataset", func(w http.ResponseWriter, r *http.Request) {
		s.created = true
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/editDataset", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			DatasetImpl string `json:"datasetImpl"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &p)
		s.editedImpl = p.DatasetImpl
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// datasetProject cria um projeto temporário com um servidor apontando para o
// stub, e configura a senha via env var (modo não-interativo).
func datasetProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{
		ID: "ds-srv", Name: "homolog", Host: u.host, Port: u.port,
		SSL: false, Username: "u", CompanyID: 1,
	}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestDatasetListJSON(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "dataset", "list", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, stdout)
	}
	if !env.OK {
		t.Errorf("envelope não ok: %+v", env)
	}
}

func TestDatasetListCustomOnly(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "list", "--custom-only", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	list, _ := data["datasets"].([]any)
	if len(list) != 2 {
		t.Errorf("--custom-only deveria retornar 2 datasets, veio %d", len(list))
	}
}

// --search filtra por id e descrição (case-insensitive).
func TestDatasetListSearch(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "list", "--search", "cadastro", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	list, _ := data["datasets"].([]any)
	if len(list) != 1 {
		t.Fatalf("--search cadastro deveria retornar 1 dataset, veio %d\n%s", len(list), stdout)
	}
	first, _ := list[0].(map[string]any)
	if first["id"] != "frm_cadastro" || first["description"] != "Formulário de Cadastro" {
		t.Errorf("dataset inesperado: %+v", first)
	}
}

func TestDatasetImportWritesFile(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)

	code, _ := runMain(t, "dataset", "import", "ds_exemplo", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	path := filepath.Join(proj, "datasets", "ds_exemplo.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("arquivo não criado: %v", err)
	}
	if len(data) == 0 {
		t.Error("arquivo vazio")
	}
}

func TestDatasetExportUpdatesExisting(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)

	dir := filepath.Join(proj, "datasets")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "ds_exemplo.js")
	os.WriteFile(file, []byte("function createDataset(){ return 1; }"), 0o644)

	code, _ := runMain(t, "dataset", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if stub.editedImpl != "function createDataset(){ return 1; }" {
		t.Errorf("editDataset não recebeu o conteúdo local: %q", stub.editedImpl)
	}
}

// Criar dataset novo em modo não-interativo exige --new.
func TestDatasetExportNewRequiresFlag(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	dir := filepath.Join(proj, "datasets")
	os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "ds_novo.js")
	os.WriteFile(file, []byte("x"), 0o644)

	// Sem --new: recusa (exit 2, alvo único).
	code, _ := runMain(t, "dataset", "export", file, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --new deveria dar exit 2, veio %d", code)
	}
	if stub.created {
		t.Error("não deveria ter criado sem --new")
	}

	// Com --new: cria.
	code, _ = runMain(t, "dataset", "export", file, "--new", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Errorf("com --new deveria criar (exit 0), veio %d", code)
	}
	if !stub.created {
		t.Error("createDataset não foi chamado com --new")
	}
}

// Lote com um item falhando → exit 6 (PARTIAL_FAILURE).
func TestDatasetImportPartialFailure(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "dataset", "import", "ds_exemplo", "nao_existe",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("esperava exit 6, veio %d\n%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.OK || env.Error == nil || env.Error.Code != output.CodePartial {
		t.Errorf("envelope de falha parcial inesperado: %+v", env)
	}
	// Mesmo com falha parcial, os resultados devem estar no data.
	data, _ := env.Data.(map[string]any)
	if data["results"] == nil {
		t.Error("results ausente no envelope de falha parcial")
	}
}

func TestDatasetQueryJSON(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "query", "colleague", "--fields", "colleagueName,login",
		"--order", "colleagueName", "--limit", "3", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["count"].(float64) != 3 {
		t.Errorf("esperava count=3, veio %v", data["count"])
	}
	rows, _ := data["rows"].([]any)
	first, _ := rows[0].(map[string]any)
	if first["login"] != "ana.andrade" {
		t.Errorf("linha[0] inesperada: %+v", first)
	}
}

// Dataset inexistente na consulta → exit 4.
func TestDatasetQueryNotFound(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, _ := runMain(t, "dataset", "query", "nao_existe", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// Mais de um campo de ordenação → erro de uso (a API aceita um só).
func TestDatasetQueryOrdemUnica(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, _ := runMain(t, "dataset", "query", "colleague", "--order", "a,b", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
}

// Modo humano: a listagem sai em tabela com bordas e cabeçalho (padrão de
// listas — ver CLAUDE.md). Sem TTY não há cor, mas a grade permanece.
func TestDatasetListTabela(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "ID", "Tipo", "Descrição", "Ativo", "ds_exemplo", "CUSTOM", "Dataset de exemplo", "sim", "não"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// disable/enable: POST no endpoint certo, action no envelope e mensagem humana.
func TestDatasetDisableEnable(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "dataset", "disable", "ds_exemplo", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("disable exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	results, _ := data["results"].([]any)
	first, _ := results[0].(map[string]any)
	if len(results) != 1 || first["action"] != "disabled" || first["success"] != true {
		t.Errorf("results inesperado: %+v", results)
	}

	code, _ = runMain(t, "dataset", "enable", "ds_exemplo", "ds_inativo", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("enable exit=%d", code)
	}
	want := []string{"disable/ds_exemplo", "enable/ds_exemplo", "enable/ds_inativo"}
	if strings.Join(stub.toggled, ",") != strings.Join(want, ",") {
		t.Errorf("POSTs = %v, quer %v", stub.toggled, want)
	}
}

// disable de dataset inexistente: 404 da API → exit 4.
func TestDatasetDisableNotFound(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, _ := runMain(t, "dataset", "disable", "nao_existe", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// history: tabela com as versões (fixture real sanitizada da homolog).
func TestDatasetHistoryTabela(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "history", "zz_fluigcli_test_hist", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "Versão", "Status", "Autor", "Atualizado em", "Linhas", "PUBLISHED", "Ana Andrade", "1", "2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

// history --json: envelope com as versões (sem o código; lines em vez de impl).
func TestDatasetHistoryJSON(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "history", "zz_fluigcli_test_hist", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	versions, _ := data["versions"].([]any)
	if len(versions) != 2 {
		t.Fatalf("esperava 2 versões, veio %d", len(versions))
	}
	v2, _ := versions[1].(map[string]any)
	if v2["version"].(float64) != 2 || v2["status"] != "PUBLISHED" || v2["author"] != "Ana Andrade" {
		t.Errorf("versão 2 inesperada: %+v", v2)
	}
	if _, temImpl := v2["impl"]; temImpl {
		t.Error("a listagem não deveria expor o código (use --version N)")
	}
}

// history --version N: imprime o código JS da versão.
func TestDatasetHistoryVersionCode(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "history", "zz_fluigcli_test_hist", "--version", "1", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "d.addRow(['v1']);") {
		t.Errorf("código da v1 não impresso:\n%s", stdout)
	}

	code, _ = runMain(t, "dataset", "history", "zz_fluigcli_test_hist", "--version", "9", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("versão inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// history de dataset sem histórico (não customizado) → exit 0 com lista vazia;
// dataset inexistente → exit 4.
func TestDatasetHistoryVazioOuInexistente(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "history", "colleague", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("sem histórico: exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if versions, _ := data["versions"].([]any); len(versions) != 0 {
		t.Errorf("esperava versões vazias, veio %v", versions)
	}

	stub2 := &fluigDatasetStub{}
	proj2 := datasetProject(t, stub2.server(t).URL)
	code, _ = runMain(t, "dataset", "history", "sumiu", "--json", "--project", proj2, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
}

// restore: manda datasetId+version e devolve a nova versão criada.
func TestDatasetRestore(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "dataset", "restore", "zz_fluigcli_test_hist", "1", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.restoreQuery.Get("datasetId") != "zz_fluigcli_test_hist" || stub.restoreQuery.Get("version") != "1" {
		t.Errorf("query do restore inesperada: %v", stub.restoreQuery)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["restoredTo"].(float64) != 1 || data["version"].(float64) != 3 || data["status"] != "PUBLISHED" {
		t.Errorf("resultado inesperado: %+v", data)
	}
}

// restore sem --yes em modo não-interativo: exit 2, sem tocar no servidor.
func TestDatasetRestoreExigeConfirmacao(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, _ := runMain(t, "dataset", "restore", "zz_fluigcli_test_hist", "1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
	if stub.restoreQuery != nil {
		t.Error("o restore não deveria ter sido chamado sem confirmação")
	}

	code, _ = runMain(t, "dataset", "restore", "zz_fluigcli_test_hist", "abc", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("versão não numérica: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// restore de versão fora do histórico: exit 4 ANTES de chamar o restore (o
// servidor real responde 500 genérico — a CLI valida antes).
func TestDatasetRestoreVersaoInexistente(t *testing.T) {
	stub := &fluigDatasetStub{}
	proj := datasetProject(t, stub.server(t).URL)
	code, _ := runMain(t, "dataset", "restore", "zz_fluigcli_test_hist", "9", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("exit=%d, quer %d", code, output.ExitNotFound)
	}
	if stub.restoreQuery != nil {
		t.Error("o restore não deveria ter sido chamado com versão inexistente")
	}

	code, _ = runMain(t, "dataset", "restore", "sem_historico", "1", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("dataset sem histórico: exit=%d, quer %d", code, output.ExitNotFound)
	}
}
