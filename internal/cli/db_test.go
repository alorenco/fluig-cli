package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// dbServerStub simula o Fluig com o fluigcliHelper 0.6.0 (rotas de db).
func dbServerStub(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.6.0"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/db/query", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(strings.ToLower(string(body)), "update") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "Somente consultas de leitura são permitidas (SELECT ou WITH)")
			return
		}
		io.WriteString(w, `{"columns":[{"name":"login","type":"nvarchar"},{"name":"obs","type":"nvarchar"}],`+
			`"rows":[["fluig",null]],"rowCount":1,"truncated":false}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/db/datasources", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `["/jdbc/AppDS","/jdbc/TotvsRM"]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDbQueryTabela(t *testing.T) {
	stub := dbServerStub(t)
	proj := serverTestProject(t, stub.URL)

	// Modo humano: cabeçalhos, valor e NULL renderizado como (null).
	code, stdout := runMain(t, "db", "query", "select suser_sname() as login, x as obs", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"login", "obs", "fluig", "(null)"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	// --json: envelope com columns/rows/rowCount; null preservado.
	code, stdout = runMain(t, "db", "query", "select 1", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["rowCount"].(float64) != 1 {
		t.Errorf("rowCount inesperado: %v", data["rowCount"])
	}
	rows, _ := data["rows"].([]any)
	first, _ := rows[0].([]any)
	if first[0] != "fluig" || first[1] != nil {
		t.Errorf("linha inesperada (null deve virar nil no json): %v", first)
	}
}

// Recusa de escrita: o 400 do helper vira exit 5 (server) com a mensagem do
// helper no envelope (--json coloca o erro no stdout).
func TestDbQueryRecusaEscrita(t *testing.T) {
	stub := dbServerStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "query", "update t set x=1", "--json", "--project", proj)
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if env.OK || env.Error == nil || !strings.Contains(env.Error.Message, "leitura") {
		t.Errorf("erro inesperado no envelope: %+v", env.Error)
	}
}

// dbGrantsStub simula o helper respondendo ao SELECT do preflight: extrai as
// permissões dos alias `AS [PERM]` do SQL e as tabelas dos params, e devolve a
// matriz. Convenção das tabelas de teste: dbo.OK concede tudo, dbo.PARTIAL só
// SELECT, dbo.GHOST não existe (tudo NULL).
func dbGrantsStub(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.6.0"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/db/query", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SQL    string   `json:"sql"`
			Params []string `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		var perms []string
		for _, p := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
			if strings.Contains(req.SQL, "AS ["+p+"]") {
				perms = append(perms, p)
			}
		}
		cols := []string{`{"name":"login","type":"nvarchar"}`, `{"name":"db","type":"nvarchar"}`,
			`{"name":"tabela","type":"nvarchar"}`, `{"name":"__oid","type":"int"}`}
		for _, p := range perms {
			cols = append(cols, `{"name":"`+p+`","type":"int"}`)
		}
		var rows []string
		for _, tbl := range req.Params {
			// __oid: NULL para dbo.GHOST (objeto inexistente), id fictício senão.
			oid := `"1234"`
			if tbl == "dbo.GHOST" {
				oid = "null"
			}
			cells := []string{`"fluig"`, `"FLUIG"`, `"` + tbl + `"`, oid}
			for _, p := range perms {
				switch {
				case tbl == "dbo.GHOST":
					cells = append(cells, "null")
				case tbl == "dbo.OK":
					cells = append(cells, `"1"`)
				case tbl == "dbo.PARTIAL" && p == "SELECT":
					cells = append(cells, `"1"`)
				default:
					cells = append(cells, `"0"`)
				}
			}
			rows = append(rows, "["+strings.Join(cells, ",")+"]")
		}
		io.WriteString(w, `{"columns":[`+strings.Join(cols, ",")+`],"rows":[`+strings.Join(rows, ",")+
			`],"rowCount":`+itoa(len(rows))+`,"truncated":false}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func itoa(n int) string { return strconv.Itoa(n) }

// db grants: tudo concedido → exit 0, tabela com ✓ e cabeçalho login/banco.
func TestDbGrantsOK(t *testing.T) {
	stub := dbGrantsStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "grants", "dbo.OK", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Login do datasource", "fluig", "FLUIG", "dbo.OK", "SELECT", "DELETE", "✓"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
}

// db grants: grant faltando → exit 6, ✗ na tabela, envelope PARTIAL com missing.
func TestDbGrantsFaltando(t *testing.T) {
	stub := dbGrantsStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "grants", "dbo.OK", "dbo.PARTIAL", "--project", proj)
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitPartial, stdout)
	}
	if !strings.Contains(stdout, "✗") {
		t.Errorf("esperava marcador ✗ para grant faltando:\n%s", stdout)
	}

	// --json: envelope ok=false, data com tables[].grants e missing.
	code, stdout = runMain(t, "db", "grants", "dbo.PARTIAL", "--json", "--project", proj)
	if code != output.ExitPartial {
		t.Fatalf("--json exit=%d, quer %d; stdout=%s", code, output.ExitPartial, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if env.OK {
		t.Errorf("envelope deveria ser ok=false: %+v", env)
	}
	data, _ := env.Data.(map[string]any)
	if data["ok"] != false {
		t.Errorf("data.ok deveria ser false: %v", data["ok"])
	}
	tables, _ := data["tables"].([]any)
	first, _ := tables[0].(map[string]any)
	missing, _ := first["missing"].([]any)
	if len(missing) == 0 {
		t.Errorf("esperava missing não vazio para dbo.PARTIAL: %v", first)
	}
	grants, _ := first["grants"].(map[string]any)
	if grants["SELECT"] != true {
		t.Errorf("SELECT deveria ser true (concedido): %v", grants)
	}
	if grants["INSERT"] != false {
		t.Errorf("INSERT deveria ser false (negado): %v", grants)
	}
}

// db grants: objeto inexistente → exit 6, ? na tabela, exists=false, grant null.
func TestDbGrantsObjetoInexistente(t *testing.T) {
	stub := dbGrantsStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "grants", "dbo.GHOST", "--project", proj)
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitPartial, stdout)
	}
	if !strings.Contains(stdout, "?") {
		t.Errorf("esperava marcador ? para objeto inexistente:\n%s", stdout)
	}

	code, stdout = runMain(t, "db", "grants", "dbo.GHOST", "--json", "--project", proj)
	if code != output.ExitPartial {
		t.Fatalf("--json exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	_ = json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	tables, _ := data["tables"].([]any)
	first, _ := tables[0].(map[string]any)
	if first["exists"] != false {
		t.Errorf("exists deveria ser false para dbo.GHOST: %v", first)
	}
	grants, _ := first["grants"].(map[string]any)
	if grants["SELECT"] != nil {
		t.Errorf("SELECT deveria ser null (indeterminado): %v", grants)
	}
}

// db grants: --perm restringe o conjunto e valida o valor.
func TestDbGrantsPermSubset(t *testing.T) {
	stub := dbGrantsStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "grants", "dbo.OK", "--perm", "INSERT,UPDATE", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "INSERT") || !strings.Contains(stdout, "UPDATE") {
		t.Errorf("esperava colunas INSERT e UPDATE:\n%s", stdout)
	}
	if strings.Contains(stdout, "DELETE") {
		t.Errorf("não esperava a coluna DELETE com --perm INSERT,UPDATE:\n%s", stdout)
	}

	// Permissão inválida → exit 2 (uso).
	code, stdout = runMain(t, "db", "grants", "dbo.OK", "--perm", "DROP", "--project", proj)
	if code != output.ExitUsage {
		t.Fatalf("permissão inválida deveria dar exit %d, deu %d; stdout=%s", output.ExitUsage, code, stdout)
	}
}

func TestDbDatasourcesTabela(t *testing.T) {
	stub := dbServerStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "datasources", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Datasource", "/jdbc/AppDS", "/jdbc/TotvsRM"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
}
