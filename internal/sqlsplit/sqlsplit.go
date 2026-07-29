// Package sqlsplit separa um script SQL em instruções.
//
// Existe porque `db query --file` precisa mandar uma instrução por requisição: o
// helper recusa múltiplas instruções na mesma chamada (política read-only do
// ROADMAP §2.11-C). Separar por `;` na força bruta quebra em literal, em
// identificador entre colchetes e em comentário — daí a varredura caractere a
// caractere.
//
// O dialeto de referência é o T-SQL (SQL Server), o banco da plataforma:
//   - literal `'...'`, com `''` como escape;
//   - identificador `[...]` e `"..."`;
//   - comentário de linha `--` e de bloco `/* ... */`, este ANINHÁVEL;
//   - `GO` sozinho numa linha separa lote, como no SSMS/sqlcmd.
package sqlsplit

import "strings"

// Statement é uma instrução do script.
type Statement struct {
	SQL  string // texto da instrução, sem o `;` final e sem espaço nas pontas
	Line int    // linha (1-based) onde a instrução começa, para citar no erro
}

// Split separa o script em instruções, descartando as vazias (só espaço ou só
// comentário). Comentários dentro de uma instrução são preservados: o texto vai
// para o servidor como o usuário escreveu.
func Split(script string) []Statement {
	var (
		out     []Statement
		cur     strings.Builder
		line    = 1
		curLine = 0 // linha do primeiro caractere não vazio da instrução
	)

	flush := func() {
		sql := strings.TrimSpace(cur.String())
		cur.Reset()
		startLine := curLine
		curLine = 0
		if sql == "" || onlyComments(sql) {
			return
		}
		out = append(out, Statement{SQL: TrimLeadingComments(sql), Line: startLine})
	}

	add := func(s string) {
		if curLine == 0 && strings.TrimSpace(s) != "" {
			curLine = line
		}
		cur.WriteString(s)
	}

	src := []rune(script)
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n':
			line++
			add("\n")

		case c == '\'', c == '"':
			// Literal ou identificador entre aspas: copia até fechar, tratando o
			// dobrado ('' / "") como escape.
			quote := c
			add(string(c))
			for i++; i < len(src); i++ {
				if src[i] == '\n' {
					line++
				}
				add(string(src[i]))
				if src[i] == quote {
					if i+1 < len(src) && src[i+1] == quote {
						i++
						add(string(src[i]))
						continue
					}
					break
				}
			}

		case c == '[':
			// Identificador entre colchetes: `]]` é ] escapado.
			add("[")
			for i++; i < len(src); i++ {
				if src[i] == '\n' {
					line++
				}
				add(string(src[i]))
				if src[i] == ']' {
					if i+1 < len(src) && src[i+1] == ']' {
						i++
						add(string(src[i]))
						continue
					}
					break
				}
			}

		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			// Comentário de linha: vai até o fim da linha (o \n fica para a volta
			// seguinte, que incrementa o contador).
			for i < len(src) && src[i] != '\n' {
				cur.WriteRune(src[i])
				i++
			}
			i--

		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			// Comentário de bloco, aninhável no T-SQL.
			depth := 0
			for i < len(src) {
				if src[i] == '\n' {
					line++
				}
				if src[i] == '/' && i+1 < len(src) && src[i+1] == '*' {
					depth++
					cur.WriteString("/*")
					i += 2
					continue
				}
				if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					depth--
					cur.WriteString("*/")
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				cur.WriteRune(src[i])
				i++
			}
			i--

		case c == ';':
			flush()

		default:
			// `GO` sozinho na linha (ignorando espaço e comentário à direita)
			// separa lote. Fora disso é identificador comum.
			if (c == 'g' || c == 'G') && atLineStart(src, i) {
				if end, ok := batchSeparator(src, i); ok {
					flush()
					for ; i < end; i++ {
						if src[i] == '\n' {
							line++
						}
					}
					i--
					continue
				}
			}
			add(string(c))
		}
	}
	flush()
	return out
}

// atLineStart informa se antes de i só há espaço até a quebra de linha.
func atLineStart(src []rune, i int) bool {
	for j := i - 1; j >= 0; j-- {
		switch src[j] {
		case ' ', '\t', '\r':
			continue
		case '\n':
			return true
		default:
			return false
		}
	}
	return true
}

