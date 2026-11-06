package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// cliPageStub imita o page-management v2, a API legada de página e o admin
// (grupos) com as fixtures reais de testdata/page_*.json.
type cliPageStub struct {
	mu       sync.Mutex
	status   string
	hasDraft bool
	created  map[string]any
	put      map[string]any
	prefs    map[string]string
	added    map[string]string
	removed  []string
	publish  int
	editor   string // login do editor do rascunho (GET ?expand=user)
	takeOver int
}

func (s *cliPageStub) server(t *testing.T) *httptest.Server {
	td := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/admin/api/v1/groups/", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/groups/")
		if code != "equipe" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"FDNEntityNotFoundException","message":"not found"}`)
			return
		}
		io.WriteString(w, `{"code":"equipe","description":"Equipe","type":"user"}`)
	})
	mux.HandleFunc("/page-management/api/v2/layouts/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/page-management/api/v2/layouts/") {
		case "portais":
			io.WriteString(w, `{"id":346,"code":"portais","title":"Portais","internal":false}`)
		case "portais/pages":
			io.WriteString(w, `{"items":[{"code":"financeiro","title":"Painel Financeiro","status":"PUBLISHED"}],"hasNext":false}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/portal/p/1/financeiro", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><head><title>Empresa - Painel Financeiro</title></head><body><div id="SlotA"></div><div id="SlotB"></div></body></html>`)
	})
	mux.HandleFunc("/page-management/api/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Write(td("page_list.json"))
	})
	mux.HandleFunc("/page-management/api/v2/pages/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/page-management/api/v2/pages/"), "/")
		if parts[0] != "financeiro" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			var raw map[string]any
			_ = json.Unmarshal(td("page_get.json"), &raw)
			if s.status != "" {
				raw["status"] = s.status
			}
			if s.editor != "" {
				raw["user"] = map[string]any{"login": s.editor, "fullName": "Maria Souza"}
			}
			_ = json.NewEncoder(w).Encode(raw)
		case len(parts) == 2 && parts[1] == "change-edit-user":
			s.takeOver++
			w.WriteHeader(http.StatusNoContent)
		case len(parts) == 1 && r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &s.put)
			io.WriteString(w, `{"id":212}`)
		case len(parts) == 1 && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case parts[1] == "application" && r.Method == http.MethodGet:
			w.Write(td("page_application.json"))
		case parts[1] == "application" && r.Method == http.MethodPost:
			if !s.hasDraft {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"code":"PageVersionDraftNotFoundException","message":"x"}`)
				return
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &s.added)
			io.WriteString(w, `{"id":9001,"slot":"`+s.added["slot"]+`","code":"`+s.added["widgetCode"]+`","order":1}`)
		case parts[1] == "application" && r.Method == http.MethodDelete:
			s.removed = append(s.removed, parts[2])
			w.WriteHeader(http.StatusNoContent)
		case parts[1] == "permission":
			w.Write(td("page_permission.json"))
		case parts[1] == "draft" && r.Method == http.MethodPost:
			s.hasDraft = true
			w.WriteHeader(http.StatusNoContent)
		case parts[1] == "publish":
			s.publish++
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/portal/api/rest/wcm/page/create", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.created)
		s.mu.Unlock()
		io.WriteString(w, `"{success: true}"`)
	})
	mux.HandleFunc("/portal/api/rest/wcm/page/findWidgetDataByInstanceId/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"invdata":[{"key":"titulo","value":"Painel"}]}`)
	})
	mux.HandleFunc("/portal/api/rest/wcm/page/application/setpreferences/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		_ = json.Unmarshal(b, &s.prefs)
		s.mu.Unlock()
		io.WriteString(w, `{"message":"ok","type":"INFO"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func pageArgs(proj string, args ...string) []string {
	return append(args, "--project", proj, "--server", "homolog")
}

func TestPageListTabela(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, pageArgs(proj, "page", "list")...)
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│", "Código", "Layout", "Status", "financeiro", "portais", "PUBLISHED"} {
		if !strings.Contains(out, want) {
			t.Errorf("tabela sem %q:\n%s", want, out)
		}
	}
}

func TestPageShowJSON(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, pageArgs(proj, "page", "show", "financeiro", "--json")...)
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	for _, want := range []string{`"instanceId":4225`, `"preferences":{"titulo":"Painel"}`, `"published":true`, `"permission":"pageView"`} {
		if !strings.Contains(out, want) {
			t.Errorf("envelope sem %s:\n%s", want, out)
		}
	}
}

func TestPageCreateComGrupos(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, pageArgs(proj, "page", "create", "financeiro", "--title", "Painel", "--layout", "portais", "--view-group", "equipe", "--json")...)
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	items := stub.created["formData"].(map[string]any)["selectedItens"].([]any)
	var views, others int
	for _, it := range items {
		m := it.(map[string]any)
		if m["permission"] == "pageView" {
			views++
			if m["groupCode"] != "equipe" {
				t.Errorf("view para quem não pediu: %+v", m)
			}
		} else {
			others++
		}
	}
	if views != 1 || others != 2 {
		t.Errorf("permissões inesperadas: %+v", items)
	}
}

func TestPageCreateValidacoes(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	cases := map[string]struct {
		args []string
		exit int
	}{
		"sem layout":         {[]string{"page", "create", "x", "--title", "T"}, output.ExitUsage},
		"layout inexistente": {[]string{"page", "create", "x", "--title", "T", "--layout", "nao_existe"}, output.ExitNotFound},
		"grupo inexistente":  {[]string{"page", "create", "x", "--title", "T", "--layout", "portais", "--view-group", "nao_existe"}, output.ExitNotFound},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if code, out := runMain(t, pageArgs(proj, append(c.args, "--json")...)...); code != c.exit {
				t.Errorf("exit=%d, esperava %d\n%s", code, c.exit, out)
			}
		})
	}
	if stub.created != nil {
		t.Errorf("validação falhou e o create foi enviado: %+v", stub.created)
	}
}

func TestPageAddWidgetAbreRascunho(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, pageArgs(proj, "page", "add-widget", "financeiro", "minha_widget", "--slot", "SlotB", "--json")...)
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if !stub.hasDraft || stub.added["slot"] != "SlotB" || !strings.Contains(out, `"draftOpened":true`) {
		t.Errorf("rascunho/slot inesperados: draft=%v added=%v\n%s", stub.hasDraft, stub.added, out)
	}
}

func TestPageAddWidgetSlotInvalido(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, pageArgs(proj, "page", "add-widget", "financeiro", "w", "--slot", "SlotZZ", "--json")...)
	if code != output.ExitUsage || !strings.Contains(out, "SlotA, SlotB") {
		t.Errorf("exit=%d\n%s", code, out)
	}
	if stub.added != nil {
		t.Errorf("widget enviada com slot inválido: %+v", stub.added)
	}
}

func TestPageRemoveWidget(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	if code, out := runMain(t, pageArgs(proj, "page", "remove-widget", "financeiro", "4225", "--json")...); code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if len(stub.removed) != 1 || stub.removed[0] != "4225" {
		t.Errorf("remoção inesperada: %v", stub.removed)
	}
	if code, _ := runMain(t, pageArgs(proj, "page", "remove-widget", "financeiro", "1", "--json")...); code != output.ExitNotFound {
		t.Errorf("instância fora da página: exit=%d", code)
	}
	stub.status = "DRAFT"
	if code, _ := runMain(t, pageArgs(proj, "page", "remove-widget", "financeiro", "4225", "--json")...); code != output.ExitUsage {
		t.Errorf("com rascunho aberto: exit=%d, esperava %d", code, output.ExitUsage)
	}
}

func TestPageSetPreferences(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	old := pageStdin
	t.Cleanup(func() { pageStdin = old })
	pageStdin = strings.NewReader(`{"titulo":"Painel","limite":10,"ativo":true}`)
	if code, out := runMain(t, pageArgs(proj, "page", "set-preferences", "financeiro", "4225", "--json")...); code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if stub.prefs["titulo"] != "Painel" || stub.prefs["limite"] != "10" || stub.prefs["ativo"] != "true" {
		t.Errorf("preferências inesperadas: %+v", stub.prefs)
	}
	pageStdin = strings.NewReader(`{"obj":{"a":1}}`)
	if code, _ := runMain(t, pageArgs(proj, "page", "set-preferences", "financeiro", "4225", "--json")...); code != output.ExitUsage {
		t.Errorf("objeto aninhado: exit=%d, esperava %d", code, output.ExitUsage)
	}
}

func TestPagePermissionMantemEdicao(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	if code, out := runMain(t, pageArgs(proj, "page", "permission", "financeiro", "--view-group", "equipe", "--json")...); code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	perms := stub.put["permissions"].([]any)
	var views []string
	var updates int
	for _, it := range perms {
		m := it.(map[string]any)
		switch m["permission"] {
		case "pageView":
			views = append(views, m["group"].(string))
		case "pageUpdate":
			updates++
		}
	}
	if strings.Join(views, ",") != "equipe" || updates == 0 {
		t.Errorf("permissões inesperadas: %+v", perms)
	}
}

func TestPagePublishSemRascunho(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	if code, _ := runMain(t, pageArgs(proj, "page", "publish", "financeiro", "--json")...); code != output.ExitUsage {
		t.Errorf("sem rascunho: exit=%d, esperava %d", code, output.ExitUsage)
	}
	stub.status = "DRAFT"
	code, out := runMain(t, pageArgs(proj, "page", "publish", "financeiro", "--json")...)
	if code != output.ExitOK || stub.publish != 1 || !strings.Contains(out, `"instanceId":4225`) {
		t.Fatalf("exit=%d publish=%d\n%s", code, stub.publish, out)
	}
}

func TestPageDeleteExigeYes(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	if code, _ := runMain(t, pageArgs(proj, "page", "delete", "financeiro", "--json")...); code != output.ExitUsage {
		t.Errorf("sem --yes: exit=%d", code)
	}
	if code, out := runMain(t, pageArgs(proj, "page", "delete", "financeiro", "--yes", "--json")...); code != output.ExitOK {
		t.Errorf("exit=%d\n%s", code, out)
	}
}

func TestPageRascunhoDeOutroUsuario(t *testing.T) {
	stub := &cliPageStub{status: "DRAFT", editor: "msouza"}
	proj := mechProject(t, stub.server(t).URL)
	code, out := runMain(t, pageArgs(proj, "page", "publish", "financeiro", "--json")...)
	if code != output.ExitServer || !strings.Contains(out, "Maria Souza (msouza)") || !strings.Contains(out, "--take-over") {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if stub.publish != 0 || stub.takeOver != 0 {
		t.Errorf("publicou sem --take-over: publish=%d takeOver=%d", stub.publish, stub.takeOver)
	}
	code, out = runMain(t, pageArgs(proj, "page", "publish", "financeiro", "--take-over", "--json")...)
	if code != output.ExitOK || stub.takeOver != 1 || stub.publish != 1 {
		t.Fatalf("exit=%d takeOver=%d publish=%d\n%s", code, stub.takeOver, stub.publish, out)
	}
}

func TestPageRascunhoDoProprioUsuario(t *testing.T) {
	// mechProject usa o usuário "u": rascunho dele não exige --take-over.
	stub := &cliPageStub{status: "DRAFT", editor: "u"}
	proj := mechProject(t, stub.server(t).URL)
	if code, out := runMain(t, pageArgs(proj, "page", "publish", "financeiro", "--json")...); code != output.ExitOK || stub.takeOver != 0 {
		t.Fatalf("exit=%d takeOver=%d\n%s", code, stub.takeOver, out)
	}
}
