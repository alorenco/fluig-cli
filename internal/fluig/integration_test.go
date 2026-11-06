//go:build integration

package fluig

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/config"
)

// Testes de integração contra a homologação. Opt-in:
//
//	FLUIGCLI_TEST_HOST=https://fluig-homolog.empresa.com.br \
//	FLUIGCLI_TEST_USERNAME=... FLUIGCLI_TEST_PASSWORD=... \
//	FLUIGCLI_TEST_COMPANY_ID=1 go test -tags=integration ./internal/fluig/
//
// FLUIGCLI_TEST_PASSWORD é opcional: sem ela, os testes usam a sessão em
// cache do fluigcli (a mesma do binário). Sem senha e sem sessão válida, os
// testes são pulados.
func integrationOptions(t *testing.T) Options {
	t.Helper()
	host := os.Getenv("FLUIGCLI_TEST_HOST")
	user := os.Getenv("FLUIGCLI_TEST_USERNAME")
	pass := os.Getenv("FLUIGCLI_TEST_PASSWORD")
	if host == "" || user == "" {
		t.Skip("defina FLUIGCLI_TEST_HOST/USERNAME (e FLUIGCLI_TEST_PASSWORD ou uma sessão em cache) para rodar a integração")
	}
	companyID, _ := strconv.Atoi(os.Getenv("FLUIGCLI_TEST_COMPANY_ID"))
	opts := Options{
		BaseURL:   host,
		Username:  user,
		Password:  pass,
		CompanyID: companyID,
		Timeout:   30 * time.Second,
		Verbose:   testing.Verbose(),
		LogWriter: os.Stderr,
	}
	// Sessão em cache: o mesmo arquivo do binário (~/.cache/fluigcli/
	// sessions.json, chave host|usuário). Com ela, a integração roda sem senha
	// depois de um login feito pelo próprio fluigcli. FLUIGCLI_TEST_NO_SESSION_CACHE=1
	// força o login com senha.
	if os.Getenv("FLUIGCLI_TEST_NO_SESSION_CACHE") == "" {
		if cache, err := config.NewDiskSessionCache(); err == nil {
			opts.SessionCache = cache
		}
	}
	if pass == "" {
		if opts.SessionCache == nil {
			t.Skip("sem FLUIGCLI_TEST_PASSWORD e com o cache de sessão desligado")
		}
		c, err := NewClient(opts)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := c.RestoreSession(context.Background())
		if err != nil {
			t.Fatalf("sessão em cache: %v", err)
		}
		if !ok {
			t.Skipf("sem FLUIGCLI_TEST_PASSWORD e sem sessão em cache válida para %s em %s; autentique com: fluigcli server test", user, host)
		}
	}
	return opts
}

func TestIntegrationEnsureSessionAndUser(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := c.EnsureSession(ctx); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if claims, ok := c.SessionClaims(); ok {
		t.Logf("jwt: tenant=%d sub=%s", claims.Tenant, claims.Sub)
	}

	user, err := c.FindUserByLogin(ctx, opts.Username)
	if err != nil {
		t.Fatalf("FindUserByLogin: %v", err)
	}
	t.Logf("usuário: fullName=%q email=%q code=%q raw=%v", user.FullName, user.Email, user.Code, user.Raw)
	if user.FullName == "" && user.Code == "" {
		t.Errorf("nenhum campo do usuário extraído — ajustar parseUser para o formato real e re-gravar testdata/findUserByLogin.json")
	}
}

func TestIntegrationWrongPassword(t *testing.T) {
	opts := integrationOptions(t)
	opts.Password = "senha-incorreta-fluigcli-test"
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	// Login direto (EnsureSession poderia reutilizar a sessão em cache do teste anterior).
	err = c.Login(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("esperava ErrAuthFailed com senha errada, veio %v", err)
	}
}
