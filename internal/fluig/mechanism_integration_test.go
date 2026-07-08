//go:build integration

package fluig

import (
	"context"
	"testing"
)

// Integração de mecanismos de atribuição (Fase 3). Opt-in via
// FLUIGCLI_TEST_*. A listagem é read-only e loga o controlClass real (útil
// para confirmar o template de criação). O ciclo create→update→delete usa um
// mecanismo com prefixo zz_fluigcli_test_.
func TestIntegrationMechanismList(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	mechs, err := c.ListMechanisms(context.Background())
	if err != nil {
		t.Fatalf("ListMechanisms: %v", err)
	}
	t.Logf("mecanismos customizados: %d", len(mechs))
	for _, m := range mechs {
		t.Logf("  - %s (%q) %d bytes de código", m.ID, m.Name, len(m.Code))
	}
}

func TestIntegrationMechanismCycle(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	const id = "zz_fluigcli_test_mec"
	// controlClass/assignmentType são fixos — CreateMechanism os aplica.
	if err := c.CreateMechanism(ctx, id, "fluigcli test", "mecanismo de teste", "function getUsers(){ return []; }"); err != nil {
		t.Fatalf("CreateMechanism: %v", err)
	}

	list, err := c.ListMechanisms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var created *Mechanism
	for i := range list {
		if list[i].ID == id {
			created = &list[i]
		}
	}
	if created == nil {
		t.Fatalf("mecanismo de teste %q não apareceu na lista após create", id)
	}

	novo := "function getUsers(){ return ['zz']; }"
	if err := c.UpdateMechanism(ctx, created, novo); err != nil {
		t.Fatalf("UpdateMechanism: %v", err)
	}

	if err := c.DeleteMechanism(ctx, id); err != nil {
		t.Errorf("DeleteMechanism (limpeza): %v", err)
	}
	t.Logf("ciclo create→update→delete ok para %q", id)
}
