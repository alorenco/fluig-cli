package fluig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// pageStub imita o page-management v2 e a API legada de página com as
// fixtures reais (testdata/page_*.json, medidas em 2026-11-06).
type pageStub struct {
	mu          sync.Mutex
	status      string // status devolvido pelo GET da página
	hasDraft    bool   // o POST de widget exige rascunho
	created     map[string]any
	put         map[string]any
	prefs       map[string]string
	calls       []string
	createError string
}

func (s *pageStub) log(r *http.Request) {
	s.mu.Lock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	s.mu.Unlock()
}

func (s *pageStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/page-management/api/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		s.log(r)
		w.Write(testdata(t, "page_list.json"))
	})
	mux.HandleFunc("/page-management/api/v2/pages/", func(w http.ResponseWriter, r *http.Request) {
		s.log(r)
		rest := strings.TrimPrefix(r.URL.Path, "/page-management/api/v2/pages/")
		parts := strings.Split(rest, "/")
		code := parts[0]
		if code == "nunca" && len(parts) == 2 && parts[1] == "application" {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"code":"EJBException","message":"Oops, something went wrong!"}`)
			return
		}
		if code != "financeiro" {
			if len(parts) == 1 {
				w.WriteHeader(http.StatusNotFound) // corpo vazio, como no servidor
				return
			}
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"WCMPageNotFoundException","message":"Page with code \"`+code+`\" not found"}`)
			return
		}
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			var raw map[string]any
			_ = json.Unmarshal(testdata(t, "page_get.json"), &raw)
			if s.status != "" {
				raw["status"] = s.status
			}
			_ = json.NewEncoder(w).Encode(raw)
		case len(parts) == 1 && r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &s.put)
			io.WriteString(w, `{"id":212,"code":"financeiro","status":"DRAFT"}`)
		case len(parts) == 1 && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case parts[1] == "application" && r.Method == http.MethodGet:
			w.Write(testdata(t, "page_application.json"))
		case parts[1] == "application" && r.Method == http.MethodPost:
			if !s.hasDraft {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"code":"PageVersionDraftNotFoundException","message":"The draft of the page was not found."}`)
				return
			}
			b, _ := io.ReadAll(r.Body)
			var in map[string]string
			_ = json.Unmarshal(b, &in)
			if in["widgetCode"] == "nao_existe" {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"code":"WCMComponentNotFound","message":"Component with code nao_existe not found."}`)
				return
			}
			io.WriteString(w, `{"id":9001,"slot":"`+in["slot"]+`","code":"`+in["widgetCode"]+`","order":1}`)
		case parts[1] == "application" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case parts[1] == "permission":
			w.Write(testdata(t, "page_permission.json"))
		case parts[1] == "draft" && r.Method == http.MethodPost:
			s.hasDraft = true
			w.WriteHeader(http.StatusNoContent)
		case parts[1] == "draft" || parts[1] == "publish" || parts[1] == "archive":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("rota inesperada: %s %s", r.Method, r.URL.Path)
		}
	})
	mux.HandleFunc("/page-management/api/v2/layouts/portais/pages", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"items":[{"code":"rascunho","title":"R","status":"DRAFT"},{"code":"financeiro","title":"Painel Financeiro","status":"PUBLISHED"}],"hasNext":false}`)
	})
	mux.HandleFunc("/portal/p/1/financeiro", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><head><title>Empresa - Painel Financeiro</title></head><body><div id="SlotA"></div><div id="SlotUsePolicy"></div><div id="SlotA"></div></body></html>`)
	})
	mux.HandleFunc("/portal/api/rest/wcm/page/create", func(w http.ResponseWriter, r *http.Request) {
		s.log(r)
		if s.createError != "" {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, s.createError)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &s.created)
		io.WriteString(w, `"{success: true}"`)
	})
	mux.HandleFunc("/portal/api/rest/wcm/page/findWidgetDataByInstanceId/4225", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"totalpages":1,"totalrecords":"1","currpage":1,"invdata":[{"value":"v","key":"k"}],"genericObject":null}`)
	})
	mux.HandleFunc("/portal/api/rest/wcm/page/application/setpreferences/4225", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &s.prefs)
		io.WriteString(w, `{"message":"Preferências alterado com sucesso!","type":"INFO"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func pageClient(t *testing.T, url string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: url, Username: "jsilva", Password: "p", CompanyID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListPagesELayout(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	pages, err := c.ListPages(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 || pages[0].Code != "saobento" || pages[0].Layout != "portais" || pages[0].Status != PageStatusDraft {
		t.Errorf("listagem inesperada: %+v", pages[0])
	}
	if !strings.Contains(stub.calls[len(stub.calls)-1], "/page-management/api/v2/pages") {
		t.Errorf("rota inesperada: %v", stub.calls)
	}
}

func TestGetPageENaoEncontrada(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	p, err := c.GetPage(context.Background(), "financeiro")
	if err != nil || p.ID != 212 || p.Layout != "portais" || p.Status != PageStatusPublished {
		t.Fatalf("GetPage: %+v, %v", p, err)
	}
	if _, err := c.GetPage(context.Background(), "nao_existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("esperava ErrNotFound, veio %v", err)
	}
}

func TestPageWidgetsEPermissoes(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	ctx := context.Background()
	ws, err := c.PageWidgets(ctx, "financeiro")
	if err != nil || len(ws) == 0 || ws[0].InstanceID != 4225 || ws[0].Widget != "financeiro_painel" || ws[0].Slot != "SlotA" {
		t.Fatalf("PageWidgets: %+v, %v", ws, err)
	}
	perms, err := c.PagePermissions(ctx, "financeiro")
	if err != nil || len(perms) == 0 || perms[0].Permission != PagePermView || perms[0].Role != "admin" {
		t.Fatalf("PagePermissions: %+v, %v", perms, err)
	}
	prefs, err := c.WidgetPreferences(ctx, 4225)
	if err != nil || prefs["k"] != "v" {
		t.Fatalf("WidgetPreferences: %+v, %v", prefs, err)
	}
}

func TestPageWidgetsNuncaPublicada(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	if _, err := c.PageWidgets(context.Background(), "nunca"); !errors.Is(err, ErrPageNotPublished) {
		t.Errorf("esperava ErrPageNotPublished, veio %v", err)
	}
}

func TestCreatePageCorpoLegado(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	perms := append([]PagePermission{{Permission: PagePermView, Group: "equipe"}}, c.DefaultPagePermissions()[1:]...)
	err := c.CreatePage(context.Background(), PageCreateInput{Code: "painel", Title: "Painel", Description: "d", Layout: "layoutdefault", Permissions: perms})
	if err != nil {
		t.Fatal(err)
	}
	fd := stub.created["formData"].(map[string]any)
	if fd["code"] != "painel" || fd["layout"] != "layoutdefault" || fd["auth"] != "1" {
		t.Errorf("formData inesperado: %+v", fd)
	}
	items := fd["selectedItens"].([]any)
	first := items[0].(map[string]any)
	if first["groupCode"] != "equipe" || first["permission"] != "pageView" || first["permissionDescription"] != "permission.page.view" {
		t.Errorf("permissão inesperada: %+v", first)
	}
	upd := items[1].(map[string]any)
	if upd["login"] != "jsilva" || upd["permission"] != "pageUpdate" {
		t.Errorf("permissão de edição inesperada: %+v", upd)
	}
}

func TestCreatePageDuplicada(t *testing.T) {
	stub := &pageStub{createError: `{"content":"ERROR","message":{"message":"Já existe uma página com o código painel.","errorCode":"WCMPageDuplicatedException"}}`}
	c := pageClient(t, stub.server(t).URL)
	err := c.CreatePage(context.Background(), PageCreateInput{Code: "painel", Title: "P", Layout: "l"})
	if !errors.Is(err, ErrPageExists) {
		t.Fatalf("esperava ErrPageExists, veio %v", err)
	}
	stub.createError = `{"success":false,"message":"Erro na criação da página. null","code":500}`
	if err := c.CreatePage(context.Background(), PageCreateInput{Code: "x", Title: "P", Layout: "l"}); err == nil {
		t.Fatal("esperava erro do formato {success:false}")
	}
}

func TestAddPageWidgetSemRascunho(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	ctx := context.Background()
	if _, err := c.AddPageWidget(ctx, "financeiro", "SlotA", "w"); !errors.Is(err, ErrPageNoDraft) {
		t.Fatalf("esperava ErrPageNoDraft, veio %v", err)
	}
	if err := c.CreatePageDraft(ctx, "financeiro"); err != nil {
		t.Fatal(err)
	}
	w, err := c.AddPageWidget(ctx, "financeiro", "SlotA", "w")
	if err != nil || w.InstanceID != 9001 || w.Slot != "SlotA" {
		t.Fatalf("AddPageWidget: %+v, %v", w, err)
	}
	if _, err := c.AddPageWidget(ctx, "financeiro", "SlotA", "nao_existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("widget inexistente: esperava ErrNotFound, veio %v", err)
	}
}

func TestUpdatePagePermissionsCorpo(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	ctx := context.Background()
	p, _ := c.GetPage(ctx, "financeiro")
	err := c.UpdatePagePermissions(ctx, p, []PagePermission{{Permission: PagePermView, Group: "equipe"}, {Permission: PagePermUpdate, Role: "admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if stub.put["title"] != "Painel Financeiro" || stub.put["authorization"] != "AUTHENTICATED" || stub.put["parent"] != float64(0) {
		t.Errorf("PUT sem os campos atuais: %+v", stub.put)
	}
	if _, ok := stub.put["parentCode"]; ok {
		t.Errorf("PUT não pode mandar parentCode (o DTO recusa): %+v", stub.put)
	}
	perms := stub.put["permissions"].([]any)
	first := perms[0].(map[string]any)
	if first["group"] != "equipe" || first["resource"] != "page.212" || first["description"] != "permission.page.view" {
		t.Errorf("permissão inesperada: %+v", first)
	}
}

func TestDeletePageArquivaAntes(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	if err := c.DeletePage(context.Background(), "financeiro"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(stub.calls, "\n")
	ia := strings.Index(joined, "DELETE /page-management/api/v2/pages/financeiro/archive")
	id := strings.LastIndex(joined, "DELETE /page-management/api/v2/pages/financeiro\n")
	if id < 0 {
		id = strings.LastIndex(joined, "DELETE /page-management/api/v2/pages/financeiro")
	}
	if ia < 0 || id < ia {
		t.Errorf("esperava archive antes do delete:\n%s", joined)
	}
}

func TestLayoutSlotsPelaPaginaRenderizada(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	slots, err := c.LayoutSlots(context.Background(), "portais")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(slots, ",") != "SlotA,SlotUsePolicy" {
		t.Errorf("slots inesperados: %v", slots)
	}
}

func TestSetWidgetPreferences(t *testing.T) {
	stub := &pageStub{}
	c := pageClient(t, stub.server(t).URL)
	if err := c.SetWidgetPreferences(context.Background(), 4225, map[string]string{"a": "1"}); err != nil {
		t.Fatal(err)
	}
	if stub.prefs["a"] != "1" {
		t.Errorf("preferências não enviadas: %+v", stub.prefs)
	}
}
