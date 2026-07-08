//go:build integration

package fluig

import (
	"context"
	"os"
	"testing"
)

// Integração de eventos globais (Fase 2). Opt-in via FLUIGCLI_TEST_*.
//
// ATENÇÃO: saveEventList substitui a lista COMPLETA de eventos. Para não
// arriscar apagar os eventos reais da homologação, o ciclo de escrita
// (export/delete) só roda com FLUIGCLI_TEST_EVENTS_WRITE=1 e apenas se a lista
// atual não vier vazia (garantia de que o merge parte de um conjunto real).
func TestIntegrationGlobalEventList(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	events, err := c.ListGlobalEvents(ctx)
	if err != nil {
		t.Fatalf("ListGlobalEvents: %v", err)
	}
	t.Logf("eventos globais no servidor: %d", len(events))
	for _, e := range events {
		t.Logf("  - %s (%d bytes de código)", e.ID, len(e.Code))
	}
}

func TestIntegrationGlobalEventWriteCycle(t *testing.T) {
	if os.Getenv("FLUIGCLI_TEST_EVENTS_WRITE") != "1" {
		t.Skip("defina FLUIGCLI_TEST_EVENTS_WRITE=1 para o ciclo de escrita (altera eventos no servidor)")
	}
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	before, err := c.ListGlobalEvents(ctx)
	if err != nil {
		t.Fatalf("ListGlobalEvents: %v", err)
	}
	// Trava de segurança: não escrever se a leitura veio vazia (evita wipe).
	if len(before) == 0 {
		t.Fatal("lista de eventos veio vazia — abortando o ciclo de escrita por segurança")
	}

	const id = "zz_fluigcli_test_event"
	merged := append([]GlobalEvent{}, before...)
	merged = append(merged, GlobalEvent{ID: id, Code: "function " + id + "(){ return 1; }"})
	if err := c.SaveGlobalEvents(ctx, merged); err != nil {
		t.Fatalf("SaveGlobalEvents: %v", err)
	}

	// Confere que o novo entrou E que os anteriores continuam lá.
	after, err := c.ListGlobalEvents(ctx)
	if err != nil {
		t.Fatalf("ListGlobalEvents após save: %v", err)
	}
	got := map[string]bool{}
	for _, e := range after {
		got[e.ID] = true
	}
	if !got[id] {
		t.Errorf("evento de teste %q não foi salvo", id)
	}
	for _, e := range before {
		if !got[e.ID] {
			t.Errorf("evento existente %q sumiu após o export — merge falhou!", e.ID)
		}
	}

	// Limpeza: remove o evento de teste.
	if err := c.DeleteGlobalEvent(ctx, id); err != nil {
		t.Errorf("DeleteGlobalEvent (limpeza): %v", err)
	}
	t.Logf("ciclo de escrita ok; Content-Type de saveEventList aceito pelo servidor")
}
