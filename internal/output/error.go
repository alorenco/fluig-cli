// Package output define o contrato de saída da CLI: envelope JSON em stdout,
// mensagens humanas em pt-BR, erros tipados e exit codes estáveis.
package output

import (
	"errors"
	"fmt"
)

// Exit codes estáveis do contrato para agentes/CI.
const (
	ExitOK            = 0 // sucesso total
	ExitGeneric       = 1 // erro genérico/inesperado
	ExitUsage         = 2 // uso incorreto (argumento faltando em modo não-interativo, flag inválida)
	ExitAuth          = 3 // falha de autenticação/sessão
	ExitNotFound      = 4 // recurso não encontrado (dataset, form, processo, servidor)
	ExitServer        = 5 // erro retornado pelo servidor Fluig
	ExitPartial       = 6 // sucesso parcial em operação em lote
	ExitMissingHelper = 7 // dependência ausente no servidor (fluiggersWidget)
)

// Códigos de erro estáveis, em inglês. Fazem parte do contrato
// JSON — mudanças são breaking change.
const (
	CodeGeneric       = "INTERNAL_ERROR"
	CodeUsage         = "USAGE_ERROR"
	CodeAuthFailed    = "AUTH_FAILED"
	CodeNotFound      = "NOT_FOUND"
	CodeServerError   = "SERVER_ERROR"
	CodePartial       = "PARTIAL_FAILURE"
	CodeMissingHelper = "HELPER_NOT_INSTALLED"
)

// Error é o erro tipado da CLI: carrega o código estável (inglês), a mensagem
// humana (pt-BR) e o exit code correspondente.
type Error struct {
	Code    string
	Message string
	Exit    int
	cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.cause }

// WithCause anexa o erro de origem (para logs verbose), preservando código e mensagem.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

func newError(code string, exit int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), Exit: exit}
}

func Genericf(format string, args ...any) *Error {
	return newError(CodeGeneric, ExitGeneric, format, args...)
}

func Usagef(format string, args ...any) *Error {
	return newError(CodeUsage, ExitUsage, format, args...)
}

func AuthFailedf(format string, args ...any) *Error {
	return newError(CodeAuthFailed, ExitAuth, format, args...)
}

func NotFoundf(format string, args ...any) *Error {
	return newError(CodeNotFound, ExitNotFound, format, args...)
}

func ServerErrorf(format string, args ...any) *Error {
	return newError(CodeServerError, ExitServer, format, args...)
}

func Partialf(format string, args ...any) *Error {
	return newError(CodePartial, ExitPartial, format, args...)
}

func MissingHelperf(format string, args ...any) *Error {
	return newError(CodeMissingHelper, ExitMissingHelper, format, args...)
}

// ExitCodeFor traduz qualquer erro para o exit code do contrato:
// nil → 0, *Error → Exit, demais → 1.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr.Exit
	}
	return ExitGeneric
}

// AsError normaliza qualquer erro para *Error (erros desconhecidos viram INTERNAL_ERROR).
func AsError(err error) *Error {
	var cliErr *Error
	if errors.As(err, &cliErr) {
		return cliErr
	}
	return Genericf("%s", err.Error()).WithCause(err)
}
