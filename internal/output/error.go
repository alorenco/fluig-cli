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
	ExitMissingHelper = 7 // dependência ausente no servidor (fluigcliHelper)
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
	// CodeAuditFailed marca auditoria reprovada por --fail-on: não é um erro
	// de execução (a auditoria rodou), é o veredito — o exit é o genérico 1.
	CodeAuditFailed = "AUDIT_FAILED"
	// CodeTimeout marca requisição que estourou o tempo limite do CLIENTE. O
	// exit é o 5 (como qualquer falha de servidor), mas o código é próprio
	// porque o caso é diferente: o resultado da operação é DESCONHECIDO. Numa
	// operação de escrita, o servidor pode ter concluído depois que a CLI
	// desistiu de esperar. Quem automatiza deve VERIFICAR o estado antes de
	// repetir — repetir às cegas duplica a escrita.
	CodeTimeout = "TIMEOUT"
	// CodePoolTaskNotAssigned e CodeNoHumanTask desambiguam o 404 do
	// `request move`. O exit segue o 4 (a tarefa SUA realmente não existe), mas
	// o código diz o que está segurando: pool sem dono ou atividade automática.
	// Antes os dois saíam como NOT_FOUND "solicitação N não encontrada", com a
	// solicitação aberta e visível — o que manda depurar o lado errado.
	CodePoolTaskNotAssigned = "POOL_TASK_NOT_ASSIGNED"
	CodeNoHumanTask         = "NO_HUMAN_TASK"
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

// AuditFailedf sinaliza auditoria reprovada (--fail-on): exit 1 com código
// próprio no envelope (o data com os achados vai via Printer.FailData).
func AuditFailedf(format string, args ...any) *Error {
	return newError(CodeAuditFailed, ExitGeneric, format, args...)
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

// BlockedTaskf reporta um 404 do move já explicado (pool sem dono ou atividade
// automática): código próprio, exit 4 como antes.
func BlockedTaskf(code, format string, args ...any) *Error {
	return newError(code, ExitNotFound, format, args...)
}

func ServerErrorf(format string, args ...any) *Error {
	return newError(CodeServerError, ExitServer, format, args...)
}

// Timeoutf sinaliza tempo limite do cliente estourado: exit 5 (como as demais
// falhas de servidor), com código próprio para quem automatiza distinguir o
// caso do resultado desconhecido.
func Timeoutf(format string, args ...any) *Error {
	return newError(CodeTimeout, ExitServer, format, args...)
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
