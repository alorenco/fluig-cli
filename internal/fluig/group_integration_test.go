//go:build integration

package fluig

import (
	"context"
	"errors"
	"testing"
)

// Ciclo completo de grupo contra a homologação (ROADMAP §2.2). Opt-in via
// FLUIGCLI_TEST_* (ver integration_test.go). Cria um grupo com prefixo
// zz_fluigcli_test_, atualiza, adiciona/remove o próprio usuário e apaga o
// grupo ao final — não deixa resíduo (diferente do dataset, grupo tem DELETE).
func TestIntegrationGroupCycle(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// list (read-only)
	groups, err := c.ListGroups(ctx, GroupFilter{})
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	t.Logf("listados %d grupos", len(groups))

	const code = "zz_fluigcli_test_grp"
	// Limpa resquício de uma rodada anterior interrompida.
	if _, err := c.GetGroup(ctx, code); err == nil {
		_ = c.DeleteGroup(ctx, code)
	}

	// create
	g, err := c.CreateGroup(ctx, code, "Grupo de teste fluigcli", "user")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g.Code != code || g.Type != "user" {
		t.Errorf("grupo criado inesperado: %+v", g)
	}

	// get
	got, err := c.GetGroup(ctx, code)
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if got.Description != "Grupo de teste fluigcli" {
		t.Errorf("descrição inesperada: %q", got.Description)
	}

	// update (mescla: type sobrevive)
	up, err := c.UpdateGroup(ctx, code, map[string]string{"description": "Descrição alterada"})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if up.Description != "Descrição alterada" || up.Type != "user" {
		t.Errorf("update não mesclou: %+v", up)
	}

	// add-user (o próprio usuário do teste) + verificação
	if err := c.AddGroupUser(ctx, code, opts.Username); err != nil {
		t.Fatalf("AddGroupUser: %v", err)
	}
	members, err := c.ListGroupUsers(ctx, code, 0)
	if err != nil {
		t.Fatalf("ListGroupUsers: %v", err)
	}
	found := false
	for _, m := range members {
		if m.Login == opts.Username {
			found = true
		}
	}
	if !found {
		t.Errorf("usuário %q não apareceu entre os %d membros", opts.Username, len(members))
	}

	// add-user com grupo inexistente = exit 4 (pré-validação)
	if err := c.AddGroupUser(ctx, "zz_fluigcli_test_inexistente", opts.Username); !errors.Is(err, ErrNotFound) {
		t.Errorf("add em grupo inexistente deveria dar ErrNotFound, veio %v", err)
	}

	// remove-user
	if err := c.RemoveGroupUser(ctx, code, opts.Username); err != nil {
		t.Fatalf("RemoveGroupUser: %v", err)
	}
	// remove de novo (não-membro) = ErrNotFound
	if err := c.RemoveGroupUser(ctx, code, opts.Username); !errors.Is(err, ErrNotFound) {
		t.Errorf("remove de não-membro deveria dar ErrNotFound, veio %v", err)
	}

	// delete + confirmação 404
	if err := c.DeleteGroup(ctx, code); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if _, err := c.GetGroup(ctx, code); !errors.Is(err, ErrNotFound) {
		t.Errorf("grupo deveria ter sumido após delete, veio %v", err)
	}
	t.Log("ciclo create→get→update→add/remove-user→delete ok; sem resíduo")
}
