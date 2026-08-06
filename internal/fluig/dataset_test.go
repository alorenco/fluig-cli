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
	"slices"
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

	deleteSeen []string // ids recebidos no DELETE do helper (§2.11-H)
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
		// Valores de tipos mistos (bool/número/null), como o dataset `document`.
		if q.Get("datasetId") == "tipado" {
			io.WriteString(w, `{"columns":["documentId","active","size","nome"],"values":[
				{"documentId":42,"active":true,"size":1024,"nome":"contrato.pdf"},
				{"documentId":43,"active":false,"size":null,"nome":null}]}`)
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
	// fluigcliHelper: ping para o requireHelper e o DELETE de dataset.
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.10.3"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/datasets/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/fluigcliHelper/api/datasets/")
		s.deleteSeen = append(s.deleteSeen, id)
		// O helper devolve deleted:true mesmo para id inexistente — é
		// justamente o defeito que a CLI passou a cobrir (§2.11-H).
		fmt.Fprintf(w, `{"id":%q,"deleted":true}`, id)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Dataset inexistente: a CLI NÃO pode mandar o DELETE nem reportar sucesso. O
// helper responde {"deleted":true} para qualquer id (medido na homologação em
// 2026-07-27), então a garantia tem de vir da confirmação prévia (§2.11-H).
func TestDeleteDatasetPermanentlyInexistenteNaoApaga(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)

	err := c.DeleteDatasetPermanently(context.Background(), "nao_existe")
	if err == nil {
		t.Fatal("esperava erro para dataset inexistente")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
	if !strings.Contains(err.Error(), "nada foi excluído") {
		t.Errorf("a mensagem tem de deixar claro que nada foi apagado: %v", err)
	}
	if len(stub.deleteSeen) != 0 {
		t.Errorf("o DELETE não podia ter sido enviado, mas foi: %v", stub.deleteSeen)
	}
}

// Dataset existente: segue apagando normalmente.
func TestDeleteDatasetPermanentlyExistente(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)

	if err := c.DeleteDatasetPermanently(context.Background(), "ds_exemplo"); err != nil {
		t.Fatalf("DeleteDatasetPermanently: %v", err)
	}
	if len(stub.deleteSeen) != 1 || stub.deleteSeen[0] != "ds_exemplo" {
		t.Errorf("esperava DELETE de ds_exemplo, veio %v", stub.deleteSeen)
	}
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
	// A fixture é real e traz 3 colunas para 2 campos pedidos — o servidor
	// repassa `field` ao dataset, que pode ignorá-lo. A CLI recorta no cliente
	// (2026-08-06), então sobram as 2 colunas pedidas, na ordem pedida.
	if len(res.Columns) != 2 || len(res.Rows) != 3 {
		t.Fatalf("resultado inesperado: %d colunas, %d linhas", len(res.Columns), len(res.Rows))
	}
	if res.Columns[0] != "colleagueName" || res.Columns[1] != "login" {
		t.Errorf("colunas fora do recorte/ordem pedidos: %v", res.Columns)
	}
	if v := res.Rows[0]["login"]; v == nil || *v != "ana.andrade" {
		t.Errorf("linha[0].login inesperado: %v", v)
	}
	if _, ok := res.Rows[0]["active"]; ok {
		t.Errorf("coluna não pedida sobrou na linha: %v", res.Rows[0])
	}
	if len(res.MissingFields) != 0 {
		t.Errorf("nenhum campo pedido está ausente, mas veio %v", res.MissingFields)
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

// O recorte por --fields acontece no cliente, porque dataset customizado que
// monta as próprias colunas ignora o parâmetro `field` (ROADMAP3 §4.10).
func TestQueryDatasetRecorteDeCampos(t *testing.T) {
	casos := []struct {
		nome    string
		fields  []string
		colunas []string
		ausente []string
	}{
		{"ordem pedida vence a do servidor", []string{"login", "colleagueName"},
			[]string{"login", "colleagueName"}, nil},
		{"um campo só", []string{"login"}, []string{"login"}, nil},
		{"caixa diferente casa e preserva o nome real", []string{"LOGIN"},
			[]string{"login"}, nil},
		{"campo repetido não duplica a coluna", []string{"login", "login"},
			[]string{"login"}, nil},
		{"campo inexistente é reportado, o resto é recortado", []string{"login", "naoExiste"},
			[]string{"login"}, []string{"naoExiste"}},
		// Recortar para nada esconderia o dado de quem só errou o nome.
		{"nenhum campo existe: devolve tudo e reporta", []string{"nadaA", "nadaB"},
			[]string{"colleagueName", "login", "active"}, []string{"nadaA", "nadaB"}},
		{"sem --fields nada é recortado", nil,
			[]string{"colleagueName", "login", "active"}, nil},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			stub := &datasetStub{}
			c := datasetClient(t, stub.server(t).URL)
			res, err := c.QueryDataset(context.Background(), "colleague", DatasetQuery{Fields: tc.fields})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(res.Columns, ",") != strings.Join(tc.colunas, ",") {
				t.Errorf("colunas = %v, quer %v", res.Columns, tc.colunas)
			}
			if strings.Join(res.MissingFields, ",") != strings.Join(tc.ausente, ",") {
				t.Errorf("MissingFields = %v, quer %v", res.MissingFields, tc.ausente)
			}
			// A linha nunca pode guardar coluna fora do recorte.
			for _, row := range res.Rows {
				for k := range row {
					if !slices.Contains(res.Columns, k) {
						t.Errorf("linha tem a coluna %q, fora de %v", k, res.Columns)
					}
				}
			}
		})
	}
}

