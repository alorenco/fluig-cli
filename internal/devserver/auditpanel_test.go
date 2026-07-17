package devserver

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// O endpoint roda o linter na pasta do formulário e devolve os achados.
func TestAuditAPI(t *testing.T) {
	ts, s, _ := newDashTestServer(t)
	dir := filepath.Join(s.opts.Root, "forms", "MeuForm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<link href="/style-guide/css/fluig-style-guide.min.css">
<form name="f"><div style="color:#fff">x</div></form>`
	if err := os.WriteFile(filepath.Join(dir, "MeuForm.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(ts.URL + "/_dev/api/audit?form=MeuForm")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Findings []struct {
			Rule string `json:"rule"`
			File string `json:"file"`
		} `json:"findings"`
		Counts  map[string]int `json:"counts"`
		Scanned int            `json:"scanned"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("resposta não é JSON: %v\n%s", err, body)
	}
	if out.Counts["error"] != 1 || out.Counts["warning"] != 2 || out.Scanned != 1 {
		t.Errorf("counts/scanned inesperados: %+v", out)
	}
	for _, f := range out.Findings {
		if f.File != "forms/MeuForm/MeuForm.html" {
			t.Errorf("file fora da pasta pedida: %+v", f)
		}
	}
}

// Nome de form vindo do navegador não escapa de forms/ (anti-traversal) e
// form inexistente responde 404.
func TestAuditAPIValidacao(t *testing.T) {
	ts, _, _ := newDashTestServer(t)
	for path, want := range map[string]int{
		"/_dev/api/audit":                        http.StatusBadRequest,
		"/_dev/api/audit?form=..%2F..":           http.StatusBadRequest,
		"/_dev/api/audit?form=NaoExiste":         http.StatusNotFound,
		"/_dev/api/audit?form=..%2Fwcm%2Fwidget": http.StatusBadRequest,
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: status=%d, quero %d", path, resp.StatusCode, want)
		}
	}
}
