package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// metaListStub responde com as fixtures reais da homologação (2026-11-21).
// A lista 121 tem os seis tipos de coluna. A coluna AREA está desativada.
type metaListStub struct {
	mu          sync.Mutex
	writes      map[string]map[string]any
	contentType map[string]string
	writeStatus int
	writeBody   string
}

func (s *metaListStub) server(t *testing.T) *httptest.Server {
	s.writes = map[string]map[string]any{}
	s.contentType = map[string]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	base := "/ecm/api/rest/ecm/metaList/"
	mux.HandleFunc(base+"findAllList", func(w http.ResponseWriter, r *http.Request) {
		w.Write(testdata(t, "metalist_findAllList.json"))
	})
	mux.HandleFunc(base+"findListById/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/121"):
			w.Write(testdata(t, "metalist_findListById.json"))
		case strings.HasSuffix(r.URL.Path, "/122"): // removida
			io.WriteString(w, "null")
		default: // nunca existiu
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(testdata(t, "metalist_findListById_inexistente.json"))
		}
	})
	mux.HandleFunc(base+"findListAndFields/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "método", http.StatusMethodNotAllowed)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/121") {
			w.Write(testdata(t, "metalist_findListAndFields.json"))
			return
		}
		w.Write(testdata(t, "metalist_findListAndFields_inexistente.json"))
	})
	mux.HandleFunc(base+"findColumnNames/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(testdata(t, "metalist_findColumnNames.json"))
	})
	mux.HandleFunc(base+"findAllListRecords/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(testdata(t, "metalist_findAllListRecords.json"))
	})
	mux.HandleFunc(base+"findAllListLabelRecords/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(testdata(t, "metalist_findAllListLabelRecords.json"))
	})
	mux.HandleFunc(base+"findRecord/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/121/1") {
			w.Write(testdata(t, "metalist_findRecord.json"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(testdata(t, "metalist_findRecord_inexistente.json"))
	})
	for _, op := range []string{"saveNewRecord", "updateRecord", "deleteRecord/", "saveNewList", "removeList/"} {
		op := op
		mux.HandleFunc(base+op, func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(b, &payload)
			s.mu.Lock()
			s.writes[op+" "+r.Method] = payload
			s.contentType[op] = r.Header.Get("Content-Type")
			status, body := s.writeStatus, s.writeBody
			s.mu.Unlock()
			if status != 0 {
				w.WriteHeader(status)
				io.WriteString(w, body)
				return
			}
			io.WriteString(w, `{"msgId":"Registro inserido!"}`)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func metaListClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-ml-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListMetaListsOrdenaPorNome(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	lists, err := c.ListMetaLists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 10 {
		t.Fatalf("esperava 10 listas, veio %d", len(lists))
	}
	if lists[0].Name != "Cartórios" || lists[0].ID != 87 {
		t.Errorf("primeira lista = %+v, quer Cartórios (87)", lists[0])
	}
	// "Órgão…" fica entre "N" e "P": a ordenação ignora o acento.
	if lists[1].Name != "Órgão Fiscalizador Notificações" {
		t.Errorf("segunda lista = %+v, quer Órgão Fiscalizador Notificações", lists[1])
	}
}

func TestGetMetaListJuntaColunas(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	m, err := c.GetMetaList(context.Background(), 121)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "zz_fluigcli_test_filho2" || len(m.Columns) != 7 {
		t.Fatalf("lista inesperada: %+v", m)
	}
	byTitle := map[string]MetaListColumn{}
	for _, col := range m.Columns {
		byTitle[col.Title] = col
	}
	// "Data Ref" vira DATA_REF: o nome real vem do findColumnNames.
	if got := byTitle["Data Ref"]; got.Name != "DATA_REF" || got.TypeName != "data" {
		t.Errorf("coluna Data Ref = %+v", got)
	}
	// Coluna desativada não aparece no findColumnNames: fica sem Name.
	if got := byTitle["area"]; !got.Disabled || got.Name != "" {
		t.Errorf("coluna area = %+v, quer desativada e sem Name", got)
	}
	if got := byTitle["pai"]; got.Type != MetaListTypeMetaList || got.RefListID != 120 || got.RefListName != "zz_fluigcli_test_pai" {
		t.Errorf("coluna pai = %+v", got)
	}
	if !byTitle["texto"].Label {
		t.Errorf("coluna texto devia ser rótulo")
	}
	if n := len(m.ActiveColumns()); n != 6 {
		t.Errorf("ActiveColumns = %d, quer 6", n)
	}
}

func TestGetMetaListInexistente(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	for _, id := range []int{99999, 122} {
		if _, err := c.GetMetaList(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("lista %d: err = %v, quer ErrNotFound", id, err)
		}
	}
}

func TestListMetaListRecordsNormalizaNumero(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	recs, err := c.ListMetaListRecords(context.Background(), 121)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 5 || recs[0].ID != 1 {
		t.Fatalf("registros inesperados: %+v", recs)
	}
	if got := recs[0].Values["NUMERICO"]; got != json.Number("12.5") {
		t.Errorf("NUMERICO = %#v, quer 12.5", got)
	}
	if got := recs[1].Values["NUMERICO"]; got != json.Number("3") {
		t.Errorf("NUMERICO = %#v, quer 3", got)
	}
	// A FK vem com o rótulo na listagem.
	if got := recs[0].Values["PAI"]; got != "Pai A" {
		t.Errorf("PAI = %#v, quer o rótulo", got)
	}
	if _, ok := recs[0].Values["metaListRecordId"]; ok {
		t.Error("metaListRecordId devia sair de Values")
	}
}

func TestGetMetaListRecord(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	rec, err := c.GetMetaListRecord(context.Background(), 121, 1)
	if err != nil {
		t.Fatal(err)
	}
	// No findRecord a FK vem com o id cru.
	if got := rec.Values["PAI"]; got != json.Number("1") {
		t.Errorf("PAI = %#v, quer o id 1", got)
	}
	_, err = c.GetMetaListRecord(context.Background(), 121, 999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quer ErrNotFound", err)
	}
}

func TestDeleteMetaListRecordsEnviaIdsComoString(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	if err := c.DeleteMetaListRecords(context.Background(), 121, []int{3, 4}); err != nil {
		t.Fatal(err)
	}
	got := stub.writes["deleteRecord/ DELETE"]
	ids, _ := got["recordId"].([]any)
	if len(ids) != 2 || ids[0] != "3" || ids[1] != "4" || got["listId"] != "121" {
		t.Errorf("payload = %#v, quer ids string", got)
	}
}

func TestCreateMetaListRecordPayload(t *testing.T) {
	stub := &metaListStub{}
	c := metaListClient(t, stub.server(t).URL)
	err := c.CreateMetaListRecord(context.Background(), 121, []MetaListValue{
		{Column: "TEXTO", Value: "Cartório Ação", Type: MetaListTypeText},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stub.writes["saveNewRecord POST"]
	if got["listId"] != "121" {
		t.Errorf("listId = %#v", got["listId"])
	}
	rec := got["dataRecord"].([]any)[0].(map[string]any)
	if rec["columnNameId"] != "TEXTO" || rec["value"] != "Cartório Ação" || rec["type"] != float64(1) {
		t.Errorf("dataRecord = %#v", rec)
	}
	if ct := stub.contentType["saveNewRecord"]; !strings.Contains(ct, "charset=UTF-8") {
		t.Errorf("Content-Type = %q, quer charset=UTF-8", ct)
	}
}

func TestMetaListWriteRejeicaoTextoPuro(t *testing.T) {
	stub := &metaListStub{writeStatus: http.StatusInternalServerError, writeBody: string(testdata(t, "metalist_saveNewRecord_erro.txt"))}
	c := metaListClient(t, stub.server(t).URL)
	err := c.CreateMetaListRecord(context.Background(), 121, []MetaListValue{{Column: "INTEIRO", Value: "1", Type: 3}})
	if !errors.Is(err, errServerRejected) || !strings.Contains(err.Error(), "Verifique o tipo de dado") {
		t.Fatalf("err = %v, quer rejeição com a frase do servidor", err)
	}
}

func TestRemoveMetaListReferenciada(t *testing.T) {
	stub := &metaListStub{writeStatus: http.StatusInternalServerError, writeBody: string(testdata(t, "metalist_removeList_referenciada.txt"))}
	c := metaListClient(t, stub.server(t).URL)
	err := c.RemoveMetaList(context.Background(), 120)
	if !errors.Is(err, errServerRejected) || !strings.Contains(err.Error(), "relacionada em outras listas") {
		t.Fatalf("err = %v", err)
	}
	ids, _ := stub.writes["removeList/ DELETE"]["listId"].([]any)
	if len(ids) != 1 || ids[0] != "120" {
		t.Errorf("listId = %#v", ids)
	}
}

func TestNormalizeMetaListValue(t *testing.T) {
	cases := []struct {
		typ     int
		in, out string
		wantErr bool
	}{
		{MetaListTypeText, " com espaço ", " com espaço ", false},
		{MetaListTypeInteger, "42", "42", false},
		{MetaListTypeInteger, "abc", "", true},
		{MetaListTypeInteger, "1.5", "", true},
		{MetaListTypeNumeric, "1,5", "1.5", false},
		{MetaListTypeNumeric, "12.50", "12.50", false},
		{MetaListTypeNumeric, "1,2,3", "", true},
		{MetaListTypeDate, "21/11/2026", "21/11/2026", false},
		{MetaListTypeDate, "2026-11-21", "21/11/2026", false},
		{MetaListTypeDate, "31/02/2026", "", true},
		{MetaListTypeDate, "21-11-2026", "", true},
		{MetaListTypeMetaList, "3", "3", false},
		{MetaListTypeMetaList, "Pai A", "", true},
		{MetaListTypeInteger, "", "", false},
	}
	for _, tc := range cases {
		got, err := NormalizeMetaListValue(tc.typ, tc.in)
		if (err != nil) != tc.wantErr || got != tc.out {
			t.Errorf("NormalizeMetaListValue(%d, %q) = %q, %v; quer %q (erro=%v)", tc.typ, tc.in, got, err, tc.out, tc.wantErr)
		}
	}
}
