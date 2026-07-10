package fluig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// processStub simula login/ping + a listagem paginada de processos e o ciclo
// export/import/versions/release de publish (REST v2).
type processStub struct {
	pages [][]byte // resposta de cada página, na ordem
	seen  []string // query strings recebidas
	fail  int      // se >0, responde esse status HTTP na listagem

	version      int    // última versão (import incrementa; default 1)
	importedXML  []byte // corpo recebido no import/xml
	releaseCalls int
	releaseFail  bool // release responde 400 de negócio
}

func (s *processStub) server(t *testing.T) *httptest.Server {
	if s.version == 0 {
		s.version = 1
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		if s.fail > 0 {
			http.Error(w, `{"message":"erro"}`, s.fail)
			return
		}
		s.seen = append(s.seen, r.URL.RawQuery)
		page := len(s.seen) - 1
		if page >= len(s.pages) {
			t.Errorf("página %d pedida além das %d disponíveis", page+1, len(s.pages))
			io.WriteString(w, `{"items":[],"hasNext":false}`)
			return
		}
		w.Write(s.pages[page])
	})
	// Rotas por processo (export/import/versions/release), como na homologação.
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/export/xml"):
			if strings.Contains(path, "naoExiste") {
				http.Error(w, `{"code":"NotFound","message":"processo inexistente"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/xml;charset=UTF-8")
			w.Write(testdata(t, "rest_process_export.xml"))
		case strings.HasSuffix(path, "/import/xml"):
			s.importedXML, _ = io.ReadAll(r.Body)
			s.version++
			io.WriteString(w, `{"processId":"zz_fluigcli_test_pub","versions":null}`)
		case strings.HasSuffix(path, "/process-versions/latest/release"):
			if s.releaseFail {
				http.Error(w, `{"code":"BPMProcessDefinitionVersionOnReleaseException","message":"Versão do processo contém erros"}`, http.StatusBadRequest)
				return
			}
			s.releaseCalls++
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "/process-versions"):
			fmt.Fprintf(w, `{"items":[{"version":%d,"active":true,"editing":true}],"hasNext":false}`, s.version)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func processClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-proc-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A listagem percorre as páginas até hasNext=false e agrega os itens.
func TestListProcessesPaginado(t *testing.T) {
	stub := &processStub{pages: [][]byte{
		testdata(t, "rest_processes_page1.json"),
		testdata(t, "rest_processes_page2.json"),
	}}
	c := processClient(t, stub.server(t).URL)
	procs, err := c.ListProcesses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 4 {
		t.Fatalf("esperava 4 processos (3+1), veio %d", len(procs))
	}
	if len(stub.seen) != 2 {
		t.Fatalf("esperava 2 requisições, houve %d: %v", len(stub.seen), stub.seen)
	}
	if stub.seen[0] != "page=1&pageSize=100" || stub.seen[1] != "page=2&pageSize=100" {
		t.Errorf("query strings inesperadas: %v", stub.seen)
	}
	first := procs[0]
	if first.ID != "Compras" || first.Description != "Compras" || first.Category != "Suprimentos" || !first.Active {
		t.Errorf("processo[0] inesperado: %+v", first)
	}
	if !procs[2].Public {
		t.Errorf("processo[2] deveria ser público: %+v", procs[2])
	}
	// Item real sem a chave categoryId (processo ad-hoc) → Category vazia.
	last := procs[3]
	if last.ID != "zz_fluigcli_test_proc" || last.Category != "" {
		t.Errorf("processo[3] inesperado: %+v", last)
	}
}

// ApplyProcessEventScripts troca o código do evento no XML real do export,
// escapando XML, e o resultado continua parseável pelo leitor de eventos.
func TestApplyProcessEventScripts(t *testing.T) {
	xmlData := testdata(t, "rest_process_export.xml")
	novo := "function beforeTaskSave(a,b,c){ if (1 < 2 && 'x') { return \"ok\"; } }"
	out, updated, missing := ApplyProcessEventScripts(xmlData, map[string]string{"beforeTaskSave": novo})
	if len(missing) != 0 || len(updated) != 1 || updated[0] != "beforeTaskSave" {
		t.Fatalf("updated=%v missing=%v", updated, missing)
	}
	s := string(out)
	if !strings.Contains(s, "1 &lt; 2 &amp;&amp;") {
		t.Errorf("script não foi escapado para XML:\n%s", s)
	}
	if strings.Contains(s, "/* v1 fluigcli */") {
		t.Error("código antigo ainda presente no XML")
	}
	// O restante do documento fica intacto.
	if !strings.Contains(s, "<processDescription>Processo de teste do fluigcli (pode apagar)</processDescription>") {
		t.Error("XML fora do evento foi alterado")
	}
	// Round-trip: o leitor de eventos (usado pelo diff) decodifica o código novo.
	events, err := parseProcessDefinitionEvents(out)
	if err != nil {
		t.Fatalf("XML resultante inválido: %v", err)
	}
	if events["beforeTaskSave"] != novo {
		t.Errorf("round-trip do script falhou:\n%q", events["beforeTaskSave"])
	}
}

// Evento local sem correspondente no XML entra em missing (nada é criado).
func TestApplyProcessEventScriptsEventoAusente(t *testing.T) {
	xmlData := testdata(t, "rest_process_export.xml")
	out, updated, missing := ApplyProcessEventScripts(xmlData, map[string]string{
		"beforeTaskSave": "x", "naoExiste": "y",
	})
	if len(updated) != 1 || len(missing) != 1 || missing[0] != "naoExiste" {
		t.Fatalf("updated=%v missing=%v", updated, missing)
	}
	if strings.Contains(string(out), "naoExiste") {
		t.Error("evento ausente não deveria ser criado no XML")
	}
}

// eventDescription vazio auto-fechado (<eventDescription/>) também é aceito.
func TestApplyProcessEventScriptsDescricaoVazia(t *testing.T) {
	xmlData := strings.Replace(string(testdata(t, "rest_process_export.xml")),
		"<eventDescription>function beforeTaskSave(colleagueId,nextSequenceId,userList){ /* v1 fluigcli */ }</eventDescription>",
		"<eventDescription/>", 1)
	out, updated, missing := ApplyProcessEventScripts([]byte(xmlData), map[string]string{"beforeTaskSave": "novo()"})
	if len(missing) != 0 || len(updated) != 1 {
		t.Fatalf("updated=%v missing=%v", updated, missing)
	}
	if !strings.Contains(string(out), "<eventDescription>novo()</eventDescription>") {
		t.Errorf("descrição vazia não foi preenchida:\n%s", out)
	}
}

// Export/import/versions/release contra o stub HTTP.
func TestProcessPublishClientOps(t *testing.T) {
	stub := &processStub{}
	srv := stub.server(t)
	c := processClient(t, srv.URL)
	ctx := context.Background()

	if _, err := c.ExportProcessXML(ctx, "naoExiste"); !errors.Is(err, ErrNotFound) {
		t.Errorf("export de processo inexistente: err=%v, quer ErrNotFound", err)
	}
	xmlData, err := c.ExportProcessXML(ctx, "zz_fluigcli_test_pub")
	if err != nil || !bytes.Contains(xmlData, []byte("<ProcessDefinition>")) {
		t.Fatalf("export: err=%v", err)
	}
	if err := c.ImportProcessXML(ctx, "zz_fluigcli_test_pub", xmlData); err != nil {
		t.Fatalf("import: %v", err)
	}
	if !bytes.Equal(stub.importedXML, xmlData) {
		t.Error("corpo do import difere do XML enviado")
	}
	vs, err := c.ProcessVersions(ctx, "zz_fluigcli_test_pub")
	if err != nil || LatestProcessVersion(vs) != 2 {
		t.Fatalf("versions=%v err=%v, quer última=2", vs, err)
	}
	if err := c.ReleaseLatestProcessVersion(ctx, "zz_fluigcli_test_pub"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if stub.releaseCalls != 1 {
		t.Errorf("release chamado %d vezes", stub.releaseCalls)
	}
	// Erro de negócio (400 {code,message}) vira rejeição com a mensagem do servidor.
	stub.releaseFail = true
	err = c.ReleaseLatestProcessVersion(ctx, "zz_fluigcli_test_pub")
	if err == nil || !strings.Contains(err.Error(), "contém erros") {
		t.Errorf("erro de negócio inesperado: %v", err)
	}
}

// --- estados e vínculo com formulário (simulação do `fluigcli dev`) ---
//
// As fixtures rest_process_states*.json e rest_processes_expand.json são
// respostas REAIS da homologação (2026-07-10, processo rh_justificativa_ponto
// v24), recortadas: a paginação dos states foi dividida em duas páginas para
// exercitar o hasNext (a resposta real veio numa página só).

// simStub simula login/ping + states paginados + listagem com expand=versions.
type simStub struct {
	statesPages [][]byte
	expandBody  []byte
	seen        []string
}

func (s *simStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		s.seen = append(s.seen, r.URL.RawQuery)
		w.Write(s.expandBody)
	})
	statesCalls := 0
	mux.HandleFunc("/process-management/api/v2/processes/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/states") {
			http.NotFound(w, r)
			return
		}
		s.seen = append(s.seen, r.URL.Path+"?"+r.URL.RawQuery)
		if statesCalls >= len(s.statesPages) {
			io.WriteString(w, `{"items":[],"hasNext":false}`)
			return
		}
		w.Write(s.statesPages[statesCalls])
		statesCalls++
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ProcessStates agrega as páginas e devolve os estados ordenados por sequence.
func TestProcessStatesPaginadoOrdenado(t *testing.T) {
	stub := &simStub{statesPages: [][]byte{
		testdata(t, "rest_process_states.json"),
		testdata(t, "rest_process_states_page2.json"),
	}}
	c := processClient(t, stub.server(t).URL)
	states, err := c.ProcessStates(context.Background(), "rh_justificativa_ponto", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 5 {
		t.Fatalf("esperava 5 estados (3+2), veio %d: %+v", len(states), states)
	}
	var seqs []int
	for _, st := range states {
		seqs = append(seqs, st.Sequence)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i-1] > seqs[i] {
			t.Fatalf("estados fora de ordem: %v", seqs)
		}
	}
	// Valores reais da homologação: o "Início" do diagrama é sequence 6 (a
	// solicitação nova abre com WKNumState=0, que não aparece na lista).
	if states[0].Sequence != 6 || states[0].Name != "Início" || states[0].BpmnType != "START_EVENT_NORMAL" {
		t.Errorf("estado[0] inesperado: %+v", states[0])
	}
	if states[2].Sequence != 9 || states[2].StateType != "SIMPLE" || states[2].Name != "Revisar Processo" {
		t.Errorf("estado[2] inesperado: %+v", states[2])
	}
	// A URL usa a versão pedida.
	if !strings.Contains(stub.seen[0], "/process-versions/24/states") {
		t.Errorf("caminho inesperado: %s", stub.seen[0])
	}
}

// FindProcessesByFormID casa o formId nas versões e devolve a maior versão.
func TestFindProcessesByFormID(t *testing.T) {
	stub := &simStub{expandBody: testdata(t, "rest_processes_expand.json")}
	c := processClient(t, stub.server(t).URL)
	links, err := c.FindProcessesByFormID(context.Background(), 1109734)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("esperava 1 processo, veio %d: %+v", len(links), links)
	}
	l := links[0]
	if l.ProcessID != "rh_justificativa_ponto" || l.Description != "Justificativa de Ponto" || l.Version != 24 {
		t.Errorf("vínculo inesperado: %+v", l)
	}
	if !strings.Contains(stub.seen[0], "expand=versions") {
		t.Errorf("faltou expand=versions na query: %s", stub.seen[0])
	}
	// Form sem processo → lista vazia, sem erro.
	if links, err = c.FindProcessesByFormID(context.Background(), 999999); err != nil || len(links) != 0 {
		t.Errorf("form sem processo: links=%v err=%v", links, err)
	}
	// formId inválido é recusado antes de bater no servidor.
	if _, err = c.FindProcessesByFormID(context.Background(), 0); err == nil {
		t.Error("formId 0 deveria ser recusado")
	}
}

// Erro HTTP do servidor vira HTTPError (não "não encontrado").
func TestListProcessesErroHTTP(t *testing.T) {
	stub := &processStub{fail: http.StatusForbidden}
	c := processClient(t, stub.server(t).URL)
	_, err := c.ListProcesses(context.Background())
	if err == nil {
		t.Fatal("esperava erro")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusForbidden {
		t.Errorf("erro inesperado: %v", err)
	}
}
