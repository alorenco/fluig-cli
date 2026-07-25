package fluig

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fluigStub simula os servlets de autenticação do Fluig.
type fluigStub struct {
	password    string
	demoMode    bool // exige license.do?demo=true antes de o ping funcionar
	demoUnlock  atomic.Bool
	loginCalls  atomic.Int32
	userFixture string
}

func (s *fluigStub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		s.loginCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("j_password") == s.password {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "sessao-ok", Path: "/"})
		}
		fmt.Fprint(w, "<html>portal</html>")
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie("JSESSIONIDSSO")
		sessionOK := err == nil && ck.Value == "sessao-ok"
		if !sessionOK || (s.demoMode && !s.demoUnlock.Load()) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/servlet/license.do", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("demo") == "true" {
			s.demoUnlock.Store(true)
		}
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("login") == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data, err := os.ReadFile(s.userFixture)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(data)
	})
	return mux
}

func newTestClient(t *testing.T, baseURL, username, password string) *Client {
	t.Helper()
	c, err := NewClient(Options{BaseURL: baseURL, Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLogin(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	t.Run("sucesso guarda a sessão no jar", func(t *testing.T) {
		c := newTestClient(t, srv.URL, "user-login-ok", "correta")
		if err := c.Login(context.Background()); err != nil {
			t.Fatalf("Login: %v", err)
		}
		if !c.hasSessionCookies() {
			t.Error("cookies de sessão ausentes após login")
		}
	})

	t.Run("senha errada → ErrAuthFailed", func(t *testing.T) {
		c := newTestClient(t, srv.URL, "user-login-errada", "errada")
		err := c.Login(context.Background())
		if !errors.Is(err, ErrAuthFailed) {
			t.Errorf("esperava ErrAuthFailed, veio %v", err)
		}
	})

	// Regressão (achado na homologação): o jar compartilhado por host+usuário
	// não pode fazer um login com senha errada "passar" usando cookies antigos.
	t.Run("senha errada falha mesmo com sessão anterior no cache", func(t *testing.T) {
		ok := newTestClient(t, srv.URL, "user-relogin", "correta")
		if err := ok.Login(context.Background()); err != nil {
			t.Fatalf("login válido: %v", err)
		}
		bad := newTestClient(t, srv.URL, "user-relogin", "errada")
		err := bad.Login(context.Background())
		if !errors.Is(err, ErrAuthFailed) {
			t.Errorf("esperava ErrAuthFailed, veio %v", err)
		}
	})
}

func TestEnsureSessionHappyPath(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	c := newTestClient(t, srv.URL, "user-ensure", "correta")
	if err := c.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping após EnsureSession: %v", err)
	}
}

// Quirk "modo demo": login ok mas ping falha até chamar license.do?demo=true.
func TestEnsureSessionDemoQuirk(t *testing.T) {
	stub := &fluigStub{password: "correta", demoMode: true}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	c := newTestClient(t, srv.URL, "user-demo", "correta")
	if err := c.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession com quirk demo: %v", err)
	}
	if !stub.demoUnlock.Load() {
		t.Error("license.do?demo=true não foi chamado")
	}
	if got := stub.loginCalls.Load(); got != 2 {
		t.Errorf("esperava 2 logins (antes e depois do license.do), veio %d", got)
	}
}

// O cache de sessão por host+usuário evita novo login no mesmo processo.
func TestEnsureSessionReusesCache(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	c1 := newTestClient(t, srv.URL, "user-cache", "correta")
	if err := c1.EnsureSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	logins := stub.loginCalls.Load()

	c2 := newTestClient(t, srv.URL, "user-cache", "correta")
	if err := c2.EnsureSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := stub.loginCalls.Load(); got != logins {
		t.Errorf("segundo client refez login (%d → %d); deveria reutilizar a sessão validada por ping", logins, got)
	}
}

// fakeSessionCache é um cache de sessão em memória para testes.
type fakeSessionCache struct {
	load  []*http.Cookie
	saved []*http.Cookie
}

func (f *fakeSessionCache) Load(string) []*http.Cookie            { return f.load }
func (f *fakeSessionCache) Save(_ string, c []*http.Cookie) error { f.saved = c; return nil }

// Uma sessão válida vinda do disco é reaproveitada sem novo login.
func TestEnsureSessionReusesDiskCache(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	cache := &fakeSessionCache{load: []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "sessao-ok", Path: "/"}}}
	c, err := NewClient(Options{BaseURL: srv.URL, Username: "user-diskcache", Password: "correta", SessionCache: cache})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureSession(context.Background()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if n := stub.loginCalls.Load(); n != 0 {
		t.Errorf("não deveria logar (sessão veio do disco), mas houve %d login(s)", n)
	}
}

