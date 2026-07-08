package fluig

import (
	"context"
	"encoding/json"
	"errors"
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
	loadStatus  int // status para loadDataset (default 200)
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
		case "getDataset":
			w.Write(testdata(t, "soap_getDataset.xml"))
		default:
			http.Error(w, "op desconhecida", http.StatusInternalServerError)
		}
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

func TestListDatasets(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	list, err := c.ListDatasets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2, veio %d", len(list))
	}
	if !list[0].Custom || list[0].ID != "ds_exemplo" {
		t.Errorf("item[0] inesperado: %+v", list[0])
	}
	if list[1].Custom {
		t.Errorf("item[1] (DEFAULT) não deveria ser custom: %+v", list[1])
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

func TestQueryDataset(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	res, err := c.QueryDataset(context.Background(), "ds_exemplo", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Columns) != 3 || len(res.Rows) != 2 {
		t.Fatalf("resultado inesperado: %d colunas, %d linhas", len(res.Columns), len(res.Rows))
	}
	if res.Rows[1][1] != nil {
		t.Errorf("valor nil esperado na linha 1, col 1")
	}
}
