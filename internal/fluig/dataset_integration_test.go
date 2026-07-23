//go:build integration

package fluig

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Ciclo completo de dataset contra a homologação (Fase 1). Opt-in via
// FLUIGCLI_TEST_* (ver integration_test.go). Cria um dataset com prefixo
// zz_fluigcli_test_ e o atualiza; a listagem/consulta são exercidas de verdade.
func TestIntegrationDatasetCycle(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// list
	datasets, err := c.ListDatasets(ctx)
	if err != nil {
		t.Fatalf("ListDatasets: %v", err)
	}
	t.Logf("listados %d datasets", len(datasets))
	custom := 0
	for _, d := range datasets {
		if d.Custom {
			custom++
		}
	}
	t.Logf("%d datasets customizados", custom)

	const id = "zz_fluigcli_test_ds"
	implV1 := "function createDataset(fields, constraints, sortFields) {\n  var d = DatasetBuilder.newDataset();\n  d.addColumn('id');\n  d.addRow(['1']);\n  return d;\n}\n"

	// create (idempotente: se já existe de uma rodada anterior, cai no update)
	if _, err := c.LoadDataset(ctx, id); err != nil {
		if err := c.CreateDataset(ctx, id, "fluigcli test dataset", implV1); err != nil {
			t.Fatalf("CreateDataset: %v", err)
		}
		t.Log("dataset de teste criado")
	}

	// load + update
	loaded, err := c.LoadDataset(ctx, id)
	if err != nil {
		t.Fatalf("LoadDataset após create: %v", err)
	}
	implV2 := strings.Replace(implV1, "['1']", "['2']", 1)
	if err := c.UpdateDataset(ctx, loaded, implV2); err != nil {
		t.Fatalf("UpdateDataset: %v", err)
	}

	// reimport e comparação byte a byte
	reloaded, err := c.LoadDataset(ctx, id)
	if err != nil {
		t.Fatalf("LoadDataset após update: %v", err)
	}
	if reloaded.Impl != implV2 {
		t.Errorf("datasetImpl não bate após o ciclo:\n--- enviado ---\n%s\n--- lido ---\n%s", implV2, reloaded.Impl)
	}
	t.Logf("ciclo create→update→reload ok; dataset de teste %q permanece no servidor para inspeção", id)
}

