// Package strsim sugere nomes parecidos a partir de uma lista de candidatos.
// Serve para transformar um "não encontrado" num conserto óbvio (ex.: processId
// digitado ou derivado do nome do arquivo que não bate com o do servidor).
//
// A semelhança combina duas pistas: tokens em comum (partes de nome com 4+
// caracteres, separando por não-alfanumérico e por camelCase) e a distância de
// edição. Só tokens não bastam para pegar erros de digitação; só distância não
// pega nomes reorganizados como "SolicitacaoAdiantamento" ↔ "Adiantamento ao
// Fornecedor" (que compartilham o token "adiantamento"). Candidatos sem
// nenhuma das duas pistas são descartados — melhor nenhuma sugestão do que uma
// sugestão ruim.
package strsim

import (
	"sort"
	"strings"
	"unicode"
)

// Suggest devolve até `limit` candidatos parecidos com target, dos mais
// parecidos para os menos. Devolve nil quando nenhum candidato é parecido o
// suficiente.
func Suggest(target string, candidates []string, limit int) []string {
	lt := strings.ToLower(strings.TrimSpace(target))
	if lt == "" || limit <= 0 {
		return nil
	}
	targetTokens := map[string]bool{}
	for _, tk := range tokens(target) {
		targetTokens[tk] = true
	}
	// Tolerância de distância proporcional ao tamanho: erros de digitação em
	// nomes longos passam, mas nomes totalmente diferentes não.
	distMax := len(lt)/3 + 1
	if distMax < 2 {
		distMax = 2
	}

	type scored struct {
		name   string
		shared int
		dist   int
	}
	var hits []scored
	seen := map[string]bool{}
	for _, c := range candidates {
		lc := strings.ToLower(strings.TrimSpace(c))
		if lc == "" || seen[lc] {
			continue
		}
		seen[lc] = true
		shared := 0
		for _, tk := range tokens(c) {
			if targetTokens[tk] {
				shared++
			}
		}
		dist := editDistance(lt, lc, distMax)
		if shared == 0 && dist > distMax {
			continue
		}
		hits = append(hits, scored{name: c, shared: shared, dist: dist})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].shared != hits[j].shared {
			return hits[i].shared > hits[j].shared // mais tokens em comum primeiro
		}
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist // menor distância primeiro
		}
		return hits[i].name < hits[j].name
	})
	if len(hits) == 0 {
		return nil
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.name
	}
	return out
}

// tokens quebra o nome em partes minúsculas de 4+ caracteres, separando por
// caractere não-alfanumérico e nas bordas camelCase (minúscula/dígito → Maiúscula).
func tokens(s string) []string {
	var out []string
	var cur []rune
	var prev rune
	flush := func() {
		if len(cur) >= 4 {
			out = append(out, strings.ToLower(string(cur)))
		}
		cur = cur[:0]
	}
	for _, r := range s {
		alnum := unicode.IsLetter(r) || unicode.IsDigit(r)
		if !alnum {
			flush()
			prev = 0
			continue
		}
		if len(cur) > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			flush()
		}
		cur = append(cur, r)
		prev = r
	}
	flush()
	return out
}

// editDistance é Levenshtein com poda: para de contar acima de max e devolve
// max+1. Opera sobre bytes (basta como heurística de proximidade).
func editDistance(a, b string, max int) int {
	if la, lb := len(a), len(b); la-lb > max || lb-la > max {
		return max + 1
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if cur[j] < rowMin {
				rowMin = cur[j]
			}
		}
		if rowMin > max {
			return max + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
