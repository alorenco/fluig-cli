//go:build integration

package fluig

import (
	"context"
	"errors"
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

// TestIntegrationFindLayout valida o preflight de colisão do `widget export`
// (ROADMAP2 §3.1) contra o servidor real, read-only:
//   - a listagem responde e traz layouts com código;
//   - o GET por código encontra um layout real;
//   - o GET de um código inexistente NÃO devolve o layout (o comportamento
//     esperado é 404, mas o fallback pela listagem cobre o quirk de 500).
func TestIntegrationFindLayout(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	layouts, err := c.ListLayouts(ctx)
	if err != nil {
		t.Fatalf("ListLayouts: %v", err)
	}
	t.Logf("%d layout(s) no servidor", len(layouts))
	if len(layouts) == 0 {
		t.Fatal("nenhum layout — esperado ao menos 1 (a plataforma traz layouts internos)")
	}
	for _, l := range layouts {
		t.Logf("layout: code=%q title=%q internal=%v", l.Code, l.Title, l.Internal)
		if l.Code == "" {
			t.Errorf("layout sem code: %+v", l)
		}
	}

	found, err := c.FindLayout(ctx, layouts[0].Code)
	if err != nil {
		t.Fatalf("FindLayout(%q): %v", layouts[0].Code, err)
	}
	if found.Code != layouts[0].Code {
		t.Errorf("FindLayout devolveu %q para a busca de %q", found.Code, layouts[0].Code)
	}

	const inexistente = "zz_fluigcli_test_layout_inexistente"
	if _, err := c.FindLayout(ctx, inexistente); !errors.Is(err, ErrNotFound) {
		t.Errorf("código inexistente devia dar ErrNotFound (senão o widget export travaria sem motivo), veio %v", err)
	}
}
