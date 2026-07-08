//go:build integration

package fluig

import (
	"context"
	"testing"
)

// Integração de formulários (Fase 4). Opt-in via FLUIGCLI_TEST_*.
// A listagem e o download são read-only. colleagueId = FLUIGCLI_TEST_USERCODE
// (ou o username, se não definido).
func TestIntegrationFormListAndDownload(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	colleague, err := c.ResolveUserCode(ctx)
	if err != nil {
		t.Fatalf("ResolveUserCode: %v", err)
	}
	t.Logf("colleagueId (userCode) = %q", colleague)

	forms, err := c.ListForms(ctx, colleague)
	if err != nil {
		t.Fatalf("ListForms: %v", err)
	}
	t.Logf("formulários no servidor: %d", len(forms))
	if len(forms) == 0 {
		t.Skip("nenhum formulário no servidor para exercitar o download")
	}
	f := forms[0]
	t.Logf("primeiro: documentId=%d %q dataset=%s v%d", f.DocumentID, f.Description, f.DatasetName, f.Version)

	names, err := c.FormAttachments(ctx, f.DocumentID)
	if err != nil {
		t.Fatalf("FormAttachments: %v", err)
	}
	t.Logf("anexos de %q: %v", f.Description, names)
	if len(names) > 0 {
		file, err := c.DownloadFormFile(ctx, f.DocumentID, colleague, f.Version, names[0])
		if err != nil {
			t.Fatalf("DownloadFormFile: %v", err)
		}
		t.Logf("baixado %q: %d bytes", file.Name, len(file.Content))
		if len(file.Content) == 0 {
			t.Errorf("conteúdo vazio para %q", file.Name)
		}
	}

	events, err := c.FormEvents(ctx, f.DocumentID)
	if err != nil {
		t.Fatalf("FormEvents: %v", err)
	}
	t.Logf("eventos de %q: %d", f.Description, len(events))
}
