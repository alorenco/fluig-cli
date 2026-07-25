package fluig

import (
	"encoding/json"
	"testing"
)

// A API devolve as linhas de TODAS as tabelas filhas num array só, com os
// campos sufixados por ___<rowId> (ver FLUIG-APIS.md, validado na produção em
// 2026-07-25). O parse tira o sufixo e separa os metadados.
func TestToChildRow(t *testing.T) {
	field := func(id, value string) cardField { return cardField{FieldID: id, Value: &value} }

	t.Run("tira o sufixo do rowId e separa os metadados", func(t *testing.T) {
		row := toChildRow([]cardField{
			field("rowId", "88"),
			field("tableId", "tabelaTributos"),
			field("trbCodTrb___88", "ICMS"),
			field("trbValor___88", "180.00"),
			field("tableid___88", "tabelatributos"),
		})
		if row.TableID != "tabelaTributos" || row.RowID != "88" {
			t.Fatalf("metadados: tableId=%q rowId=%q", row.TableID, row.RowID)
		}
		want := map[string]string{"trbCodTrb": "ICMS", "trbValor": "180.00", "tableid": "tabelatributos"}
		for k, v := range want {
			if row.Values[k] != v {
				t.Errorf("campo %q = %q, quer %q", k, row.Values[k], v)
			}
		}
		// rowId/tableId são metadados: não repetem dentro de Values.
		if _, ok := row.Values["rowId"]; ok {
			t.Error("rowId não deveria estar em Values")
		}
		if _, ok := row.Values["tableId"]; ok {
			t.Error("tableId não deveria estar em Values")
		}
		if len(row.Values) != len(want) {
			t.Errorf("Values com %d campos, quer %d: %v", len(row.Values), len(want), row.Values)
		}
	})

	// Defensivo: sufixo diferente do rowId fica como veio. Perder dado em
	// silêncio seria pior que devolver um nome estranho.
	t.Run("sufixo que não casa com o rowId sobrevive", func(t *testing.T) {
		row := toChildRow([]cardField{
			field("rowId", "7"),
			field("tableId", "tabelaItens"),
			field("itemValor___7", "10.00"),
			field("outroCampo___9", "valor"),
			field("semSufixo", "x"),
		})
		if row.Values["itemValor"] != "10.00" {
			t.Errorf("campo da própria linha: %v", row.Values)
		}
		if row.Values["outroCampo___9"] != "valor" {
			t.Errorf("sufixo divergente deveria sobreviver: %v", row.Values)
		}
		if row.Values["semSufixo"] != "x" {
			t.Errorf("campo sem sufixo deveria sobreviver: %v", row.Values)
		}
	})

	t.Run("linha sem metadados não quebra", func(t *testing.T) {
		row := toChildRow([]cardField{field("item", "linha 1")})
		if row.TableID != "" || row.RowID != "" || row.Values["item"] != "linha 1" {
			t.Errorf("linha crua: %+v", row)
		}
	})
}

// Parse do card real da produção (fixture sanitizada): duas tabelas filhas no
// mesmo array, agrupáveis por tableId.
func TestCardFindChildrenFixture(t *testing.T) {
	var cf cardFind
	if err := json.Unmarshal(testdata(t, "rest_form_card_children.json"), &cf); err != nil {
		t.Fatal(err)
	}
	rec := cf.toRecord()
	if len(rec.Children) != 4 {
		t.Fatalf("linhas filhas: %d, quer 4", len(rec.Children))
	}
	byTable := map[string]int{}
	for _, ch := range rec.Children {
		byTable[ch.TableID]++
		if ch.RowID == "" {
			t.Errorf("linha sem rowId: %+v", ch)
		}
		for k := range ch.Values {
			if k == "rowId" || k == "tableId" {
				t.Errorf("metadado vazou para Values: %q", k)
			}
		}
	}
	if byTable["tabelaItens"] != 2 || byTable["tabelaTributos"] != 2 {
		t.Errorf("agrupamento por tabela: %v", byTable)
	}
	// A linha 2 de tabelaItens carrega campos de outra tabela filha (parcelas e
	// anexo) — caso real, não erro.
	for _, ch := range rec.Children {
		if ch.TableID == "tabelaItens" && ch.RowID == "2" {
			if ch.Values["parcelaVencimento"] != "10/08/2026" || ch.Values["itemProdutoServico"] != "Mesa de reunião" {
				t.Errorf("linha mista perdeu campos: %v", ch.Values)
			}
			if ch.Values["tableid"] != "tabelaitens" {
				t.Errorf("coluna interna tableid: %q", ch.Values["tableid"])
			}
		}
	}
}
