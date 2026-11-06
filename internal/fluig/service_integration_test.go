//go:build integration

package fluig

import (
	"context"
	"errors"
	"testing"
)

// Integração do cadastro de serviços (ROADMAP §5.20). Opt-in via
// FLUIGCLI_TEST_*. O ciclo create→duplicado→update→delete usa um serviço com
// prefixo zz_fluigcli_test_ e remove o artefato ao final.
func TestIntegrationServiceCycle(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const code = "zz_fluigcli_test_svc"
	t.Cleanup(func() { _ = c.DeleteService(context.Background(), code) })

	list, err := c.ListServices(ctx, ServiceFilter{Types: []string{ServiceTypeREST}})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	t.Logf("serviços REST: %d", len(list))

	if err := c.CreateService(ctx, code, ServiceInput{URL: strPtr("https://example.com/api"), TestPath: strPtr("/health")}); err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if err := c.CreateService(ctx, code, ServiceInput{URL: strPtr("https://example.com")}); !errors.Is(err, ErrServiceExists) {
		t.Errorf("create duplicado: esperava ErrServiceExists, veio %v", err)
	}
	cur, err := c.GetService(ctx, code)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if cur.URL != "https://example.com/api" || cur.AuthType != ServiceAuthNone || cur.TestPath != "/health" {
		t.Errorf("serviço criado inesperado: %+v", cur)
	}
	if err := c.UpdateService(ctx, cur, ServiceInput{URL: strPtr("https://example.org/v2")}); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}
	cur, err = c.GetService(ctx, code)
	if err != nil || cur.URL != "https://example.org/v2" {
		t.Errorf("update não refletiu: %+v, %v", cur, err)
	}
	if err := c.DeleteService(ctx, code); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	if _, err := c.GetService(ctx, code); !errors.Is(err, ErrNotFound) {
		t.Errorf("serviço ainda existe após delete: %v", err)
	}
}

func strPtr(s string) *string { return &s }
