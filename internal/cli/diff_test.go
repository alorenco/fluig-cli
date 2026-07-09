package cli

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// diffListFormsXML segue o formato real de soap_listForms.xml, com um segundo
// formulário que não existe localmente (only-server).
const diffListFormsXML = `<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <ns2:getCardIndexesWithoutApproverResponse xmlns:ns2="http://ws.dm.ecm.technology.totvs.com/">
      <result>
        <item>
          <documentId>42</documentId>
          <documentDescription>Formulario de Teste</documentDescription>
          <datasetName>ds_teste</datasetName>
          <version>3</version>
          <cardDescription>titulo</cardDescription>
        </item>
        <item>
          <documentId>77</documentId>
          <documentDescription>Form Sem Local</documentDescription>
          <datasetName>ds_outro</datasetName>
          <version>1</version>
          <cardDescription>t2</cardDescription>
        </item>
      </result>
    </ns2:getCardIndexesWithoutApproverResponse>
  </soap:Body>
</soap:Envelope>`

// comprasProcessXML segue a estrutura do export nativo (SPEC §5.7).
const comprasProcessXML = `<?xml version="1.0" encoding="UTF-8"?>
<ProcessDefinition>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <companyId>1</companyId><processId>Compras</processId>
      <version>5</version><eventId>beforeTaskSave</eventId>
    </workflowProcessEventPK>
    <eventDescription>function beforeTaskSave(){ /* codigo A */ }</eventDescription>
  </WorkflowProcessEvent>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <companyId>1</companyId><processId>Compras</processId>
      <version>5</version><eventId>afterProcessFinish</eventId>
    </workflowProcessEventPK>
    <eventDescription>function afterProcessFinish(){ /* codigo B */ }</eventDescription>
  </WorkflowProcessEvent>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <companyId>1</companyId><processId>Compras</processId>
      <version>5</version><eventId>validateForm</eventId>
    </workflowProcessEventPK>
    <eventDescription>function validateForm(){ /* so servidor */ }</eventDescription>
  </WorkflowProcessEvent>
</ProcessDefinition>`

