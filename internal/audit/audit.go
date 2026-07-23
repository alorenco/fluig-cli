package audit

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
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

// Regras (ids estáveis — fazem parte do contrato --json).
const (
	RuleLegacyCSS    = "SG001" // referência ao CSS legado do style guide
	RuleExternalRes  = "SG002" // recurso externo (CDN, fontes) em vez de local
	RuleHardcodedHex = "SG003" // cor fixa (hex/rgb) em vez de variável do tema
	RuleImportant    = "SG004" // !important sobre classe do style guide
	RuleInlineStyle  = "SG005" // style= inline (escapa do tema e do CSS do projeto)
	RuleUnknownClass = "SG006" // classe fs-* inexistente no catálogo
	RuleNativeDialog = "SG007" // alert/confirm/prompt nativos em vez de FLUIGC

	RuleUnknownHAPI    = "FL001" // método inexistente em hAPI.*
	RuleUnknownWKVar   = "FL002" // variável desconhecida em getValue("WK...")
	RuleUnknownFormAPI = "FL003" // método inexistente no FormController (form.*)
	RuleUnknownAPI     = "FL004" // membro inexistente em FLUIGC/DatasetFactory/docAPI/WCMAPI/...

	RuleJavaStrictEq = "RHINO001" // === / !== entre retorno java.lang.String e literal de texto
)

// RuleTitles explica cada regra em uma linha — os hints das UIs (dashboard do
// dev) e de futuros relatórios. Toda regra nova precisa entrar aqui (guarda em
// audit_test.go).
var RuleTitles = map[string]string{
	RuleLegacyCSS:    "Referência ao CSS legado do style guide (fluig-style-guide.min.css, descontinuado no 2.0) — troque pelo flat",
	RuleExternalRes:  "Recurso externo (CDN, fontes remotas) — sirva do projeto ou do servidor",
	RuleHardcodedHex: "Cor fixa (hex/rgb) — use variável do tema para funcionar no claro e no escuro",
	RuleImportant:    "!important sobrescrevendo classe do style guide — quebra o tema",
	RuleInlineStyle:  "Estilo inline (style=) — escapa do tema e do CSS do projeto",
	RuleUnknownClass: "Classe fs-* que não existe no catálogo do style guide (provável typo)",
	RuleNativeDialog: "alert/confirm/prompt nativos — use FLUIGC.message/toast",

	RuleUnknownHAPI:    "Método hAPI.* que não existe na referência de APIs (fluig.d.ts) — provável typo",
	RuleUnknownWKVar:   "Variável WK* desconhecida em getValue() — o Fluig devolve null em silêncio",
	RuleUnknownFormAPI: "Método form.* que não existe no FormController (fluig.d.ts) — provável typo",
	RuleUnknownAPI:     "Membro inexistente em API do Fluig (FLUIGC, DatasetFactory, docAPI, WCMAPI…) — provável typo",

	RuleJavaStrictEq: "=== / !== entre retorno java.lang.String (getFieldName, getString…) e literal de texto — no Rhino do Fluig é sempre false; use == ou String(...)",
}

