package devserver

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// peopleUpstream simula os módulos /admin/api/v1 (users/groups/roles + membros)
// e o process-management (para o "Onde é usado?"). Serve as fixtures reais e
// captura as chamadas de escrita de membership para o teste garantir que só o
// usuário logado é escrito.
type peopleUpstream struct {
	srv     *httptest.Server
	mu      sync.Mutex
	adds    []string // "kind|code|login" das inclusões recebidas
	removes []string // "kind|code|login" das remoções recebidas
	forbid  bool     // quando true, os módulos admin respondem 401 (sem privilégio)
}

func newPeopleUpstream(t *testing.T) *peopleUpstream {
	t.Helper()
	load := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatalf("fixture %s: %v", name, err)
		}
		return b
	}
	users := load("rest_admin_users.json")
	groups := load("rest_admin_groups.json")
	roles := load("rest_admin_roles.json")
	members := load("rest_group_users.json")
	procExport := load("rest_process_export_full.xml")

	up := &peopleUpstream{}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"pong"}`))
	})

	admin := func(w http.ResponseWriter) bool {
		up.mu.Lock()
		defer up.mu.Unlock()
		if up.forbid {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("<html><body>Unauthorized</body></html>"))
			return false
		}
		return true
	}

	// Usuários: lista (exato) e get por login (prefixo).
	mux.HandleFunc("/admin/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w) {
			return
		}
		_, _ = w.Write(users)
	})
	mux.HandleFunc("/admin/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w) {
			return
		}
		login := strings.TrimPrefix(r.URL.Path, "/admin/api/v1/users/")
		if login == "user1" {
			_, _ = w.Write([]byte(`{"login":"user1","code":"user1","email":"user1@exemplo.com","fullName":"Ana Andrade","state":"ACTIVE","roles":["diretor"],"groups":["Compras"]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"FDNEntityNotFoundException"}`))
	})

	// Grupos: lista (exato); prefixo trata get/membros/add/remove.
	mux.HandleFunc("/admin/api/v1/groups", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w) {
			return
		}
		_, _ = w.Write(groups)
	})
	mux.HandleFunc("/admin/api/v1/groups/", func(w http.ResponseWriter, r *http.Request) {
		up.handleMembership(w, r, "group", "/admin/api/v1/groups/", members)
	})

	// Papéis: idem.
	mux.HandleFunc("/admin/api/v1/roles", func(w http.ResponseWriter, r *http.Request) {
		if !admin(w) {
			return
		}
		_, _ = w.Write(roles)
	})
	mux.HandleFunc("/admin/api/v1/roles/", func(w http.ResponseWriter, r *http.Request) {
		up.handleMembership(w, r, "role", "/admin/api/v1/roles/", members)
	})

	// Processos (para o "Onde é usado?").
	mux.HandleFunc("/process-management/api/v2/processes", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"processId":"compras_entrada_documento","processDescription":"Entrada de Documento","active":true}],"hasNext":false}`))
	})
	mux.HandleFunc("/process-management/api/v2/processes/compras_entrada_documento/export/xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(procExport)
	})
	mux.HandleFunc("/process-management/api/v2/processes/compras_entrada_documento/process-versions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"version":25,"active":true}],"hasNext":false}`))
	})

	up.srv = httptest.NewServer(mux)
	t.Cleanup(up.srv.Close)
	return up
}

// handleMembership serve GET {code} (pré-validação), GET {code}/users
// (membros), POST {code}/users (add) e DELETE {code}/users/{login} (remove).
func (up *peopleUpstream) handleMembership(w http.ResponseWriter, r *http.Request, kind, prefix string, members []byte) {
	up.mu.Lock()
	if up.forbid {
		up.mu.Unlock()
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("<html><body>Unauthorized</body></html>"))
		return
	}
	up.mu.Unlock()

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(rest, "/", 3)
	code := parts[0]
	// GET {code} → o próprio grupo/papel (pré-validação de Add/Remove*User).
	if len(parts) == 1 {
		if kind == "group" {
			_, _ = w.Write([]byte(`{"code":"` + code + `","description":"` + code + `","type":"user"}`))
		} else {
			_, _ = w.Write([]byte(`{"code":"` + code + `","description":"` + code + `"}`))
		}
		return
	}
	if parts[1] != "users" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_, _ = w.Write(members)
	case http.MethodPost:
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		login := extractLogin(string(body))
		up.mu.Lock()
		up.adds = append(up.adds, kind+"|"+code+"|"+login)
		up.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		login := ""
		if len(parts) == 3 {
			login = parts[2]
		}
		up.mu.Lock()
		up.removes = append(up.removes, kind+"|"+code+"|"+login)
		up.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func extractLogin(body string) string {
	i := strings.Index(body, `"login"`)
	if i < 0 {
		return ""
	}
	rest := body[i+7:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	rest = rest[j+1:]
	k := strings.Index(rest, `"`)
	if k < 0 {
		return ""
	}
	return rest[:k]
}

// newPeopleServer monta o dev server contra o upstream, com o usuário logado
// informado (para exercer o membership só-do-usuário-da-sessão).
func newPeopleServer(t *testing.T, up *peopleUpstream, withClient bool, username string) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	u, err := url.Parse(up.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Root: root, Upstream: u, Port: 0, Debounce: 10 * time.Millisecond, Username: username}
	jar, _ := cookiejar.New(nil)
	opts.Jar = jar
	if withClient {
		client, err := fluig.NewClient(fluig.Options{BaseURL: up.srv.URL, Username: "dev-" + t.Name(), Password: "p", CompanyID: 1})
		if err != nil {
			t.Fatal(err)
		}
		opts.Client = client
		opts.CompanyID = 1
	}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.handler)
	t.Cleanup(ts.Close)
	return ts
}

