package cli

import (
	"errors"
	"net/http"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// sessionOnlyProject cadastra o servidor do stub num projeto SEM nenhuma
// fonte de senha (sem env var, sem keyring).
func sessionOnlyProject(t *testing.T, stubURL string) (proj string, server config.Server) {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	server = config.Server{ID: "sess-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}
	return proj, server
}

// Com uma sessão válida em cache, o comando funciona sem senha nenhuma —
// a sessão é a credencial universal.
func TestComandoAutenticaSoComSessaoEmCache(t *testing.T) {
	stub := healthyServerStub(t, true)
	proj, server := sessionOnlyProject(t, stub.URL)

	cache, err := config.NewDiskSessionCache()
	if err != nil {
		t.Fatal(err)
	}
	key, err := fluig.SessionKeyFor(server.BaseURL(), server.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(key, []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"}}); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	root := newRootCmd(app)
	root.SetArgs([]string{"server", "test", "homolog", "--json", "--project", proj})
	if err := root.Execute(); err != nil {
		t.Fatalf("com sessão em cache válida não deveria precisar de senha: %v", err)
	}
}

// Sem sessão em cache e sem fonte de senha, o erro continua sendo o de
// autenticação (exit 3), com a mensagem que orienta as fontes de senha.
func TestComandoSemSessaoESemSenhaFalha(t *testing.T) {
	stub := healthyServerStub(t, true)
	proj, _ := sessionOnlyProject(t, stub.URL)

	app := &App{}
	root := newRootCmd(app)
	root.SetArgs([]string{"server", "test", "homolog", "--json", "--project", proj})
	err := root.Execute()
	if err == nil {
		t.Fatal("sem sessão e sem senha deveria falhar")
	}
	var cliErr *output.Error
	if !errors.As(err, &cliErr) || cliErr.Exit != output.ExitAuth {
		t.Errorf("exit = %d, quer %d (auth)", output.ExitCodeFor(err), output.ExitAuth)
	}
}
