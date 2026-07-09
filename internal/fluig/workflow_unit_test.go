package fluig

import (
	"archive/zip"
	"bytes"
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

func TestParseProcessEventScriptsZipInvalido(t *testing.T) {
	if _, err := parseProcessEventScripts([]byte("não é zip")); err == nil {
		t.Error("zip inválido deveria dar erro")
	}
	if _, err := parseProcessEventScripts(zipWithXML(t, "so.txt", "x")); err == nil {
		t.Error("zip sem XML deveria dar erro")
	}
}
