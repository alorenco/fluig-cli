package devserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// deployUpstream simula o Fluig para a publicação: login/ping/findUser, o
// SOAP do ECMCardIndexService (list/create/update com fixtures reais) e a
// listagem REST de datasets.
type deployUpstream struct {
	mu         sync.Mutex
	soapCalls  []string
	lastUpdate string
	lastCreate string
}

func (d *deployUpstream) server(t *testing.T) *httptest.Server {
	t.Helper()
	readTD := func(name string) []byte {
		t.Helper()
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
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"fullName":"Dev","email":"d@x","userCode":"dev-code"}}`)
	})
	mux.HandleFunc("/dataset/api/v2/datasets", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[{"datasetId":"ds_teste","type":"CUSTOM","custom":true,"active":true},
			{"datasetId":"ds_outro","type":"CUSTOM","custom":true,"active":true}],"hasNext":false}`)
	})
	mux.HandleFunc("/webdesk/ECMCardIndexService", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		action := r.Header.Get("SOAPAction")
		d.mu.Lock()
		d.soapCalls = append(d.soapCalls, action)
		d.mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		switch action {
		case "getCardIndexesWithoutApprover":
			w.Write(readTD("soap_listForms.xml"))
		case "updateSimpleCardIndexWithDatasetAndGeneralInfo":
			d.mu.Lock()
			d.lastUpdate = string(body)
			d.mu.Unlock()
			w.Write(readTD("soap_writeForm.xml"))
		case "createSimpleCardIndexWithDatasetPersisteType":
			d.mu.Lock()
			d.lastCreate = string(body)
			d.mu.Unlock()
			w.Write(readTD("soap_writeForm.xml"))
		default:
			http.Error(w, "op inesperada: "+action, http.StatusInternalServerError)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newDeployTestServer sobe o dev server com três servidores para o diálogo:
// o conectado (homolog), um "outro" que exige senha na primeira vez e um de
// produção. Todos apontam para o mesmo upstream fake.
func newDeployTestServer(t *testing.T, upstream *httptest.Server) (*httptest.Server, *Server) {
	t.Helper()
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	newClient := func(user string) *fluig.Client {
		c, err := fluig.NewClient(fluig.Options{BaseURL: upstream.URL, Username: user + "-" + t.Name(), Password: "p", CompanyID: 1})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	root := projRoot(t)
	const scope = "homolog:8080/1"
	formsJSON := `{"version":"2.0.0","servers":{"` + scope + `":[{"folder":"Meu Form","documentId":42,"name":"Formulario de Teste"}]}}`
	if err := os.MkdirAll(filepath.Join(root, ".fluigcli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".fluigcli", "forms.json"), []byte(formsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	s, err := New(Options{
		Root: root, Upstream: u, Jar: jar, Port: 0, Debounce: 10 * time.Millisecond,
		Client: newClient("dev"), FormScope: scope, CompanyID: 1,
		DeployServers: []DeployServerInfo{
			{Name: "homolog", Env: "hml", URL: upstream.URL, Default: true, Current: true},
			{Name: "outro", Env: "dev", URL: upstream.URL},
			{Name: "producao", Env: "prod", URL: upstream.URL},
		},
		DeployConnect: func(ctx context.Context, name, password string) (*fluig.Client, string, error) {
			switch name {
			case "outro":
				if password == "" {
					return nil, "", ErrDeployNeedsPassword
				}
				return newClient("outro"), "outro:8080/1", nil
			case "producao":
				return newClient("prod"), "producao:8080/1", nil
			}
			return nil, "", ErrDeployNeedsPassword
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts, s
}

func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("resposta não-JSON (%d): %s", resp.StatusCode, raw)
		}
	}
	return resp.StatusCode, out
}

func TestDeployServersEForms(t *testing.T) {
	up := &deployUpstream{}
	ts, _ := newDeployTestServer(t, up.server(t))

	// Lista de servidores para o diálogo.
	resp, _ := http.Get(ts.URL + "/_dev/api/formsim/deploy/servers")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var srvs struct {
		Servers []DeployServerInfo `json:"servers"`
	}
	if err := json.Unmarshal(body, &srvs); err != nil || len(srvs.Servers) != 3 {
		t.Fatalf("servers: %v %s", err, body)
	}
	if !srvs.Servers[0].Current || srvs.Servers[2].Env != "prod" {
		t.Errorf("lista inesperada: %+v", srvs.Servers)
	}

	// Formulários do servidor conectado: fixture + vínculo + datasets.
	status, out := postJSON(t, ts.URL+"/_dev/api/formsim/deploy/forms",
		map[string]any{"server": "homolog", "folder": "Meu Form"})
	if status != http.StatusOK {
		t.Fatalf("forms: status=%d %v", status, out)
	}
	forms := out["forms"].([]any)
	if len(forms) != 1 || forms[0].(map[string]any)["name"] != "Formulario de Teste" {
		t.Errorf("forms inesperados: %v", forms)
	}
	if out["linkedDocumentId"].(float64) != 42 {
		t.Errorf("linkedDocumentId: %v", out["linkedDocumentId"])
	}
	ds := out["datasets"].([]any)
	if len(ds) != 2 || ds[0] != "ds_outro" {
		t.Errorf("datasets: %v", ds)
	}

	// Servidor sem credencial: 401 needsPassword; com senha, conecta.
	status, out = postJSON(t, ts.URL+"/_dev/api/formsim/deploy/forms",
		map[string]any{"server": "outro", "folder": "Meu Form"})
	if status != http.StatusUnauthorized || out["needsPassword"] != true {
		t.Errorf("sem senha: status=%d %v", status, out)
	}
	status, _ = postJSON(t, ts.URL+"/_dev/api/formsim/deploy/forms",
		map[string]any{"server": "outro", "password": "s3nh4", "folder": "Meu Form"})
	if status != http.StatusOK {
		t.Errorf("com senha: status=%d", status)
	}
}

func TestDeployAtualizaECria(t *testing.T) {
	up := &deployUpstream{}
	ts, s := newDeployTestServer(t, up.server(t))

	// Atualização do formulário vinculado (fixture documentId 42; o SOAP de
	// escrita devolve 99 — o vínculo local segue o servidor).
	status, out := postJSON(t, ts.URL+"/_dev/api/formsim/deploy", map[string]any{
		"server": "homolog", "folder": "Meu Form", "documentId": 42, "versionMode": "new",
	})
	if status != http.StatusOK || out["action"] != "updated" || out["documentId"].(float64) != 99 {
		t.Fatalf("update: status=%d %v", status, out)
	}
	up.mu.Lock()
	hasUpdate := strings.Contains(strings.Join(up.soapCalls, ","), "updateSimpleCardIndexWithDatasetAndGeneralInfo")
	up.mu.Unlock()
	if !hasUpdate {
		t.Error("SOAP de update não foi chamado")
	}
	fm, _ := os.ReadFile(filepath.Join(s.opts.Root, ".fluigcli", "forms.json"))
	if !strings.Contains(string(fm), `"documentId": 99`) && !strings.Contains(string(fm), `"documentId":99`) {
		t.Errorf("vínculo não atualizado: %s", fm)
	}

	// Criação: dataset e pasta são obrigatórios.
	status, out = postJSON(t, ts.URL+"/_dev/api/formsim/deploy", map[string]any{
		"server": "homolog", "folder": "Meu Form", "documentId": 0,
		"create": map[string]any{"name": "Novo Form", "parentId": 123},
	})
	if status != http.StatusBadRequest || !strings.Contains(out["error"].(string), "dataset") {
		t.Errorf("criação sem dataset: status=%d %v", status, out)
	}
	status, out = postJSON(t, ts.URL+"/_dev/api/formsim/deploy", map[string]any{
		"server": "homolog", "folder": "Meu Form", "documentId": 0,
		"create": map[string]any{"name": "Novo Form", "datasetName": "ds_novo", "parentId": 123, "persistenceType": "single"},
	})
	if status != http.StatusOK || out["action"] != "created" || out["documentId"].(float64) != 99 {
		t.Fatalf("create: status=%d %v", status, out)
	}
	up.mu.Lock()
	create := up.lastCreate
	up.mu.Unlock()
	if !strings.Contains(create, "Novo Form") || !strings.Contains(create, "ds_novo") {
		t.Errorf("corpo do create sem os dados: %.300s", create)
	}
}

func TestDeployProdExigeConfirmacao(t *testing.T) {
	up := &deployUpstream{}
	ts, _ := newDeployTestServer(t, up.server(t))

	status, out := postJSON(t, ts.URL+"/_dev/api/formsim/deploy", map[string]any{
		"server": "producao", "folder": "Meu Form", "documentId": 42,
	})
	if status != http.StatusBadRequest || !strings.Contains(out["error"].(string), "PRODUÇÃO") {
		t.Fatalf("prod sem confirmação: status=%d %v", status, out)
	}
	status, out = postJSON(t, ts.URL+"/_dev/api/formsim/deploy", map[string]any{
		"server": "producao", "folder": "Meu Form", "documentId": 42, "confirm": "producao",
	})
	if status != http.StatusOK || out["action"] != "updated" {
		t.Fatalf("prod confirmado: status=%d %v", status, out)
	}

	// Servidor fora da lista é recusado.
	status, _ = postJSON(t, ts.URL+"/_dev/api/formsim/deploy", map[string]any{
		"server": "invasor", "folder": "Meu Form", "documentId": 42,
	})
	if status != http.StatusBadRequest {
		t.Errorf("servidor desconhecido: status=%d", status)
	}
}
