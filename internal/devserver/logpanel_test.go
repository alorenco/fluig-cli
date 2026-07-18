package devserver

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// logUpstream simula o Fluig com o fluigcliHelper 0.3.0: o primeiro read
// devolve uma linha nova; os seguintes, nada (arquivo parado).
func logUpstream(t *testing.T, helper bool) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	served := false
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !helper {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.3.0"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/logs", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"name":"console.log","size":10,"lastModified":1784500000000},`+
			`{"name":"server.log","size":100,"lastModified":1784551800000}]`)
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/tail", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"file":"server.log","size":100,"entries":[`+
			`"2026-07-18 09:00:01,000 INFO  [c] (t) antiga um",`+
			`"2026-07-18 09:00:02,000 ERROR [c] (t) antiga dois\n\tat com.example.Foo.bar(Foo.java:1)"`+
			`],"truncated":false}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/read", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		first := !served
		served = true
		mu.Unlock()
		if first {
			io.WriteString(w, `{"file":"server.log","from":100,"to":141,"size":141,`+
				`"content":"2026-07-18 09:00:03,000 WARN  [c] (t) nova\n"}`)
			return
		}
		io.WriteString(w, `{"file":"server.log","from":141,"to":141,"size":141,"content":""}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newLogTestServer(t *testing.T, upstream *httptest.Server, withClient bool) (*Server, *httptest.Server) {
	t.Helper()
	u, _ := url.Parse(upstream.URL)
	jar, _ := cookiejar.New(nil)
	opts := Options{Root: projRoot(t), Upstream: u, Jar: jar, Port: 0, Debounce: 10 * time.Millisecond}
	if withClient {
		client, err := fluig.NewClient(fluig.Options{BaseURL: upstream.URL, Username: "dev-" + t.Name(), Password: "p", CompanyID: 1})
		if err != nil {
			t.Fatal(err)
		}
		opts.Client = client
	}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return s, ts
}

func TestLogFilesAPI(t *testing.T) {
	_, ts := newLogTestServer(t, logUpstream(t, true), true)
	resp, err := http.Get(ts.URL + "/_dev/api/log/files")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload struct {
		Files []fluig.ServerLogFile `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Files) != 2 || payload.Files[1].Name != "server.log" {
		t.Errorf("files: %+v", payload.Files)
	}
}

func TestLogFilesAPISemClient(t *testing.T) {
	_, ts := newLogTestServer(t, logUpstream(t, true), false)
	resp, err := http.Get(ts.URL + "/_dev/api/log/files")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status=%d, quer 503", resp.StatusCode)
	}
}

// sseEvents lê os eventos "data:" de uma conexão SSE num canal.
func sseEvents(t *testing.T, body io.Reader) chan logEvent {
	t.Helper()
	events := make(chan logEvent, 16)
	go func() {
		sc := bufio.NewScanner(body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var ev logEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev) == nil {
				events <- ev
			}
		}
	}()
	return events
}

func nextEvent(t *testing.T, events chan logEvent) logEvent {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("evento SSE não chegou")
		return logEvent{}
	}
}

// Stream completo: backlog do tail (com a entrada multi-linha aberta em
// linhas) e depois a linha nova do poll; sem assinantes, o hub morre.
func TestLogStream(t *testing.T) {
	old := logPollInterval
	logPollInterval = 30 * time.Millisecond
	t.Cleanup(func() { logPollInterval = old })

	s, ts := newLogTestServer(t, logUpstream(t, true), true)
	resp, err := http.Get(ts.URL + "/_dev/api/log/stream")
	if err != nil {
		t.Fatal(err)
	}
	events := sseEvents(t, resp.Body)

	first := nextEvent(t, events)
	if first.File != "server.log" || len(first.Lines) != 0 || first.Error != "" {
		t.Errorf("primeiro evento (meta) inesperado: %+v", first)
	}
	backlog := nextEvent(t, events)
	if len(backlog.Lines) != 3 || !strings.Contains(backlog.Lines[2], "Foo.java:1") {
		t.Errorf("backlog inesperado: %+v", backlog.Lines)
	}
	live := nextEvent(t, events)
	if len(live.Lines) != 1 || !strings.Contains(live.Lines[0], "nova") {
		t.Errorf("evento ao vivo inesperado: %+v", live)
	}

	// Fechar a conexão derruba o último assinante → o hub (e o poller) morre.
	resp.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.logMu.Lock()
		n := len(s.logHubs)
		s.logMu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hub não foi descartado sem assinantes (%d restando)", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Sem o helper, o stream entrega o erro orientando o install-helper.
func TestLogStreamSemHelper(t *testing.T) {
	_, ts := newLogTestServer(t, logUpstream(t, false), true)
	resp, err := http.Get(ts.URL + "/_dev/api/log/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := sseEvents(t, resp.Body)
	_ = nextEvent(t, events) // meta (file, backlog vazio)
	ev := nextEvent(t, events)
	if !strings.Contains(ev.Error, "componente auxiliar") {
		t.Errorf("esperava o erro do helper, veio: %+v", ev)
	}
}

// A página e o tile existem.
func TestLogPanelPage(t *testing.T) {
	_, ts := newLogTestServer(t, logUpstream(t, true), true)
	resp, err := http.Get(ts.URL + "/_dev/logs/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "logs do servidor") || !strings.Contains(string(body), "/_dev/api/log/stream") {
		t.Error("página do painel sem o conteúdo esperado")
	}
	if !strings.Contains(dashboardHTML, "/_dev/logs/") {
		t.Error("dashboard sem o tile de logs")
	}
}
