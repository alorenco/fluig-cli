package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// O helper responde 403 com uma mensagem pronta em text/plain (>= 0.9.0). A
// CLI tem de repassá-la, e não engolir tudo num "HTTP 403" cru (§2.11-I).
func TestMapFluigError403DoHelperExplica(t *testing.T) {
	err := mapFluigError(&fluig.HTTPError{
		StatusCode: 403,
		URL:        fluig.HelperFluigcli + "/db/query",
		Body:       "Acesso restrito aos administradores do tenant.",
	})

	var cliErr *output.Error
	if !errors.As(err, &cliErr) {
		t.Fatalf("esperava *output.Error, veio %T", err)
	}
	// Exit code é contrato estável: 403 continua sendo erro de servidor (5).
	if cliErr.Exit != output.ExitServer {
		t.Errorf("exit=%d, quer %d", cliErr.Exit, output.ExitServer)
	}
	if cliErr.Code != output.CodeServerError {
		t.Errorf("code=%q, quer %q", cliErr.Code, output.CodeServerError)
	}

	msg := err.Error()
	if !strings.Contains(msg, "administrador do tenant") {
		t.Errorf("a mensagem tem de dizer o que falta: %q", msg)
	}
	if !strings.Contains(msg, "Acesso restrito aos administradores do tenant.") {
		t.Errorf("a mensagem do servidor tem de ser repassada: %q", msg)
	}
	if strings.Contains(msg, "respondeu HTTP 403") {
		t.Errorf("não deve cair na mensagem crua: %q", msg)
	}
}

// 403 de outra rota do Fluig (não do helper) não pode falar em helper.
func TestMapFluigError403DeOutraRota(t *testing.T) {
	msg := mapFluigError(&fluig.HTTPError{
		StatusCode: 403,
		URL:        "/admin/api/v1/users",
		Body:       "Forbidden",
	}).Error()

	if strings.Contains(msg, "fluigcliHelper") {
		t.Errorf("403 de outra rota não pode culpar o helper: %q", msg)
	}
	if !strings.Contains(msg, "/admin/api/v1/users") {
		t.Errorf("a mensagem tem de dizer onde foi recusado: %q", msg)
	}
}

// Corpo HTML (página de erro do container) não vira mensagem para o usuário.
func TestMapFluigError403IgnoraCorpoHTML(t *testing.T) {
	msg := mapFluigError(&fluig.HTTPError{
		StatusCode: 403,
		URL:        fluig.HelperFluigcli + "/ping",
		Body:       "<html><head><title>Error</title></head><body>Forbidden</body></html>",
	}).Error()

	if strings.Contains(msg, "<html>") {
		t.Errorf("HTML não pode vazar para a mensagem: %q", msg)
	}
	if !strings.Contains(msg, "administrador do tenant") {
		t.Errorf("sem corpo aproveitável, a explicação própria tem de sair: %q", msg)
	}
}

// Outros status seguem no caminho antigo, sem regressão.
func TestMapFluigErroOutrosStatusInalterados(t *testing.T) {
	msg := mapFluigError(&fluig.HTTPError{StatusCode: 500, URL: "/x", Body: "boom"}).Error()
	if !strings.Contains(msg, "HTTP 500") {
		t.Errorf("500 deveria manter a mensagem padrão: %q", msg)
	}
}
