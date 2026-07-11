package devserver

import (
	"bytes"
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

// fakeWatch implementa WatchBridge para os testes.
type fakeWatch struct {
	mu      sync.Mutex
	status  WatchStatus
	lastSet []string
}

func (f *fakeWatch) Status() WatchStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeWatch) Set(enabled bool, types []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.Enabled = enabled
	f.status.Types = types
	f.lastSet = types
	return nil
}

func newDashTestServer(t *testing.T) (*httptest.Server, *Server, *fakeWatch) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "upstream:"+r.URL.Path)
	}))
	t.Cleanup(upstream.Close)
	u, _ := url.Parse(upstream.URL)
	jar, _ := cookiejar.New(nil)
	fw := &fakeWatch{status: WatchStatus{Available: true, Enabled: true, Types: []string{"dataset"}, Recent: []string{"✓ 10:00:00 dataset \"ds_x\" publicado"}}}
	s, err := New(Options{
		Root: projRoot(t), Upstream: u, Jar: jar, Port: 0, Debounce: 300 * time.Millisecond,
		ServerName: "homolog", ServerEnv: "hml", Username: "alorenco", CompanyID: 1,
		Watch: fw,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts, s, fw
}

// A raiz exata serve o dashboard; os demais caminhos seguem para o proxy.
func TestDashboardRota(t *testing.T) {
	ts, _, _ := newDashTestServer(t)

	resp, _ := http.Get(ts.URL + "/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	out := string(body)
	if !strings.Contains(out, "<title>fluigcli dev</title>") || !strings.Contains(out, "/_dev/api/dash") {
		t.Errorf("dashboard não servido na raiz:\n%.300s", out)
	}
	// Caminho não-raiz continua no proxy.
	resp, _ = http.Get(ts.URL + "/portal/qualquer")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "upstream:/portal/qualquer") {
		t.Errorf("proxy quebrou: %s", body)
	}
}

func TestDashboardAPI(t *testing.T) {
	ts, s, fw := newDashTestServer(t)

	resp, _ := http.Get(ts.URL + "/_dev/api/dash")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var dash struct {
		Server struct {
			Name string `json:"name"`
			Env  string `json:"env"`
			User string `json:"user"`
		} `json:"server"`
		PortalPath string   `json:"portalPath"`
		FormsCount int      `json:"formsCount"`
		Mounts     []string `json:"mounts"`
		Watch      *struct {
			Enabled bool     `json:"enabled"`
			Types   []string `json:"types"`
			Recent  []string `json:"recent"`
		} `json:"watch"`
		WatchTypes []struct {
			ID string `json:"id"`
		} `json:"watchTypes"`
		Reload struct {
			Enabled    bool  `json:"enabled"`
			DebounceMs int64 `json:"debounceMs"`
		} `json:"reload"`
	}
	if err := json.Unmarshal(body, &dash); err != nil {
		t.Fatalf("dash inválido: %v\n%s", err, body)
	}
	if dash.Server.Name != "homolog" || dash.Server.Env != "hml" || dash.Server.User != "alorenco" {
		t.Errorf("server: %+v", dash.Server)
	}
	if dash.PortalPath != "/portal/p/1/home" || dash.FormsCount != 1 || len(dash.Mounts) != 2 {
		t.Errorf("portal/forms/mounts: %q %d %v", dash.PortalPath, dash.FormsCount, dash.Mounts)
	}
	if dash.Watch == nil || !dash.Watch.Enabled || len(dash.Watch.Recent) != 1 || len(dash.WatchTypes) != 5 {
		t.Errorf("watch: %+v types=%d", dash.Watch, len(dash.WatchTypes))
	}
	if !dash.Reload.Enabled || dash.Reload.DebounceMs != 300 {
		t.Errorf("reload: %+v", dash.Reload)
	}

	// Watch: tipos válidos são aplicados na ponte; inválido é recusado.
	post := func(path string, payload any) (int, []byte) {
		b, _ := json.Marshal(payload)
		r, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		out, _ := io.ReadAll(r.Body)
		return r.StatusCode, out
	}
	code, _ := post("/_dev/api/dash/watch", map[string]any{"enabled": true, "types": []string{"form", "workflow"}})
	if code != http.StatusOK || len(fw.lastSet) != 2 {
		t.Errorf("watch set: code=%d lastSet=%v", code, fw.lastSet)
	}
	code, _ = post("/_dev/api/dash/watch", map[string]any{"enabled": true, "types": []string{"invasor"}})
	if code != http.StatusBadRequest {
		t.Errorf("tipo inválido aceito: code=%d", code)
	}

	// Reload: pausa e ajusta o debounce dinamicamente; faixa validada.
	code, _ = post("/_dev/api/dash/reload", map[string]any{"enabled": false, "debounceMs": 900})
	if code != http.StatusOK || s.reloadEnabled() || s.reloadDebounceNow() != 900*time.Millisecond {
		t.Errorf("reload: code=%d enabled=%v debounce=%v", code, s.reloadEnabled(), s.reloadDebounceNow())
	}
	code, _ = post("/_dev/api/dash/reload", map[string]any{"enabled": true, "debounceMs": 20})
	if code != http.StatusBadRequest {
		t.Errorf("debounce fora da faixa aceito: code=%d", code)
	}

	// Limpar caches zera o cache do painel.
	s.sim.mu.Lock()
	s.sim.processes = []fluig.ProcessSummary{{ID: "x"}}
	s.sim.mu.Unlock()
	code, _ = post("/_dev/api/dash/clear-caches", map[string]any{})
	s.sim.mu.Lock()
	cleared := s.sim.processes == nil && s.deploys == nil
	s.sim.mu.Unlock()
	if code != http.StatusOK || !cleared {
		t.Errorf("clear-caches: code=%d cleared=%v", code, cleared)
	}
}
