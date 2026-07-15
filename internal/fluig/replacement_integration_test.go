//go:build integration

package fluig

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Ciclo completo de substituição de usuário contra a homologação (SOAP
// ECMColleagueReplacementService + REST v2 user-replacements). Opt-in via
// FLUIGCLI_TEST_* (ver integration_test.go). Cria dois usuários
// zz_fluigcli_test_ (titular e substituto), roda create→get→update→delete e
// desativa os usuários ao final (não há exclusão de usuário na API).
func TestIntegrationReplacementCycle(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// leitura global (só garante que a rota responde)
	if _, err := c.ListReplacements(ctx, ReplacementFilter{Limit: 5}); err != nil {
		t.Fatalf("ListReplacements: %v", err)
	}

	const titular = "zz_fluigcli_test_repl_tit"
	const subst = "zz_fluigcli_test_repl_sub"
	ensureUser(t, c, ctx, titular)
	ensureUser(t, c, ctx, subst)
	defer func() {
		_ = c.SetAdminUserActive(ctx, titular, false)
		_ = c.SetAdminUserActive(ctx, subst, false)
	}()

	start := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	// create
	r, err := c.CreateReplacement(ctx, titular, subst, ReplacementInput{
		Start: start, End: end, WorkflowTasks: true, GEDTasks: false,
	})
	if err != nil {
		t.Fatalf("CreateReplacement: %v", err)
	}
	if r.ReplacedBy == nil || r.ReplacedBy.Code != subst {
		t.Errorf("substituição criada inesperada: %+v", r)
	}

	// duplicado → rejeição de negócio (exit 5)
	if _, err := c.CreateReplacement(ctx, titular, subst, ReplacementInput{Start: start, End: end}); err == nil {
		t.Error("create duplicado deveria falhar")
	}

	// get (SOAP; deve conter o par com as flags)
	list, err := c.GetUserReplacements(ctx, titular, false)
	if err != nil {
		t.Fatalf("GetUserReplacements: %v", err)
	}
	found := false
	for _, it := range list {
		if it.ReplacedBy != nil && it.ReplacedBy.Code == subst {
			found = true
			if it.EndDate == nil || it.EndDate.Format("2006-01-02") != "2026-07-31" {
				t.Errorf("data fim inesperada (formato zoneless não persistiu o dia?): %+v", it.EndDate)
			}
			if it.WorkflowTasks == nil || !*it.WorkflowTasks {
				t.Errorf("flag workflow inesperada: %+v", it)
			}
		}
	}
	if !found {
		t.Fatalf("substituição criada não apareceu no get (%d itens)", len(list))
	}

	// update (merge: muda ged e fim, preserva workflow e início)
	newEnd := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	ged := true
	up, err := c.UpdateReplacement(ctx, titular, subst, ReplacementChanges{End: &newEnd, GEDTasks: &ged})
	if err != nil {
		t.Fatalf("UpdateReplacement: %v", err)
	}
	if up.GEDTasks == nil || !*up.GEDTasks {
		t.Errorf("update não aplicou ged: %+v", up)
	}
	if up.WorkflowTasks == nil || !*up.WorkflowTasks {
		t.Errorf("update não preservou workflow (merge): %+v", up)
	}
	if up.EndDate == nil || up.EndDate.Format("2006-01-02") != "2026-08-31" {
		t.Errorf("update não aplicou o fim: %+v", up.EndDate)
	}

	// update de par inexistente = ErrNotFound
	if _, err := c.UpdateReplacement(ctx, titular, titular, ReplacementChanges{GEDTasks: &ged}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update de par inexistente deveria dar ErrNotFound, veio %v", err)
	}

	// delete
	if err := c.DeleteReplacement(ctx, titular, subst); err != nil {
		t.Fatalf("DeleteReplacement: %v", err)
	}
	// delete de novo = ErrNotFound (NOK "não foi encontrado")
	if err := c.DeleteReplacement(ctx, titular, subst); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete de par inexistente deveria dar ErrNotFound, veio %v", err)
	}

	t.Log("ciclo create→dup→get→update→delete ok; usuários desativados no defer")
}

// ensureUser cria (e reativa) um usuário de teste ativo.
func ensureUser(t *testing.T, c *Client, ctx context.Context, login string) {
	t.Helper()
	if _, err := c.GetAdminUser(ctx, login); err == nil {
		if err := c.SetAdminUserActive(ctx, login, true); err != nil {
			t.Fatalf("reativar %s: %v", login, err)
		}
		return
	}
	_, err := c.CreateAdminUser(ctx, AdminUserCreate{
		Login: login, Email: login + "@example.com",
		FirstName: "ZZ", LastName: "Test", Password: "Zz!fluigcli2026",
	})
	if err != nil {
		t.Fatalf("criar %s: %v", login, err)
	}
}