// Finding é um achado da auditoria. Fix, quando presente, é o texto que o
// `audit --fix` grava no lugar do trecho apontado (correção determinística).
type Finding struct {
	Rule       string   `json:"rule"`
	Severity   Severity `json:"severity"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
	Fix        string   `json:"fix,omitempty"`

	fixOld string // trecho exato substituído pelo --fix (interno)
}

// Result é o resultado de uma execução da auditoria.
type Result struct {
	Findings []Finding
	Scanned  int      // arquivos efetivamente auditados
	Ignored  []string // arquivos pulados (com o motivo)
}

// Config são as exceções e ajustes do projeto (.fluigcli/audit.json). Cada
// entrada de Ignore casa por: caminho relativo exato, prefixo de diretório
// (termina em "/") ou glob sobre o caminho/nome do arquivo. Severity muda o
// nível de uma regra ("error", "warning") ou a desliga ("off").
type Config struct {
	Ignore   []string          `json:"ignore"`
	Severity map[string]string `json:"severity"`
}

// LoadConfig lê e valida o .fluigcli/audit.json do projeto (ausente = vazio).
func LoadConfig(root string) (Config, error) {
	var cfg Config
	raw, err := os.ReadFile(filepath.Join(root, ".fluigcli", "audit.json"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf(".fluigcli/audit.json inválido: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf(".fluigcli/audit.json: %w", err)
	}
	return cfg, nil
}

// Validate confere os overrides de severidade.
func (c Config) Validate() error {
	for rule, sev := range c.Severity {
		switch sev {
		case "error", "warning", "off":
		default:
			return fmt.Errorf("severidade inválida para %s: %q (use error, warning ou off)", rule, sev)
		}
	}
	return nil
}

// defaultTargets são as pastas auditadas: as cobertas pelo style guide
// (forms, widgets) e as de JS server-side, cobertas pelas regras FL* de API.
var defaultTargets = []string{
	"forms", filepath.Join("wcm", "widget"),
	"datasets", "events", "mechanisms", filepath.Join("workflow", "scripts"),
}

// Run audita os alvos (default: forms/, wcm/widget/, datasets/, events/,
// mechanisms/ e workflow/scripts/) sob a raiz do projeto.
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
	res.Findings = applySeverity(res.Findings, cfg)
	sort.Slice(res.Findings, func(i, j int) bool {
		if res.Findings[i].File != res.Findings[j].File {
			return res.Findings[i].File < res.Findings[j].File
		}
		return res.Findings[i].Line < res.Findings[j].Line
	})
	return res, nil
}

// applySeverity aplica os overrides de severidade do projeto ("off" descarta).
func applySeverity(findings []Finding, cfg Config) []Finding {
	if len(cfg.Severity) == 0 {
		return findings
	}
	out := findings[:0]
	for _, f := range findings {
		switch cfg.Severity[f.Rule] {
		case "off":
			continue
		case "error":
			f.Severity = SeverityError
		case "warning":
			f.Severity = SeverityWarning
		}
		out = append(out, f)
	}
	return out
}

// ApplyFixes grava as correções determinísticas (achados com Fix) e devolve
// quantas aplicou. As linhas não mudam de número — só o trecho é trocado.
func ApplyFixes(root string, findings []Finding) (int, error) {
	byFile := map[string][]Finding{}
	for _, f := range findings {
		if f.Fix != "" && f.fixOld != "" {
			byFile[f.File] = append(byFile[f.File], f)
		}
	}
	applied := 0
	for rel, fs := range byFile {
		p := filepath.Join(root, filepath.FromSlash(rel))
		raw, err := os.ReadFile(p)
		if err != nil {
			return applied, err
		}
		lines := strings.Split(string(raw), "\n")
		changed := false
		for _, f := range fs {
			if f.Line < 1 || f.Line > len(lines) {
				continue
			}
			// \b evita corromper tokens maiores (ex.: #fff dentro de #ffffff).
			re, err := regexp.Compile(regexp.QuoteMeta(f.fixOld) + `\b`)
			if err != nil {
				continue
			}
			replaced := re.ReplaceAllString(lines[f.Line-1], f.Fix)
			if replaced != lines[f.Line-1] {
				lines[f.Line-1] = replaced
				changed = true
				applied++
			}
		}
		if changed {
			if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return applied, err
			}
		}
	}
	return applied, nil
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
	isJS := ext == ".js"
	if !isCSS && !isMarkup && !isJS {
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
	switch {
	case isCSS:
		res.Findings = append(res.Findings, scanCSS(rel, content, cat)...)
	case isJS:
		res.Findings = append(res.Findings, scanJS(rel, content)...)
	default:
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