// Após um login novo, a sessão é persistida no cache.
func TestEnsureSessionSavesToDiskCache(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	cache := &fakeSessionCache{}
	c, err := NewClient(Options{BaseURL: srv.URL, Username: "user-savecache", Password: "correta", SessionCache: cache})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cache.saved) == 0 {
		t.Error("a sessão deveria ter sido salva no cache após o login")
	}
}

func TestFindUserByLogin(t *testing.T) {
	stub := &fluigStub{
		password:    "correta",
		userFixture: filepath.Join("..", "..", "testdata", "findUserByLogin.json"),
	}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	c := newTestClient(t, srv.URL, "user-find", "correta")
	user, err := c.FindUserByLogin(context.Background(), "admin.deploy")
	if err != nil {
		t.Fatalf("FindUserByLogin: %v", err)
	}
	if user.FullName != "Administrador Deploy" {
		t.Errorf("FullName = %q", user.FullName)
	}
	if user.Email != "admin.deploy@empresa.com.br" {
		t.Errorf("Email = %q", user.Email)
	}
	if user.Code != "uqzevbfj42c1yul51556120291152" {
		t.Errorf("Code = %q", user.Code)
	}
	if user.Raw == nil {
		t.Error("Raw deveria preservar a resposta completa")
	}
}

func TestParseJWTClaims(t *testing.T) {
	payload := func(body string) string {
		return "header." + base64.RawURLEncoding.EncodeToString([]byte(body)) + ".assinatura"
	}

	t.Run("tenant numérico", func(t *testing.T) {
		claims, err := parseJWTClaims(payload(`{"tenant":42,"sub":"admin.deploy"}`))
		if err != nil || claims.Tenant != 42 || claims.Sub != "admin.deploy" {
			t.Errorf("claims=%+v err=%v", claims, err)
		}
	})

	t.Run("tenant string", func(t *testing.T) {
		claims, err := parseJWTClaims(payload(`{"tenant":"7","sub":"x"}`))
		if err != nil || claims.Tenant != 7 {
			t.Errorf("claims=%+v err=%v", claims, err)
		}
	})

	t.Run("malformado", func(t *testing.T) {
		if _, err := parseJWTClaims("sem-pontos"); err == nil {
			t.Error("esperava erro para jwt malformado")
		}
		if _, err := parseJWTClaims("a.###.b"); err == nil {
			t.Error("esperava erro para base64 inválido")
		}
	})
}

// --verbose nunca pode vazar senha em query string.
func TestMaskURL(t *testing.T) {
	u, _ := url.Parse("https://h.test/login?user=x&password=segredo&j_password=outra")
	masked := maskURL(u)
	for _, leak := range []string{"segredo", "outra"} {
		if strings.Contains(masked, leak) {
			t.Errorf("URL mascarada vazou %q: %s", leak, masked)
		}
	}
	if !strings.Contains(masked, "user=x") {
		t.Errorf("parâmetros inofensivos não deveriam ser mascarados: %s", masked)
	}
}

// Uma sessão em cache válida basta por si só — sem senha e sem login (a
// sessão é a credencial universal).
func TestRestoreSessionSemSenha(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	cache := &fakeSessionCache{load: []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "sessao-ok", Path: "/"}}}
	c, err := NewClient(Options{BaseURL: srv.URL, Username: "user-restore", Password: "", SessionCache: cache})
	if err != nil {
		t.Fatal(err)
	}
	ok, rerr := c.RestoreSession(context.Background())
	if !ok || rerr != nil {
		t.Fatalf("sessão em cache válida deveria ser restaurada sem senha (ok=%v err=%v)", ok, rerr)
	}
	if n := stub.loginCalls.Load(); n != 0 {
		t.Errorf("RestoreSession não pode logar, mas houve %d login(s)", n)
	}
}

