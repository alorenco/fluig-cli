package output

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// Table renderiza uma grade com bordas em caracteres de caixa (estilo comum de
// CLIs modernas). É puramente de apresentação — não sabe de JSON nem de cores;
// a coloração é opcional via Style, para não interferir no cálculo de largura.
type Table struct {
	Headers []string
	Rows    [][]string
	// Style, se não-nil, recebe o índice da linha (-1 = cabeçalho), a coluna e o
	// texto da célula já preenchido com espaços, e devolve a versão possivelmente
	// colorida. DEVE preservar a largura visível (só embrulhar em códigos ANSI).
	Style func(row, col int, padded string) string
}

// Render escreve a tabela no writer. Colunas alinhadas à esquerda, uma coluna
// de padding de cada lado do conteúdo.
func (t Table) Render(w io.Writer) {
	cols := len(t.Headers)
	if cols == 0 {
		return
	}
	widths := make([]int, cols)
	for c, h := range t.Headers {
		widths[c] = utf8.RuneCountInString(h)
	}
	for _, row := range t.Rows {
		for c := 0; c < cols && c < len(row); c++ {
			if n := utf8.RuneCountInString(row[c]); n > widths[c] {
				widths[c] = n
			}
		}
	}

	rule := func(left, mid, right string) string {
		var b strings.Builder
		b.WriteString(left)
		for c := 0; c < cols; c++ {
			b.WriteString(strings.Repeat("─", widths[c]+2))
			if c < cols-1 {
				b.WriteString(mid)
			}
		}
		b.WriteString(right)
		return b.String()
	}

	line := func(rowIdx int, cells []string) string {
		var b strings.Builder
		b.WriteString("│")
		for c := 0; c < cols; c++ {
			cell := ""
			if c < len(cells) {
				cell = cells[c]
			}
			padded := " " + cell + strings.Repeat(" ", widths[c]-utf8.RuneCountInString(cell)) + " "
			if t.Style != nil {
				padded = t.Style(rowIdx, c, padded)
			}
			b.WriteString(padded)
			b.WriteString("│")
		}
		return b.String()
	}

	fmt.Fprintln(w, rule("┌", "┬", "┐"))
	fmt.Fprintln(w, line(-1, t.Headers))
	fmt.Fprintln(w, rule("├", "┼", "┤"))
	for i, row := range t.Rows {
		fmt.Fprintln(w, line(i, row))
	}
	fmt.Fprintln(w, rule("└", "┴", "┘"))
}

// --- cores (ANSI) ---

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiCyan  = "\x1b[36m"
	ansiGreen = "\x1b[32m"
)

// ColorEnabled indica se devemos emitir ANSI em stdout: só quando stdout é um
// terminal e NO_COLOR não está definido (convenção https://no-color.org).
func ColorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return StdoutIsTTY()
}

// Bold, Dim, Cyan e Green embrulham s em códigos ANSI (sem checar TTY — quem
// chama decide via ColorEnabled).
func Bold(s string) string  { return ansiBold + s + ansiReset }
func Dim(s string) string   { return ansiDim + s + ansiReset }
func Cyan(s string) string  { return ansiCyan + s + ansiReset }
func Green(s string) string { return ansiGreen + s + ansiReset }
