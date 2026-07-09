package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// Envelope é o documento JSON único emitido em stdout com --json.
type Envelope struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Server  string         `json:"server"`
	Data    any            `json:"data"`
	Error   *EnvelopeError `json:"error"`
}

type EnvelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Printer centraliza toda a escrita da CLI: com JSON=true, stdout recebe
// exatamente um envelope e todo o resto vai para stderr.
type Printer struct {
	JSON    bool
	Command string // ex.: "server test"
	Server  string // nome do servidor alvo ("" se não se aplica)
	Stdout  io.Writer
	Stderr  io.Writer

	emitted bool
}

func NewPrinter(jsonMode bool, command string) *Printer {
	return &Printer{JSON: jsonMode, Command: command, Stdout: os.Stdout, Stderr: os.Stderr}
}

// Successf imprime uma mensagem humana em stdout (ignorada em modo JSON —
// nesse caso o dado vai no envelope via Done).
func (p *Printer) Successf(format string, args ...any) {
	if p.JSON {
		return
	}
	fmt.Fprintf(p.Stdout, format+"\n", args...)
}

// Infof imprime informação auxiliar para humanos (stderr em modo JSON,
// para não poluir o envelope).
func (p *Printer) Infof(format string, args ...any) {
	w := p.Stdout
	if p.JSON {
		w = p.Stderr
	}
	fmt.Fprintf(w, format+"\n", args...)
}

// Warnf imprime aviso em stderr (sempre).
func (p *Printer) Warnf(format string, args ...any) {
	fmt.Fprintf(p.Stderr, "aviso: "+format+"\n", args...)
}

// Table renderiza uma tabela para humanos em stdout. Em modo JSON não imprime
// nada (o dado estruturado vai no envelope via Done).
func (p *Printer) Table(t Table) {
	if p.JSON {
		return
	}
	t.Render(p.Stdout)
}

// Done finaliza a execução com sucesso: em modo JSON emite o envelope com data.
func (p *Printer) Done(data any) {
	if !p.JSON || p.emitted {
		return
	}
	p.emitted = true
	p.writeEnvelope(Envelope{OK: true, Command: p.Command, Server: p.Server, Data: data})
}

// Partial finaliza uma operação em lote com falhas parciais: mantém os
// resultados em data e marca PARTIAL_FAILURE no envelope. O exit code (6) é
// devolvido pelo comando via output.Partialf. Em modo humano não imprime nada
// (as falhas por item já foram reportadas).
func (p *Printer) Partial(data any) {
	if !p.JSON || p.emitted {
		return
	}
	p.emitted = true
	p.writeEnvelope(Envelope{
		OK:      false,
		Command: p.Command,
		Server:  p.Server,
		Data:    data,
		Error:   &EnvelopeError{Code: CodePartial, Message: "alguns itens falharam"},
	})
}

// Fail reporta o erro (envelope em stdout no modo JSON; mensagem pt-BR em
// stderr no modo humano) e retorna o exit code correspondente.
func (p *Printer) Fail(err error) int {
	cliErr := AsError(err)
	if p.JSON {
		if !p.emitted {
			p.emitted = true
			p.writeEnvelope(Envelope{
				OK:      false,
				Command: p.Command,
				Server:  p.Server,
				Error:   &EnvelopeError{Code: cliErr.Code, Message: cliErr.Message},
			})
		}
	} else {
		fmt.Fprintf(p.Stderr, "erro: %s\n", cliErr.Message)
	}
	return cliErr.Exit
}

func (p *Printer) writeEnvelope(env Envelope) {
	enc := json.NewEncoder(p.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		fmt.Fprintf(p.Stderr, "erro: falha ao serializar saída JSON: %v\n", err)
	}
}

// StdoutIsTTY informa se stdout é um terminal interativo; quando não é,
// a CLI se comporta como --non-interactive.
func StdoutIsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// StdinIsTTY informa se stdin é um terminal (necessário para prompts).
func StdinIsTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// StderrIsTTY informa se stderr é um terminal (usado para avisos que só fazem
// sentido para humanos, como o de versão nova).
func StderrIsTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