// wfExportEnvelope embrulha o zip (base64) na resposta SOAP do export.
func wfExportEnvelope(b64 string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>` +
		`<ns2:exportProcessInZipFormatResponse xmlns:ns2="http://ws.workflow.ecm.technology.totvs.com/">` +
		`<result>` + b64 + `</result>` +
		`</ns2:exportProcessInZipFormatResponse></soap:Body></soap:Envelope>`
}

// processZipBase64 monta o zip do export (um XML de definição) em base64.
func processZipBase64(t *testing.T, xmlContent string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("Compras.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(xmlContent)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// diffServerStub simula o servidor com os cinco tipos de artefato, usando as
// fixtures reais de testdata: dataset ds_exemplo (CUSTOM) + colleague
// (DEFAULT), eventos beforeConvertViewToPDF e displayCustomThemes, mecanismo
// mec_gestor_area, formulário 42 (anexos + evento onNotify) e o processo
// Compras (export nativo).
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
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":{"login":"u","userCode":"uc"}}`)
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
	mux.HandleFunc("/webdesk/ECMCardIndexService", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		switch r.Header.Get("SOAPAction") {
		case "getCardIndexesWithoutApprover":
			io.WriteString(w, diffListFormsXML)
		case "getAttachmentsList":
			_, _ = w.Write(readTD("soap_attachmentsList.xml"))
		case "getCardIndexContent":
			_, _ = w.Write(readTD("soap_cardContent.xml")) // "conteudo" em base64
		case "getCustomizationEvents":
			_, _ = w.Write(readTD("soap_customEvents.xml"))
		default:
			http.Error(w, "op?", 500)
		}
	})
	mux.HandleFunc("/webdesk/ECMWorkflowEngineService", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		if r.Header.Get("SOAPAction") != "exportProcess" {
			http.Error(w, "op?", 500)
			return
		}
		if strings.Contains(string(body), ">Compras<") {
			io.WriteString(w, wfExportEnvelope(processZipBase64(t, comprasProcessXML)))
			return
		}
		// Processo inexistente: export vazio → ErrNotFound no cliente.
		io.WriteString(w, wfExportEnvelope(""))
	})
	// Processos do servidor (REST v2): Compras tem scripts locais no projeto;
	// SoServidor não tem nenhum → only-server na varredura.
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[`+
			`{"processId":"Compras","processDescription":"Compras","active":true,"categoryId":"Suprimentos","public":false},`+
			`{"processId":"SoServidor","processDescription":"Só no Servidor","active":true,"public":false}`+
			`],"hasNext":false}`)
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

	// Formulário 42, vinculado à pasta meu_form pelo mapeamento: o html difere
	// do servidor ("conteudo"); o anexo script.js não existe localmente
	// (only-server); onNotify é igual; novoEvento é only-local.
	write(".fluigcli/forms.json",
		`{"version":"1.0.0","forms":[{"folder":"meu_form","documentId":42,"name":"Formulario de Teste"}]}`)
	write("forms/meu_form/Formulario de Teste.html", "conteudo NOVO")
	write("forms/meu_form/events/onNotify.js", "function onNotify(){ /* codigo */ }")
	write("forms/meu_form/events/novoEvento.js", "function novo(){}")
	// Pasta de formulário sem contraparte no servidor.
	write("forms/SoLocal/index.html", "<html></html>")

	// Scripts do processo Compras: um modificado, um igual (CRLF), um só local.
	write("workflow/scripts/Compras.beforeTaskSave.js",
		"function beforeTaskSave(){ /* codigo NOVO */ }")
	write("workflow/scripts/Compras.afterProcessFinish.js",
		"function afterProcessFinish(){ /* codigo B */ }\r\n")
	write("workflow/scripts/Compras.naoTem.js", "function naoTem(){}")
	// Processo que não existe no servidor.
	write("workflow/scripts/Fantasma.beforeTaskSave.js", "function beforeTaskSave(){}")
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
		"dataset ds_exemplo":                     "equal",
		"event beforeConvertViewToPDF":           "modified",
		"event displayCustomThemes":              "only-server",
		"mechanism mec_gestor_area":              "equal",
		"mechanism novo":                         "only-local",
		"form meu_form/Formulario de Teste.html": "modified",
		"form meu_form/script.js":                "only-server",
		"form meu_form/events/onNotify.js":       "equal",
		"form meu_form/events/novoEvento.js":     "only-local",
		"form SoLocal":                           "only-local",
		"form Form Sem Local":                    "only-server",
		"workflow Compras.beforeTaskSave":        "modified",
		"workflow Compras.afterProcessFinish":    "equal",
		"workflow Compras.naoTem":                "only-local",
		"workflow Compras.validateForm":          "only-server",
		"workflow Fantasma.beforeTaskSave":       "only-local",
		"workflow SoServidor":                    "only-server",
	}
	got := map[string]string{}
	for _, a := range arts {
		m, _ := a.(map[string]any)
		got[fmt.Sprintf("%v %v", m["type"], m["id"])] = fmt.Sprintf("%v", m["status"])
	}
	if len(got) != len(want) {
		t.Errorf("veio %d artefatos, quer %d:\n%s", len(got), len(want), stdout)
	}
	for key, status := range want {
		if got[key] != status {
			t.Errorf("%s: status = %q, quer %q", key, got[key], status)
		}
	}

	// Diffs unificados dos modificados.
	for _, a := range arts {
		m, _ := a.(map[string]any)
		key := fmt.Sprintf("%v %v", m["type"], m["id"])
		d, _ := m["diff"].(string)
		switch key {
		case "event beforeConvertViewToPDF":
			if !strings.Contains(d, "-function beforeConvertViewToPDF(){ /* codigo A */ }") ||
				!strings.Contains(d, "+function beforeConvertViewToPDF(){ /* codigo NOVO */ }") {
				t.Errorf("diff unificado inesperado:\n%s", d)
			}
		case "form meu_form/Formulario de Teste.html":
			if !strings.Contains(d, "-conteudo") || !strings.Contains(d, "+conteudo NOVO") {
				t.Errorf("diff do anexo inesperado:\n%s", d)
			}
		case "workflow Compras.beforeTaskSave":
			if !strings.Contains(d, "-function beforeTaskSave(){ /* codigo A */ }") ||
				!strings.Contains(d, "+function beforeTaskSave(){ /* codigo NOVO */ }") {
				t.Errorf("diff do script inesperado:\n%s", d)
			}
		}
	}

	counts, _ := data["counts"].(map[string]any)
	if counts["equal"] != float64(4) || counts["modified"] != float64(3) ||
		counts["only-local"] != float64(5) || counts["only-server"] != float64(5) {
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

// diff de uma pasta de formulário: compara os arquivos da pasta (incluindo os
// que o export removeria do servidor), mas não lista outros formulários.
func TestDiffFormPasta(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, stdout := runMain(t, "diff", filepath.Join(proj, "forms", "meu_form"),
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
	want := map[string]string{
		"meu_form/Formulario de Teste.html": "modified",
		"meu_form/script.js":                "only-server",
		"meu_form/events/onNotify.js":       "equal",
		"meu_form/events/novoEvento.js":     "only-local",
	}
	if len(arts) != len(want) {
		t.Fatalf("veio %d artefatos, quer %d: %s", len(arts), len(want), stdout)
	}
	for _, a := range arts {
		m, _ := a.(map[string]any)
		id := fmt.Sprintf("%v", m["id"])
		if fmt.Sprintf("%v", m["status"]) != want[id] {
			t.Errorf("%s: status = %v, quer %v", id, m["status"], want[id])
		}
	}
	if strings.Contains(stdout, "Form Sem Local") {
		t.Error("diff de uma pasta não deveria listar outros formulários do servidor")
	}
}

// diff de um único script de processo: só ele, sem only-server dos demais eventos.
func TestDiffWorkflowScriptUnico(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, stdout := runMain(t, "diff",
		filepath.Join(proj, "workflow", "scripts", "Compras.beforeTaskSave.js"),
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
		t.Fatalf("veio %d artefatos, quer 1: %s", len(arts), stdout)
	}
	m, _ := arts[0].(map[string]any)
	if m["id"] != "Compras.beforeTaskSave" || m["status"] != "modified" {
		t.Errorf("artefato inesperado: %#v", m)
	}
}

func TestDiffCaminhoForaDaConvencao(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, _ := runMain(t, "diff", filepath.Join(proj, "outra", "x.js"), "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("caminho fora da convenção: exit = %d, quer %d", code, output.ExitUsage)
	}
}

// Conteúdo binário: byte a byte, sem diff textual.
func TestFillContentDiffBinario(t *testing.T) {
	bin1 := []byte{0x89, 'P', 'N', 'G', 0}
	bin2 := []byte{0x89, 'P', 'N', 'G', 0, 1}

	var e diffEntry
	fillContentDiff(&e, "s", "l", bin1, append([]byte(nil), bin1...))
	if e.Status != diffEqual {
		t.Errorf("binários iguais: status = %q", e.Status)
	}
	e = diffEntry{}
	fillContentDiff(&e, "s", "l", bin1, bin2)
	if e.Status != diffModified || e.Diff != "" {
		t.Errorf("binários diferentes: status = %q, diff = %q (deveria ser modified sem diff)", e.Status, e.Diff)
	}
	// Texto com CRLF vs LF: igual.
	e = diffEntry{}
	fillContentDiff(&e, "s", "l", []byte("a\r\nb\r\n"), []byte("a\nb"))
	if e.Status != diffEqual {
		t.Errorf("texto CRLF/LF: status = %q, quer equal", e.Status)
	}
}

// Modo humano: mensagens orientam o caminho de volta por tipo.
func TestDiffVarreduraModoHumano(t *testing.T) {
	stub := diffServerStub(t)
	proj := diffProject(t, stub.URL)

	code, stdout := runMain(t, "diff", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d; stdout=%s", code, stdout)
	}
	for _, want := range []string{
		"── event beforeConvertViewToPDF difere do servidor:",
		"+function beforeConvertViewToPDF(){ /* codigo NOVO */ }",
		"── form Form Sem Local só existe no servidor — importe com: fluigcli form import \"Form Sem Local\"",
		"── form meu_form/script.js só existe no servidor — o export da pasta o removeria",
		"── workflow Compras.validateForm só existe no servidor — crie workflow/scripts/Compras.validateForm.js",
		"── workflow SoServidor (processo) não tem scripts locais — se ele tiver eventos, versione-os em workflow/scripts/SoServidor.<evento>.js",
		"4 igual(is), 3 diferente(s), 5 só local(is), 5 só no servidor",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída humana sem %q:\n%s", want, stdout)
		}
	}
}
