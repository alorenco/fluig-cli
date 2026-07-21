package audit

import (
	"regexp"
	"strings"
	"sync"

	skillassets "github.com/alorenco/fluig-cli/skills"
)

// Catálogo das APIs de script do Fluig, extraído mecanicamente do
// reference/fluig.d.ts embutido na skill (fork do fluig-declaration-type da
// comunidade Fluiggers, MIT) — a mesma fonte que a skill manda o agente
// consultar. Alimenta as regras FL* (métodos/variáveis desconhecidos).
//
// O d.ts é reconhecidamente INCOMPLETO (o upstream o evolui sob demanda);
// por isso as regras FL* nascem como AVISO, e uma API real que faltar no
// arquivo deve ser adicionada ao fork (skills/fluigcli/reference/fluig.d.ts).

// APICatalog indexa os membros válidos por objeto global (hAPI, FLUIGC,
// FLUIGC.message, form, ...) e as variáveis aceitas pelo getValue global.
type APICatalog struct {
	members map[string]map[string]struct{}
	wkVars  map[string]struct{}
}

// apiObjectDecls são as declarações do d.ts que viram objetos do catálogo.
// FormController é a classe do objeto global `form` dos eventos de formulário;
// customHTML idem (o d.ts a declara como classe com membros estáticos).
var apiObjectDecls = map[string]string{
	"hAPI":           "hAPI",
	"FLUIGC":         "FLUIGC",
	"DatasetFactory": "DatasetFactory",
	"DatasetBuilder": "DatasetBuilder",
	"docAPI":         "docAPI",
	"WCMAPI":         "WCMAPI",
	"fluigAPI":       "fluigAPI",
	"FormController": "form",
	"customHTML":     "customHTML",
}

var (
	apiCatalogOnce sync.Once
	apiCatalogVal  *APICatalog
)

// apiCatalog devolve o catálogo de APIs (parse único por execução). Se o
// embed estiver corrompido, devolve um catálogo vazio — as regras FL* viram
// no-op em vez de derrubar a auditoria (o guarda de teste pega a corrupção).
func apiCatalog() *APICatalog {
	apiCatalogOnce.Do(func() {
		data, err := skillassets.FS.ReadFile("fluigcli/reference/fluig.d.ts")
		if err != nil {
			apiCatalogVal = &APICatalog{members: map[string]map[string]struct{}{}, wkVars: map[string]struct{}{}}
			return
		}
		apiCatalogVal = parseAPICatalog(string(data))
	})
	return apiCatalogVal
}

var (
	dtsDeclRe = regexp.MustCompile(`(?m)^\s*declare (namespace|class) ([\w.]+)\s*\{`)
	// O upstream mistura `declare const x` e `const x` (sem declare) dentro dos
	// namespaces — os dois contam como membro (ex.: WCMAPI.user).
	dtsMemberRe = regexp.MustCompile(`(?m)^\s*(?:declare\s+)?(?:function|const|var)\s+(\w+)`)
	dtsMethodRe = regexp.MustCompile(`(?m)^\s*(\w+)\s*\(`)
	dtsWKTypeRe = regexp.MustCompile(`type getValueProperties\w+\s*=([^;]+);`)
	dtsWKNameRe = regexp.MustCompile(`"(\w+)"`)
)

// parseAPICatalog extrai o catálogo do fonte do fluig.d.ts.
func parseAPICatalog(src string) *APICatalog {
	src = stripDTSComments(src)
	cat := &APICatalog{
		members: map[string]map[string]struct{}{},
		wkVars:  map[string]struct{}{},
	}
	add := func(obj, member string) {
		if cat.members[obj] == nil {
			cat.members[obj] = map[string]struct{}{}
		}
		cat.members[obj][member] = struct{}{}
	}

	for _, m := range dtsDeclRe.FindAllStringSubmatchIndex(src, -1) {
		kind := src[m[2]:m[3]]
		name := src[m[4]:m[5]]
		// Só as declarações de interesse: o nome exato ou um sub-namespace
		// dele (FLUIGC.message). Os pacotes Java (com.fluig.sdk.*) ficam fora.
		rootName := name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			rootName = name[:i]
		}
		obj, wanted := apiObjectDecls[rootName]
		if !wanted {
			continue
		}
		body := dtsBlock(src, m[1]-1) // m[1]-1 = índice do '{'
		key := obj
		if rootName != name { // sub-namespace: FLUIGC.message → chave própria
			key = obj + name[len(rootName):]
			add(obj, strings.SplitN(name[len(rootName)+1:], ".", 2)[0])
		}
		if kind == "namespace" {
			for _, mm := range dtsMemberRe.FindAllStringSubmatch(body, -1) {
				add(key, mm[1])
			}
		} else {
			for _, mm := range dtsMethodRe.FindAllStringSubmatch(body, -1) {
				if mm[1] == "constructor" {
					continue
				}
				add(key, mm[1])
			}
			// Classes também expõem consts (ex.: WCMAPI-like); inofensivo se vazio.
			for _, mm := range dtsMemberRe.FindAllStringSubmatch(body, -1) {
				add(key, mm[1])
			}
		}
	}

	for _, m := range dtsWKTypeRe.FindAllStringSubmatch(src, -1) {
		for _, n := range dtsWKNameRe.FindAllStringSubmatch(m[1], -1) {
			cat.wkVars[n[1]] = struct{}{}
		}
	}
	return cat
}

// dtsBlock devolve o corpo entre a chave de abertura em src[open] e o seu
// fechamento (profundidade balanceada). Comentários já foram removidos.
func dtsBlock(src string, open int) string {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : i]
			}
		}
	}
	return src[open+1:]
}

// stripDTSComments apaga os comentários /* */ e // (os exemplos de código nos
// JSDoc têm chaves e quebrariam o balanceamento), preservando as quebras de
// linha para o casamento por início de linha dos métodos.
func stripDTSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			for i += 2; i < len(src); i++ {
				if src[i] == '\n' {
					b.WriteByte('\n')
				} else if src[i] == '*' && i+1 < len(src) && src[i+1] == '/' {
					i++
					break
				}
			}
			continue
		}
		if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			for ; i < len(src) && src[i] != '\n'; i++ {
			}
			b.WriteByte('\n')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// HasMember informa se o objeto existe no catálogo e tem o membro.
func (c *APICatalog) HasMember(obj, member string) bool {
	set, ok := c.members[obj]
	if !ok {
		return false
	}
	_, hit := set[member]
	return hit
}

// KnownObject informa se o objeto (ou sub-namespace) está indexado.
func (c *APICatalog) KnownObject(obj string) bool {
	return len(c.members[obj]) > 0
}

// NearestMember sugere o membro mais próximo (distância de edição ≤ 2).
func (c *APICatalog) NearestMember(obj, member string) string {
	best, bestDist := "", 3
	for m := range c.members[obj] {
		if d := editDistance(member, m, bestDist); d < bestDist {
			best, bestDist = m, d
		}
	}
	return best
}

// HasWKVar informa se a variável é aceita pelo getValue global.
func (c *APICatalog) HasWKVar(name string) bool {
	_, ok := c.wkVars[name]
	return ok
}

// NearestWKVar sugere a variável WK* mais próxima (distância ≤ 2).
func (c *APICatalog) NearestWKVar(name string) string {
	best, bestDist := "", 3
	for v := range c.wkVars {
		if d := editDistance(name, v, bestDist); d < bestDist {
			best, bestDist = v, d
		}
	}
	return best
}
