package fluig

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// layoutHelperStub simula o fluigcliHelper com (ou sem) as rotas /layouts.
type layoutHelperStub struct {
	version      string // "" = sem /api/version (helper muito antigo)
	routeMissing bool   // helper < 0.12.0: /layouts responde 404
	warMissing   bool   // rota existe, arquivo não
}

func (s *layoutHelperStub) client(t *testing.T) *Client {
	t.Helper()
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
		if s.version == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"name":"fluigcliHelper","version":"`+s.version+`"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/layouts", func(w http.ResponseWriter, r *http.Request) {
		if s.routeMissing {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `[{"code":"kit_layout","title":"Portal","description":"d","filename":"wcm-layout-kit.war"}]`)
	})
	mux.HandleFunc("/fluigcliHelper/api/layouts/", func(w http.ResponseWriter, r *http.Request) {
		if s.routeMissing || s.warMissing {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Accept") == "application/json" {
			w.WriteHeader(http.StatusNotAcceptable) // o RESTEasy real faz isso
			return
		}
		w.Write([]byte("PK\x03\x04war"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(Options{BaseURL: srv.URL, Username: "u-lay-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Caminho feliz: a listagem traz o arquivo (que não é <código>.war) e o
// download devolve o binário.
func TestListLayoutsHelperEDownload(t *testing.T) {
	c := (&layoutHelperStub{version: "0.12.0"}).client(t)
	layouts, err := c.ListLayoutsHelper(context.Background())
	if err != nil || len(layouts) != 1 || layouts[0].Filename != "wcm-layout-kit.war" {
		t.Fatalf("layouts=%+v err=%v", layouts, err)
	}
	war, err := c.DownloadLayout(context.Background(), layouts[0].Filename)
	if err != nil || string(war) != "PK\x03\x04war" {
		t.Fatalf("war=%q err=%v", war, err)
	}
}

// Helper instalado mas anterior ao 0.12.0: a listagem responde 404 → o erro é
// "desatualizado", não "não encontrado".
func TestListLayoutsHelperAntigo(t *testing.T) {
	c := (&layoutHelperStub{version: "0.11.0", routeMissing: true}).client(t)
	_, err := c.ListLayoutsHelper(context.Background())
	if !errors.Is(err, ErrHelperOutdated) {
		t.Fatalf("esperava ErrHelperOutdated, veio %v", err)
	}
	_, err = c.DownloadLayout(context.Background(), "x.war")
	if !errors.Is(err, ErrHelperOutdated) {
		t.Fatalf("download: esperava ErrHelperOutdated, veio %v", err)
	}
}

// Rota presente (helper 0.12.0), arquivo inexistente: ErrNotFound.
func TestDownloadLayoutInexistente(t *testing.T) {
	c := (&layoutHelperStub{version: "0.12.0", warMissing: true}).client(t)
	_, err := c.DownloadLayout(context.Background(), "nao_existe.war")
	if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrHelperOutdated) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
}

// Helper sem /api/version (muito antigo): versão vazia = antes do 0.12.0.
func TestDownloadLayoutHelperSemVersao(t *testing.T) {
	c := (&layoutHelperStub{routeMissing: true}).client(t)
	_, err := c.DownloadLayout(context.Background(), "x.war")
	if !errors.Is(err, ErrHelperOutdated) {
		t.Fatalf("esperava ErrHelperOutdated, veio %v", err)
	}
}
