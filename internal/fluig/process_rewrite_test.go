package fluig

import (
	"strings"
	"testing"
)

func TestRewriteProcessXMLID(t *testing.T) {
	xml := []byte(`<list><ProcessDefinition><processDefinitionPK><processId>antigo</processId></processDefinitionPK></ProcessDefinition><WorkflowProcessEvent><workflowProcessEventPK><processId>antigo</processId><eventId>beforeTaskSave</eventId></workflowProcessEventPK></WorkflowProcessEvent></list>`)
	out := RewriteProcessXMLID(xml, "NovoProc")
	s := string(out)
	if !strings.Contains(s, "<processId>NovoProc</processId>") {
		t.Errorf("processId não reescrito:\n%s", s)
	}
	if strings.Contains(s, ">antigo<") {
		t.Errorf("ID antigo ainda presente:\n%s", s)
	}
	if !strings.Contains(s, "<eventId>beforeTaskSave</eventId>") {
		t.Errorf("evento foi alterado:\n%s", s)
	}

	// Sem tag processId: devolve igual.
	sem := []byte(`<list/>`)
	if got := string(RewriteProcessXMLID(sem, "X")); got != `<list/>` {
		t.Errorf("sem tag deveria devolver igual, veio %q", got)
	}
}
