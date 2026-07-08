package soap

import (
	"strings"
	"testing"
)

func TestParseListForms(t *testing.T) {
	docs, err := ParseListForms(fixture(t, "soap_listForms.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("esperava 1 formulário, veio %d", len(docs))
	}
	d := docs[0]
	if d.DocumentID != 42 || d.DocumentDescription != "Formulario de Teste" || d.DatasetName != "ds_teste" || d.Version != 3 {
		t.Errorf("documento inesperado: %+v", d)
	}
}

func TestParseAttachmentsList(t *testing.T) {
	names, err := ParseAttachmentsList(fixture(t, "soap_attachmentsList.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "Formulario de Teste.html" || names[1] != "script.js" {
		t.Errorf("anexos inesperados: %v", names)
	}
}

func TestParseCardIndexContent(t *testing.T) {
	b64, err := ParseCardIndexContent(fixture(t, "soap_cardContent.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b64) != "Y29udGV1ZG8=" {
		t.Errorf("base64 inesperado: %q", b64)
	}
}

func TestParseCustomizationEvents(t *testing.T) {
	events, err := ParseCustomizationEvents(fixture(t, "soap_customEvents.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "onNotify" || !strings.Contains(events[0].EventDescription, "codigo") {
		t.Errorf("eventos inesperados: %+v", events)
	}
}

func TestParseWriteForm(t *testing.T) {
	res, err := ParseWriteForm(fixture(t, "soap_writeForm.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Message != "ok" || res.DocumentID != 99 {
		t.Errorf("resultado inesperado: %+v", res)
	}
}

// A ordem RPC dos parâmetros do create deve seguir o WSDL.
func TestBuildCreateFormOrder(t *testing.T) {
	body, err := BuildCreateForm(CreateFormParams{
		CompanyID: 1, Username: "u", Password: "p", PublisherID: "uc",
		ParentDocumentID: 5, DocumentDescription: "Form X", CardDescription: "titulo",
		DatasetName: "ds", PersistenceType: 1,
		Attachments:  []Attachment{{FileName: "Form X.html", FileContent: "AAA", Principal: true}},
		CustomEvents: []CardEvent{{EventID: "onLoad", EventDescription: "code"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// parentDocumentId antes de publisherId antes de documentDescription (ordem WSDL).
	iParent := strings.Index(s, "<parentDocumentId>")
	iPub := strings.Index(s, "<publisherId>")
	iDoc := strings.Index(s, "<documentDescription>")
	if iParent >= iPub || iPub >= iDoc {
		t.Errorf("ordem dos parâmetros RPC incorreta: parent=%d pub=%d doc=%d", iParent, iPub, iDoc)
	}
	if !strings.Contains(s, "<Attachments><item>") || !strings.Contains(s, "<principal>true</principal>") {
		t.Errorf("anexo não serializado corretamente:\n%s", s)
	}
}