// A página é servida com os marcadores esperados.
func TestPeoplePanelPage(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "user1")
	status, body := getBody(t, ts.URL+"/_dev/people/")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{"fluigcli", "/_dev/api/people/", "Grupos", "Papéis", "Onde é usado", "Incluir-me"} {
		if !strings.Contains(body, want) {
			t.Errorf("página não contém %q", want)
		}
	}
}

// As três listagens respondem e trazem o "me".
func TestPeopleLists(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "user1")
	for _, tc := range []struct{ path, want string }{
		{"/_dev/api/people/users", `"login":"user1"`},
		{"/_dev/api/people/users", `"me":"user1"`},
		{"/_dev/api/people/groups", `"code":"Compras"`},
		{"/_dev/api/people/roles", `"code":"diretor"`},
	} {
		status, body := getBody(t, ts.URL+tc.path)
		if status != http.StatusOK || !strings.Contains(body, tc.want) {
			t.Errorf("%s inesperado (%d): sem %q", tc.path, status, tc.want)
		}
	}
}

// A lista de usuários é cacheada — o segundo request não bate no upstream.
func TestPeopleUsersCache(t *testing.T) {
	up := newPeopleUpstream(t)
	ts := newPeopleServer(t, up, true, "user1")
	if status, _ := getBody(t, ts.URL+"/_dev/api/people/users"); status != http.StatusOK {
		t.Fatalf("primeira carga falhou: %d", status)
	}
	up.srv.Close() // se o cache funciona, o segundo request ainda responde
	status, body := getBody(t, ts.URL+"/_dev/api/people/users")
	if status != http.StatusOK || !strings.Contains(body, `"login":"user1"`) {
		t.Errorf("cache de usuários não usado (%d)", status)
	}
}

// members lista os membros de um grupo/papel.
func TestPeopleMembers(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "user1")
	status, body := getBody(t, ts.URL+"/_dev/api/people/members?kind=group&code=Compras")
	if status != http.StatusOK || !strings.Contains(body, `"login":"user1"`) {
		t.Fatalf("membros do grupo inesperados (%d): %s", status, body)
	}
	// kind inválido → 400.
	if status, _ := getBody(t, ts.URL+"/_dev/api/people/members?kind=x&code=Compras"); status != http.StatusBadRequest {
		t.Errorf("kind inválido deveria dar 400, deu %d", status)
	}
}

// user traz a visão reversa (grupos + papéis do usuário).
func TestPeopleUserReverse(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "user1")
	status, body := getBody(t, ts.URL+"/_dev/api/people/user?login=user1")
	if status != http.StatusOK {
		t.Fatalf("user status %d: %s", status, body)
	}
	for _, want := range []string{`"groups":["Compras"]`, `"roles":["diretor"]`} {
		if !strings.Contains(body, want) {
			t.Errorf("visão reversa sem %q", want)
		}
	}
	// Login inexistente → 404.
	if status, _ := getBody(t, ts.URL+"/_dev/api/people/user?login=fantasma"); status != http.StatusNotFound {
		t.Errorf("login inexistente deveria dar 404, deu %d", status)
	}
}

