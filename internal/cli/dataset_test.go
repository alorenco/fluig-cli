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
