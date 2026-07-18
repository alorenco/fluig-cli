package fluig

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// logStub simula o fluigcliHelper com (ou sem) as rotas de log.
type logStub struct {
	version        string // "" = helper antigo, sem GET /api/version
	hasLogAPI      bool
	tailFixture    string // resposta do tail ("" = helper_log_tail.json)
	tailQuery      url.Values
	readQuery      url.Values
	acceptDownload string
}

func (s *logStub) server(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/fluigcliHelper/api/logs", func(w http.ResponseWriter, r *http.Request) {
		if !s.hasLogAPI {
			http.NotFound(w, r)
			return
		}
		w.Write(testdata(t, "helper_logs.json"))
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/", func(w http.ResponseWriter, r *http.Request) {
		if !s.hasLogAPI {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Path {
		case "/fluigcliHelper/api/logs/server.log/tail":
			s.tailQuery = r.URL.Query()
			fixture := s.tailFixture
			if fixture == "" {
				fixture = "helper_log_tail.json"
			}
			w.Write(testdata(t, fixture))
		case "/fluigcliHelper/api/logs/server.log/read":
			s.readQuery = r.URL.Query()
			io.WriteString(w, `{"file":"server.log","from":100,"to":142,"size":142,"content":"2026-07-18 09:00:04,000 INFO  [c] (t) x\n"}`)
		case "/fluigcliHelper/api/logs/server.log/download":
			s.acceptDownload = r.Header.Get("Accept")
			io.WriteString(w, "conteudo completo do log\n")
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListServerLogs(t *testing.T) {
	stub := &logStub{version: "0.3.0", hasLogAPI: true}
	c := helperClient(t, stub.server(t).URL)
	logs, err := c.ListServerLogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 3 {
		t.Fatalf("quer 3 arquivos, veio %d", len(logs))
	}
	if logs[0].Name != "server.log" || logs[0].Size != 2123021 {
		t.Errorf("primeiro arquivo inesperado: %+v", logs[0])
	}
	if logs[0].LastModified == nil || logs[0].LastModified.IsZero() {
		t.Errorf("lastModified não parseado: %+v", logs[0])
	}
}

// Helper 0.2.0 (ping ok, sem rotas de log): a listagem orienta a atualizar.
func TestListServerLogsHelperAntigo(t *testing.T) {
	stub := &logStub{version: "0.2.0", hasLogAPI: false}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.ListServerLogs(context.Background())
	if !errors.Is(err, ErrHelperOutdated) {
		t.Errorf("quer ErrHelperOutdated, veio %v", err)
	}
}

func TestTailServerLog(t *testing.T) {
	stub := &logStub{version: "0.3.0", hasLogAPI: true}
	c := helperClient(t, stub.server(t).URL)
	tail, err := c.TailServerLog(context.Background(), ServerLogTailOptions{
		Lines: 50, Skip: 10, Level: "warn", Grep: "widget",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stub.tailQuery; got.Get("lines") != "50" || got.Get("skip") != "10" ||
		got.Get("level") != "warn" || got.Get("grep") != "widget" {
		t.Errorf("query enviada: %v", got)
	}
	if tail.File != "server.log" || len(tail.Entries) != 3 || tail.Truncated {
		t.Errorf("tail inesperado: file=%q entries=%d truncated=%v", tail.File, len(tail.Entries), tail.Truncated)
	}
	if !strings.Contains(tail.Entries[1], "Redeployed") {
		t.Errorf("entrada inesperada: %q", tail.Entries[1])
	}
}

// Entrada multi-linha (fixture real: log de SQL de dataset) vem inteira,
// com as continuações na mesma entrada.
func TestTailServerLogMultilinha(t *testing.T) {
	stub := &logStub{version: "0.3.0", hasLogAPI: true, tailFixture: "helper_log_tail_multiline.json"}
	c := helperClient(t, stub.server(t).URL)
	tail, err := c.TailServerLog(context.Background(), ServerLogTailOptions{Lines: 1, Grep: "Dataset query:"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Entries) != 1 || !strings.Contains(tail.Entries[0], "\n") ||
		!strings.Contains(tail.Entries[0], "USER_TYPE") {
		t.Errorf("entrada multi-linha inesperada: %+v", tail.Entries)
	}
}

// 404 com helper atual = arquivo inexistente (exit 4); com helper antigo,
// a mesma resposta vira orientação de atualizar (exit 7).
func TestTailServerLog404(t *testing.T) {
	atual := &logStub{version: "0.3.0", hasLogAPI: true}
	c := helperClient(t, atual.server(t).URL)
	_, err := c.TailServerLog(context.Background(), ServerLogTailOptions{File: "nao-existe.log"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("quer ErrNotFound, veio %v", err)
	}

	antigo := &logStub{version: "0.2.0", hasLogAPI: false}
	c2 := helperClient(t, antigo.server(t).URL)
	_, err = c2.TailServerLog(context.Background(), ServerLogTailOptions{})
	if !errors.Is(err, ErrHelperOutdated) {
		t.Errorf("quer ErrHelperOutdated, veio %v", err)
	}
}

func TestReadServerLog(t *testing.T) {
	stub := &logStub{version: "0.3.0", hasLogAPI: true}
	c := helperClient(t, stub.server(t).URL)
	chunk, err := c.ReadServerLog(context.Background(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if stub.readQuery.Get("from") != "100" {
		t.Errorf("from enviado: %v", stub.readQuery)
	}
	if chunk.To != 142 || chunk.Size != 142 || !strings.HasSuffix(chunk.Content, "\n") {
		t.Errorf("chunk inesperado: %+v", chunk)
	}
}

func TestDownloadServerLog(t *testing.T) {
	stub := &logStub{version: "0.3.0", hasLogAPI: true}
	c := helperClient(t, stub.server(t).URL)
	var buf bytes.Buffer
	n, err := c.DownloadServerLog(context.Background(), "server.log", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(buf.Len()) || !strings.Contains(buf.String(), "conteudo completo") {
		t.Errorf("download inesperado: n=%d corpo=%q", n, buf.String())
	}
	// ⚠️ Accept: application/json responde 406 no RESTEasy (pegadinha conhecida).
	if stub.acceptDownload != "*/*" {
		t.Errorf("Accept do download = %q, quer */*", stub.acceptDownload)
	}
}

func TestHelperHasLogAPI(t *testing.T) {
	cases := map[string]bool{
		"":       false,
		"0.2.0":  false,
		"0.3.0":  true,
		"0.10.1": true,
		"1.0.0":  true,
		"x.y":    false,
	}
	for version, want := range cases {
		if got := helperHasLogAPI(version); got != want {
			t.Errorf("helperHasLogAPI(%q) = %v, quer %v", version, got, want)
		}
	}
}
