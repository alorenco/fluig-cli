package devserver

import (
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

// datasetLabUpstream simula os endpoints do Fluig que o lab usa: login/ping da
// sessão, a listagem REST v2 e o dataset-handle/search (colunas + linhas, com
// null preservado). O dataset "vazio_meta" devolve columns/values null (o que a
// homologação faz para dataset que exige constraint) → sonda com probeError.
func datasetLabUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/dataset/api/v2/datasets", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[
			{"datasetId":"meu_ds","datasetDescription":"Meu dataset","type":"CUSTOM","custom":true,"active":true},
			{"datasetId":"colleague","datasetDescription":"Usuários","type":"BUILTIN","custom":false,"active":true}],
			"hasNext":false}`)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("datasetId") {
		case "meu_ds":
			io.WriteString(w, `{"columns":["nome","idade"],"values":[
				{"nome":"Ana","idade":"30"},
				{"nome":"Bruno","idade":null}]}`)
		default:
			io.WriteString(w, `{"columns":null,"values":null}`)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newDatasetLabServer(t *testing.T, upstream *httptest.Server, withClient bool) *httptest.Server {
	t.Helper()
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Root: projRoot(t), Upstream: u, Port: 0, Debounce: 10 * time.Millisecond}
	jar, _ := cookiejar.New(nil)
	opts.Jar = jar
	if withClient {
		client, err := fluig.NewClient(fluig.Options{BaseURL: upstream.URL, Username: "dev-" + t.Name(), Password: "p", CompanyID: 1})
		if err != nil {
			t.Fatal(err)
		}
		opts.Client = client
		opts.CompanyID = 1
	}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts
}

func getBody(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// A página do lab é servida com os marcadores esperados.
func TestDatasetLabPage(t *testing.T) {
	ts := newDatasetLabServer(t, datasetLabUpstream(t), true)
	status, body := getBody(t, ts.URL+"/_dev/datasets/")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{"fluigcli", "Configurar parâmetros", "/_dev/api/dataset/list", "Exportar CSV", "sqlLimit"} {
		if !strings.Contains(body, want) {
			t.Errorf("página não contém %q", want)
		}
	}
}

// list devolve os datasets; fields sonda as colunas; dataset sem metadados cai
// em probeError (columns vazio) sem virar erro HTTP.
func TestDatasetLabListEFields(t *testing.T) {
	ts := newDatasetLabServer(t, datasetLabUpstream(t), true)

	status, body := getBody(t, ts.URL+"/_dev/api/dataset/list")
	if status != http.StatusOK || !strings.Contains(body, `"meu_ds"`) || !strings.Contains(body, `"colleague"`) {
		t.Fatalf("list inesperado (%d): %s", status, body)
	}

	status, body = getBody(t, ts.URL+"/_dev/api/dataset/fields?id=meu_ds")
	if status != http.StatusOK || !strings.Contains(body, `"nome"`) || !strings.Contains(body, `"idade"`) {
		t.Fatalf("fields inesperado (%d): %s", status, body)
	}

	// Dataset que devolve columns/values null → probeError, columns vazio, 200.
	status, body = getBody(t, ts.URL+"/_dev/api/dataset/fields?id=vazio_meta")
	if status != http.StatusOK || !strings.Contains(body, "probeError") {
		t.Fatalf("fields deveria trazer probeError (%d): %s", status, body)
	}

	// Sem id → 400.
	status, _ = getBody(t, ts.URL+"/_dev/api/dataset/fields")
	if status != http.StatusBadRequest {
		t.Errorf("fields sem id deveria dar 400, deu %d", status)
	}
}

// query executa e devolve colunas/linhas (null preservado), duração e o aviso
// de truncamento quando o número de linhas bate o limite.
func TestDatasetLabQuery(t *testing.T) {
	ts := newDatasetLabServer(t, datasetLabUpstream(t), true)

	body := `{"id":"meu_ds","limit":2}`
	resp, err := http.Post(ts.URL+"/_dev/api/dataset/query", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	s := string(out)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query status %d: %s", resp.StatusCode, s)
	}
	for _, want := range []string{`"count":2`, `"nome"`, `"idade":null`, `"durationMs"`, `"truncated":true`} {
		if !strings.Contains(s, want) {
			t.Errorf("query sem %q em: %s", want, s)
		}
	}

	// Tipo de filtro inválido → 400.
	bad := `{"id":"meu_ds","constraints":[{"field":"nome","initial":"x","type":"CONTAINS"}]}`
	resp, err = http.Post(ts.URL+"/_dev/api/dataset/query", "application/json", strings.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("filtro inválido deveria dar 400, deu %d", resp.StatusCode)
	}

	// Sem id → 400.
	resp, err = http.Post(ts.URL+"/_dev/api/dataset/query", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("query sem id deveria dar 400, deu %d", resp.StatusCode)
	}
}

// Sem cliente autenticado, a API do lab responde 503 (a página segue servindo).
func TestDatasetLabSemCliente(t *testing.T) {
	ts := newDatasetLabServer(t, datasetLabUpstream(t), false)
	status, _ := getBody(t, ts.URL+"/_dev/api/dataset/list")
	if status != http.StatusServiceUnavailable {
		t.Errorf("sem cliente deveria dar 503, deu %d", status)
	}
	if status, _ := getBody(t, ts.URL+"/_dev/datasets/"); status != http.StatusOK {
		t.Errorf("página deveria abrir mesmo sem cliente, deu %d", status)
	}
}
