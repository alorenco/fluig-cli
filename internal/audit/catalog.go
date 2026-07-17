// Package audit é o linter de conformidade com o Fluig Style Guide 2.0:
// analisa estaticamente formulários e widgets contra o contrato extraível do
// style guide (classes CSS válidas e variáveis de tema). Sem cobra nem I/O de
// terminal — a tradução para mensagens/exit codes fica em internal/cli.
package audit

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// catalog.json é extraído do fluig-style-guide-flat.min.css REAL da
// homologação (Voyager 2.0.0) — regenerável com `audit --sync` em runtime.
//
//go:embed catalog.json
var embeddedCatalog []byte

// VarValues são os valores de uma variável CSS do tema nos dois modos.
type VarValues struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// Catalog é o contrato do style guide de um servidor: as classes CSS que
// existem e as variáveis de tema com seus valores.
type Catalog struct {
	Source      string               `json:"source"`
	Server      string               `json:"server"`
	ExtractedAt string               `json:"extractedAt"`
	Classes     []string             `json:"classes"`
	Vars        map[string]VarValues `json:"vars"`

	classSet   map[string]struct{}
	valueToVar map[string]string // hex (light, normalizado) → nome da variável
	neutrals   []neutralVar      // variáveis neutras, para vizinho por luminância
	fsClasses  []string          // subconjunto fs-* (sugestão de typo)
}

type neutralVar struct {
	name string
	hex  string
	lum  float64
}

// Embedded carrega o catálogo embutido no binário.
func Embedded() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(embeddedCatalog, &c); err != nil {
		return nil, fmt.Errorf("catálogo embutido inválido: %w", err)
	}
	c.index()
	return &c, nil
}

// FetchFromServer baixa o CSS flat do style guide do servidor (público, sem
// autenticação) e monta o catálogo a partir dele.
func FetchFromServer(ctx context.Context, baseURL string, timeout time.Duration) (*Catalog, error) {
	url := strings.TrimRight(baseURL, "/") + "/style-guide/css/fluig-style-guide-flat.min.css"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	cat := ParseCSS(body, url)
	if len(cat.Classes) < 100 {
		return nil, fmt.Errorf("resposta de %s não parece o CSS do style guide (%d classes)", url, len(cat.Classes))
	}
	return cat, nil
}

var (
	cssClassRe = regexp.MustCompile(`\.(-?[a-zA-Z][a-zA-Z0-9_-]+)`)
	cssBlockRe = regexp.MustCompile(`([^{}]{1,100})\{([^{}]*)\}`)
	cssVarRe   = regexp.MustCompile(`(--fs-[a-z0-9-]+)\s*:\s*([^;]+)`)
)

// ParseCSS extrai o catálogo de um CSS do style guide: classes de todos os
// seletores; variáveis --fs-* do :root (light) e do html.theme-dark (dark).
func ParseCSS(css []byte, source string) *Catalog {
	s := string(css)
	set := map[string]struct{}{}
	for _, m := range cssClassRe.FindAllStringSubmatch(s, -1) {
		set[m[1]] = struct{}{}
	}
	classes := make([]string, 0, len(set))
	for c := range set {
		classes = append(classes, c)
	}
	sort.Strings(classes)

	light := map[string]string{}
	dark := map[string]string{}
	for _, m := range cssBlockRe.FindAllStringSubmatch(s, -1) {
		sel := strings.TrimSpace(m[1])
		var dst map[string]string
		switch sel {
		case ":root":
			dst = light
		case "html.theme-dark":
			dst = dark
		default:
			continue
		}
		for _, v := range cssVarRe.FindAllStringSubmatch(m[2], -1) {
			if _, ok := dst[v[1]]; !ok {
				dst[v[1]] = strings.TrimSpace(v[2])
			}
		}
	}
	vars := map[string]VarValues{}
	for name, lv := range light {
		dv := dark[name]
		if dv == "" {
			dv = lv
		}
		vars[name] = VarValues{Light: lv, Dark: dv}
	}

	cat := &Catalog{Source: source, ExtractedAt: time.Now().Format("2006-01-02"), Classes: classes, Vars: vars}
	cat.index()
	return cat
}