// Sem sessão aproveitável, RestoreSession devolve false sem tentar login.
func TestRestoreSessionSemCache(t *testing.T) {
	stub := &fluigStub{password: "correta"}
	srv := httptest.NewServer(stub.handler())
	defer srv.Close()

	c, err := NewClient(Options{BaseURL: srv.URL, Username: "user-restore-neg", Password: ""})
	if err != nil {
		t.Fatal(err)
	}
	ok, rerr := c.RestoreSession(context.Background())
	if ok {
		t.Fatal("sem cache não há sessão a restaurar")
	}
	// Sem sessão NÃO é falha de transporte: o erro tem de vir nil para o fluxo
	// seguir para o login com senha (ROADMAP §2.10-H).
	if rerr != nil {
		t.Errorf("ausência de sessão não é erro de transporte: %v", rerr)
	}
	if n := stub.loginCalls.Load(); n != 0 {
		t.Errorf("RestoreSession não pode logar, mas houve %d login(s)", n)
	}
}

// Falha de TRANSPORTE no ping (rede/timeout) não é "sessão inválida": o
// RestoreSession devolve a causa para quem chama abortar, em vez de deixar a CLI
// cair na resolução de senha e morrer com "nenhuma senha disponível"
// (ROADMAP §2.10-H).
func TestRestoreSessionDistingueTransporte(t *testing.T) {
	t.Run("ping lento vira erro de transporte", func(t *testing.T) {
		lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			io.WriteString(w, `{"message":"pong"}`)
		}))
		defer lento.Close()

		cache := &fakeSessionCache{load: []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "s", Path: "/"}}}
		c, err := NewClient(Options{BaseURL: lento.URL, Username: "u", SessionCache: cache, Timeout: 40 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		ok, rerr := c.RestoreSession(context.Background())
		if ok {
			t.Fatal("o ping estourou: não há sessão confirmada")
		}
		if rerr == nil {
			t.Fatal("timeout no ping tem de devolver erro, não silêncio")
		}
		if errors.Is(rerr, ErrAuthFailed) {
			t.Errorf("timeout não é falha de autenticação: %v", rerr)
		}
	})

	t.Run("servidor fora do ar vira erro de transporte", func(t *testing.T) {
		morto := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := morto.URL
		morto.Close() // ninguém escutando na porta

		cache := &fakeSessionCache{load: []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "s", Path: "/"}}}
		c, err := NewClient(Options{BaseURL: url, Username: "u", SessionCache: cache})
		if err != nil {
			t.Fatal(err)
		}
		if ok, rerr := c.RestoreSession(context.Background()); ok || rerr == nil {
			t.Fatalf("servidor fora do ar: ok=%v err=%v", ok, rerr)
		}
	})

	// Sessão que o servidor RECUSA (ping sem pong) mantém o caminho normal:
	// erro nil, para o fluxo seguir para o login com senha.
	t.Run("sessão recusada não é erro de transporte", func(t *testing.T) {
		recusa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer recusa.Close()

		cache := &fakeSessionCache{load: []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "velha", Path: "/"}}}
		c, err := NewClient(Options{BaseURL: recusa.URL, Username: "u", SessionCache: cache})
		if err != nil {
			t.Fatal(err)
		}
		ok, rerr := c.RestoreSession(context.Background())
		if ok {
			t.Fatal("sessão recusada não pode passar por válida")
		}
		if rerr != nil {
			t.Errorf("sessão recusada tem de deixar o fluxo seguir para a senha (err=%v)", rerr)
		}
	})
}

// EnsureSession não gasta um segundo timeout tentando login quando o ping já
// falhou por transporte: devolve a causa real.
func TestEnsureSessionFalhaDeTransporteNaoTentaLogin(t *testing.T) {
	var pings, logins int32
	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "login.do") {
			atomic.AddInt32(&logins, 1)
			return
		}
		atomic.AddInt32(&pings, 1)
		time.Sleep(200 * time.Millisecond)
	}))
	defer lento.Close()

	cache := &fakeSessionCache{load: []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "s", Path: "/"}}}
	c, err := NewClient(Options{BaseURL: lento.URL, Username: "u", Password: "p", SessionCache: cache, Timeout: 40 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureSession(context.Background()); err == nil {
		t.Fatal("deveria falhar")
	} else if errors.Is(err, ErrAuthFailed) {
		t.Errorf("a causa é transporte, não autenticação: %v", err)
	}
	if n := atomic.LoadInt32(&logins); n != 0 {
		t.Errorf("não deveria tentar login depois de falha de transporte, houve %d", n)
	}
}
