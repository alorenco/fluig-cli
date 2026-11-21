//go:build integration

package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Integração das listas customizadas (ROADMAP §5.23). Opt-in via
// FLUIGCLI_TEST_*. Cria a lista zz_fluigcli_test_ml, faz o ciclo dos
// registros e remove a lista. A lista de teste não tem coluna metalista: uma
// lista referenciada por FK não sai mais do servidor (medido em 2026-11-21).
func TestIntegrationMetaListCycle(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const name = "zz_fluigcli_test_ml"

	findID := func() int {
		lists, err := c.ListMetaLists(ctx)
		if err != nil {
			t.Fatalf("ListMetaLists: %v", err)
		}
		for _, l := range lists {
			if l.Name == name {
				return l.ID
			}
		}
		return 0
	}
	// Sobra de execução anterior interrompida.
	if id := findID(); id != 0 {
		_ = c.RemoveMetaList(ctx, id)
	}

	err = c.CreateMetaList(ctx, name, "lista de teste do fluigcli", []MetaListNewColumn{
		{Title: "texto", Type: MetaListTypeText, Label: true},
		{Title: "qtd", Type: MetaListTypeInteger},
		{Title: "Data Ref", Type: MetaListTypeDate},
		{Title: "valor", Type: MetaListTypeNumeric},
	})
	if err != nil {
		t.Fatalf("CreateMetaList: %v", err)
	}
	id := findID()
	if id == 0 {
		t.Fatal("lista criada não apareceu no findAllList")
	}
	t.Cleanup(func() {
		if err := c.RemoveMetaList(context.Background(), id); err != nil {
			t.Errorf("RemoveMetaList(%d): %v", id, err)
		}
	})

	m, err := c.GetMetaList(ctx, id)
	if err != nil {
		t.Fatalf("GetMetaList: %v", err)
	}
	names := map[string]int{}
	for _, col := range m.ActiveColumns() {
		names[col.Name] = col.Type
	}
	if names["DATA_REF"] != MetaListTypeDate || names["QTD"] != MetaListTypeInteger || len(names) != 4 {
		t.Fatalf("colunas inesperadas: %+v", m.Columns)
	}

	// Acento no valor: confere o charset=UTF-8 da escrita.
	err = c.CreateMetaListRecord(ctx, id, []MetaListValue{
		{Column: "TEXTO", Value: "Cartório Ação", Type: MetaListTypeText},
		{Column: "QTD", Value: "7", Type: MetaListTypeInteger},
		{Column: "DATA_REF", Value: "21/11/2026", Type: MetaListTypeDate},
		{Column: "VALOR", Value: "12.5", Type: MetaListTypeNumeric},
	})
	if err != nil {
		t.Fatalf("CreateMetaListRecord: %v", err)
	}
	recs, err := c.ListMetaListRecords(ctx, id)
	if err != nil {
		t.Fatalf("ListMetaListRecords: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("esperava 1 registro, veio %d", len(recs))
	}
	r := recs[0]
	if r.Values["TEXTO"] != "Cartório Ação" || r.Values["DATA_REF"] != "21/11/2026" || r.Values["VALOR"] != json.Number("12.5") {
		t.Errorf("registro gravado diferente: %+v", r.Values)
	}

	// Update parcial preserva as outras colunas.
	if err := c.UpdateMetaListRecord(ctx, id, r.ID, []MetaListValue{{Column: "QTD", Value: "8", Type: MetaListTypeInteger}}); err != nil {
		t.Fatalf("UpdateMetaListRecord: %v", err)
	}
	got, err := c.GetMetaListRecord(ctx, id, r.ID)
	if err != nil {
		t.Fatalf("GetMetaListRecord: %v", err)
	}
	if got.Values["QTD"] != json.Number("8") || got.Values["TEXTO"] != "Cartório Ação" {
		t.Errorf("update parcial: %+v", got.Values)
	}

	// Erro de tipo volta como rejeição do servidor (500 texto puro).
	err = c.CreateMetaListRecord(ctx, id, []MetaListValue{{Column: "QTD", Value: "abc", Type: MetaListTypeInteger}})
	if !errors.Is(err, errServerRejected) {
		t.Errorf("inteiro inválido: err = %v, quer errServerRejected", err)
	}

	if _, err := c.GetMetaListRecord(ctx, id, 99999); !errors.Is(err, ErrNotFound) {
		t.Errorf("registro inexistente: err = %v, quer ErrNotFound", err)
	}

	if err := c.DeleteMetaListRecords(ctx, id, []int{r.ID}); err != nil {
		t.Fatalf("DeleteMetaListRecords: %v", err)
	}
	recs, err = c.ListMetaListRecords(ctx, id)
	if err != nil {
		t.Fatalf("ListMetaListRecords: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("registro não foi removido: %+v", recs)
	}

	if _, err := c.GetMetaList(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("lista inexistente: err = %v, quer ErrNotFound", err)
	}
}

// Fase 2: cria, altera (colunas e, em separado, nome/descrição) e remove.
func TestIntegrationMetaListSchema(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const name, renamed = "zz_fluigcli_test_schema", "zz_fluigcli_test_schema2"

	findID := func(n string) int {
		lists, err := c.ListMetaLists(ctx)
		if err != nil {
			t.Fatalf("ListMetaLists: %v", err)
		}
		for _, l := range lists {
			if l.Name == n {
				return l.ID
			}
		}
		return 0
	}
	for _, n := range []string{name, renamed} {
		if id := findID(n); id != 0 {
			_ = c.RemoveMetaList(ctx, id)
		}
	}

	if err := c.CreateMetaList(ctx, name, "", []MetaListNewColumn{
		{Title: "Descrição", Type: MetaListTypeText, Label: true},
		{Title: "qtd", Type: MetaListTypeInteger},
	}); err != nil {
		t.Fatalf("CreateMetaList: %v", err)
	}
	id := findID(name)
	if id == 0 {
		t.Fatal("lista criada não apareceu")
	}
	t.Cleanup(func() {
		if err := c.RemoveMetaList(context.Background(), id); err != nil {
			t.Errorf("RemoveMetaList(%d): %v", id, err)
		}
	})

	m, err := c.GetMetaList(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var qtd MetaListColumn
	for _, col := range m.Columns {
		if col.Name == "QTD" {
			qtd = col
		}
	}
	// Passo 1: desativa qtd, inclui "Data Ref" como rótulo, com o nome atual.
	err = c.UpdateMetaList(ctx, id, m.Name, m.Description,
		[]MetaListColumnChange{{Column: qtd, Label: false, Disabled: true}},
		[]MetaListNewColumn{{Title: "Data Ref", Type: MetaListTypeDate, Label: true}}, m.MaxSeq())
	if err != nil {
		t.Fatalf("UpdateMetaList (colunas): %v", err)
	}
	// Passo 2: nome e descrição, sem colunas.
	if err := c.UpdateMetaList(ctx, id, renamed, "descrição nova", nil, nil, 0); err != nil {
		t.Fatalf("UpdateMetaList (nome): %v", err)
	}
	m, err = c.GetMetaList(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != renamed || m.Description != "descrição nova" || m.FieldsUnavailable {
		t.Errorf("lista = %+v", m)
	}
	got := map[string]MetaListColumn{}
	for _, col := range m.Columns {
		got[col.Title] = col
	}
	if !got["qtd"].Disabled || got["Data Ref"].Name != "DATA_REF" || !got["Data Ref"].Label || !got["Descrição"].Label || len(m.Columns) != 3 {
		t.Errorf("colunas = %+v", m.Columns)
	}

	// Palavra reservada: o servidor recusa com texto puro.
	err = c.UpdateMetaList(ctx, id, m.Name, m.Description, nil, []MetaListNewColumn{{Title: "select", Type: MetaListTypeText}}, m.MaxSeq())
	if !errors.Is(err, errServerRejected) {
		t.Errorf("palavra reservada: err = %v, quer errServerRejected", err)
	}
}
