//go:build integration

package fluig

import (
	"context"
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
