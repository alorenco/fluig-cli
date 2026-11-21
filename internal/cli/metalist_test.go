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

	// Listas (fase 2). hide121 tira a lista 121 da listagem até um
	// saveNewList, que a "cria" (o findListAndFields dela é a fixture).
	hide121     bool
	lists       []map[string]any
	newLists    []map[string]any
	listUpdates []map[string]any
	removed     []string
	failUpdate  string // texto do 500 no próximo updateList com coluna nova
	failRemove  string
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
	if err := json.Unmarshal(readTD("metalist_findAllList.json"), &s.lists); err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc(base+"findAllList", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []map[string]any{}
		for _, l := range s.lists {
			if s.hide121 && l["listId"] == float64(121) {
				continue
			}
			out = append(out, l)
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc(base+"saveNewList", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.newLists = append(s.newLists, body)
		for _, l := range s.lists {
			if l["listId"] == float64(121) {
				l["listName"] = body["name"]
			}
		}
		s.hide121 = false
		s.mu.Unlock()
		io.WriteString(w, `{}`)
	})
	mux.HandleFunc(base+"updateList", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.listUpdates = append(s.listUpdates, body)
		fail := s.failUpdate
		s.mu.Unlock()
		if fail != "" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(readTD(fail))
			return
		}
		w.Write(readTD("metalist_updateList_ok.json"))
	})
	mux.HandleFunc(base+"removeList/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		fail := s.failRemove
		for _, v := range body["listId"].([]any) {
			s.removed = append(s.removed, v.(string))
		}
		s.mu.Unlock()
		if fail != "" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(readTD(fail))
			return
		}
		io.WriteString(w, `{"msgId":"Lista(s) removida (s)"}`)
	})
	mux.HandleFunc(base+"findListById/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/121") || strings.HasSuffix(r.URL.Path, "/120") {
			w.Write(readTD("metalist_findListById.json"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(readTD("metalist_findListById_inexistente.json"))
	})
	mux.HandleFunc(base+"findListAndFields/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/120") {
			// Lista pai de teste da homologação: uma coluna de texto, rótulo.
			io.WriteString(w, `{"name":"zz_fluigcli_test_pai","description":"teste fluigcli","listId":120,"fieldsVO":[{"title":"nome","label":true,"type":1,"idRow":0,"fieldSeq":1,"metalistFKId":0,"metalistFKName":null,"disabled":false,"newField":null}]}`)
			return
		}
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

func TestMetalistCreateValidaEDescobreID(t *testing.T) {
	stub := &mlStub{hide121: true}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "create", "zz_nova",
		"--column", "Descrição:texto", "--column", "Data Ref:data", "--column", "pai:metalista:zz_fluigcli_test_pai",
		"--label", "descrição", "--description", "teste",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, `"id":121`) {
		t.Errorf("envelope sem o id da lista nova: %s", stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.newLists) != 1 {
		t.Fatalf("saveNewList chamado %d vezes", len(stub.newLists))
	}
	body := stub.newLists[0]
	if body["name"] != "zz_nova" || body["description"] != "teste" {
		t.Errorf("payload = %#v", body)
	}
	fields := body["fieldsVO"].([]any)
	f0, f2 := fields[0].(map[string]any), fields[2].(map[string]any)
	if f0["label"] != true || f0["type"] != "1" || f0["title"] != "Descrição" {
		t.Errorf("coluna 0 = %#v", f0)
	}
	if f2["type"] != "6" || f2["metalistFKId"] != float64(120) || f2["metalistFKName"] != "zz_fluigcli_test_pai" {
		t.Errorf("coluna FK = %#v", f2)
	}
}

func TestMetalistCreateRecusaAntesDeGravar(t *testing.T) {
	stub := &mlStub{hide121: true}
	proj := mechProject(t, stub.server(t).URL)
	cases := [][]string{
		{"Cartórios", "--column", "a:texto"},    // nome já usado
		{"zz_x"},                                // sem coluna
		{"zz_x", "--column", "cidade/uf:texto"}, // "/" quebraria o SQL
		{"zz_x", "--column", "Ação:texto", "--column", "acao:texto"}, // mesmo nome gerado
		{"zz_x", "--column", "a:booleano"},                           // tipo inválido
		{"zz_x", "--column", "pai:metalista"},                        // FK sem lista
		{"zz_x", "--column", "pai:metalista:120", "--label", "pai"},  // FK não é rótulo
		{"zz_x", "--column", "a:texto", "--label", "b"},              // rótulo inexistente
		{"zz_x", "--column", "abcdefghijabcdefghijabcdefghijk:texto"},
	}
	for _, c := range cases {
		args := append([]string{"metalist", "create"}, c...)
		args = append(args, "--json", "--project", proj, "--server", "homolog")
		code, _ := runMain(t, args...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d", c, code, output.ExitUsage)
		}
	}
	code, _ := runMain(t, "metalist", "create", "zz_x", "--column", "pai:metalista:99999", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("lista referenciada inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.newLists) != 0 {
		t.Errorf("gravou com entrada inválida: %v", stub.newLists)
	}
}

func TestMetalistUpdateDuasChamadas(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "update", "121",
		"--name", "zz_renomeada", "--add-column", "Obs:area-texto", "--enable-column", "area", "--no-label", "texto",
		"--json", "--project", proj, "--server", "homolog")
	// A fixture não tem a coluna nova OBS depois do update: a CLI acusa.
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.listUpdates) != 2 {
		t.Fatalf("updateList chamado %d vezes, quer 2", len(stub.listUpdates))
	}
	first, second := stub.listUpdates[0], stub.listUpdates[1]
	// Passo 1: colunas com o nome ATUAL.
	if first["name"] != "zz_fluigcli_test_filho2" {
		t.Errorf("passo 1 com nome %v, quer o atual", first["name"])
	}
	fields := first["fieldsVO"].([]any)
	if len(fields) != 3 {
		t.Fatalf("passo 1: %d colunas, quer 3 (area, texto, Obs)", len(fields))
	}
	for _, f := range fields {
		fm := f.(map[string]any)
		switch fm["title"] {
		case "area":
			if fm["disabled"] != false || fm["newField"] != false {
				t.Errorf("area = %#v", fm)
			}
		case "texto":
			if fm["label"] != false {
				t.Errorf("texto = %#v", fm)
			}
		case "Obs":
			if fm["newField"] != true || fm["type"] != "2" {
				t.Errorf("Obs = %#v", fm)
			}
		default:
			t.Errorf("coluna inesperada %#v", fm)
		}
	}
	// Passo 2: nome novo sem colunas.
	if second["name"] != "zz_renomeada" || len(second["fieldsVO"].([]any)) != 0 {
		t.Errorf("passo 2 = %#v", second)
	}
}