// index monta as estruturas derivadas (conjunto de classes, mapa cor→variável
// e a lista de neutras para a sugestão por proximidade).
func (c *Catalog) index() {
	c.classSet = make(map[string]struct{}, len(c.Classes))
	for _, cl := range c.Classes {
		c.classSet[cl] = struct{}{}
		if strings.HasPrefix(cl, "fs-") {
			c.fsClasses = append(c.fsClasses, cl)
		}
	}
	c.valueToVar = map[string]string{}
	names := make([]string, 0, len(c.Vars))
	for name := range c.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hex, ok := normalizeHex(c.Vars[name].Light)
		if !ok {
			continue // valores var(...), fontes etc. ficam fora do mapa
		}
		// Preferência determinística quando valores coincidem: neutras vencem
		// (são as que o "Check color" oficial sugere), senão a primeira.
		if cur, exists := c.valueToVar[hex]; !exists ||
			(!strings.Contains(cur, "neutral") && strings.Contains(name, "neutral")) {
			c.valueToVar[hex] = name
		}
		if strings.Contains(name, "-color-neutral-") {
			r, g, b, _ := hexRGB(hex)
			c.neutrals = append(c.neutrals, neutralVar{name: name, hex: hex, lum: luminance(r, g, b)})
		}
	}
}

// HasClass informa se a classe existe no catálogo.
func (c *Catalog) HasClass(name string) bool {
	_, ok := c.classSet[name]
	return ok
}

// NearestClass sugere a classe fs-* mais próxima (distância de edição ≤ 2).
func (c *Catalog) NearestClass(name string) string {
	best, bestDist := "", 3
	for _, cl := range c.fsClasses {
		if d := editDistance(name, cl, bestDist); d < bestDist {
			best, bestDist = cl, d
		}
	}
	return best
}

// SuggestColor devolve a sugestão de correção para uma cor fixa (hex ou
// rgb já convertido para hex #rrggbb).
func (c *Catalog) SuggestColor(raw string) string {
	hex, ok := normalizeHex(raw)
	if !ok {
		return ""
	}
	if v, hit := c.valueToVar[hex]; hit {
		return fmt.Sprintf("troque por var(%s) — mesmo valor no modo light, e o dark passa a funcionar", v)
	}
	r, g, b, _ := hexRGB(hex)
	if isGrayish(r, g, b) && len(c.neutrals) > 0 {
		lum := luminance(r, g, b)
		best := c.neutrals[0]
		for _, n := range c.neutrals[1:] {
			if abs(n.lum-lum) < abs(best.lum-lum) {
				best = n
			}
		}
		return fmt.Sprintf("cor neutra — a mais próxima do tema é var(%s) (%s no light)", best.name, best.hex)
	}
	return "use uma variável do tema (--fs-color-brand/action/feedback-*; ver style-guide/css.html)"
}

// --- helpers de cor ---

// normalizeHex aceita #rgb/#rrggbb (qualquer caixa) e devolve #rrggbb minúsculo.
func normalizeHex(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(s, "#") {
		return "", false
	}
	h := s[1:]
	switch len(h) {
	case 3:
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	case 6:
	default:
		return "", false
	}
	if _, err := strconv.ParseUint(h, 16, 32); err != nil {
		return "", false
	}
	return "#" + h, true
}

func hexRGB(hex string) (r, g, b int, ok bool) {
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(n >> 16 & 0xff), int(n >> 8 & 0xff), int(n & 0xff), true
}

func rgbToHex(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func isGrayish(r, g, b int) bool {
	max, min := r, r
	for _, v := range []int{g, b} {
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	return max-min <= 16
}

func luminance(r, g, b int) float64 {
	return 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// editDistance é Levenshtein com poda: para de contar acima de max.
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
