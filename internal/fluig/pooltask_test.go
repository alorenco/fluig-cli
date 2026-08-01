package fluig

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// poolTaskStub simula login/ping + findUserByLogin + a rota legada da central
// de tarefas (getTasks/pool). Sem pages, serve a fixture real sanitizada; com
// pages, gera as páginas com o comportamento medido ao vivo: rows+1 itens
// quando há próxima página, e o último item repetido como primeiro da seguinte.
type poolTaskStub struct {
	pages     [][]string // itens JSON por página (já com o "espião" incluído)
	userCodes []string   // userCode visto no caminho a cada chamada
	queries   []url.Values
}

func (s *poolTaskStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"`+r.URL.Query().Get("login")+`","userCode":"uc-pool-test"}}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/centralTasks/getTasks/pool/", func(w http.ResponseWriter, r *http.Request) {
		s.userCodes = append(s.userCodes, strings.TrimPrefix(r.URL.Path, "/ecm/api/rest/ecm/centralTasks/getTasks/pool/"))
		s.queries = append(s.queries, r.URL.Query())
		if s.pages == nil {
			b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_central_tasks_pool.json"))
			if err != nil {
				t.Error(err)
			}
			w.Write(b)
			return
		}
		page := r.URL.Query().Get("page")
		for i, items := range s.pages {
			if page == fmt.Sprint(i+1) {
				// totalpages/totalrecords reproduzem o lixo real (rows+1 acumulado).
				fmt.Fprintf(w, `{"totalpages":%d,"totalrecords":"%d","currpage":%d,"invdata":[%s]}`,
					i+2, (i+1)*101, i+1, strings.Join(items, ","))
				return
			}
		}
		io.WriteString(w, `{"totalpages":1,"totalrecords":"0","currpage":1,"invdata":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// poolItem gera um item da rota legada com os campos que o parse usa.
func poolItem(id int, expired, warning bool) string {
	return fmt.Sprintf(`{"processInstanceId":%d,"processId":"contratos_troca_contribuinte",
		"processDescription":"Troca de Contribuinte","requesterId":"jsilva","requesterName":"João Silva",
		"requesterLogin":"jsilva","stateDescription":"Corrigir Integração","colleagueName":"Para o Grupo TI",
		"movementSequence":10,"stateSequence":30,"startDateProcess":1778188601532,
		"expired":%t,"approachingExpiration":%t}`, id, expired, warning)
}

// Fixture real: mapeia campos, sintetiza o responsável como o pool e marca
// tudo como em aberto (a rota só devolve tarefa em aberto).
func TestListPoolTasks(t *testing.T) {
	stub := &poolTaskStub{}
	c := datasetClient(t, stub.server(t).URL)

	tasks, err := c.ListPoolTasks(context.Background(), "Pool:Group:TI", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("esperava 3 tarefas, veio %d", len(tasks))
	}
	if len(stub.userCodes) != 1 || stub.userCodes[0] != "uc-pool-test" {
		t.Errorf("caminho deveria levar o userCode resolvido: %v", stub.userCodes)
	}
	q := stub.queries[0]
	if q.Get("taskId") != "Pool:Group:TI" || q.Get("sidx") != "processInstanceId" || q.Get("rows") != "100" {
		t.Errorf("query inesperada: %v", q)
	}
	first := tasks[0]
	if first.RequestID != 219876 || first.ProcessID != "contratos_troca_contribuinte" ||
		first.StateName != "Corrigir Integração" || first.Movement != 10 || first.Sequence != 30 {
		t.Errorf("task[0] inesperada: %+v", first)
	}
	if first.Assignee == nil || first.Assignee.Code != "Pool:Group:TI" || first.Assignee.Name != "Para o Grupo TI" {
		t.Errorf("assignee deveria ser o pool: %+v", first.Assignee)
	}
	if first.Requester == nil || first.Requester.Login != "jsilva" || first.Requester.Name != "João Silva" {
		t.Errorf("requester inesperado: %+v", first.Requester)
	}
	if first.Status != "NOT_COMPLETED" || first.SLAStatus != "ON_TIME" {
		t.Errorf("status/sla inesperados: %s/%s", first.Status, first.SLAStatus)
	}
	if first.StartDate == nil || first.StartDate.UnixMilli() != 1778188601532 {
		t.Errorf("startDate inesperado: %v", first.StartDate)
	}
}

// Paginação medida ao vivo: rows+1 itens = há próxima página e o último item
// repete como primeiro dela — o "espião" é descartado, sem duplicar tarefa.
// Também cobre o mapeamento de SLA (expired/approachingExpiration) e o limite.
func TestListPoolTasksPaginacao(t *testing.T) {
	// 205 tarefas: página 1 = itens 1..101, página 2 = 101..201, página 3 = 201..205.
	page := func(from, to int) []string {
		var items []string
		for id := from; id <= to; id++ {
			items = append(items, poolItem(id, id == 1, id == 2))
		}
		return items
	}
	stub := &poolTaskStub{pages: [][]string{page(1, 101), page(101, 201), page(201, 205)}}
	c := datasetClient(t, stub.server(t).URL)

	tasks, err := c.ListPoolTasks(context.Background(), "Pool:Group:TI", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 205 {
		t.Fatalf("esperava 205 tarefas (sem duplicar o espião), veio %d", len(tasks))
	}
	for i, tk := range tasks {
		if tk.RequestID != i+1 {
			t.Fatalf("tarefa %d fora de ordem ou duplicada: id=%d", i, tk.RequestID)
		}
	}
	if tasks[0].SLAStatus != "EXPIRED" || tasks[1].SLAStatus != "WARNING" || tasks[2].SLAStatus != "ON_TIME" {
		t.Errorf("mapeamento de SLA inesperado: %s/%s/%s", tasks[0].SLAStatus, tasks[1].SLAStatus, tasks[2].SLAStatus)
	}

	stub2 := &poolTaskStub{pages: [][]string{page(1, 101), page(101, 201), page(201, 205)}}
	c2 := datasetClient(t, stub2.server(t).URL)
	tasks, err = c2.ListPoolTasks(context.Background(), "Pool:Group:TI", 150)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 150 {
		t.Fatalf("limite não aplicado: veio %d", len(tasks))
	}
}
