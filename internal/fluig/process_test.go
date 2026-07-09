package fluig

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// processStub simula login/ping + a listagem paginada de processos (REST v2).
type processStub struct {
	pages [][]byte // resposta de cada página, na ordem
	seen  []string // query strings recebidas
	fail  int      // se >0, responde esse status HTTP
}

func (s *processStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		if s.fail > 0 {
			http.Error(w, `{"message":"erro"}`, s.fail)
			return
		}
		s.seen = append(s.seen, r.URL.RawQuery)
		page := len(s.seen) - 1
		if page >= len(s.pages) {
			t.Errorf("página %d pedida além das %d disponíveis", page+1, len(s.pages))
			io.WriteString(w, `{"items":[],"hasNext":false}`)
			return
		}
		w.Write(s.pages[page])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func processClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-proc-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A listagem percorre as páginas até hasNext=false e agrega os itens.
func TestListProcessesPaginado(t *testing.T) {
	stub := &processStub{pages: [][]byte{
		testdata(t, "rest_processes_page1.json"),
		testdata(t, "rest_processes_page2.json"),
	}}
	c := processClient(t, stub.server(t).URL)
	procs, err := c.ListProcesses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 4 {
		t.Fatalf("esperava 4 processos (3+1), veio %d", len(procs))
	}
	if len(stub.seen) != 2 {
		t.Fatalf("esperava 2 requisições, houve %d: %v", len(stub.seen), stub.seen)
	}
	if stub.seen[0] != "page=1&pageSize=100" || stub.seen[1] != "page=2&pageSize=100" {
		t.Errorf("query strings inesperadas: %v", stub.seen)
	}
	first := procs[0]
	if first.ID != "Compras" || first.Description != "Compras" || first.Category != "Suprimentos" || !first.Active {
		t.Errorf("processo[0] inesperado: %+v", first)
	}
	if !procs[2].Public {
		t.Errorf("processo[2] deveria ser público: %+v", procs[2])
	}
	// Item real sem a chave categoryId (processo ad-hoc) → Category vazia.
	last := procs[3]
	if last.ID != "zz_fluigcli_test_proc" || last.Category != "" {
		t.Errorf("processo[3] inesperado: %+v", last)
	}
}

// Erro HTTP do servidor vira HTTPError (não "não encontrado").
func TestListProcessesErroHTTP(t *testing.T) {
	stub := &processStub{fail: http.StatusForbidden}
	c := processClient(t, stub.server(t).URL)
	_, err := c.ListProcesses(context.Background())
	if err == nil {
		t.Fatal("esperava erro")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusForbidden {
		t.Errorf("erro inesperado: %v", err)
	}
}
