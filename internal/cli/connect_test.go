package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// O caminho do relato (ROADMAP §2.10-H): existe sessão em cache, o servidor está
// lento e NÃO há senha disponível. Antes o ping estourava, a CLI caía na
// resolução de senha e morria com AUTH_FAILED "nenhuma senha disponível" — a
// senha nunca foi o problema. Agora a causa real (TIMEOUT) chega a quem chamou.
func TestAuthenticateTimeoutNoPingNaoPedeSenha(t *testing.T) {
	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		io.WriteString(w, `{"message":"pong"}`)
	}))
	defer lento.Close()

	u := mustParseHostPort(t, lento.URL)
	server := &config.Server{
		Name: "homolog", Host: u.host, Port: u.port, SSL: false,
		Username: "u", UserCode: "u", CompanyID: 1,
	}

	// Cache de sessão num diretório temporário, com uma sessão para este host.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "") // sem senha em nenhuma fonte
	cache, err := config.NewDiskSessionCache()
	if err != nil {
		t.Fatal(err)
	}
	key, err := fluig.SessionKeyFor(server.BaseURL(), server.Username)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Save(key, []*http.Cookie{{Name: "JSESSIONIDSSO", Value: "sessao", Path: "/"}}); err != nil {
		t.Fatal(err)
	}

	app := &App{
		Timeout:        60 * time.Millisecond,
		NonInteractive: true,
		Keyring:        &memKeyring{m: map[string]string{}},
		printer:        output.NewPrinter(true, "teste"),
	}
	_, err = app.authenticate(context.Background(), server, false)
	if err == nil {
		t.Fatal("deveria falhar: o servidor não responde no tempo limite")
	}
	cliErr := output.AsError(err)
	if cliErr.Code == output.CodeAuthFailed {
		t.Fatalf("timeout no ping não é falha de credencial: %+v", cliErr)
	}
	if cliErr.Code != output.CodeTimeout {
		t.Errorf("code=%s, quer %s (mensagem: %s)", cliErr.Code, output.CodeTimeout, cliErr.Message)
	}
	if errors.Is(err, fluig.ErrAuthFailed) {
		t.Errorf("o erro não deveria ser de autenticação: %v", err)
	}
}
