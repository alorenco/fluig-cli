package fluig

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// processConvertStub serve as fixtures reais da API legada de conversão.
type processConvertStub struct {
	queries        []url.Values
	convertBody    string // corpo recebido no convertProcess
	convertLog     string // conversionLog a responder
	htmlRedirect   bool   // simula sessão sem papel de admin (HTML 200)
	instancesTotal int    // >0: gera instâncias sintéticas com o quirk rows+1
}

// writeSyntheticInstances reproduz a paginação medida ao vivo: com página
// seguinte, o grid devolve rows+1 itens e o último repete como primeiro da
// próxima página.
func (s *processConvertStub) writeSyntheticInstances(w http.ResponseWriter, r *http.Request) {
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	start := (page - 1) * rows // o item-espião reaparece aqui naturalmente
	end := start + rows
	if end < s.instancesTotal {
		end++ // item-espião da próxima página
	}
	if end > s.instancesTotal {
		end = s.instancesTotal
	}
	var items []string
	for i := start; i < end; i++ {
		items = append(items, `{"processInstanceId":`+strconv.Itoa(150000+i)+
			`,"processDescription":"Compras","requesterName":"João Silva",`+
			`"stateDescription":"Aguardar Nota","colleagueName":"João Silva","deadlineText":"Desde 01/09/2024 00:00:00"}`)
	}
	io.WriteString(w, `{"totalpages":0,"totalrecords":"0","currpage":`+strconv.Itoa(page)+
		`,"invdata":[`+strings.Join(items, ",")+`]}`)
}

func (s *processConvertStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	fixture := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	guard := func(w http.ResponseWriter) bool {
		if s.htmlRedirect {
			io.WriteString(w, `<html><body><script>window.location.replace("/portal/home");</script></body></html>`)
			return true
		}
		return false
	}
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/getAllProcessVersions", func(w http.ResponseWriter, r *http.Request) {
		s.queries = append(s.queries, r.URL.Query())
		if guard(w) {
			return
		}
		w.Write(fixture("rest_processconvert_versions.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/getOpenProcessStateVersions", func(w http.ResponseWriter, r *http.Request) {
		s.queries = append(s.queries, r.URL.Query())
		w.Write(fixture("rest_processconvert_state_counts.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/getInstancesToConvert", func(w http.ResponseWriter, r *http.Request) {
		s.queries = append(s.queries, r.URL.Query())
		if s.instancesTotal > 0 {
			s.writeSyntheticInstances(w, r)
			return
		}
		w.Write(fixture("rest_processconvert_instances.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/processconvert/convertProcess", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.convertBody = string(body)
		log := s.convertLog
		if log == "" {
			log = "Solicitação convertida com sucesso!"
		}
		io.WriteString(w, `{"conversionLog":"`+log+`","conversionSequence":0}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestConvertVersions(t *testing.T) {
	stub := &processConvertStub{}
	c := newTestClient(t, stub.server(t).URL, "user-convert-versions", "p")
	got, err := c.ConvertVersions(context.Background(), "Compras")
	if err != nil {
		t.Fatal(err)
	}
	// Fixture real reduzida: versões 1, 20 e 29 com 2, 41 e 95 tarefas abertas.
	want := []ConvertVersionInfo{{1, 2}, {20, 41}, {29, 95}}
	if len(got) != len(want) {
		t.Fatalf("esperava %d versões, veio %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("versão[%d] = %+v, esperava %+v", i, got[i], w)
		}
	}
	if q := stub.queries[0].Get("processId"); q != "Compras" {
		t.Errorf("processId enviado = %q", q)
	}
}

func TestConvertStateCounts(t *testing.T) {
	stub := &processConvertStub{}
	c := newTestClient(t, stub.server(t).URL, "user-convert-counts", "p")
	got, err := c.ConvertStateCounts(context.Background(), "Compras", 20, []int{4, 5, 13, 17, 20, 27, 29, 30, 66, 72, 74, 76})
	if err != nil {
		t.Fatal(err)
	}
	// Fixture real: 5 tarefas na etapa 13 e 36 na 17; as demais zeradas somem.
	if len(got) != 2 || got[13] != 5 || got[17] != 36 {
		t.Errorf("contagens = %v, esperava map[13:5 17:36]", got)
	}
	if q := stub.queries[0].Get("states"); q != "[4,5,13,17,20,27,29,30,66,72,74,76]" {
		t.Errorf("states enviado = %q", q)
	}
}

// Fixture real (16 solicitações com rows=15 — o quirk rows+1 ao vivo): com o
// pageSize de 100 do cliente cabe numa página só.
func TestConvertInstancesFixture(t *testing.T) {
	stub := &processConvertStub{}
	c := newTestClient(t, stub.server(t).URL, "user-convert-instances", "p")
	got, err := c.ConvertInstances(context.Background(), "Compras", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 16 {
		t.Fatalf("esperava 16 solicitações, veio %d", len(got))
	}
	first := got[0]
	if first.ID != 151158 || first.State != "Aguardar Nota" || first.Assignee != "Para o Papel Follow-up de Compras" {
		t.Errorf("primeira solicitação inesperada: %+v", first)
	}
}

// Paginação com o quirk rows+1: 130 solicitações → página de 101 itens (corta
// o espião) + página de 30, sem duplicar nem perder.
func TestConvertInstancesPaginado(t *testing.T) {
	stub := &processConvertStub{instancesTotal: 130}
	c := newTestClient(t, stub.server(t).URL, "user-convert-paginado", "p")
	got, err := c.ConvertInstances(context.Background(), "Compras", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 130 {
		t.Fatalf("esperava 130 solicitações, veio %d", len(got))
	}
	seen := map[int]bool{}
	for _, in := range got {
		if seen[in.ID] {
			t.Fatalf("solicitação %d duplicada (item-espião não descartado)", in.ID)
		}
		seen[in.ID] = true
	}
	if len(stub.queries) < 2 {
		t.Errorf("esperava 2 páginas consultadas, veio %d", len(stub.queries))
	}
}

func TestConvertInstanceVersion(t *testing.T) {
	stub := &processConvertStub{}
	c := newTestClient(t, stub.server(t).URL, "user-convert-post", "p")
	log, err := c.ConvertInstanceVersion(context.Background(), ConvertCall{
		InstanceID:   151158,
		NewVersion:   29,
		ActualStates: []int{4, 17},
		NewStates:    []int{4, 17},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "sucesso") {
		t.Errorf("conversionLog = %q", log)
	}
	for _, want := range []string{`"processInstanceId":151158`, `"newVersion":29`, `"actualStates":[4,17]`, `"newStates":[4,17]`, `"conversionSequence":0`} {
		if !strings.Contains(stub.convertBody, want) {
			t.Errorf("corpo do convertProcess sem %s: %s", want, stub.convertBody)
		}
	}
}

// Sessão sem papel de admin: o portal responde 200 com HTML de redirect (não
// 401) — o guard traduz para ErrAuthFailed com mensagem acionável.
func TestConvertVersionsSemAdmin(t *testing.T) {
	stub := &processConvertStub{htmlRedirect: true}
	c := newTestClient(t, stub.server(t).URL, "user-convert-noadmin", "p")
	_, err := c.ConvertVersions(context.Background(), "Compras")
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("esperava ErrAuthFailed, veio %v", err)
	}
	if !strings.Contains(err.Error(), "administrador") {
		t.Errorf("mensagem sem a dica de admin: %v", err)
	}
}
