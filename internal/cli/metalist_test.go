package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// mlStub simula as rotas metaList com as fixtures reais da homologação
// (2026-11-21) e guarda os registros em memória, para o create descobrir o id
// novo pela diferença. Reproduz o quirk do deleteRecord: só remove id string.
type mlStub struct {
	mu      sync.Mutex
	records map[int]map[string]any
	nextID  int
	saved   []map[string]any
	updated []map[string]any
	deletes int
}

func (s *mlStub) server(t *testing.T) *httptest.Server {
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var seed []map[string]any
	if err := json.Unmarshal(readTD("metalist_findAllListRecords.json"), &seed); err != nil {
		t.Fatal(err)
	}
	s.records = map[int]map[string]any{}
	for _, r := range seed {
		id := int(r["metaListRecordId"].(float64))
		s.records[id] = r
		if id >= s.nextID {
			s.nextID = id + 1
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	base := "/ecm/api/rest/ecm/metaList/"
	mux.HandleFunc(base+"findAllList", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("metalist_findAllList.json"))
	})
	mux.HandleFunc(base+"findListById/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/121") {
			w.Write(readTD("metalist_findListById.json"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(readTD("metalist_findListById_inexistente.json"))
	})
	mux.HandleFunc(base+"findListAndFields/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("metalist_findListAndFields.json"))
	})
	mux.HandleFunc(base+"findColumnNames/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("metalist_findColumnNames.json"))
	})
	mux.HandleFunc(base+"findAllListLabelRecords/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("metalist_findAllListLabelRecords.json"))
	})
	mux.HandleFunc(base+"findAllListRecords/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		ids := make([]int, 0, len(s.records))
		for id := range s.records {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		out := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			out = append(out, s.records[id])
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc(base+"findRecord/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		id, _ := strconv.Atoi(parts[len(parts)-1])
		s.mu.Lock()
		rec, ok := s.records[id]
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(readTD("metalist_findRecord_inexistente.json"))
			return
		}
		if id == 1 {
			w.Write(readTD("metalist_findRecord.json"))
			return
		}
		cp := map[string]any{}
		for k, v := range rec {
			if k != "metaListRecordId" {
				cp[k] = v
			}
		}
		json.NewEncoder(w).Encode(cp)
	})
	mux.HandleFunc(base+"saveNewRecord", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.saved = append(s.saved, body)
		rec := map[string]any{"metaListRecordId": float64(s.nextID)}
		for _, f := range body["dataRecord"].([]any) {
			fm := f.(map[string]any)
			rec[fm["columnNameId"].(string)] = fm["value"]
		}
		s.records[s.nextID] = rec
		s.nextID++
		s.mu.Unlock()
		io.WriteString(w, `{"msgId":"Registro inserido!"}`)
	})
	mux.HandleFunc(base+"updateRecord", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.updated = append(s.updated, body)
		s.mu.Unlock()
		io.WriteString(w, `{"msgId":"Registro inserido!"}`)
	})
	mux.HandleFunc(base+"deleteRecord/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.deletes++
		for _, v := range body["recordId"].([]any) {
			// Quirk real: id numérico é ignorado.
			if str, ok := v.(string); ok {
				id, _ := strconv.Atoi(str)
				delete(s.records, id)
			}
		}
		s.mu.Unlock()
		io.WriteString(w, `{"msgId":"Registro(s) removido(s)"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMetalistListTabela(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "list", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "ID", "Nome", "Cartórios", "87"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
}

func TestMetalistShowPorNome(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "show", "ZZ_FLUIGCLI_TEST_FILHO2", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	for _, want := range []string{"DATA_REF", "data", "metalista", "zz_fluigcli_test_pai (120)", "Desativada"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
}

func TestMetalistShowNaoEncontrada(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	for _, arg := range []string{"99999", "nao_existe"} {
		code, _ := runMain(t, "metalist", "show", arg, "--project", proj, "--server", "homolog", "--json")
		if code != output.ExitNotFound {
			t.Errorf("show %s: exit=%d, quer %d", arg, code, output.ExitNotFound)
		}
	}
}

func TestMetalistRecordsListTabelaEWhere(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "records", "list", "121", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "TEXTO", "NUMERICO", "r1_editado", "12.5", "Pai A"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "AREA") {
		t.Errorf("coluna desativada apareceu:\n%s", stdout)
	}

	code, stdout = runMain(t, "metalist", "records", "list", "121", "--where", "texto=R2_ISO", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	var env struct {
		Data struct {
			Records []struct {
				ID int `json:"id"`
			} `json:"records"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Records) != 1 || env.Data.Records[0].ID != 2 {
		t.Errorf("--where devolveu %+v, quer só o registro 2", env.Data.Records)
	}
}

func TestMetalistRecordsCreateValidaENormaliza(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "records", "create", "121",
		"--field", "Data Ref=2026-11-21", "--field", "numerico=1,5", "--field", "PAI=1", "--field", "texto=Ação",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, `"id":6`) {
		t.Errorf("envelope sem o id novo: %s", stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.saved) != 1 {
		t.Fatalf("saveNewRecord chamado %d vezes", len(stub.saved))
	}
	got := map[string]any{}
	for _, f := range stub.saved[0]["dataRecord"].([]any) {
		fm := f.(map[string]any)
		got[fm["columnNameId"].(string)] = fm["value"]
	}
	want := map[string]any{"DATA_REF": "21/11/2026", "NUMERICO": "1.5", "PAI": "1", "TEXTO": "Ação"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dataRecord = %v, quer %v", got, want)
	}
}

func TestMetalistRecordsCreateRecusaValorInvalido(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	cases := [][]string{
		{"--field", "NAOEXISTE=x"}, // o servidor criaria um registro vazio
		{"--field", "area=x"},      // coluna desativada
		{"--field", "INTEIRO=abc"},
		{"--field", "DATA_REF=21-11-2026"}, // o servidor gravaria null
		{"--field", "PAI=999"},             // FK inexistente
	}
	for _, c := range cases {
		args := append([]string{"metalist", "records", "create", "121"}, c...)
		args = append(args, "--json", "--project", proj, "--server", "homolog")
		code, _ := runMain(t, args...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d", c, code, output.ExitUsage)
		}
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.saved) != 0 {
		t.Errorf("gravou valor inválido: %v", stub.saved)
	}
}

func TestMetalistRecordsUpdateInexistente(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, _ := runMain(t, "metalist", "records", "update", "121", "999", "--field", "TEXTO=x", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d", code, output.ExitNotFound)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.updated) != 0 {
		t.Errorf("updateRecord chamado para id inexistente")
	}
}

func TestMetalistRecordsUpdate(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "records", "update", "zz_fluigcli_test_filho2", "1", "--field", "TEXTO=novo", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.updated) != 1 || stub.updated[0]["metaListRecordId"] != "1" || stub.updated[0]["listId"] != "121" {
		t.Errorf("updateRecord = %v", stub.updated)
	}
	// A coluna desativada (AREA) não sai no envelope.
	if strings.Contains(stdout, "AREA") {
		t.Errorf("envelope com coluna desativada: %s", stdout)
	}
}

func TestMetalistRecordsDeleteParcial(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "records", "delete", "121", "3", "999", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitPartial, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if _, ok := stub.records[3]; ok {
		t.Error("registro 3 não foi removido")
	}
	if !strings.Contains(stdout, `"id":"3","action":"deleted"`) || !strings.Contains(stdout, `"id":"999","action":"failed"`) {
		t.Errorf("resultados inesperados: %s", stdout)
	}
}
