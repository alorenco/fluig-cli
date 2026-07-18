package fluig

import (
	"errors"
	"fmt"
)

// Erros sentinela do cliente Fluig. O pacote é agnóstico de CLI: quem consome
// mapeia estes erros para códigos/exit codes (internal/cli).
var (
	// ErrAuthFailed indica credenciais inválidas ou sessão irrecuperável.
	ErrAuthFailed = errors.New("autenticação no Fluig falhou")
	// ErrNotFound indica recurso inexistente no servidor.
	ErrNotFound = errors.New("recurso não encontrado no Fluig")
	// ErrHelperMissing indica que nenhum componente auxiliar está instalado
	// (nem fluigcliHelper nem fluiggersWidget) — exit 7.
	ErrHelperMissing = errors.New("componente auxiliar não instalado (instale com: fluigcli server install-helper)")
)

// HTTPError representa uma resposta inesperada do servidor Fluig.
type HTTPError struct {
	StatusCode int
	URL        string
	Body       string // truncado; nunca contém credenciais
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("servidor Fluig respondeu HTTP %d em %s", e.StatusCode, e.URL)
}
