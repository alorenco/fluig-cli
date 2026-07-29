package fluig

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BuildWAR usa STORE e preserva conteúdo binário byte a byte.
func TestBuildWARStoreAndBinary(t *testing.T) {
	binary := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0x10, 0x42} // PNG-ish + NUL
	files := []WARFile{
		{Name: "WEB-INF/application.xml", Content: []byte("<application/>")},
		{Name: "resources/img/logo.png", Content: binary},
	}
	war, err := BuildWAR(files)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(war), int64(len(war)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]byte{}
	for _, f := range zr.File {
		if f.Method != zip.Store {
			t.Errorf("%s não está em STORE (method=%d)", f.Name, f.Method)
		}
		rc, _ := f.Open()
		content := new(bytes.Buffer)
		content.ReadFrom(rc)
		rc.Close()
		found[f.Name] = content.Bytes()
	}
	if !bytes.Equal(found["resources/img/logo.png"], binary) {
		t.Errorf("binário corrompido no WAR: %v", found["resources/img/logo.png"])
	}
	if string(found["WEB-INF/application.xml"]) != "<application/>" {
		t.Errorf("xml inesperado: %q", found["WEB-INF/application.xml"])
	}
}

// layoutStub responde as duas rotas de layout do page-management.
type layoutStub struct {
	getStatus  int    // status do GET por código (0 = 200 com o corpo abaixo)
	getBody    string // corpo do GET por código
	listStatus int    // status da listagem (0 = 200)
	listBody   string // corpo da listagem
	getHits    int
	listHits   int
}

func (s *layoutStub) client(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/page-management/api/v2/layouts/", func(w http.ResponseWriter, r *http.Request) {
		s.getHits++
		if s.getStatus != 0 {
			w.WriteHeader(s.getStatus)
		}
		io.WriteString(w, s.getBody)
	})
	mux.HandleFunc("/page-management/api/v2/layouts", func(w http.ResponseWriter, r *http.Request) {
		s.listHits++
		if s.listStatus != 0 {
			w.WriteHeader(s.listStatus)
		}
		io.WriteString(w, s.listBody)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(Options{BaseURL: srv.URL, Username: "u-layout-" + t.Name(), Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Layout existente: FindLayout devolve código e título, sem tocar a listagem.
func TestFindLayoutExistente(t *testing.T) {
	stub := &layoutStub{getBody: `{"id":7,"code":"proc_jud","title":"Processos","internal":false}`}
	c := stub.client(t)

	layout, err := c.FindLayout(context.Background(), "proc_jud")
	if err != nil {
		t.Fatalf("FindLayout: %v", err)
	}
	if layout.Code != "proc_jud" || layout.Title != "Processos" {
		t.Errorf("layout inesperado: %+v", layout)
	}
	if stub.listHits != 0 {
		t.Errorf("a listagem não devia ser consultada no caminho feliz (%d chamada(s))", stub.listHits)
	}
}

// 404 é a resposta declarada no swagger para código inexistente.
func TestFindLayoutInexistente404(t *testing.T) {
	stub := &layoutStub{getStatus: http.StatusNotFound, getBody: `{"message":"not found"}`}
	c := stub.client(t)

	_, err := c.FindLayout(context.Background(), "nao_existe")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
	if stub.listHits != 0 {
		t.Errorf("404 é conclusivo; a listagem não devia ser consultada (%d)", stub.listHits)
	}
}

// Corpo 200 sem código também significa "não existe" (o Fluig às vezes responde
// 200 com corpo vazio em GET por chave).
func TestFindLayoutCorpoVazio(t *testing.T) {
	stub := &layoutStub{getBody: `{}`}
	c := stub.client(t)

	if _, err := c.FindLayout(context.Background(), "vazio"); !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound com corpo vazio, veio %v", err)
	}
}

// GET por código com 500 (quirk conhecido do Fluig, ver loadDataset): o fallback
// pela listagem decide — e o casamento do código ignora a caixa.
func TestFindLayoutFallbackPelaListagem(t *testing.T) {
	stub := &layoutStub{
		getStatus: http.StatusInternalServerError,
		getBody:   `{"message":"erro"}`,
		listBody:  `{"items":[{"id":1,"code":"Proc_Jud","title":"Processos"}],"hasNext":false}`,
	}
	c := stub.client(t)

	layout, err := c.FindLayout(context.Background(), "proc_jud")
	if err != nil {
		t.Fatalf("FindLayout via listagem: %v", err)
	}
	if layout.Code != "Proc_Jud" {
		t.Errorf("layout inesperado: %+v", layout)
	}
	if stub.listHits == 0 {
		t.Error("a listagem devia ter sido consultada no fallback")
	}
}

// GET 500 e código ausente da listagem: não existe.
func TestFindLayoutFallbackSemColisao(t *testing.T) {
	stub := &layoutStub{
		getStatus: http.StatusInternalServerError,
		listBody:  `{"items":[{"id":1,"code":"outro","title":"Outro"}],"hasNext":false}`,
	}
	c := stub.client(t)

	if _, err := c.FindLayout(context.Background(), "proc_jud"); !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
}

// Sobre as respostas REAIS da homologação (gravadas em 2026-07-29): a listagem
// devolve `responsiveLayout` booleano e o GET por código devolve **null** no
// mesmo campo. O parser tem de aceitar as duas formas.
func TestFindLayoutRespostasReais(t *testing.T) {
	stub := &layoutStub{
		getBody:  string(testdata(t, "rest_layout_kit_layout.json")),
		listBody: string(testdata(t, "rest_layouts.json")),
	}
	c := stub.client(t)
	ctx := context.Background()

	layout, err := c.FindLayout(ctx, "kit_layout")
	if err != nil {
		t.Fatalf("FindLayout com a resposta real: %v", err)
	}
	if layout.Code != "kit_layout" || layout.Title != "Portal" {
		t.Errorf("layout inesperado: %+v", layout)
	}

	layouts, err := c.ListLayouts(ctx)
	if err != nil {
		t.Fatalf("ListLayouts com a resposta real: %v", err)
	}
	if len(layouts) != 4 {
		t.Fatalf("esperava 4 layouts na fixture, veio %d", len(layouts))
	}
	// A fixture traz layouts internos da plataforma e customizados — a guarda do
	// widget export vale para os dois.
	internos := 0
	for _, l := range layouts {
		if l.Internal {
			internos++
		}
	}
	if internos != 2 {
		t.Errorf("esperava 2 layouts internos na fixture, veio %d", internos)
	}
}

// Os dois caminhos quebrados: erro (quem chama decide falhar em aberto).
func TestFindLayoutTudoQuebradoDevolveErro(t *testing.T) {
	stub := &layoutStub{
		getStatus:  http.StatusInternalServerError,
		listStatus: http.StatusInternalServerError,
	}
	c := stub.client(t)

	_, err := c.FindLayout(context.Background(), "proc_jud")
	if err == nil {
		t.Fatal("esperava erro com as duas rotas quebradas")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("indisponibilidade não pode virar ErrNotFound (viraria publicação silenciosa): %v", err)
	}
}
