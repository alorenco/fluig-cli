//go:build integration

package fluig

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

// Testes de integração contra a homologação. Opt-in:
//
//	FLUIGCLI_TEST_HOST=https://fluig-homolog.empresa.com.br \
//	FLUIGCLI_TEST_USERNAME=... FLUIGCLI_TEST_PASSWORD=... \
//	FLUIGCLI_TEST_COMPANY_ID=1 go test -tags=integration ./internal/fluig/
func integrationOptions(t *testing.T) Options {
	t.Helper()
	host := os.Getenv("FLUIGCLI_TEST_HOST")
	user := os.Getenv("FLUIGCLI_TEST_USERNAME")
	pass := os.Getenv("FLUIGCLI_TEST_PASSWORD")
	if host == "" || user == "" || pass == "" {
		t.Skip("defina FLUIGCLI_TEST_HOST/USERNAME/PASSWORD para rodar a integração")
	}
	companyID, _ := strconv.Atoi(os.Getenv("FLUIGCLI_TEST_COMPANY_ID"))
	return Options{
		BaseURL:   host,
		Username:  user,
		Password:  pass,
		CompanyID: companyID,
		Timeout:   30 * time.Second,
		Verbose:   testing.Verbose(),
		LogWriter: os.Stderr,
	}
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
