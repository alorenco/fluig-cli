package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
