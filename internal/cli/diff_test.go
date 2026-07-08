package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// diffServerStub simula o servidor com os três tipos de artefato, usando as
// fixtures reais de testdata: dataset ds_exemplo (CUSTOM) + colleague
// (DEFAULT), eventos beforeConvertViewToPDF e displayCustomThemes, mecanismo
// mec_gestor_area.
func diffServerStub(t *testing.T) *httptest.Server {
	t.Helper()
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	})
	mux.HandleFunc("/webdesk/ECMDatasetService", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("SOAPAction") == "findAllFormulariesDatasets" {
			_, _ = w.Write(readTD("soap_findAllDatasets.xml"))
			return
		}
		http.Error(w, "op?", 500)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/loadDataset", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") == "ds_exemplo" {
			_, _ = w.Write(readTD("loadDataset.json"))
			return
		}
		http.Error(w, "não existe", 500)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/getEventList", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readTD("getEventList.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/getCustomAttributionMechanismList", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readTD("getMechanismList.json"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// diffProject monta o projeto local do cenário de diff.
func diffProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	s := config.Server{ID: "diff-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(s, false); err != nil {
		t.Fatal(err)
	}

	write := func(rel, content string) {
		path := filepath.Join(proj, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Igual ao servidor, mas com CRLF — não pode contar como diferença.
	write("datasets/ds_exemplo.js",
		"function createDataset(fields, constraints, sortFields) {\r\n  return null;\r\n}\r\n")
	// Diverge do servidor (fixture tem "codigo A").
	write("events/beforeConvertViewToPDF.js",
		"function beforeConvertViewToPDF(){ /* codigo NOVO */ }")
	// Igual byte a byte.
	write("mechanisms/mec_gestor_area.js",
		"function getUsers(mecanismId, colleagueId){ return ['gestor']; }")
	// Não existe no servidor.
	write("mechanisms/novo.js", "function getUsers(){ return []; }")
	return proj
}

func TestDiffVarredura(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, stdout := runMain(t, "diff", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d, quer 0; stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é JSON: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	arts, _ := data["artifacts"].([]any)

	want := map[string]string{ // "tipo id" → status
		"dataset ds_exemplo":           "equal",
		"event beforeConvertViewToPDF": "modified",
		"event displayCustomThemes":    "only-server",
		"mechanism mec_gestor_area":    "equal",
		"mechanism novo":               "only-local",
	}
	if len(arts) != len(want) {
		t.Fatalf("veio %d artefatos, quer %d: %s", len(arts), len(want), stdout)
	}
	for _, a := range arts {
		m, _ := a.(map[string]any)
		key := m["type"].(string) + " " + m["id"].(string)
		if m["status"] != want[key] {
			t.Errorf("%s: status = %v, quer %v", key, m["status"], want[key])
		}
		if key == "event beforeConvertViewToPDF" {
			d, _ := m["diff"].(string)
			if !strings.Contains(d, "-function beforeConvertViewToPDF(){ /* codigo A */ }") ||
				!strings.Contains(d, "+function beforeConvertViewToPDF(){ /* codigo NOVO */ }") {
				t.Errorf("diff unificado inesperado:\n%s", d)
			}
		}
	}
	counts, _ := data["counts"].(map[string]any)
	if counts["equal"] != float64(2) || counts["modified"] != float64(1) {
		t.Errorf("counts inesperados: %#v", counts)
	}
	// O dataset "colleague" é DEFAULT no servidor — não pode aparecer.
	if strings.Contains(stdout, "colleague") {
		t.Error("dataset DEFAULT do servidor não deveria entrar no diff")
	}
}

func TestDiffCaminhoUnico(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, stdout := runMain(t, "diff", filepath.Join(proj, "datasets", "ds_exemplo.js"),
		"--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d; stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	arts, _ := data["artifacts"].([]any)
	if len(arts) != 1 {
		t.Fatalf("veio %d artefatos, quer 1 (sem only-server em modo caminho)", len(arts))
	}
	m, _ := arts[0].(map[string]any)
	if m["id"] != "ds_exemplo" || m["status"] != "equal" {
		t.Errorf("artefato inesperado: %#v", m)
	}
}

func TestDiffCaminhoForaDaConvencao(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, _ := runMain(t, "diff", filepath.Join(proj, "forms", "x.js"), "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("caminho fora da convenção: exit = %d, quer %d", code, output.ExitUsage)
	}
}
