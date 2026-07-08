package fluig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mechanismStub struct {
	updated map[string]any
	created map[string]any
	deleted []string
}

func (s *mechanismStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/getCustomAttributionMechanismList", func(w http.ResponseWriter, r *http.Request) {
		w.Write(testdata(t, "getMechanismList.json"))
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/updateAttributionMechanism", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &s.updated)
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/createAttributionMechanism", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &s.created)
		io.WriteString(w, `{"content":"OK"}`)
	})
	mux.HandleFunc("/ecm/api/rest/ecm/mechanism/deleteAttributionMechanism", func(w http.ResponseWriter, r *http.Request) {
		s.deleted = append(s.deleted, r.URL.Query().Get("mechanismId"))
		io.WriteString(w, `{"content":"OK"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func mechanismClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "u-mec-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListMechanisms(t *testing.T) {
	stub := &mechanismStub{}
	c := mechanismClient(t, stub.server(t).URL)
	mechs, err := c.ListMechanisms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(mechs) != 1 {
		t.Fatalf("esperava 1 mecanismo, veio %d", len(mechs))
	}
	m := mechs[0]
	if m.ID != "mec_gestor_area" || m.Name != "Gestor da Área" {
		t.Errorf("mecanismo inesperado: %+v", m)
	}
	// O código JS vem de attributionMecanismDescription.
	if m.Code == "" {
		t.Errorf("código (attributionMecanismDescription) não carregado: %+v", m)
	}
	if m.Description != "Atribui ao gestor da área do solicitante" {
		t.Errorf("description (metadado) inesperado: %q", m.Description)
	}
}

// Update preserva o DTO e troca só o attributionMecanismDescription (o código).
func TestUpdateMechanismKeepsDTO(t *testing.T) {
	stub := &mechanismStub{}
	c := mechanismClient(t, stub.server(t).URL)
	mechs, _ := c.ListMechanisms(context.Background())
	novo := "function getUsers(){ return ['x']; }"
	if err := c.UpdateMechanism(context.Background(), &mechs[0], novo); err != nil {
		t.Fatal(err)
	}
	if stub.updated["attributionMecanismDescription"] != novo {
		t.Errorf("código = %v, quer %q", stub.updated["attributionMecanismDescription"], novo)
	}
	if stub.updated["controlClass"] != customControlClass {
		t.Errorf("controlClass deveria ser preservado: %v", stub.updated["controlClass"])
	}
	pk, _ := stub.updated["attributionMecanismPK"].(map[string]any)
	if pk == nil || pk["attributionMecanismId"] != "mec_gestor_area" {
		t.Errorf("PK não preservado: %v", stub.updated["attributionMecanismPK"])
	}
}

func TestCreateMechanism(t *testing.T) {
	stub := &mechanismStub{}
	c := mechanismClient(t, stub.server(t).URL)
	err := c.CreateMechanism(context.Background(), "mec_novo", "Novo", "Desc", "function getUsers(){}")
	if err != nil {
		t.Fatal(err)
	}
	if stub.created["assignmentType"].(float64) != 1 {
		t.Errorf("assignmentType = %v, quer 1", stub.created["assignmentType"])
	}
	if stub.created["controlClass"] != customControlClass {
		t.Errorf("controlClass = %v, quer %q", stub.created["controlClass"], customControlClass)
	}
	if stub.created["attributionMecanismDescription"] != "function getUsers(){}" {
		t.Errorf("código não foi para attributionMecanismDescription: %v", stub.created["attributionMecanismDescription"])
	}
	if stub.created["configurationClass"] != "" {
		t.Errorf("configurationClass deveria ser vazio, veio %v", stub.created["configurationClass"])
	}
	pk, _ := stub.created["attributionMecanismPK"].(map[string]any)
	if pk["attributionMecanismId"] != "mec_novo" {
		t.Errorf("PK inesperado: %v", pk)
	}
}

func TestDeleteMechanism(t *testing.T) {
	stub := &mechanismStub{}
	c := mechanismClient(t, stub.server(t).URL)
	if err := c.DeleteMechanism(context.Background(), "mec_gestor_area"); err != nil {
		t.Fatal(err)
	}
	if len(stub.deleted) != 1 || stub.deleted[0] != "mec_gestor_area" {
		t.Errorf("delete recebeu %v", stub.deleted)
	}
}
