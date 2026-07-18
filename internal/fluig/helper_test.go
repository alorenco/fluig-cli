package fluig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type helperStub struct {
	installed    bool   // o fluigcliHelper responde ao ping
	version      string // resposta do /api/version ("" = helper antigo, rota 404)
	eventsBody   []WorkflowEvent
	eventsPath   string
	uploadedName string
	uploadedSize int
}

func (s *helperStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !s.installed {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		if !s.installed || s.version == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"name":"fluigcliHelper","version":"`+s.version+`"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/workflows/", func(w http.ResponseWriter, r *http.Request) {
		s.eventsPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &s.eventsBody)
		io.WriteString(w, `{"processId":"Compras","version":5,"hasError":false,"totalProcessed":1,"errors":[],"successes":["beforeTaskSave"]}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/product/uploadfile", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		s.uploadedName = r.FormValue("fileName")
		if f, _, err := r.FormFile("attachment"); err == nil {
			b, _ := io.ReadAll(f)
			s.uploadedSize = len(b)
		}
		io.WriteString(w, `{}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func helperClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-h-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestHelperInstalled(t *testing.T) {
	stub := &helperStub{installed: false}
	c := helperClient(t, stub.server(t).URL)
	if ok, _ := c.HelperInstalled(context.Background()); ok {
		t.Error("deveria reportar não instalado (ping 404)")
	}
	stub2 := &helperStub{installed: true}
	c2 := helperClient(t, stub2.server(t).URL)
	if ok, err := c2.HelperInstalled(context.Background()); err != nil || !ok {
		t.Errorf("deveria reportar instalado; ok=%v err=%v", ok, err)
	}
}

// HelperStatus: instalado com versão; instalado antigo (sem /api/version) =
// versão vazia; ausente.
func TestHelperStatus(t *testing.T) {
	com := &helperStub{installed: true, version: "0.2.0"}
	c := helperClient(t, com.server(t).URL)
	if info, err := c.HelperStatus(context.Background()); err != nil || !info.Installed || info.Version != "0.2.0" {
		t.Errorf("com versão: %+v err=%v", info, err)
	}

	antigo := &helperStub{installed: true}
	c2 := helperClient(t, antigo.server(t).URL)
	if info, err := c2.HelperStatus(context.Background()); err != nil || !info.Installed || info.Version != "" {
		t.Errorf("helper antigo: %+v err=%v (quer instalado sem versão)", info, err)
	}

	ausente := &helperStub{}
	c3 := helperClient(t, ausente.server(t).URL)
	if info, err := c3.HelperStatus(context.Background()); err != nil || info.Installed {
		t.Errorf("ausente: %+v err=%v", info, err)
	}
}

func TestUpdateWorkflowEvents(t *testing.T) {
	stub := &helperStub{installed: true}
	c := helperClient(t, stub.server(t).URL)
	res, err := c.UpdateWorkflowEvents(context.Background(), "Compras", 5, []WorkflowEvent{
		{Name: "beforeTaskSave", Contents: "function beforeTaskSave(){}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.HasError {
		t.Errorf("resultado com erro: %+v", res)
	}
	if !strings.HasSuffix(stub.eventsPath, "/Compras/5/events") || !strings.HasPrefix(stub.eventsPath, "/fluigcliHelper/") {
		t.Errorf("path inesperado: %s", stub.eventsPath)
	}
	if len(stub.eventsBody) != 1 || stub.eventsBody[0].Name != "beforeTaskSave" || stub.eventsBody[0].Contents == "" {
		t.Errorf("corpo inesperado: %+v", stub.eventsBody)
	}
}

// Sem o fluigcliHelper publicado, o update falha com ErrHelperMissing (exit 7).
func TestUpdateWorkflowEventsSemHelper(t *testing.T) {
	stub := &helperStub{installed: false}
	c := helperClient(t, stub.server(t).URL)
	_, err := c.UpdateWorkflowEvents(context.Background(), "Compras", 5, []WorkflowEvent{{Name: "x", Contents: "y"}})
	if err == nil || !strings.Contains(err.Error(), "componente auxiliar") {
		t.Errorf("esperava ErrHelperMissing, veio %v", err)
	}
}

func TestUpdateWorkflowEventsHasError(t *testing.T) {
	errStub := &errorEventsStub{}
	c := helperClient(t, errStub.server(t).URL)
	_, err := c.UpdateWorkflowEvents(context.Background(), "Compras", 5, []WorkflowEvent{{Name: "x", Contents: "y"}})
	if err == nil || !strings.Contains(err.Error(), "sintaxe inválida") {
		t.Errorf("esperava erro de negócio com a mensagem do helper, veio %v", err)
	}
}

type errorEventsStub struct{}

func (s *errorEventsStub) server(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/fluigcliHelper/api/workflows/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"hasError":true,"errors":["sintaxe inválida no evento"],"successes":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestUploadWidgetWAR(t *testing.T) {
	stub := &helperStub{installed: true}
	c := helperClient(t, stub.server(t).URL)
	war := []byte("PK\x03\x04 fake war bytes")
	if err := c.UploadWidgetWAR(context.Background(), "fluigcliHelper.war", war); err != nil {
		t.Fatal(err)
	}
	if stub.uploadedName != "fluigcliHelper.war" || stub.uploadedSize != len(war) {
		t.Errorf("upload inesperado: nome=%q size=%d", stub.uploadedName, stub.uploadedSize)
	}
}
