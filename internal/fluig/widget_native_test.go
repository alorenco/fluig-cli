package fluig

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A listagem nativa percorre as páginas de applications e agrega os widgets.
func TestListWidgetsNativePaginado(t *testing.T) {
	var seen []string
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	pages := [][]byte{
		testdata(t, "rest_applications_page1.json"),
		testdata(t, "rest_applications_page2.json"),
	}
	mux.HandleFunc("/page-management/api/v2/applications", func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RawQuery)
		w.Write(pages[len(seen)-1])
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := NewClient(Options{BaseURL: srv.URL, Username: "u-wgn-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	widgets, err := c.ListWidgetsNative(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(widgets) != 3 {
		t.Fatalf("esperava 3 widgets (2+1), veio %d", len(widgets))
	}
	if len(seen) != 2 || seen[0] != "internal=false&page=1&pageSize=100" {
		t.Errorf("query strings inesperadas: %v", seen)
	}
	if widgets[0].Code != "meu_widget" || widgets[0].Title != "Meu Widget" {
		t.Errorf("widget[0] inesperado: %+v", widgets[0])
	}
	// A API nativa não informa o arquivo do WAR (limitação documentada).
	if widgets[0].Filename != "" {
		t.Errorf("filename deveria ser vazio na listagem nativa: %+v", widgets[0])
	}
}
