//go:build integration

package fluig

import (
	"context"
	"testing"
)

// TestIntegrationProcessConvertReadOnly exercita os três endpoints de leitura
// da conversão de versão contra o servidor real: acha um processo com tarefa
// aberta, cruza a contagem por versão com a contagem por etapa e com a lista
// de solicitações. Read-only — o convertProcess (POST) foi validado ao vivo em
// 2026-08-07 (Compras, instância 151158, v20→v29 e reversão); um ciclo de
// escrita autossuficiente não é viável porque a API de import de processo não
// carrega etapas (o fluxo .process ficou fora, decisão de 2026-07-19).
func TestIntegrationProcessConvertReadOnly(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	procs, err := c.ListProcesses(ctx)
	if err != nil {
		t.Fatalf("ListProcesses: %v", err)
	}
	var processID string
	var source ConvertVersionInfo
	for _, pr := range procs {
		versions, err := c.ConvertVersions(ctx, pr.ID)
		if err != nil {
			t.Fatalf("ConvertVersions(%s): %v", pr.ID, err)
		}
		for _, v := range versions {
			if v.OpenTasks > 0 {
				processID, source = pr.ID, v
				break
			}
		}
		if processID != "" {
			break
		}
	}
	if processID == "" {
		t.Skip("nenhum processo com tarefa aberta no servidor de teste")
	}
	t.Logf("processo %q, versão %d com %d tarefa(s) aberta(s)", processID, source.Version, source.OpenTasks)

	states, err := c.ProcessStates(ctx, processID, source.Version)
	if err != nil {
		t.Fatalf("ProcessStates: %v", err)
	}
	seqs := make([]int, 0, len(states))
	for _, s := range states {
		seqs = append(seqs, s.Sequence)
	}
	counts, err := c.ConvertStateCounts(ctx, processID, source.Version, seqs)
	if err != nil {
		t.Fatalf("ConvertStateCounts: %v", err)
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total != source.OpenTasks {
		t.Errorf("soma por etapa = %d, versão diz %d", total, source.OpenTasks)
	}

	instances, err := c.ConvertInstances(ctx, processID, source.Version)
	if err != nil {
		t.Fatalf("ConvertInstances: %v", err)
	}
	if len(instances) == 0 {
		t.Fatalf("versão com %d tarefa(s) aberta(s) mas lista de solicitações vazia", source.OpenTasks)
	}
	seen := map[int]bool{}
	for _, in := range instances {
		if in.ID <= 0 {
			t.Errorf("solicitação sem id: %+v", in)
		}
		if seen[in.ID] {
			t.Errorf("solicitação %d duplicada na lista", in.ID)
		}
		seen[in.ID] = true
	}
	// A lista pode ser menor que o total de tarefas (tarefa duplicada/consenso
	// numa mesma solicitação), nunca maior.
	if len(instances) > total {
		t.Errorf("%d solicitações para %d tarefa(s) aberta(s)", len(instances), total)
	}
}