func TestMetalistUpdateRecusaColisaoESemMudanca(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	cases := [][]string{
		{},                              // nada a alterar
		{"--add-column", "Texto:texto"}, // colide com TEXTO
		{"--add-column", "Área:texto"},  // colide com a desativada AREA
		{"--name", "Cartórios"},         // nome de outra lista
		{"--label", "pai"},              // FK não é rótulo
		{"--label", "texto", "--no-label", "texto"},
		{"--disable-column", "naoexiste"},
	}
	for _, c := range cases {
		args := append([]string{"metalist", "update", "121"}, c...)
		args = append(args, "--json", "--project", proj, "--server", "homolog")
		code, _ := runMain(t, args...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d", c, code, output.ExitUsage)
		}
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.listUpdates) != 0 {
		t.Errorf("gravou com entrada inválida: %v", stub.listUpdates)
	}
}

func TestMetalistUpdateRejeicaoNaoRenomeia(t *testing.T) {
	stub := &mlStub{failUpdate: "metalist_updateList_reservada.txt"}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "update", "121", "--name", "zz_outro", "--add-column", "select:texto",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer || !strings.Contains(stdout, "palavra reservada") {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	// Só o passo 1 rodou, com o nome atual: a lista não foi renomeada.
	if len(stub.listUpdates) != 1 || stub.listUpdates[0]["name"] != "zz_fluigcli_test_filho2" {
		t.Errorf("updates = %v", stub.listUpdates)
	}
}

func TestMetalistDeleteReferenciada(t *testing.T) {
	stub := &mlStub{failRemove: "metalist_removeList_referenciada.txt"}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "delete", "121", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitServer || !strings.Contains(stdout, "relacionada em outras listas") {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
}

func TestMetalistDeleteLote(t *testing.T) {
	stub := &mlStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "metalist", "delete", "zz_fluigcli_test_filho2", "nao_existe", "--yes", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitPartial {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitPartial, stdout)
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.removed) != 1 || stub.removed[0] != "121" {
		t.Errorf("removeList = %v, quer [\"121\"]", stub.removed)
	}
}