// Ciclo administrativo de dataset (2026-07-13): histórico, enable/disable e
// restore, sobre o mesmo dataset de teste do ciclo básico.
func TestIntegrationDatasetAdminCycle(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const id = "zz_fluigcli_test_ds"

	// Garante o dataset (mesma semente do ciclo básico).
	implV1 := "function createDataset(fields, constraints, sortFields) {\n  var d = DatasetBuilder.newDataset();\n  d.addColumn('id');\n  d.addRow(['1']);\n  return d;\n}\n"
	if _, err := c.LoadDataset(ctx, id); err != nil {
		if err := c.CreateDataset(ctx, id, "fluigcli test dataset", implV1); err != nil {
			t.Fatalf("CreateDataset: %v", err)
		}
	}
	loaded, err := c.LoadDataset(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	implV2 := strings.Replace(implV1, "['1']", "['2']", 1)
	if err := c.UpdateDataset(ctx, loaded, implV2); err != nil {
		t.Fatalf("UpdateDataset: %v", err)
	}

	// history: precisa ter ao menos duas versões após o update.
	versions, err := c.DatasetHistory(ctx, id)
	if err != nil {
		t.Fatalf("DatasetHistory: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("esperava ≥2 versões no histórico, veio %d", len(versions))
	}
	t.Logf("histórico com %d versões (última: v%d %s por %s)", len(versions),
		versions[len(versions)-1].Version, versions[len(versions)-1].Status, versions[len(versions)-1].Author)

	// disable → inativo na listagem; enable → ativo de novo.
	checkActive := func(want bool) {
		t.Helper()
		datasets, lerr := c.ListDatasets(ctx)
		if lerr != nil {
			t.Fatalf("ListDatasets: %v", lerr)
		}
		for _, d := range datasets {
			if d.ID == id {
				if d.Active != want {
					t.Errorf("active=%v, quer %v", d.Active, want)
				}
				return
			}
		}
		t.Errorf("dataset %q sumiu da listagem", id)
	}
	if err := c.DisableDataset(ctx, id); err != nil {
		t.Fatalf("DisableDataset: %v", err)
	}
	checkActive(false)
	if err := c.EnableDataset(ctx, id); err != nil {
		t.Fatalf("EnableDataset: %v", err)
	}
	checkActive(true)

	// disable de inexistente → ErrNotFound (404 real).
	if err := c.DisableDataset(ctx, "zz_fluigcli_nao_existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("disable de inexistente deveria dar ErrNotFound, veio %v", err)
	}

	// restore para a penúltima versão: cria versão nova com o código antigo.
	target := versions[len(versions)-2]
	if hasDraft, derr := c.DatasetHasDraft(ctx, id); derr != nil || hasDraft {
		t.Logf("DatasetHasDraft: draft=%v err=%v", hasDraft, derr)
	}
	entry, err := c.RestoreDatasetVersion(ctx, id, target.Version)
	if err != nil {
		t.Fatalf("RestoreDatasetVersion: %v", err)
	}
	if entry == nil || entry.Version <= versions[len(versions)-1].Version {
		t.Fatalf("restore deveria criar versão nova; veio %+v", entry)
	}
	restored, err := c.LoadDataset(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Impl != target.Impl {
		t.Errorf("código após restore ≠ código da v%d", target.Version)
	}
	t.Logf("restore ok: v%d → nova v%d (%s)", target.Version, entry.Version, entry.Status)
}

// Consulta de valores via REST v2 (dataset-handle/search), read-only.
func TestIntegrationDatasetQuery(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.QueryDataset(context.Background(), "colleague", DatasetQuery{
		Fields:  []string{"login"},
		OrderBy: "login",
		Limit:   3,
	})
	if err != nil {
		t.Fatalf("QueryDataset: %v", err)
	}
	t.Logf("colunas=%v linhas=%d", res.Columns, len(res.Rows))
	if len(res.Columns) == 0 || len(res.Rows) == 0 || len(res.Rows) > 3 {
		t.Errorf("resultado inesperado: %d colunas, %d linhas", len(res.Columns), len(res.Rows))
	}

	// Dataset inexistente responde 200 com nulls → ErrNotFound.
	if _, err := c.QueryDataset(context.Background(), "zz_fluigcli_nao_existe", DatasetQuery{}); err == nil {
		t.Error("dataset inexistente deveria dar erro")
	}
}

// Hard-delete via helper (ROADMAP §1.2, 2026-07-23): cria um dataset descartável
// e o remove FISICAMENTE com DeleteDatasetPermanently (EJB deletePermanently via
// fluigcliHelper >= 0.7.0). Confirma que some da listagem — diferente do
// enable/disable (reversível) e da REST legada deleteDataset (que só desativa).
// Requer o helper 0.7.0 instalado na homologação.
func TestIntegrationDatasetHardDelete(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.requireHelper(ctx); err != nil {
		t.Skipf("fluigcliHelper ausente: %v", err)
	}
	const id = "zz_fluigcli_test_hdel"

	impl := "function createDataset(fields, constraints, sortFields) {\n  var d = DatasetBuilder.newDataset();\n  d.addColumn('id');\n  d.addRow(['1']);\n  return d;\n}\n"
	if err := c.CreateDataset(ctx, id, "fluigcli test hard-delete", impl); err != nil {
		t.Logf("CreateDataset (pode já existir): %v", err)
	}
	if _, err := c.LoadDataset(ctx, id); err != nil {
		t.Fatalf("dataset de teste não existe após create: %v", err)
	}

	// hard-delete → o dataset some da listagem por completo.
	if err := c.DeleteDatasetPermanently(ctx, id); err != nil {
		t.Fatalf("DeleteDatasetPermanently: %v", err)
	}
	datasets, err := c.ListDatasets(ctx)
	if err != nil {
		t.Fatalf("ListDatasets: %v", err)
	}
	for _, d := range datasets {
		if d.ID == id {
			t.Errorf("dataset %q ainda aparece na listagem após o hard-delete", id)
		}
	}
	t.Logf("hard-delete ok: %q removido de vez do servidor", id)
}
