package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// datasetStub simula login/ping + os endpoints SOAP/REST de dataset.
type datasetStub struct {
	editedImpl  string // datasetImpl recebido no último editDataset
	createdBody map[string]any
	loadStatus  int  // status para loadDataset (default 200)
	restMissing bool // REST v2 ausente (Fluig antigo) → 404, cai no SOAP

	handleSeen []string // query strings recebidas no dataset-handle/search
	handleBig  bool     // 1ª página cheia (força a paginação por offset)
}

func (s *datasetStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/webdesk/ECMDatasetService", func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		w.Header().Set("Content-Type", "text/xml")
		switch action {
		case "findAllFormulariesDatasets":
			w.Write(testdata(t, "soap_findAllDatasets.xml"))
		default:
			http.Error(w, "op desconhecida", http.StatusInternalServerError)
		}
	})
	// REST v2: listagem paginada de datasets.
	restPages := [][]byte{
		testdata(t, "rest_datasets_page1.json"),
		testdata(t, "rest_datasets_page2.json"),
	}
	restCalls := 0
	mux.HandleFunc("/dataset/api/v2/datasets", func(w http.ResponseWriter, r *http.Request) {
		if s.restMissing {
			http.NotFound(w, r)
			return
		}
		if restCalls >= len(restPages) {
			io.WriteString(w, `{"items":[],"hasNext":false}`)
			return
		}
		restCalls++
		w.Write(restPages[restCalls-1])
	})
	// REST v2: consulta de valores (dataset-handle/search).
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		s.handleSeen = append(s.handleSeen, r.URL.RawQuery)
		q := r.URL.Query()
		// Inexistente/consulta inválida: 200 com columns/values null (real).
		if q.Get("datasetId") == "nao_existe" {
			io.WriteString(w, `{"columns":null,"values":null}`)
			return
		}
		if s.handleBig && q.Get("offset") == "0" {
			// 1ª página cheia (== limit pedido) para forçar a paginação.
			limit := q.Get("limit")
			n := 0
			fmt.Sscanf(limit, "%d", &n)
			var b strings.Builder
			b.WriteString(`{"columns":["login"],"values":[`)
			for i := 0; i < n; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"login":"u%d"}`, i)
			}
			b.WriteString(`]}`)
			io.WriteString(w, b.String())
			return
		}
		w.Write(testdata(t, "rest_dataset_handle.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/loadDataset", func(w http.ResponseWriter, r *http.Request) {
		status := s.loadStatus
		if status == 0 {
			status = http.StatusOK
		}
		// O Fluig responde HTTP 500 para dataset inexistente (não 404) — quirk real.
		if r.URL.Query().Get("datasetId") == "nao_existe" {
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			w.Write(testdata(t, "loadDataset.json"))
		}
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/createDataset", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &s.createdBody)
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/dataset/editDataset", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			DatasetImpl string `json:"datasetImpl"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		s.editedImpl = payload.DatasetImpl
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func datasetClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-ds-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Listagem via REST v2: pagina até hasNext=false e mapeia os campos novos.
func TestListDatasets(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	list, err := c.ListDatasets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("esperava 4 (3+1 das duas páginas), veio %d", len(list))
	}
	byID := map[string]DatasetSummary{}
	for _, d := range list {
		byID[d.ID] = d
	}
	ex := byID["ds_exemplo"]
	if !ex.Custom || ex.Type != "CUSTOM" || ex.Description != "Dataset de exemplo" || !ex.Active {
		t.Errorf("ds_exemplo inesperado: %+v", ex)
	}
	if byID["ds_inativo"].Active {
		t.Errorf("ds_inativo deveria estar inativo: %+v", byID["ds_inativo"])
	}
	if byID["colleague"].Custom {
		t.Errorf("colleague (BUILTIN) não deveria ser custom: %+v", byID["colleague"])
	}
	if !byID["frm_cadastro"].Draft {
		t.Errorf("frm_cadastro deveria ter draft: %+v", byID["frm_cadastro"])
	}
}

// Servidor sem a REST v2 de datasets (404) → fallback SOAP.
func TestListDatasetsFallbackSOAP(t *testing.T) {
	stub := &datasetStub{restMissing: true}
	c := datasetClient(t, stub.server(t).URL)
	list, err := c.ListDatasets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2 do SOAP, veio %d", len(list))
	}
	if !list[0].Custom || list[0].ID != "ds_exemplo" {
		t.Errorf("item[0] inesperado: %+v", list[0])
	}
	if list[0].Description != "" || !list[0].Active {
		t.Errorf("fallback SOAP deveria vir sem descrição e ativo: %+v", list[0])
	}
}

