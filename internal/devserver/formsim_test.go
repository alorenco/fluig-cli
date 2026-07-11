package devserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// O preview injeta o bootstrap da simulação com o fonte do displayFields
// embutido (escapado para <script>) e o runtime /_dev/formsim.js.
func TestFormSimInjecao(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	ts, s, _ := newTestServer(t, upstream)

	// Formulário SEM events/ → bootstrap com event:null (painel avisa).
	resp, _ := http.Get(ts.URL + "/_dev/forms/Meu%20Form/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	out := string(body)
	if !strings.Contains(out, "window.__fluigcliFormSim=") ||
		!strings.Contains(out, `"event":null`) ||
		!strings.Contains(out, `src="/_dev/formsim.js"`) {
		t.Errorf("bootstrap sem evento não injetado:\n%s", out)
	}

	// Com events/displayFields.js, o fonte entra escapado (sem </script> cru).
	evDir := filepath.Join(s.opts.Root, "forms", "Meu Form", "events")
	if err := os.MkdirAll(evDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ev := "function displayFields(form, customHTML) {\n  form.setValue(\"currentState\", getValue(\"WKNumState\"));\n  // </script> no comentário não pode quebrar o HTML\n}\n"
	if err := os.WriteFile(filepath.Join(evDir, "displayFields.js"), []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, _ = http.Get(ts.URL + "/_dev/forms/Meu%20Form/")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	out = string(body)
	if !strings.Contains(out, `</script>`) || strings.Contains(out, "// </script>") {
		t.Errorf("fonte do evento deveria entrar escapado para <script>:\n%s", out)
	}
	if !strings.Contains(out, "WKNumState") {
		t.Errorf("fonte do evento não embutido:\n%s", out)
	}
	// A injeção fica antes do </body>.
	if strings.LastIndex(out, "__fluigcliFormSim") > strings.LastIndex(strings.ToLower(out), "</body>") {
		t.Error("bootstrap deveria ficar antes do </body>")
	}

	// O runtime é servido.
	resp, _ = http.Get(ts.URL + "/_dev/formsim.js")
	js, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type do runtime: %q", ct)
	}
	if !strings.Contains(string(js), "displayFields") || !strings.Contains(string(js), "fluigcli-sim") {
		t.Error("runtime da simulação incompleto")
	}
}

// Formulário com tabela pai×filho (tablename=) ganha a máquina REAL do
// servidor (wdkdetail.js) injetada antes do runtime — desde que o upstream a
// tenha; formulário sem tabela não ganha.
func TestFormSimInjetaWdkDetail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ecm_resources/resources/assets/forms/wdkdetail.js" {
			_, _ = io.WriteString(w, "// máquina real")
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	ts, s, _ := newTestServer(t, upstream)

	// Sem tablename → sem wdkdetail.
	resp, _ := http.Get(ts.URL + "/_dev/forms/Meu%20Form/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "wdkdetail.js") {
		t.Error("form sem tabela pai×filho não deveria ganhar o wdkdetail.js")
	}

	// Com tablename → wdkdetail injetado ANTES do runtime da simulação.
	dir := filepath.Join(s.opts.Root, "forms", "comtabela")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<html><body><table tablename="tabelaItens" id="tabelaItens"><tbody><tr><td><input name="item"/></td></tr></tbody></table></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "comtabela.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, _ = http.Get(ts.URL + "/_dev/forms/comtabela/")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	out := string(body)
	iWdk := strings.Index(out, "wdkdetail.js")
	iSim := strings.Index(out, "/_dev/formsim.js")
	if iWdk < 0 || iSim < 0 || iWdk > iSim {
		t.Errorf("wdkdetail.js deveria ser injetado antes do runtime (wdk=%d sim=%d):\n%s", iWdk, iSim, out)
	}
}

// Upstream sem o wdkdetail.js (Fluig antigo) → nada é injetado e o runtime
// cai na emulação local.
func TestFormSimSemWdkDetailNoServidor(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	ts, s, _ := newTestServer(t, upstream)
	dir := filepath.Join(s.opts.Root, "forms", "comtabela")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<html><body><table tablename="t" id="t"><tbody><tr><td><input name="x"/></td></tr></tbody></table></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "comtabela.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, _ := http.Get(ts.URL + "/_dev/forms/comtabela/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "wdkdetail.js") {
		t.Error("upstream sem wdkdetail.js não deveria gerar injeção")
	}
}

