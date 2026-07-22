package fluig

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// auditDocStub simula login/ping + o dataset-handle/search do dataset
// `document`, devolvendo linhas ordenadas por createDate DESC.
type auditDocStub struct {
	offsetsSeen []string // offsets pedidos no dataset-handle/search
}

func dayMillis(y int, m time.Month, d int) int64 {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).UnixMilli()
}

func (s *auditDocStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		s.offsetsSeen = append(s.offsetsSeen, offset)
		if offset != "0" {
			// Não deveria chegar aqui — a parada antecipada corta na 1ª página.
			io.WriteString(w, `{"columns":["documentPK.documentId"],"values":[]}`)
			return
		}
		newer := dayMillis(2026, 7, 5) // fora do intervalo (mais novo) → pulado
		inDay := dayMillis(2026, 7, 3) // dentro do intervalo → mantido
		older := dayMillis(2026, 7, 1) // mais antigo → dispara a parada
		// Página cheia (== pageSize) para provar a parada antecipada: a linha
		// "older" aparece antes do fim e nenhuma 2ª página é buscada.
		var b strings.Builder
		b.WriteString(`{"columns":["documentPK.documentId","documentPK.version","documentDescription","documentType","createDate","deleted"],"values":[`)
		row := func(id, ver int, desc, typ string, cd int64, del bool) string {
			return fmt.Sprintf(`{"documentPK.documentId":%d,"documentPK.version":%d,"documentDescription":%q,"documentType":%q,"createDate":"%d","deleted":%t}`,
				id, ver, desc, typ, cd, del)
		}
		rows := []string{
			row(100, 1, "mais nova", "7", newer, false), // pulada (fora do dia)
			row(200, 1, "nota A v1", "7", inDay, false), // mantida
			row(200, 2, "nota A v2", "7", inDay, false), // duplicata (mesmo id) → ignorada
			row(201, 1, "registro B", "5", inDay, true), // mantida (excluída)
			row(300, 1, "antiga", "7", older, false),    // dispara parada
		}
		// completa a página até auditDocPageSize com linhas antigas (nunca lidas
		// após a parada — validado pela ausência de offset=500).
		for len(rows) < auditDocPageSize {
			rows = append(rows, row(9999, 1, "antiga", "7", older, false))
		}
		b.WriteString(strings.Join(rows, ","))
		b.WriteString(`]}`)
		io.WriteString(w, b.String())
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDocumentsCreatedBy(t *testing.T) {
	stub := &auditDocStub{}
	c := datasetClient(t, stub.server(t).URL)

	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	docs, err := c.DocumentsCreatedBy(context.Background(), "codigo-do-marlon", day, day)
	if err != nil {
		t.Fatal(err)
	}
	// Espera 2 documentos únicos no dia (200 e 201); 100 é mais novo, 300+ mais
	// antigos, e a 2ª versão do 200 é deduplicada.
	if len(docs) != 2 {
		t.Fatalf("esperava 2 documentos, veio %d: %+v", len(docs), docs)
	}
	if docs[0].DocumentID != 200 || docs[1].DocumentID != 201 {
		t.Fatalf("ids inesperados: %d, %d", docs[0].DocumentID, docs[1].DocumentID)
	}
	if docs[0].Version != 1 {
		t.Errorf("esperava a 1ª versão vista (v1), veio v%d", docs[0].Version)
	}
	if !docs[1].Deleted {
		t.Errorf("documento 201 deveria estar marcado como excluído")
	}
	if docs[1].Type != "5" || docs[1].TypeLabel != "Registro de formulário" {
		t.Errorf("tipo do 201 esperado 5/Registro de formulário, veio %q/%q", docs[1].Type, docs[1].TypeLabel)
	}
	if docs[0].TypeLabel != "Anexo de processo" {
		t.Errorf("rótulo do tipo 7 esperado 'Anexo de processo', veio %q", docs[0].TypeLabel)
	}
	if docs[0].CreatedAt == nil || !docs[0].CreatedAt.Equal(day) {
		t.Errorf("createdAt esperado %v, veio %v", day, docs[0].CreatedAt)
	}
	// Parada antecipada: só a 1ª página (offset=0) deve ter sido buscada.
	for _, off := range stub.offsetsSeen {
		if off != "0" {
			t.Fatalf("não deveria paginar além da 1ª página; offsets vistos: %v", stub.offsetsSeen)
		}
	}
}

// Usuário sem documentos: dataset devolve valores vazios (não é erro).
func TestDocumentsCreatedByVazio(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/dataset/api/v2/dataset-handle/search", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"columns":["documentPK.documentId"],"values":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := datasetClient(t, srv.URL)
	day := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	docs, err := c.DocumentsCreatedBy(context.Background(), "x", day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("esperava 0 documentos, veio %d", len(docs))
	}
}
