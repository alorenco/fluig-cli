package devserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// statusUpstream simula o Fluig para o /_dev/api/status: login/ping + versão
// do produto + /environment (admin=false responde 401 com o HTML real).
func statusUpstream(t *testing.T, admin bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/api/public/wcm/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"value":"TOTVS Fluig Plataforma - Voyager 2.0.0-260707"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.2.0"}`)
	})
	unauthorized := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `<html><head><title>Error</title></head><body>Unauthorized</body></html>`)
	}
	mux.HandleFunc("/environment/api/v2/statistics", func(w http.ResponseWriter, r *http.Request) {
		if !admin {
			unauthorized(w)
			return
		}
		io.WriteString(w, `{"CONNECTED_USERS":{"connectedUsers":7},"RUNTIME":{"uptime":86400000},
			"MEMORY":{"heap-memory-usage":1073741824,"non-heap-memory-usage":268435456},
			"THREADING":{"count":120,"peakCount":150},
			"DATABASE_INFO":{"databaseName":"SQL Server","databaseVersion":"14.0"},
			"DATABASE_SIZE":{"size":2147483648},
			"OPERATION_SYSTEM":{"server-memory-size":17179869184,"server-memory-free":4294967296}}`)
	})
	mux.HandleFunc("/environment/api/v2/monitors", func(w http.ResponseWriter, r *http.Request) {
		if !admin {
			unauthorized(w)
			return
		}
		io.WriteString(w, `{"items":[{"name":"Memória","status":"OK","sucessRate":100},
			{"name":"Solr","status":"FAILURE","sucessRate":40},
			{"name":"Mail","status":"NONE","sucessRate":0}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newStatusTestServer(t *testing.T, upstream *httptest.Server, withClient bool) *httptest.Server {
	t.Helper()
	u, _ := url.Parse(upstream.URL)
	jar, _ := cookiejar.New(nil)
	opts := Options{
		Root: projRoot(t), Upstream: u, Jar: jar, Port: 0, Debounce: 10 * time.Millisecond,
		ServerName: "producao", ServerEnv: "prod", Username: "alorenco", CompanyID: 1,
	}
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
	return ts
}

type statusPayload struct {
	Server struct {
		Name string `json:"name"`
		Env  string `json:"env"`
		User string `json:"user"`
	} `json:"server"`
	Version       string `json:"version"`
	VersionError  string `json:"versionError"`
	StatsError    string `json:"statsError"`
	MonitorsError string `json:"monitorsError"`
	Unavailable   string `json:"unavailable"`
	Stats         *struct {
		ConnectedUsers int   `json:"connectedUsers"`
		UptimeMillis   int64 `json:"uptimeMillis"`
		ThreadCount    int   `json:"threadCount"`
	} `json:"stats"`
	Monitors []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"monitors"`
	Helper *struct {
		Installed bool   `json:"installed"`
		Version   string `json:"version"`
	} `json:"helper"`
}

func getStatus(t *testing.T, ts *httptest.Server) statusPayload {
	t.Helper()
	resp, err := http.Get(ts.URL + "/_dev/api/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var st statusPayload
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("status inválido: %v\n%s", err, body)
	}
	return st
}

// Com admin: identificação + versão + stats + monitores completos.
func TestStatusAPICompleto(t *testing.T) {
	ts := newStatusTestServer(t, statusUpstream(t, true), true)
	st := getStatus(t, ts)
	if st.Server.Name != "producao" || st.Server.Env != "prod" || st.Server.User != "alorenco" {
		t.Errorf("identificação: %+v", st.Server)
	}
	if !strings.Contains(st.Version, "Voyager 2.0.0") {
		t.Errorf("versão: %q (err=%q)", st.Version, st.VersionError)
	}
	if st.Stats == nil || st.Stats.ConnectedUsers != 7 || st.Stats.UptimeMillis != 86400000 || st.Stats.ThreadCount != 120 {
		t.Errorf("stats: %+v (err=%q)", st.Stats, st.StatsError)
	}
	if len(st.Monitors) != 3 || st.Monitors[1].Status != "FAILURE" {
		t.Errorf("monitores: %+v (err=%q)", st.Monitors, st.MonitorsError)
	}
	if st.Helper == nil || !st.Helper.Installed || st.Helper.Version != "0.2.0" {
		t.Errorf("helper: %+v (quer instalado v0.2.0)", st.Helper)
	}
}

// Sem admin: a versão continua vindo; stats/monitores degradam para a
// explicação de privilégio (o 401 HTML vira a mensagem amigável).
func TestStatusAPISemAdmin(t *testing.T) {
	ts := newStatusTestServer(t, statusUpstream(t, false), true)
	st := getStatus(t, ts)
	if !strings.Contains(st.Version, "Voyager 2.0.0") {
		t.Errorf("a versão não exige admin, deveria vir: %q (err=%q)", st.Version, st.VersionError)
	}
	if st.Stats != nil || !strings.Contains(st.StatsError, "privilégio administrativo") {
		t.Errorf("stats deveriam degradar com a mensagem de privilégio: %+v err=%q", st.Stats, st.StatsError)
	}
	if len(st.Monitors) != 0 || !strings.Contains(st.MonitorsError, "privilégio administrativo") {
		t.Errorf("monitores: %+v err=%q", st.Monitors, st.MonitorsError)
	}
}

// Sem Client (execução sem sessão): só a identificação, com o aviso.
func TestStatusAPISemClient(t *testing.T) {
	ts := newStatusTestServer(t, statusUpstream(t, true), false)
	st := getStatus(t, ts)
	if st.Server.Name != "producao" || st.Unavailable == "" {
		t.Errorf("esperava identificação + unavailable: %+v %q", st.Server, st.Unavailable)
	}
	if st.Version != "" || st.Stats != nil {
		t.Errorf("sem client não deveria consultar nada: version=%q stats=%+v", st.Version, st.Stats)
	}
}