// Sem cliente autenticado (só nos testes — o comando dev sempre passa), a API
// local responde 503 com mensagem, e o painel segue com valores manuais.
func TestFormSimAPISemCliente(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	ts, _, _ := newTestServer(t, upstream)

	resp, _ := http.Get(ts.URL + "/_dev/api/formsim/context?folder=x")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "error") {
		t.Errorf("status=%d body=%s", resp.StatusCode, body)
	}
}

// simUpstream simula o Fluig para a API do painel: login/ping, findUserByLogin,
// listagem de processos (com e sem expand=versions), states e o dataset
// colleague (contando as consultas, para o teste de cache).
func simUpstream(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	datasetHits := new(int)
	mux := http.NewServeMux()
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("datasetId") != "colleague" {
			io.WriteString(w, `{"columns":null,"values":null}`)
			return
		}
		*datasetHits++
		io.WriteString(w, `{"columns":["colleagueId","colleagueName","active"],"values":[
			{"colleagueId":"c-alana","colleagueName":"Alana","active":"true"},
			{"colleagueId":"c-bruno","colleagueName":"Bruno","active":"false"},
			{"colleagueId":"","colleagueName":"Sem código","active":"true"}]}`)
	})
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"fullName":"Dev","email":"d@x","userCode":"dev-uuid-1"}}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("expand") == "versions" {
			io.WriteString(w, `{"items":[
				{"processId":"rh_ponto","processDescription":"Justificativa de Ponto","versions":[
					{"version":3,"formId":4711,"active":true,"editing":false}]},
				{"processId":"outro","processDescription":"Outro","versions":[
					{"version":1,"formId":9,"active":true,"editing":false}]}],"hasNext":false}`)
			return
		}
		io.WriteString(w, `{"items":[
			{"processId":"rh_ponto","processDescription":"Justificativa de Ponto","active":true},
			{"processId":"outro","processDescription":"Outro","active":false}],"hasNext":false}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/states") {
			io.WriteString(w, `{"items":[
				{"sequence":7,"stateName":"Revisar","stateType":"TASK","bpmnType":"USER_TASK"},
				{"sequence":0,"stateName":"Início","stateType":"START","bpmnType":"START_EVENT"}],"hasNext":false}`)
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, datasetHits
}

// newSimTestServer sobe o dev server com cliente autenticado e vínculo no
// forms.json, apontando para o upstream fake.
func newSimTestServer(t *testing.T, upstream *httptest.Server) (*httptest.Server, *Server) {
	t.Helper()
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := fluig.NewClient(fluig.Options{BaseURL: upstream.URL, Username: "dev-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	root := projRoot(t)
	const scope = "fluig-test:8080/1"
	formsJSON := `{"version":"2.0.0","servers":{"` + scope + `":[{"folder":"Meu Form","documentId":4711,"name":"Form no Servidor"}]}}`
	if err := os.MkdirAll(filepath.Join(root, ".fluigcli"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".fluigcli", "forms.json"), []byte(formsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	s, err := New(Options{
		Root: root, Upstream: u, Jar: jar, Port: 0, Debounce: 10 * time.Millisecond,
		Client: client, FormScope: scope, CompanyID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts, s
}

// O context resolve userCode, o vínculo do forms.json e o processo cujo
// formId casa com o documentId; states vêm ordenados por sequence.
func TestFormSimAPIContextEStates(t *testing.T) {
	upstream, _ := simUpstream(t)
	ts, _ := newSimTestServer(t, upstream)

	resp, err := http.Get(ts.URL + "/_dev/api/formsim/context?folder=" + url.QueryEscape("Meu Form"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var ctx struct {
		UserCode  string `json:"userCode"`
		CompanyID int    `json:"companyId"`
		Form      *struct {
			DocumentID int    `json:"documentId"`
			Name       string `json:"name"`
		} `json:"form"`
		Processes []struct {
			ProcessID string `json:"processId"`
			Version   int    `json:"version"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(body, &ctx); err != nil {
		t.Fatalf("context inválido: %v\n%s", err, body)
	}
	if ctx.UserCode != "dev-uuid-1" || ctx.CompanyID != 1 {
		t.Errorf("user/company: %+v", ctx)
	}
	if ctx.Form == nil || ctx.Form.DocumentID != 4711 || ctx.Form.Name != "Form no Servidor" {
		t.Errorf("vínculo: %+v", ctx.Form)
	}
	if len(ctx.Processes) != 1 || ctx.Processes[0].ProcessID != "rh_ponto" || ctx.Processes[0].Version != 3 {
		t.Errorf("detecção do processo: %+v", ctx.Processes)
	}

	// Pasta sem vínculo → form null, sem processos, sem erro.
	resp, _ = http.Get(ts.URL + "/_dev/api/formsim/context?folder=SemVinculo")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"form":null`) {
		t.Errorf("sem vínculo: status=%d body=%s", resp.StatusCode, body)
	}

	// Processos para o seletor manual.
	resp, _ = http.Get(ts.URL + "/_dev/api/formsim/processes")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var procs []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &procs); err != nil || len(procs) != 2 {
		t.Errorf("processes: err=%v body=%s", err, body)
	}

	// Etapas da versão detectada, ordenadas por sequence.
	resp, _ = http.Get(ts.URL + "/_dev/api/formsim/states?process=rh_ponto&version=3")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var st struct {
		Version int `json:"version"`
		States  []struct {
			Sequence int    `json:"sequence"`
			Name     string `json:"stateName"`
		} `json:"states"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("states inválido: %v\n%s", err, body)
	}
	if st.Version != 3 || len(st.States) != 2 || st.States[0].Sequence != 0 || st.States[1].Name != "Revisar" {
		t.Errorf("states: %+v", st)
	}

	// Parâmetros obrigatórios.
	resp, _ = http.Get(ts.URL + "/_dev/api/formsim/states")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("states sem process: status=%d", resp.StatusCode)
	}
	resp, _ = http.Get(ts.URL + "/_dev/api/formsim/context")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("context sem folder: status=%d", resp.StatusCode)
	}
}

