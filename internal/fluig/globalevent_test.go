package fluig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// globalEventStub simula login/ping + os endpoints de evento global.
type globalEventStub struct {
	saved     []globalEventDTO
	saveCType string
	deleted   []string
	list      []byte // corpo de getEventList (default: fixture)
}

func (s *globalEventStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/getEventList", func(w http.ResponseWriter, r *http.Request) {
		if s.list != nil {
			w.Write(s.list)
			return
		}
		w.Write(testdata(t, "getEventList.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/saveEventList", func(w http.ResponseWriter, r *http.Request) {
		s.saveCType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &s.saved)
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/globalevent/deleteGlobalEvent", func(w http.ResponseWriter, r *http.Request) {
		s.deleted = append(s.deleted, r.URL.Query().Get("eventName"))
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func eventClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-ev-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListGlobalEvents(t *testing.T) {
	stub := &globalEventStub{}
	c := eventClient(t, stub.server(t).URL)
	events, err := c.ListGlobalEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("esperava 2 eventos, veio %d", len(events))
	}
	if events[0].ID != "beforeConvertViewToPDF" || events[0].Code == "" {
		t.Errorf("evento[0] inesperado: %+v", events[0])
	}
}

func TestSaveGlobalEventsSendsFullListAndCompanyID(t *testing.T) {
	stub := &globalEventStub{}
	c := eventClient(t, stub.server(t).URL)
	events := []GlobalEvent{
		{ID: "a", Code: "codeA"},
		{ID: "b", Code: "codeB"},
	}
	if err := c.SaveGlobalEvents(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	if len(stub.saved) != 2 {
		t.Fatalf("saveEventList recebeu %d eventos, quer 2", len(stub.saved))
	}
	if stub.saved[0].GlobalEventPK.CompanyID != 1 || stub.saved[0].GlobalEventPK.EventID != "a" {
		t.Errorf("PK inesperado: %+v", stub.saved[0].GlobalEventPK)
	}
	if stub.saved[0].EventDescription != "codeA" {
		t.Errorf("eventDescription = %q", stub.saved[0].EventDescription)
	}
}

func TestDeleteGlobalEvent(t *testing.T) {
	stub := &globalEventStub{}
	c := eventClient(t, stub.server(t).URL)
	if err := c.DeleteGlobalEvent(context.Background(), "beforeConvertViewToPDF"); err != nil {
		t.Fatal(err)
	}
	if len(stub.deleted) != 1 || stub.deleted[0] != "beforeConvertViewToPDF" {
		t.Errorf("deleteGlobalEvent recebeu %v", stub.deleted)
	}
}
