//go:build integration

package fluig

import (
	"context"
	"errors"
	"testing"
)

// Ciclo completo de papel contra a homologação (ROADMAP §2.2). Opt-in via
// FLUIGCLI_TEST_* (ver integration_test.go). Cria um papel zz_fluigcli_test_,
// atualiza, vincula/desvincula o próprio usuário e apaga ao final (sem
// resíduo).
func TestIntegrationRoleCycle(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	roles, err := c.ListRoles(ctx, RoleFilter{})
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	t.Logf("listados %d papéis", len(roles))

	const code = "zz_fluigcli_test_role"
	if _, err := c.GetRole(ctx, code); err == nil {
		_ = c.DeleteRole(ctx, code)
	}

	// create (description omitida no cliente é o próprio code; aqui passamos)
	r, err := c.CreateRole(ctx, code, "Papel de teste fluigcli")
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if r.Code != code {
		t.Errorf("papel criado inesperado: %+v", r)
	}

	// update (mescla)
	up, err := c.UpdateRole(ctx, code, map[string]string{"description": "Descrição alterada"})
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if up.Description != "Descrição alterada" {
		t.Errorf("update não mesclou: %+v", up)
	}

	// add-user + verificação
	if err := c.AddRoleUser(ctx, code, opts.Username); err != nil {
		t.Fatalf("AddRoleUser: %v", err)
	}
	users, err := c.ListRoleUsers(ctx, code, 0)
	if err != nil {
		t.Fatalf("ListRoleUsers: %v", err)
	}
	found := false
	for _, u := range users {
		if u.Login == opts.Username {
			found = true
		}
	}
	if !found {
		t.Errorf("usuário %q não apareceu entre os %d vinculados", opts.Username, len(users))
	}

	// papel inexistente = ErrNotFound (pré-validação)
	if err := c.AddRoleUser(ctx, "zz_fluigcli_test_inexistente", opts.Username); !errors.Is(err, ErrNotFound) {
		t.Errorf("add em papel inexistente deveria dar ErrNotFound, veio %v", err)
	}

	// remove-user + não-vinculado
	if err := c.RemoveRoleUser(ctx, code, opts.Username); err != nil {
		t.Fatalf("RemoveRoleUser: %v", err)
	}
	if err := c.RemoveRoleUser(ctx, code, opts.Username); !errors.Is(err, ErrNotFound) {
		t.Errorf("remove de não-vinculado deveria dar ErrNotFound, veio %v", err)
	}

	// delete + 404
	if err := c.DeleteRole(ctx, code); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	if _, err := c.GetRole(ctx, code); !errors.Is(err, ErrNotFound) {
		t.Errorf("papel deveria ter sumido após delete, veio %v", err)
	}
	t.Log("ciclo create→update→add/remove-user→delete ok; sem resíduo")
}
