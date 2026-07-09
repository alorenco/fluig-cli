package fluig

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// processDefinitionXML reproduz a estrutura observada no export da homologação
// (SPEC §5.7): um <ProcessDefinition> com os scripts em <WorkflowProcessEvent>
// (workflowProcessEventPK{…, eventId} + eventDescription = código JS). Traz
// eventos de duas versões para validar que só a mais alta prevalece.
const processDefinitionXML = `<?xml version="1.0" encoding="UTF-8"?>
<ProcessDefinition>
  <processId>Compras</processId>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <companyId>1</companyId>
      <processId>Compras</processId>
      <version>4</version>
      <eventId>beforeTaskSave</eventId>
    </workflowProcessEventPK>
    <eventDescription>function beforeTaskSave(){ /* versao antiga */ }</eventDescription>
  </WorkflowProcessEvent>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <companyId>1</companyId>
      <processId>Compras</processId>
      <version>5</version>
      <eventId>beforeTaskSave</eventId>
    </workflowProcessEventPK>
    <eventDescription>function beforeTaskSave(){ /* codigo A */ }</eventDescription>
  </WorkflowProcessEvent>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <companyId>1</companyId>
      <processId>Compras</processId>
      <version>5</version>
      <eventId>afterProcessFinish</eventId>
    </workflowProcessEventPK>
    <eventDescription>function afterProcessFinish(){ /* codigo B */ }</eventDescription>
  </WorkflowProcessEvent>
</ProcessDefinition>`

// zipWithXML monta um zip em memória com o XML de definição (formato do export).
func zipWithXML(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseProcessEventScripts(t *testing.T) {
	events, err := parseProcessEventScripts(zipWithXML(t, "Compras.xml", processDefinitionXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("esperava 2 eventos da versão 5, veio %d: %v", len(events), events)
	}
	if got := events["beforeTaskSave"]; got != "function beforeTaskSave(){ /* codigo A */ }" {
		t.Errorf("beforeTaskSave deveria vir da versão 5 (a mais alta), veio: %q", got)
	}
	if got := events["afterProcessFinish"]; got != "function afterProcessFinish(){ /* codigo B */ }" {
		t.Errorf("afterProcessFinish inesperado: %q", got)
	}
}

func TestParseProcessEventScriptsSemEventos(t *testing.T) {
	events, err := parseProcessEventScripts(zipWithXML(t, "Vazio.xml", `<ProcessDefinition/>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("processo sem eventos deveria dar mapa vazio, veio %v", events)
	}
}

// Regressão da homologação (2026-07-09): o servidor exporta o XML em
// ISO-8859-1 — acentos viravam "invalid UTF-8" no decoder. O parser precisa
// normalizar para UTF-8 e aceitar o encoding declarado.
func TestParseProcessEventScriptsISO88591(t *testing.T) {
	xmlLatin1 := `<?xml version="1.0" encoding="ISO-8859-1"?>
<ProcessDefinition>
  <WorkflowProcessEvent>
    <workflowProcessEventPK>
      <version>2</version><eventId>beforeTaskSave</eventId>
    </workflowProcessEventPK>
    <eventDescription>function beforeTaskSave(){ /* aprova` + "\xe7\xe3o" + ` */ }</eventDescription>
  </WorkflowProcessEvent>
</ProcessDefinition>`
	events, err := parseProcessEventScripts(zipWithXML(t, "P.xml", xmlLatin1))
	if err != nil {
		t.Fatalf("XML ISO-8859-1 deveria ser aceito: %v", err)
	}
	if got := events["beforeTaskSave"]; !strings.Contains(got, "aprovação") {
		t.Errorf("acentos Latin-1 não convertidos: %q", got)
	}
}

func TestParseProcessEventScriptsZipInvalido(t *testing.T) {
	if _, err := parseProcessEventScripts([]byte("não é zip")); err == nil {
		t.Error("zip inválido deveria dar erro")
	}
	if _, err := parseProcessEventScripts(zipWithXML(t, "so.txt", "x")); err == nil {
		t.Error("zip sem XML deveria dar erro")
	}
}
