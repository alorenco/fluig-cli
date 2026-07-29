package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// --- db query --file (ROADMAP2 §3.7) ---

// dbFileStub registra os SQLs recebidos, uma requisição por instrução, e falha a
// instrução que contiver "quebra".
func dbFileStub(t *testing.T, seen *[]string) *httptest.Server {
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
			SQL string `json:"sql"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		*seen = append(*seen, req.SQL)
		if strings.Contains(req.SQL, "quebra") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "Invalid object name 'quebra'")
			return
		}
		io.WriteString(w, `{"columns":[{"name":"n","type":"int"}],"rows":[["1"]],"rowCount":1,"truncated":false}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// scriptSQL grava um script no projeto e devolve o caminho.
func scriptSQL(t *testing.T, proj, name, content string) string {
	t.Helper()
	path := filepath.Join(proj, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// O script vai instrução por instrução, na ordem, uma requisição para cada. O
// `;` dentro de literal e o comentário não podem separar nada (era a gambiarra
// em Python que o usuário teve de escrever).
func TestDbQueryFileExecutaEmSequencia(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", `-- diagnóstico
select 1 as n;
select 'a;b' as texto; -- ; aqui não separa
GO
select 3 as n
`)

	code, stdout := runMain(t, "db", "query", "--file", script, "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if len(seen) != 3 {
		t.Fatalf("esperava 3 requisições (uma por instrução), veio %d: %v", len(seen), seen)
	}
	if seen[1] != "select 'a;b' as texto" {
		t.Errorf("a segunda instrução foi cortada no literal: %q", seen[1])
	}

	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	stmts, _ := data["statements"].([]any)
	if len(stmts) != 3 {
		t.Fatalf("esperava 3 statements no envelope, veio %v", data["statements"])
	}
	first, _ := stmts[0].(map[string]any)
	if first["index"].(float64) != 1 || first["success"] != true {
		t.Errorf("primeiro statement inesperado: %v", first)
	}
	if first["line"].(float64) != 2 {
		t.Errorf("linha do primeiro statement = %v, quer 2", first["line"])
	}
}

// Uma instrução falha e as outras seguem: exit 6 com o detalhe por item.
func TestDbQueryFileFalhaParcial(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", "select 1 as n;\nselect * from quebra;\nselect 3 as n;\n")

	code, stdout := runMain(t, "db", "query", "--file", script, "--json", "--project", proj)
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitPartial, stdout)
	}
	if len(seen) != 3 {
		t.Errorf("a falha interrompeu o script: %v", seen)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	stmts, _ := data["statements"].([]any)
	segundo, _ := stmts[1].(map[string]any)
	if segundo["success"] != false || segundo["error"] == nil {
		t.Errorf("o item que falhou não trouxe o motivo: %v", segundo)
	}
	terceiro, _ := stmts[2].(map[string]any)
	if terceiro["success"] != true {
		t.Errorf("a instrução seguinte à falha não rodou: %v", terceiro)
	}
}

// Script com uma instrução só é alvo único: o erro dela é o erro do comando
// (exit 5), não uma "falha parcial" de um item.
func TestDbQueryFileInstrucaoUnicaQueFalha(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", "select * from quebra;\n")

	code, stdout := runMain(t, "db", "query", "--file", script, "--json", "--project", proj)
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d (alvo único); stdout=%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "quebra") {
		t.Errorf("erro do banco não chegou ao envelope: %+v", env.Error)
	}
}

// --list não executa nada: é o que dá confiança no splitter antes de rodar.
func TestDbQueryFileList(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", "select 1 as n;\nselect 2 as n;\n")

	code, stdout := runMain(t, "db", "query", "--file", script, "--list", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if len(seen) != 0 {
		t.Errorf("--list executou SQL no servidor: %v", seen)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["executed"] != false {
		t.Errorf("executed devia ser false com --list: %v", data["executed"])
	}
	stmts, _ := data["statements"].([]any)
	if len(stmts) != 2 {
		t.Errorf("esperava 2 instruções listadas: %v", data["statements"])
	}
}

// --statement N roda só a escolhida, preservando o índice do arquivo.
func TestDbQueryFileStatement(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", "select 1 as n;\nselect 2 as n;\nselect 3 as n;\n")

	code, stdout := runMain(t, "db", "query", "--file", script, "--statement", "2", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if len(seen) != 1 || seen[0] != "select 2 as n" {
		t.Fatalf("--statement 2 executou o SQL errado: %v", seen)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	stmts, _ := data["statements"].([]any)
	item, _ := stmts[0].(map[string]any)
	if item["index"].(float64) != 2 {
		t.Errorf("o índice devia ser o do arquivo (2), veio %v", item["index"])
	}
}

// Erros de uso: nada é enviado ao servidor.
func TestDbQueryFileUsoIncorreto(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", "select 1;\nselect 2;\n")
	soComentario := scriptSQL(t, proj, "vazio.sql", "-- só comentário\n/* nada */\n")

	casos := []struct {
		nome string
		args []string
		quer string
	}{
		{"sem sql e sem file", []string{"db", "query"}, "--file"},
		{"sql e file juntos", []string{"db", "query", "select 1", "--file", script}, "não os dois"},
		{"statement sem file", []string{"db", "query", "select 1", "--statement", "2"}, "--statement só faz sentido"},
		{"list sem file", []string{"db", "query", "select 1", "--list"}, "--list só faz sentido"},
		{"statement fora do intervalo", []string{"db", "query", "--file", script, "--statement", "9"}, "não existe"},
		{"arquivo sem instrução", []string{"db", "query", "--file", soComentario}, "nenhuma instrução SQL"},
		{"param ambíguo com várias instruções", []string{"db", "query", "--file", script, "--param", "x"}, "ambíguo"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			args := append(c.args, "--json", "--project", proj)
			code, stdout := runMain(t, args...)
			if code != output.ExitUsage {
				t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
			}
			if !strings.Contains(stdout, c.quer) {
				t.Errorf("mensagem sem %q: %s", c.quer, stdout)
			}
		})
	}
	if len(seen) != 0 {
		t.Errorf("erro de uso não podia ter chamado o servidor: %v", seen)
	}
}

// Arquivo inexistente: NOT_FOUND, não erro genérico.
func TestDbQueryFileInexistente(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "db", "query", "--file", filepath.Join(proj, "nao_existe.sql"),
		"--json", "--project", proj)
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitNotFound, stdout)
	}
}

// --param com --statement é permitido: aí não há ambiguidade.
func TestDbQueryFileParamComStatement(t *testing.T) {
	var seen []string
	stub := dbFileStub(t, &seen)
	proj := serverTestProject(t, stub.URL)
	script := scriptSQL(t, proj, "diag.sql", "select 1;\nselect ? as n;\n")

	code, stdout := runMain(t, "db", "query", "--file", script, "--statement", "2",
		"--param", "x", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if len(seen) != 1 {
		t.Errorf("esperava 1 requisição: %v", seen)
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