// batchSeparator reconhece `GO` (opcionalmente seguido de espaço/comentário) até
// o fim da linha e devolve o índice onde a linha termina.
func batchSeparator(src []rune, i int) (int, bool) {
	if i+1 >= len(src) || (src[i+1] != 'o' && src[i+1] != 'O') {
		return 0, false
	}
	j := i + 2
	// Nada de sufixo: GOTO, GOAL etc. não são separador.
	if j < len(src) && isWordRune(src[j]) {
		return 0, false
	}
	for ; j < len(src) && src[j] != '\n'; j++ {
		if src[j] == ' ' || src[j] == '\t' || src[j] == '\r' {
			continue
		}
		// Comentário depois do GO é aceitável; qualquer outra coisa não.
		if src[j] == '-' && j+1 < len(src) && src[j+1] == '-' {
			for ; j < len(src) && src[j] != '\n'; j++ {
			}
			break
		}
		return 0, false
	}
	return j, true
}

func isWordRune(c rune) bool {
	return c == '_' || c == '$' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// onlyComments informa se o texto não tem nada além de espaço e comentário —
// nesse caso não existe instrução para enviar.
func onlyComments(s string) bool {
	src := []rune(s)
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			depth := 0
			for i < len(src) {
				if src[i] == '/' && i+1 < len(src) && src[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				i++
			}
			i--
		default:
			return false
		}
	}
	return true
}

// FirstLine devolve a primeira linha de SQL da instrução, sem os comentários,
// para identificar o item numa listagem. Corta em max runas (0 = não corta).
//
// Os comentários são removidos pela mesma varredura do Split, não por prefixo de
// linha: um comentário de bloco de várias linhas tem linhas que não começam com
// `/*`, e a heurística de prefixo escolhia justamente uma delas como "a
// instrução" (visto na homologação em 2026-07-29).
func FirstLine(sql string, max int) string {
	corte := func(s string) string {
		r := []rune(s)
		if max > 0 && len(r) > max {
			return string(r[:max]) + "…"
		}
		return s
	}
	for _, raw := range strings.Split(StripComments(sql), "\n") {
		if l := strings.TrimSpace(raw); l != "" {
			return corte(l)
		}
	}
	// Só comentário: melhor devolver o texto original do que string vazia.
	for _, raw := range strings.Split(sql, "\n") {
		if l := strings.TrimSpace(raw); l != "" {
			return corte(l)
		}
	}
	return ""
}

// TrimLeadingComments remove os comentários (e espaços) que antecedem o primeiro
// token de SQL da instrução. Os comentários internos ficam intactos.
//
// ⚠️ Não é cosmético: a política read-only do fluigcliHelper decide pela PRIMEIRA
// palavra do texto recebido, sem pular comentário. Medido na homologação em
// 2026-07-29 — um `-- nota` acima de um `select` fazia o servidor responder
// "Somente consultas de leitura são permitidas (SELECT ou WITH)". Como o script
// do usuário é comentado de ponta a ponta, sem isto o `--file` recusaria quase
// tudo.
func TrimLeadingComments(sql string) string {
	src := []rune(sql)
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			depth := 0
			for i < len(src) {
				if src[i] == '/' && i+1 < len(src) && src[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				i++
			}
		default:
			return strings.TrimSpace(string(src[i:]))
		}
	}
	return ""
}

// StripComments remove comentários de linha e de bloco, preservando literais e
// identificadores entre colchetes/aspas.
func StripComments(sql string) string {
	var out strings.Builder
	src := []rune(sql)
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\'', c == '"':
			quote := c
			out.WriteRune(c)
			for i++; i < len(src); i++ {
				out.WriteRune(src[i])
				if src[i] == quote {
					if i+1 < len(src) && src[i+1] == quote {
						i++
						out.WriteRune(src[i])
						continue
					}
					break
				}
			}

		case c == '[':
			out.WriteRune(c)
			for i++; i < len(src); i++ {
				out.WriteRune(src[i])
				if src[i] == ']' {
					if i+1 < len(src) && src[i+1] == ']' {
						i++
						out.WriteRune(src[i])
						continue
					}
					break
				}
			}

		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			i-- // o \n volta no próximo passo, preservando as linhas

		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			depth := 0
			for i < len(src) {
				if src[i] == '/' && i+1 < len(src) && src[i+1] == '*' {
					depth++
					i += 2
					continue
				}
				if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
					continue
				}
				// Preserva a quebra de linha para não juntar duas instruções na
				// mesma linha lógica.
				if src[i] == '\n' {
					out.WriteRune('\n')
				}
				i++
			}
			i--

		default:
			out.WriteRune(c)
		}
	}
	return out.String()
}