// A REST materializa uma linha vazia quando o dataset não devolve linha
// nenhuma (ROADMAP3 §4.9, reproduzido ao vivo na resposta crua). A CLI precisa
// reconhecer o caso sem descartar a linha.
func TestDatasetResultOnlyEmptyRow(t *testing.T) {
	str := func(s string) *string { return &s }
	casos := []struct {
		nome string
		res  DatasetResult
		quer bool
	}{
		{"linha única toda vazia é suspeita",
			DatasetResult{Columns: []string{"a", "b"}, Rows: []map[string]*string{{"a": str(""), "b": str("")}}}, true},
		{"campo nulo conta como vazio",
			DatasetResult{Columns: []string{"a", "b"}, Rows: []map[string]*string{{"a": nil, "b": str("")}}}, true},
		{"linha única com um valor NÃO é suspeita",
			DatasetResult{Columns: []string{"a", "b"}, Rows: []map[string]*string{{"a": str("x"), "b": str("")}}}, false},
		// Com resultado real o servidor não acrescenta a linha fantasma
		// (conferido ao vivo com 10 linhas do dataset `document`).
		{"duas linhas nunca são suspeitas",
			DatasetResult{Columns: []string{"a"}, Rows: []map[string]*string{{"a": str("")}, {"a": str("")}}}, false},
		{"resultado vazio de verdade não é suspeito",
			DatasetResult{Columns: []string{"a"}}, false},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			if got := tc.res.OnlyEmptyRow(); got != tc.quer {
				t.Errorf("OnlyEmptyRow() = %v, quer %v", got, tc.quer)
			}
		})
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

// Valores não-string (bool/número/null) são coagidos para *string: o literal
// JSON vira texto e null vira ausência. Regressão do erro real observado no
// dataset `document` (2026-07-15): "cannot unmarshal bool into string".
func TestQueryDatasetValoresTipados(t *testing.T) {
	stub := &datasetStub{}
	c := datasetClient(t, stub.server(t).URL)
	res, err := c.QueryDataset(context.Background(), "tipado", DatasetQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("esperava 2 linhas, veio %d", len(res.Rows))
	}
	get := func(row int, col string) string {
		if v := res.Rows[row][col]; v != nil {
			return *v
		}
		return "<nil>"
	}
	if get(0, "documentId") != "42" || get(0, "active") != "true" || get(0, "size") != "1024" || get(0, "nome") != "contrato.pdf" {
		t.Errorf("linha 0 mal coagida: %v", res.Rows[0])
	}
	if get(1, "active") != "false" || res.Rows[1]["size"] != nil || res.Rows[1]["nome"] != nil {
		t.Errorf("linha 1 (null) mal coagida: %v", res.Rows[1])
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
