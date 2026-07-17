package audit

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Severity é o nível de um achado.
type Severity string

// Severidades dos achados.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Regras da fase 1 (ids estáveis — fazem parte do contrato --json).
const (
	RuleLegacyCSS    = "SG001" // referência ao CSS legado do style guide
	RuleExternalRes  = "SG002" // recurso externo (CDN, fontes) em vez de local
	RuleHardcodedHex = "SG003" // cor fixa (hex/rgb) em vez de variável do tema
	RuleUnknownClass = "SG006" // classe fs-* inexistente no catálogo
)

// Finding é um achado da auditoria.
type Finding struct {
	Rule       string   `json:"rule"`
	Severity   Severity `json:"severity"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
}

// Result é o resultado de uma execução da auditoria.
type Result struct {
	Findings []Finding
	Scanned  int      // arquivos efetivamente auditados
	Ignored  []string // arquivos pulados (com o motivo)
}

// Config são as exceções do projeto (.fluigcli/audit.json). Cada entrada de
// Ignore casa por: caminho relativo exato, prefixo de diretório (termina em
// "/") ou glob sobre o caminho/nome do arquivo.
type Config struct {
	Ignore []string `json:"ignore"`
}

// defaultTargets são as pastas cobertas pelo style guide.
var defaultTargets = []string{"forms", filepath.Join("wcm", "widget")}

// Run audita os alvos (default: forms/ e wcm/widget/) sob a raiz do projeto.
// Alvos explícitos podem ser arquivos ou pastas, relativos à raiz ou ao cwd.
func Run(root string, targets []string, cat *Catalog, cfg Config) (*Result, error) {
	roots := targets
	if len(roots) == 0 {
		roots = defaultTargets
	}
	res := &Result{}
	spaCache := map[string]bool{}
	for _, t := range roots {
		base := t
		if !filepath.IsAbs(base) {
			if _, err := os.Stat(base); err != nil {
				base = filepath.Join(root, t)
			}
		}
		info, err := os.Stat(base)
		if os.IsNotExist(err) {
			if len(targets) == 0 {
				continue // pasta convencional ausente não é erro
			}
			return nil, err
		} else if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			auditFile(root, base, cat, cfg, spaCache, res)
			continue
		}
		walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			auditFile(root, p, cat, cfg, spaCache, res)
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	sort.Slice(res.Findings, func(i, j int) bool {
		if res.Findings[i].File != res.Findings[j].File {
			return res.Findings[i].File < res.Findings[j].File
		}
		return res.Findings[i].Line < res.Findings[j].Line
	})
	return res, nil
}

// auditFile decide se o arquivo entra na auditoria e roda as regras.
func auditFile(root, p string, cat *Catalog, cfg Config, spaCache map[string]bool, res *Result) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		rel = p
	}
	rel = filepath.ToSlash(rel)

	ext := strings.ToLower(filepath.Ext(p))
	isCSS := ext == ".css"
	isMarkup := ext == ".html" || ext == ".htm" || ext == ".ftl"
	if !isCSS && !isMarkup {
		return
	}
	if ignored(cfg, rel) {
		res.Ignored = append(res.Ignored, rel+" — .fluigcli/audit.json")
		return
	}
	if strings.Contains(strings.ToLower(filepath.Base(p)), ".min.") {
		res.Ignored = append(res.Ignored, rel+" — minificado/vendorado")
		return
	}
	if isCSS && isSPABundle(root, rel, spaCache) {
		res.Ignored = append(res.Ignored, rel+" — bundle gerado pelo build (widget SPA)")
		return
	}
	content, err := os.ReadFile(p)
	if err != nil {
		return
	}
	if looksMinified(content) {
		res.Ignored = append(res.Ignored, rel+" — minificado/vendorado")
		return
	}
	res.Scanned++
	if isCSS {
		res.Findings = append(res.Findings, scanCSS(rel, content, cat)...)
	} else {
		res.Findings = append(res.Findings, scanMarkup(rel, content, cat)...)
	}
}

// ignored aplica as exceções do .fluigcli/audit.json.
func ignored(cfg Config, rel string) bool {
	base := path.Base(rel)
	for _, entry := range cfg.Ignore {
		entry = filepath.ToSlash(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if entry == rel {
			return true
		}
		if strings.HasSuffix(entry, "/") && strings.HasPrefix(rel, entry) {
			return true
		}
		if ok, _ := path.Match(entry, rel); ok {
			return true
		}
		if ok, _ := path.Match(entry, base); ok {
			return true
		}
	}
	return false
}

// isSPABundle detecta a saída de build de widget SPA (vue/react/vuetify):
// css/js sob resources/ de uma widget que tem package.json na raiz — o
// arquivo é gerado, quem manda é a fonte da SPA.
func isSPABundle(root, rel string, cache map[string]bool) bool {
	parts := strings.Split(rel, "/")
	// wcm/widget/<code>/src/main/webapp/resources/...
	if len(parts) < 7 || parts[0] != "wcm" || parts[1] != "widget" {
		return false
	}
	if parts[3] != "src" || parts[4] != "main" || parts[5] != "webapp" || parts[6] != "resources" {
		return false
	}
	widgetDir := filepath.Join(root, parts[0], parts[1], parts[2])
	spa, seen := cache[widgetDir]
	if !seen {
		_, err := os.Stat(filepath.Join(widgetDir, "package.json"))
		spa = err == nil
		cache[widgetDir] = spa
	}
	return spa
}

// looksMinified detecta CSS/JS vendorado minificado (linhas gigantes).
func looksMinified(content []byte) bool {
	if len(content) < 20000 {
		return false
	}
	lines := 1
	for _, b := range content {
		if b == '\n' {
			lines++
		}
	}
	return len(content)/lines > 1000
}
