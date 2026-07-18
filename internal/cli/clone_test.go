package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// cloneServerStub simula o servidor com os SEIS tipos do clone, reusando as
// fixtures reais de testdata (mesmo cenário do diff) + o fluigcliHelper do
// stub de widget: 2 formulários, dataset ds_exemplo (CUSTOM via fallback
// SOAP), processo Compras (3 scripts), 2 eventos globais, 1 mecanismo e a
// widget meu_widget.
type cloneServerStub struct {
	helperMissing bool
	widgetStub    widgetStub // reusa o zip do WAR
}

func (s *cloneServerStub) server(t *testing.T) *httptest.Server {
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
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc"}}`)
	})
	// Datasets: sem a rota REST v2 (404) → fallback SOAP com ds_exemplo CUSTOM.
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
	mux.HandleFunc("/webdesk/ECMCardIndexService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		switch r.Header.Get("SOAPAction") {
		case "getCardIndexesWithoutApprover":
			io.WriteString(w, diffListFormsXML)
		case "getAttachmentsList":
			_, _ = w.Write(readTD("soap_attachmentsList.xml"))
		case "getCardIndexContent":
			_, _ = w.Write(readTD("soap_cardContent.xml"))
		case "getCustomizationEvents":
			_, _ = w.Write(readTD("soap_customEvents.xml"))
		default:
			http.Error(w, "op?", 500)
		}
	})
	mux.HandleFunc("/webdesk/ECMWorkflowEngineService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		if r.Header.Get("SOAPAction") != "exportProcess" {
			http.Error(w, "op?", 500)
			return
		}
		io.WriteString(w, wfExportEnvelope(processZipBase64(t, comprasProcessXML)))
	})
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[`+
			`{"processId":"Compras","processDescription":"Compras","active":true,"public":false}`+
			`],"hasNext":false}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if s.helperMissing {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/widgets", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `[{"code":"meu_widget","title":"Meu Widget","description":"d","filename":"meu_widget.war"}]`)
	})
	mux.HandleFunc("/fluigcliHelper/api/widgets/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(s.widgetStub.widgetZip(t))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func cloneProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	srv := config.Server{ID: "cl-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(srv, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func cloneEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope inválido: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	if data == nil {
		t.Fatalf("envelope sem data: %s", stdout)
	}
	return data
}

// --all clona os seis tipos: um arquivo de cada tipo aparece no projeto e o
// envelope traz inventário + resultados.
func TestCloneAllJSON(t *testing.T) {
	stub := &cloneServerStub{}
	proj := cloneProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "clone", "--all", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, rel := range []string{
		"forms/Formulario de Teste/Formulario de Teste.html",
		"forms/Form Sem Local/Formulario de Teste.html",
		"datasets/ds_exemplo.js",
		"workflow/scripts/Compras.beforeTaskSave.js",
		"workflow/scripts/Compras.afterProcessFinish.js",
		"workflow/scripts/Compras.validateForm.js",
		"events/beforeConvertViewToPDF.js",
		"events/displayCustomThemes.js",
		"mechanisms/mec_gestor_area.js",
		"wcm/widget/meu_widget/src/main/webapp/resources/js/app.js",
		".fluigcli/forms.json",
	} {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(rel))); err != nil {
			t.Errorf("arquivo esperado não existe: %s", rel)
		}
	}

	data := cloneEnvelope(t, stdout)
	selected, _ := data["selected"].([]any)
	if len(selected) != len(cloneTypeDefs) {
		t.Errorf("selected = %v, quer os %d tipos", selected, len(cloneTypeDefs))
	}
	available, _ := data["available"].(map[string]any)
	wants := map[string]float64{"forms": 2, "datasets": 1, "workflows": 1, "events": 2, "mechanisms": 1, "widgets": 1}
	for key, want := range wants {
		if got, _ := available[key].(float64); got != want {
			t.Errorf("available.%s = %v, quer %v", key, available[key], want)
		}
	}
	results, _ := data["results"].(map[string]any)
	if wf, _ := results["workflows"].([]any); len(wf) != 3 {
		t.Errorf("results.workflows = %d itens, quer 3 (um por script)", len(wf))
	}
}