// A lista de usuários (dataset colleague) sai ordenada do servidor, pula
// linhas sem código e fica em cache — force=1 renova.
func TestFormSimAPIUsers(t *testing.T) {
	upstream, datasetHits := simUpstream(t)
	ts, _ := newSimTestServer(t, upstream)

	get := func(q string) []struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	} {
		t.Helper()
		resp, err := http.Get(ts.URL + "/_dev/api/formsim/users" + q)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}
		var users []struct {
			Code   string `json:"code"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
		}
		if err := json.Unmarshal(body, &users); err != nil {
			t.Fatalf("resposta inválida: %v\n%s", err, body)
		}
		return users
	}

	users := get("")
	if len(users) != 2 {
		t.Fatalf("esperava 2 usuários (linha sem código fora), veio %d: %+v", len(users), users)
	}
	if users[0].Code != "c-alana" || !users[0].Active || users[1].Code != "c-bruno" || users[1].Active {
		t.Errorf("usuários inesperados: %+v", users)
	}
	// Cache: segunda chamada não bate no dataset; force renova.
	_ = get("")
	if *datasetHits != 1 {
		t.Errorf("dataset consultado %d vez(es), queria 1 (cache)", *datasetHits)
	}
	_ = get("?force=1")
	if *datasetHits != 2 {
		t.Errorf("force=1 deveria renovar a consulta (hits=%d)", *datasetHits)
	}
}
