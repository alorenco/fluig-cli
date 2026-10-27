package fluig

import (
	"strings"
	"testing"
)

func TestProcessFormID(t *testing.T) {
	full := testdata(t, "rest_process_export_full.xml")
	if got := ProcessFormID(full); got != 263801 {
		t.Errorf("ProcessFormID = %d, quer 263801", got)
	}
	if got := ProcessFormID([]byte(`<list/>`)); got != 0 {
		t.Errorf("sem formId deveria ser 0, veio %d", got)
	}
}

func TestRewriteProcessFormID(t *testing.T) {
	full := testdata(t, "rest_process_export_full.xml")
	out := RewriteProcessFormID(full, 408796)
	s := string(out)
	if !strings.Contains(s, "<formId>408796</formId>") {
		t.Errorf("formId não reescrito:\n%s", s[:500])
	}
	if strings.Contains(s, "263801") {
		t.Error("ID antigo ainda presente")
	}
}

func TestRewriteProcessDescription(t *testing.T) {
	full := testdata(t, "rest_process_export_full.xml")
	out := RewriteProcessDescription(full, "Invoice")
	s := string(out)
	if !strings.Contains(s, "<processDescription>Invoice</processDescription>") {
		t.Errorf("descrição não reescrita:\n%s", s[:500])
	}
	if strings.Contains(s, "<processDescription>Entrada de Documento</processDescription>") {
		t.Error("descrição antiga ainda presente na tag processDescription")
	}
	// Etapa homônima não muda: o rewrite toca só a tag processDescription.
	if !strings.Contains(s, "<stateName>Entrada de Documento</stateName>") {
		t.Error("etapa homônima não podia mudar")
	}
}

func TestRewriteProcessDescriptionEscape(t *testing.T) {
	out := RewriteProcessDescription([]byte(`<list><processDescription>A</processDescription></list>`), "Teste & Cia <x>")
	if got := string(out); got != `<list><processDescription>Teste &amp; Cia &lt;x&gt;</processDescription></list>` {
		t.Errorf("texto não escapado: %q", got)
	}
}