// --only limita aos tipos pedidos; os demais não tocam o disco.
func TestCloneOnly(t *testing.T) {
	stub := &cloneServerStub{}
	proj := cloneProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "clone", "--only", "forms,datasets", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(proj, "datasets", "ds_exemplo.js")); err != nil {
		t.Errorf("datasets/ds_exemplo.js deveria existir")
	}
	for _, rel := range []string{"events", "mechanisms", "wcm", "workflow"} {
		if _, err := os.Stat(filepath.Join(proj, rel)); !os.IsNotExist(err) {
			t.Errorf("pasta %q não deveria existir com --only forms,datasets", rel)
		}
	}
	data := cloneEnvelope(t, stdout)
	selected, _ := data["selected"].([]any)
	if len(selected) != 2 || selected[0] != "forms" || selected[1] != "datasets" {
		t.Errorf("selected = %v, quer [forms datasets]", selected)
	}
}

// Não-interativo sem --all/--only é erro de uso (exit 2), sem tocar o servidor.
func TestCloneNaoInterativoExigeSelecao(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, stdout := runMain(t, "clone", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
	}
}

// Tipo desconhecido em --only é erro de uso.
func TestCloneTipoInvalido(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, stdout := runMain(t, "clone", "--only", "bogus", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "bogus") {
		t.Errorf("mensagem deveria citar o tipo inválido: %+v", env.Error)
	}
}

// Pedir widgets explicitamente sem o helper = exit 7 com a orientação.
func TestCloneOnlyWidgetsSemHelper(t *testing.T) {
	stub := &cloneServerStub{helperMissing: true}
	proj := cloneProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "clone", "--only", "widgets", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitMissingHelper {
		t.Fatalf("exit=%d, quer %d; stdout=%s", code, output.ExitMissingHelper, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "install-helper") {
		t.Errorf("mensagem deveria orientar o install-helper: %+v", env.Error)
	}
}

// Com --all e o helper ausente, widgets são pulados com aviso e o resto segue.
func TestCloneAllSemHelperPulaWidgets(t *testing.T) {
	stub := &cloneServerStub{helperMissing: true}
	proj := cloneProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "clone", "--all", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(proj, "wcm")); !os.IsNotExist(err) {
		t.Errorf("wcm/ não deveria existir sem o helper")
	}
	if _, err := os.Stat(filepath.Join(proj, "datasets", "ds_exemplo.js")); err != nil {
		t.Errorf("os demais tipos deveriam ter sido clonados")
	}
	data := cloneEnvelope(t, stdout)
	unavailable, _ := data["unavailable"].(map[string]any)
	if _, ok := unavailable["widgets"]; !ok {
		t.Errorf("data.unavailable.widgets deveria estar presente: %v", data)
	}
	available, _ := data["available"].(map[string]any)
	if _, ok := available["widgets"]; ok {
		t.Errorf("available não deveria contar widgets sem o helper: %v", available)
	}
}

// Modo humano: o inventário sai em tabela antes da execução.
func TestCloneInventarioTabela(t *testing.T) {
	stub := &cloneServerStub{}
	proj := cloneProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "clone", "--all", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Formulários", "Datasets customizados", "Processos", "Eventos globais", "Mecanismos", "Widgets", "Itens"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela de inventário sem %q:\n%s", want, stdout)
		}
	}
}

// parseCloneTypes aceita nomes, singulares e números; deduplica e devolve na
// ordem canônica.
func TestParseCloneTypes(t *testing.T) {
	got, err := parseCloneTypes([]string{"widget", "1", "forms", "DATASETS"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"forms", "datasets", "widgets"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if _, err := parseCloneTypes([]string{"7"}); err == nil {
		t.Error("número fora da tabela deveria ser erro")
	}
	if _, err := parseCloneTypes([]string{"xyz"}); err == nil {
		t.Error("tipo desconhecido deveria ser erro")
	}
}
