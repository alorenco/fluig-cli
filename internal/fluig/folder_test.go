package fluig

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// As fixtures soap_rootFolders.xml/soap_subFolders.xml são respostas reais da
// homologação (2026-07-11), sanitizadas e recortadas.
func TestListGEDFolders(t *testing.T) {
	var actions []string
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"userCode":"uc-1"}}`)
	})
	mux.HandleFunc("/webdesk/ECMFolderService", func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		actions = append(actions, action)
		w.Header().Set("Content-Type", "text/xml")
		switch action {
		case "getRootFolders":
			w.Write(testdata(t, "soap_rootFolders.xml"))
		case "getSubFolders":
			w.Write(testdata(t, "soap_subFolders.xml"))
		default:
			http.Error(w, "op?", http.StatusInternalServerError)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(Options{BaseURL: srv.URL, Username: "u-fold-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Raiz (parentID 0) → getRootFolders, ordenado por nome.
	roots, err := c.ListGEDFolders(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 3 || roots[0].Name != "01 - Documentos Oficiais" || roots[0].ID != 2864 ||
		roots[1].Name != "02 - Treinamentos" {
		t.Errorf("raízes inesperadas: %+v", roots)
	}
	// Subpastas → getSubFolders.
	subs, err := c.ListGEDFolders(ctx, 7374)
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 || subs[0].Name != "Anexos" || subs[1].ID != 7380 {
		t.Errorf("subpastas inesperadas: %+v", subs)
	}
	if actions[0] != "getRootFolders" || actions[1] != "getSubFolders" {
		t.Errorf("SOAPActions: %v", actions)
	}
}
