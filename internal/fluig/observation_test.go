package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// observationStub simula o fluigcliHelper com (ou sem) as rotas de observação.
type observationStub struct {
	version   string // versão anunciada pelo GET /api/version
	hasRoutes bool   // false = helper antigo (404 na rota)
	lastBody  map[string]any
	lastQuery string
	postCode  int    // 0 = 201
	postError string // corpo do erro
	getCode   int    // 0 = 200
	getError  string
	getBody   string
}

const obsCriada = `{"id":123456,"processInstanceId":235189,"stateSequence":34,"movementSequence":5,` +
	`"threadSequence":0,"colleagueId":"sabia","observationDate":"2026-09-17T14:03:11.000-04:00",` +
	`"observation":"<b>teste helper</b> observação sem move"}`

func (s *observationStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"`+s.version+`"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/workflows/235189/observations", func(w http.ResponseWriter, r *http.Request) {
		if !s.hasRoutes {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			s.lastBody = nil
			_ = json.Unmarshal(body, &s.lastBody)
			if s.postCode != 0 {
				w.WriteHeader(s.postCode)
				io.WriteString(w, s.postError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, obsCriada)
		case http.MethodGet:
			s.lastQuery = r.URL.RawQuery
			if s.getCode != 0 {
				w.WriteHeader(s.getCode)
				io.WriteString(w, s.getError)
				return
			}
			out := s.getBody
			if out == "" {
				out = "[" + obsCriada + "]"
			}
			io.WriteString(w, out)
		}
	})
	mux.HandleFunc("/fluigcliHelper/api/workflows/1/observations", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Solicitação 1 não encontrada")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateRequestObservation(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true}
	c := helperClient(t, stub.server(t).URL)
	obs, err := c.CreateRequestObservation(context.Background(), 235189, ObservationOptions{
		Text: "<b>teste helper</b> observação sem move",
	})
	if err != nil {
		t.Fatal(err)
	}
	if obs.ID != 123456 || obs.StateSequence != 34 || obs.MovementSequence != 5 || obs.ColleagueID != "sabia" {
		t.Errorf("observação inesperada: %+v", obs)
	}
	if obs.ObservationDate != "2026-09-17T14:03:11.000-04:00" {
		t.Errorf("data deve vir como o helper mandou: %q", obs.ObservationDate)
	}
	// Sem etapa/movimento, o corpo NÃO leva os campos (o helper resolve a
	// tarefa corrente); a thread vai sempre (default 0).
	if _, ok := stub.lastBody["stateSequence"]; ok {
		t.Errorf("stateSequence não devia ir no corpo: %v", stub.lastBody)
	}
	if _, ok := stub.lastBody["movementSequence"]; ok {
		t.Errorf("movementSequence não devia ir no corpo: %v", stub.lastBody)
	}
	if stub.lastBody["threadSequence"] != float64(0) {
		t.Errorf("threadSequence default 0 deve ir no corpo: %v", stub.lastBody)
	}
}

func TestCreateRequestObservationComEtapaInformada(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.CreateRequestObservation(context.Background(), 235189, ObservationOptions{
		Text: "laudo", StateSequence: 34, MovementSequence: 5, ThreadSequence: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stub.lastBody["stateSequence"] != float64(34) || stub.lastBody["movementSequence"] != float64(5) ||
		stub.lastBody["threadSequence"] != float64(2) {
		t.Errorf("corpo inesperado: %v", stub.lastBody)
	}
}

func TestCreateRequestObservationHelperAntigo(t *testing.T) {
	stub := &observationStub{version: "0.10.3", hasRoutes: false}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.CreateRequestObservation(context.Background(), 235189, ObservationOptions{Text: "x"})
	if !errors.Is(err, ErrHelperOutdated) {
		t.Errorf("quer ErrHelperOutdated, veio %v", err)
	}
	_, err = c.ListRequestObservations(context.Background(), 235189, 0, 0)
	if !errors.Is(err, ErrHelperOutdated) {
		t.Errorf("list: quer ErrHelperOutdated, veio %v", err)
	}
}

func TestCreateRequestObservationInexistente(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.CreateRequestObservation(context.Background(), 1, ObservationOptions{Text: "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("quer ErrNotFound, veio %v", err)
	}
	if !strings.Contains(err.Error(), "Solicitação 1 não encontrada") {
		t.Errorf("deve levar a mensagem do helper: %v", err)
	}
}

func TestCreateRequestObservationConflito(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true,
		postCode: http.StatusConflict, postError: "solicitação finalizada; não aceita observação"}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.CreateRequestObservation(context.Background(), 235189, ObservationOptions{Text: "x"})
	var conflito *RequestConflictError
	if !errors.As(err, &conflito) {
		t.Fatalf("quer RequestConflictError, veio %T %v", err, err)
	}
	if conflito.ProcessInstanceID != 235189 || conflito.Message != "solicitação finalizada; não aceita observação" {
		t.Errorf("conflito inesperado: %+v", conflito)
	}
}

func TestCreateRequestObservationTextoRecusado(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true,
		postCode: http.StatusBadRequest, postError: "O campo observation é obrigatório e não pode ficar vazio"}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.CreateRequestObservation(context.Background(), 235189, ObservationOptions{Text: "   "})
	if !errors.Is(err, errServerRejected) || !strings.Contains(err.Error(), "obrigatório") {
		t.Errorf("quer errServerRejected com a mensagem do helper, veio %v", err)
	}
}

func TestListRequestObservations(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true}
	c := helperClient(t, stub.server(t).URL)
	list, err := c.ListRequestObservations(context.Background(), 235189, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != 123456 {
		t.Errorf("lista inesperada: %+v", list)
	}
	if stub.lastQuery != "threadSequence=0" {
		t.Errorf("sem --state a query leva só a thread: %q", stub.lastQuery)
	}

	if _, err := c.ListRequestObservations(context.Background(), 235189, 34, 1); err != nil {
		t.Fatal(err)
	}
	if stub.lastQuery != "threadSequence=1&stateSequence=34" {
		t.Errorf("query inesperada: %q", stub.lastQuery)
	}

	// Lista vazia vem como slice vazio (não nil) para o --json sair como [].
	stub.getBody = "[]"
	list, err = c.ListRequestObservations(context.Background(), 235189, 34, 0)
	if err != nil || list == nil || len(list) != 0 {
		t.Errorf("lista vazia deve ser [] sem erro: %v %v", list, err)
	}
}

func TestListRequestObservationsParalelas(t *testing.T) {
	stub := &observationStub{version: "0.11.0", hasRoutes: true,
		getCode: http.StatusConflict, getError: "solicitação tem tarefas paralelas; informe stateSequence e movementSequence"}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.ListRequestObservations(context.Background(), 235189, 0, 0)
	var conflito *RequestConflictError
	if !errors.As(err, &conflito) || !strings.Contains(conflito.Message, "paralelas") {
		t.Errorf("quer RequestConflictError de paralelismo, veio %v", err)
	}
}
