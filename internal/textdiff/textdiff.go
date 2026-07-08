// Package textdiff produz diffs unificados simples entre dois textos, sem
// dependências externas — suficiente para os artefatos de código do projeto.
package textdiff

import (
	"fmt"
	"strings"
)

// maxDPLines limita o custo O(n·m) do LCS: acima disso, o miolo diferente é
// emitido como um bloco único de remoção+adição (sem alinhamento fino).
const maxDPLines = 3000

// contextLines é o número de linhas de contexto em volta de cada mudança.
const contextLines = 3

// Unified devolve o diff unificado entre a e b, com cabeçalhos
// "--- aName" / "+++ bName". Vazio quando os textos são iguais.
func Unified(aName, bName, a, b string) string {
	if a == b {
		return ""
	}
	ops := diffOps(splitLines(a), splitLines(b))
	body := formatHunks(ops)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("--- %s\n+++ %s\n%s", aName, bName, body)
}

// splitLines separa em linhas sem gerar uma linha fantasma para o \n final.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// op é uma linha do edit script: ' ' contexto, '-' só em a, '+' só em b.
type op struct {
	kind byte
	text string
}

// diffOps monta o edit script: apara prefixo/sufixo comuns e alinha o miolo
// por LCS (ou bloco único, acima de maxDPLines).
func diffOps(a, b []string) []op {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	ma, mb := a[p:len(a)-s], b[p:len(b)-s]

	var mid []op
	if len(ma) > maxDPLines || len(mb) > maxDPLines {
		for _, l := range ma {
			mid = append(mid, op{'-', l})
		}
		for _, l := range mb {
			mid = append(mid, op{'+', l})
		}
	} else {
		mid = lcsOps(ma, mb)
	}

	ops := make([]op, 0, p+len(mid)+s)
	for _, l := range a[:p] {
		ops = append(ops, op{' ', l})
	}
	ops = append(ops, mid...)
	for _, l := range a[len(a)-s:] {
		ops = append(ops, op{' ', l})
	}
	return ops
}

// lcsOps alinha a e b pela subsequência comum mais longa (DP clássico).
func lcsOps(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}
	return ops
}

// formatHunks agrupa as mudanças em hunks @@ com linhas de contexto.
func formatHunks(ops []op) string {
	var changed []int
	for i, o := range ops {
		if o.kind != ' ' {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return ""
	}

	type span struct{ start, end int } // faixa de ops, inclusive
	var hunks []span
	cur := span{max(0, changed[0]-contextLines), min(len(ops)-1, changed[0]+contextLines)}
	for _, ci := range changed[1:] {
		if ci-contextLines <= cur.end+1 {
			if ci+contextLines > cur.end {
				cur.end = min(len(ops)-1, ci+contextLines)
			}
		} else {
			hunks = append(hunks, cur)
			cur = span{max(0, ci-contextLines), min(len(ops)-1, ci+contextLines)}
		}
	}
	hunks = append(hunks, cur)

	var sb strings.Builder
	aLine, bLine := 1, 1
	i := 0
	for _, h := range hunks {
		for ; i < h.start; i++ {
			switch ops[i].kind {
			case ' ':
				aLine++
				bLine++
			case '-':
				aLine++
			case '+':
				bLine++
			}
		}
		aStart, bStart := aLine, bLine
		aCount, bCount := 0, 0
		var body strings.Builder
		for ; i <= h.end; i++ {
			o := ops[i]
			body.WriteByte(o.kind)
			body.WriteString(o.text)
			body.WriteByte('\n')
			switch o.kind {
			case ' ':
				aLine++
				bLine++
				aCount++
				bCount++
			case '-':
				aLine++
				aCount++
			case '+':
				bLine++
				bCount++
			}
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
		sb.WriteString(body.String())
	}
	return sb.String()
}