// membership escreve SEMPRE o usuário da sessão (opts.Username), nunca outro, e
// invalida o cache de membros.
func TestPeopleMembershipSoUsuarioDaSessao(t *testing.T) {
	up := newPeopleUpstream(t)
	ts := newPeopleServer(t, up, true, "user1")

	status, out := postJSON(t, ts.URL+"/_dev/api/people/membership", map[string]any{"kind": "group", "code": "Compras", "action": "add"})
	if status != http.StatusOK || out["ok"] != true {
		t.Fatalf("add falhou (%d): %v", status, out)
	}
	if out["login"] != "user1" {
		t.Errorf("membership escreveu login %v, esperado user1", out["login"])
	}
	up.mu.Lock()
	adds := append([]string(nil), up.adds...)
	up.mu.Unlock()
	if len(adds) != 1 || adds[0] != "group|Compras|user1" {
		t.Errorf("inclusão registrada = %v, esperado [group|Compras|user1]", adds)
	}

	// remove de papel.
	status, out = postJSON(t, ts.URL+"/_dev/api/people/membership", map[string]any{"kind": "role", "code": "diretor", "action": "remove"})
	if status != http.StatusOK {
		t.Fatalf("remove falhou (%d): %v", status, out)
	}
	up.mu.Lock()
	removes := append([]string(nil), up.removes...)
	up.mu.Unlock()
	if len(removes) != 1 || removes[0] != "role|diretor|user1" {
		t.Errorf("remoção registrada = %v, esperado [role|diretor|user1]", removes)
	}

	// action/kind inválidos → 400.
	if status, _ := postJSON(t, ts.URL+"/_dev/api/people/membership", map[string]any{"kind": "group", "code": "Compras", "action": "x"}); status != http.StatusBadRequest {
		t.Errorf("action inválida deveria dar 400, deu %d", status)
	}
}

// Sem identidade resolvida (Username vazio), membership recusa com 400.
func TestPeopleMembershipSemIdentidade(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "")
	status, _ := postJSON(t, ts.URL+"/_dev/api/people/membership", map[string]any{"kind": "group", "code": "Compras", "action": "add"})
	if status != http.StatusBadRequest {
		t.Errorf("sem identidade deveria dar 400, deu %d", status)
	}
}

// usage cruza um papel com as etapas dos processos (o export real tem a
// atribuição do papel "faturista" na etapa 17).
func TestPeopleUsage(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "user1")
	status, body := getBody(t, ts.URL+"/_dev/api/people/usage?kind=role&code=faturista")
	if status != http.StatusOK {
		t.Fatalf("usage status %d: %s", status, body)
	}
	for _, want := range []string{`"compras_entrada_documento"`, `"sequence":17`, `"scanned":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("usage sem %q: %s", want, body)
		}
	}
	// Papel que não aparece em nenhuma etapa → zero hits.
	_, body = getBody(t, ts.URL+"/_dev/api/people/usage?kind=role&code=inexistente_xyz")
	if !strings.Contains(body, `"hits":[]`) {
		t.Errorf("usage de papel não usado deveria vir com hits vazio: %s", body)
	}
}

// Sem privilégio admin, as listagens degradam para needsAdmin (200, banner).
func TestPeopleNeedsAdmin(t *testing.T) {
	up := newPeopleUpstream(t)
	up.forbid = true
	ts := newPeopleServer(t, up, true, "user1")
	status, body := getBody(t, ts.URL+"/_dev/api/people/users")
	if status != http.StatusOK || !strings.Contains(body, `"needsAdmin":true`) {
		t.Errorf("sem admin deveria vir needsAdmin=true (200), deu %d: %s", status, body)
	}
}

// Sem cliente, a API responde 503 e a página segue abrindo.
func TestPeopleSemCliente(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), false, "user1")
	if status, _ := getBody(t, ts.URL+"/_dev/api/people/users"); status != http.StatusServiceUnavailable {
		t.Errorf("sem cliente deveria dar 503, deu %d", status)
	}
	if status, _ := getBody(t, ts.URL+"/_dev/people/"); status != http.StatusOK {
		t.Errorf("página deveria abrir sem cliente, deu %d", status)
	}
}

// O tile de Pessoas aparece no dashboard e o Explorador linka para cá.
func TestPeopleTileELinkExplorador(t *testing.T) {
	ts := newPeopleServer(t, newPeopleUpstream(t), true, "user1")
	if _, body := getBody(t, ts.URL+"/"); !strings.Contains(body, "/_dev/people/") {
		t.Errorf("dashboard sem o tile de pessoas")
	}
	// A página do explorador referencia /_dev/people/ nos chips de atribuição.
	if _, body := getBody(t, ts.URL+"/_dev/processes/"); !strings.Contains(body, "/_dev/people/?") {
		t.Errorf("explorador não linka para a subtela Pessoas")
	}
}
