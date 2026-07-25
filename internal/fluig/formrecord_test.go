package fluig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// cardDeleteStub replica a semântica REAL da homologação (2026-07-25): o GET
// valida o par formulário/registro (400 quando não bate) e o DELETE NÃO valida —
// responde 204 e apaga o documento pelo id, seja ele de outro formulário ou um
// arquivo do GED.
type cardDeleteStub struct {
	ownerForm  int  // formulário a que o card realmente pertence
	cardExists bool // o card existe em algum lugar
	deleteHits []string
}

func (s *cardDeleteStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm-forms/api/v2/cardindex/", func(w http.ResponseWriter, r *http.Request) {
		partes := strings.Split(strings.TrimPrefix(r.URL.Path, "/ecm-forms/api/v2/cardindex/"), "/")
		if len(partes) < 3 {
			http.NotFound(w, r)
			return
		}
		formID, cardID := partes[0], partes[2]
		if r.Method == http.MethodDelete {
			// O servidor real ignora o formulário: apaga e responde 204.
			s.deleteHits = append(s.deleteHits, formID+"/"+cardID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// GET: valida o par.
		if !s.cardExists {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"message":"Oops, something went wrong!"}`)
			return
		}
		if formID != strconv.Itoa(s.ownerForm) {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"message":"Documento não encontrado. Documento: `+cardID+`"}`)
			return
		}
		io.WriteString(w, `{"cardId":`+cardID+`,"parentDocumentId":`+formID+`,"activeVersion":true,"values":[],"children":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A guarda do DeleteFormRecord (ROADMAP §2.10-I) existe porque o DELETE do Fluig
// apaga por id sem validar o formulário — um id trocado destruía o documento
// errado, em silêncio.
func TestDeleteFormRecordConfirmaAntes(t *testing.T) {
	t.Run("registro do formulário certo é excluído", func(t *testing.T) {
		stub := &cardDeleteStub{ownerForm: 1111295, cardExists: true}
		c := helperClient(t, stub.server(t).URL)
		if err := c.DeleteFormRecord(context.Background(), 1111295, 1111297); err != nil {
			t.Fatalf("deveria excluir: %v", err)
		}
		if len(stub.deleteHits) != 1 || stub.deleteHits[0] != "1111295/1111297" {
			t.Errorf("DELETE não chegou como esperado: %v", stub.deleteHits)
		}
	})

	// O caso que destruiu um PDF do GED na homologação.
	t.Run("formulário errado NÃO chega a apagar", func(t *testing.T) {
		stub := &cardDeleteStub{ownerForm: 1111295, cardExists: true}
		c := helperClient(t, stub.server(t).URL)
		err := c.DeleteFormRecord(context.Background(), 28, 1111297)
		if err == nil {
			t.Fatal("a exclusão pelo formulário errado tinha de falhar")
		}
		if len(stub.deleteHits) != 0 {
			t.Fatalf("NADA deveria ter sido apagado, mas o DELETE foi chamado: %v", stub.deleteHits)
		}
		if !strings.Contains(err.Error(), "cancelada") || !strings.Contains(err.Error(), "28") {
			t.Errorf("a mensagem deveria dizer que cancelou e citar o formulário: %v", err)
		}
	})

	t.Run("registro inexistente vira NOT_FOUND sem apagar", func(t *testing.T) {
		stub := &cardDeleteStub{ownerForm: 1111295, cardExists: false}
		c := helperClient(t, stub.server(t).URL)
		err := c.DeleteFormRecord(context.Background(), 1111295, 999998)
		if err == nil || len(stub.deleteHits) != 0 {
			t.Fatalf("err=%v deleteHits=%v (não deveria apagar nada)", err, stub.deleteHits)
		}
		if !strings.Contains(err.Error(), "nada foi excluído") {
			t.Errorf("a mensagem deveria deixar claro que nada foi excluído: %v", err)
		}
	})
}
