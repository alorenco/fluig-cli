//go:build integration

package fluig

import (
	"context"
	"testing"
)

// TestIntegrationListWidgetsNative confirma a listagem nativa de widgets
// (page-management/applications, read-only) — o fallback do widget list.
func TestIntegrationListWidgetsNative(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	widgets, err := c.ListWidgetsNative(context.Background())
	if err != nil {
		t.Fatalf("ListWidgetsNative: %v", err)
	}
	t.Logf("%d widget(s) na listagem nativa", len(widgets))
	if len(widgets) == 0 {
		t.Error("nenhum widget na listagem nativa — esperado ao menos 1 na homologação")
	}
	for _, w := range widgets {
		if w.Code == "" {
			t.Errorf("widget sem code: %+v", w)
		}
	}
}
