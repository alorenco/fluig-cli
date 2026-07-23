package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// dbStub simula o fluigcliHelper com (ou sem) as rotas de db.
type dbStub struct {
	version    string // "" = sem GET /api/version
	hasDbAPI   bool
	lastQuery  dbQueryRequest // corpo recebido no /db/query
	queryBody  string         // resposta 200 do /db/query ("" = default)
	queryCode  int            // código do /db/query (0 = 200)
	queryError string         // corpo de erro (quando queryCode != 200)
}

func (s *dbStub) server(t *testing.T) *httptest.Server {
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
		if s.version == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"name":"fluigcliHelper","version":"`+s.version+`"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/db/query", func(w http.ResponseWriter, r *http.Request) {
		if !s.hasDbAPI {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &s.lastQuery)
		if s.queryCode != 0 && s.queryCode != http.StatusOK {
			w.WriteHeader(s.queryCode)
			io.WriteString(w, s.queryError)
			return
		}
		out := s.queryBody
		if out == "" {
			out = `{"columns":[{"name":"login","type":"nvarchar"},{"name":"obs","type":"nvarchar"}],` +
				`"rows":[["fluig",null]],"rowCount":1,"truncated":false}`
		}
		io.WriteString(w, out)
	})
	mux.HandleFunc("/fluigcliHelper/api/db/datasources", func(w http.ResponseWriter, r *http.Request) {
		if !s.hasDbAPI {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `["/jdbc/AppDS","/jdbc/TotvsRM"]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDbQuery(t *testing.T) {
	stub := &dbStub{version: "0.6.0", hasDbAPI: true}
	c := helperClient(t, stub.server(t).URL)
	res, err := c.DbQuery(context.Background(), DbQueryOptions{
		JNDI: "/jdbc/TotvsRM", SQL: "select ? as a", Params: []string{"x"}, MaxRows: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	// corpo enviado ao helper
	if stub.lastQuery.JNDI != "/jdbc/TotvsRM" || stub.lastQuery.SQL != "select ? as a" ||
		len(stub.lastQuery.Params) != 1 || stub.lastQuery.Params[0] != "x" || stub.lastQuery.MaxRows != 50 {
		t.Errorf("corpo enviado inesperado: %+v", stub.lastQuery)
	}
	if len(res.Columns) != 2 || res.Columns[0].Name != "login" || res.RowCount != 1 {
		t.Errorf("resultado inesperado: %+v", res)
	}
	// null preservado como nil; valor como *string
	if res.Rows[0][0] == nil || *res.Rows[0][0] != "fluig" {
		t.Errorf("célula 0 inesperada: %v", res.Rows[0][0])
	}
	if res.Rows[0][1] != nil {
		t.Errorf("célula null deveria virar nil, veio %q", *res.Rows[0][1])
	}
}

func TestDbQueryRecusaLeitura(t *testing.T) {
	stub := &dbStub{version: "0.6.0", hasDbAPI: true, queryCode: http.StatusBadRequest,
		queryError: "Somente consultas de leitura são permitidas (SELECT ou WITH)"}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.DbQuery(context.Background(), DbQueryOptions{SQL: "update t set x=1"})
	if !errors.Is(err, errServerRejected) {
		t.Fatalf("quer errServerRejected, veio %v", err)
	}
	if !strings.Contains(err.Error(), "Somente consultas de leitura") {
		t.Errorf("mensagem do helper não repassada: %v", err)
	}
}

func TestDbQueryDatasourceInexistente(t *testing.T) {
	stub := &dbStub{version: "0.6.0", hasDbAPI: true, queryCode: http.StatusNotFound,
		queryError: "Datasource não encontrado: /jdbc/NAOEXISTE"}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.DbQuery(context.Background(), DbQueryOptions{JNDI: "/jdbc/NAOEXISTE", SQL: "select 1"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("quer ErrNotFound, veio %v", err)
	}
	if !strings.Contains(err.Error(), "NAOEXISTE") {
		t.Errorf("mensagem do helper não repassada: %v", err)
	}
}

// Helper antigo (0.5.0, sem /db): o 404 vira ErrHelperOutdated, não ErrNotFound.
func TestDbQueryHelperAntigo(t *testing.T) {
	stub := &dbStub{version: "0.5.0", hasDbAPI: false}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.DbQuery(context.Background(), DbQueryOptions{SQL: "select 1"})
	if !errors.Is(err, ErrHelperOutdated) {
		t.Errorf("quer ErrHelperOutdated, veio %v", err)
	}
}

func TestListDatasources(t *testing.T) {
	stub := &dbStub{version: "0.6.0", hasDbAPI: true}
	c := helperClient(t, stub.server(t).URL)
	ds, err := c.ListDatasources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 || ds[0] != "/jdbc/AppDS" || ds[1] != "/jdbc/TotvsRM" {
		t.Errorf("datasources inesperados: %v", ds)
	}
}