func TestLoadDataset(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	ds, err := c.LoadDataset(context.Background(), "ds_exemplo")
	if err != nil {
		t.Fatal(err)
	}
	if ds.ID != "ds_exemplo" || ds.Description != "Dataset de exemplo" {
		t.Errorf("dataset inesperado: %+v", ds)
	}
	if !strings.Contains(ds.Impl, "createDataset") {
		t.Errorf("datasetImpl não carregado: %q", ds.Impl)
	}
}

// loadDataset de um dataset inexistente responde HTTP 500 no Fluig real; deve
// virar ErrNotFound para o fluxo de create-vs-update do export funcionar.
func TestLoadDatasetNotFound(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	_, err := c.LoadDataset(context.Background(), "nao_existe")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound (500→not-found), veio %v", err)
	}
}

// Update carrega a estrutura e reenvia só com o datasetImpl trocado.
func TestUpdateDatasetKeepsStructure(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	loaded, err := c.LoadDataset(context.Background(), "ds_exemplo")
	if err != nil {
		t.Fatal(err)
	}
	novo := "function createDataset(){ return 42; }"
	if err := c.UpdateDataset(context.Background(), loaded, novo); err != nil {
		t.Fatal(err)
	}
	if stub.editedImpl != novo {
		t.Errorf("editDataset recebeu datasetImpl %q, quer %q", stub.editedImpl, novo)
	}
}

func TestCreateDataset(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	if err := c.CreateDataset(context.Background(), "ds_novo", "Novo", "function createDataset(){}"); err != nil {
		t.Fatal(err)
	}
	pk, _ := stub.createdBody["datasetPK"].(map[string]any)
	if pk == nil || pk["datasetId"] != "ds_novo" {
		t.Errorf("datasetPK inesperado: %v", stub.createdBody["datasetPK"])
	}
	if stub.createdBody["type"] != datasetTypeCustom {
		t.Errorf("type = %v, quer CUSTOM", stub.createdBody["type"])
	}
	if stub.createdBody["datasetBuilder"] != customDatasetBuilder {
		t.Errorf("datasetBuilder inesperado: %v", stub.createdBody["datasetBuilder"])
	}
}

// Query via REST: parâmetros mapeados na query string e resultado decodificado.
func TestQueryDataset(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	res, err := c.QueryDataset(context.Background(), "colleague", DatasetQuery{
		Fields:      []string{"colleagueName", "login"},
		Constraints: []DatasetConstraint{{Field: "active", Initial: "true"}},
		OrderBy:     "colleagueName",
		Limit:       3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Columns) != 3 || len(res.Rows) != 3 {
		t.Fatalf("resultado inesperado: %d colunas, %d linhas", len(res.Columns), len(res.Rows))
	}
	if v := res.Rows[0]["login"]; v == nil || *v != "ana.andrade" {
		t.Errorf("linha[0].login inesperado: %v", v)
	}
	if len(stub.handleSeen) != 1 {
		t.Fatalf("esperava 1 requisição, houve %d", len(stub.handleSeen))
	}
	qs := stub.handleSeen[0]
	for _, want := range []string{
		"datasetId=colleague", "field=colleagueName", "field=login",
		"constraintsField=active", "constraintsInitialValue=true", "constraintsFinalValue=true",
		"constraintsType=MUST", "constraintsLikeSearch=false",
		"orderby=colleagueName", "limit=3", "offset=0",
	} {
		if !strings.Contains(qs, want) {
			t.Errorf("query string sem %q:\n%s", want, qs)
		}
	}
}

// Dataset inexistente (ou consulta inválida): 200 com nulls → ErrNotFound.
func TestQueryDatasetNotFound(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	_, err := c.QueryDataset(context.Background(), "nao_existe", DatasetQuery{})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
}

// Limit=0 (todas as linhas): pagina por offset até a página vir incompleta.
func TestQueryDatasetPaginaSemLimite(t *testing.T) {
	stub := &datasetStub{handleBig: true}
	c := datasetClient(t, stub.server(t).URL)
	res, err := c.QueryDataset(context.Background(), "colleague", DatasetQuery{Fields: []string{"login"}})
	if err != nil {
		t.Fatal(err)
	}
	// 1ª página cheia (500 sintéticos) + 2ª página com a fixture (3).
	if len(res.Rows) != datasetHandleMaxPage+3 {
		t.Fatalf("esperava %d linhas, veio %d", datasetHandleMaxPage+3, len(res.Rows))
	}
	if len(stub.handleSeen) != 2 || !strings.Contains(stub.handleSeen[1], "offset=500") {
		t.Errorf("paginação inesperada: %v", stub.handleSeen)
	}
}
