//go:build integration

package fluig

import (
	"context"
	"errors"
	"os"
	"testing"
)

// Integração de páginas (ROADMAP §5.21). Opt-in via FLUIGCLI_TEST_*. Usa uma
// página com prefixo zz_fluigcli_test_ e remove o artefato ao final. A widget
// vem de FLUIGCLI_TEST_PAGE_WIDGET (uma widget já publicada no servidor).
func TestIntegrationPageCycle(t *testing.T) {
	widget := os.Getenv("FLUIGCLI_TEST_PAGE_WIDGET")
	if widget == "" {
		t.Skip("defina FLUIGCLI_TEST_PAGE_WIDGET com uma widget publicada")
	}
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const code = "zz_fluigcli_test_page"
	t.Cleanup(func() { _ = c.DeletePage(context.Background(), code) })

	err = c.CreatePage(ctx, PageCreateInput{Code: code, Title: "ZZ fluigcli test", Description: "teste", Layout: "layoutdefault", Permissions: c.DefaultPagePermissions()})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if err := c.CreatePage(ctx, PageCreateInput{Code: code, Title: "x", Description: "x", Layout: "layoutdefault"}); !errors.Is(err, ErrPageExists) {
		t.Errorf("create duplicado: esperava ErrPageExists, veio %v", err)
	}
	if _, err := c.PageWidgets(ctx, code); !errors.Is(err, ErrPageNotPublished) {
		t.Errorf("página nova: esperava ErrPageNotPublished, veio %v", err)
	}
	if _, err := c.AddPageWidget(ctx, code, "SlotA", widget); err != nil {
		t.Fatalf("AddPageWidget: %v", err)
	}
	if err := c.PublishPage(ctx, code, "teste"); err != nil {
		t.Fatalf("PublishPage: %v", err)
	}
	ws, err := c.PageWidgets(ctx, code)
	if err != nil || len(ws) != 1 || ws[0].Widget != widget {
		t.Fatalf("widgets publicadas: %+v, %v", ws, err)
	}
	if err := c.SetWidgetPreferences(ctx, ws[0].InstanceID, map[string]string{"zz": "1"}); err != nil {
		t.Fatalf("SetWidgetPreferences: %v", err)
	}
	if prefs, err := c.WidgetPreferences(ctx, ws[0].InstanceID); err != nil || prefs["zz"] != "1" {
		t.Errorf("preferências: %+v, %v", prefs, err)
	}
	// Rascunho: ids próprios, move no rascunho, update, publish.
	if err := c.CreatePageDraft(ctx, code); err != nil {
		t.Fatalf("CreatePageDraft: %v", err)
	}
	draft, err := c.DraftWidgets(ctx, mustPageID(t, c, code), []string{"SlotA", "SlotB"})
	if err != nil || len(draft) != 1 || draft[0].InstanceID == ws[0].InstanceID {
		t.Fatalf("DraftWidgets: %+v, %v", draft, err)
	}
	if err := c.MovePageWidget(ctx, code, draft[0].InstanceID, "SlotB", 1); err != nil {
		t.Fatalf("MovePageWidget: %v", err)
	}
	p, err := c.GetPage(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	perms, err := c.PagePermissions(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	title := "ZZ fluigcli test 2"
	if err := c.UpdatePage(ctx, p, PageUpdate{Title: &title}, perms); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	if err := c.PublishPage(ctx, code, "teste 2"); err != nil {
		t.Fatalf("PublishPage 2: %v", err)
	}
	ws, err = c.PageWidgets(ctx, code)
	if err != nil || len(ws) != 1 || ws[0].Slot != "SlotB" {
		t.Fatalf("widgets depois do move: %+v, %v", ws, err)
	}
	if p, err = c.GetPage(ctx, code); err != nil || p.Title != title {
		t.Errorf("título depois do update: %+v, %v", p, err)
	}
	if prefs, err := c.WidgetPreferences(ctx, ws[0].InstanceID); err != nil || prefs["zz"] != "1" {
		t.Errorf("preferências não sobreviveram ao publish: %+v, %v", prefs, err)
	}
	if err := c.RemovePageWidget(ctx, code, ws[0].InstanceID); err != nil {
		t.Fatalf("RemovePageWidget: %v", err)
	}
	if err := c.DeletePage(ctx, code); err != nil {
		t.Fatalf("DeletePage: %v", err)
	}
	if _, err := c.GetPage(ctx, code); !errors.Is(err, ErrNotFound) {
		t.Errorf("página ainda existe após delete: %v", err)
	}
}

func mustPageID(t *testing.T, c *Client, code string) int {
	t.Helper()
	p, err := c.GetPage(context.Background(), code)
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}
