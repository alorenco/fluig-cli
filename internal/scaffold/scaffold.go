// Package scaffold gera esqueletos de artefatos Fluig (widgets, datasets,
// formulários, eventos globais, mecanismos e scripts de processo) a partir de
// templates embutidos no binário. Sem cobra nem I/O de terminal — a tradução
// de erros para mensagens/exit codes fica em internal/cli.
//
// Convenções dos templates (templates/<nome>/...):
//   - Arquivos de texto passam por text/template com delimitadores [[ ]]
//     (escolhidos para não conflitar com `${...}` do FreeMarker nem com
//     `{{...}}` de Mustache/Vue, que aparecem literalmente nos templates).
//   - O token __code__ no nome de um arquivo é substituído pelo código do
//     widget (ex.: __code__.properties → meu_widget.properties).
//   - Arquivos binários (.png) são copiados sem passar pelo template.
package scaffold

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

//go:embed all:templates
var templatesFS embed.FS

// Erros sentinela — a CLI os traduz para os exit codes do contrato.
var (
	// ErrInvalidCode indica código de widget fora do padrão aceito.
	ErrInvalidCode = errors.New("código inválido")
	// ErrUnknownTemplate indica template inexistente.
	ErrUnknownTemplate = errors.New("template desconhecido")
	// ErrDirExists indica que a pasta de destino já existe.
	ErrDirExists = errors.New("a pasta de destino já existe")
	// ErrVuetifyTemplate indica a variante Vuetify pedida fora do template vue.
	ErrVuetifyTemplate = errors.New("a variante Vuetify só existe para o template vue")
)

// codeRe valida o código do widget: ele vira context-root, id de DOM, nome de
// arquivo e global JS — minúsculo, começando por letra, só [a-z0-9_].
var codeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// templateLayers define cada template como uma pilha de camadas: os arquivos
// das camadas seguintes sobrepõem os das anteriores (mesmo caminho relativo).
// Camadas com prefixo "_" são núcleos compartilhados, não selecionáveis —
// é o que garante que as variantes de framework (vue, react) não divirjam na
// casca Fluig (application.info, FTLs, WEB-INF...).
var templateLayers = map[string][]string{
	"classic": {"classic"},
	"vue":     {"_spa_core", "vue"},
	"react":   {"_spa_core", "react"},
}

// Options parametriza a geração de um widget.
type Options struct {
	Code          string // código do widget (obrigatório)
	Title         string // título humano; vazio = Code
	Category      string // categoria no application.info; vazio = SYSTEM
	Template      string // nome do template; vazio = classic
	Vuetify       bool   // variante Vuetify 3 (só com Template "vue")
	DeveloperCode string // developer.code do application.info; vazio = DeveloperName
	DeveloperName string // developer.name; vazio = "fluigcli"
}

// widgetData é o contexto passado aos templates.
type widgetData struct {
	Code          string
	CamelCode     string // ex.: meu_widget → MeuWidget (global JS e id de DOM)
	Title         string // título cru (para README/FTL, UTF-8 livre)
	TitleProp     string // título com escape \uXXXX (para .properties/.info)
	Category      string
	Vuetify       bool // liga os blocos [[if .Vuetify]] da camada vue
	DeveloperCode string
	DeveloperName string
}

// Templates lista os templates de widget selecionáveis, em ordem alfabética.
func Templates() []string {
	names := make([]string, 0, len(templateLayers))
	for name := range templateLayers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateCode confere o código do widget contra o padrão aceito.
func ValidateCode(code string) error {
	if !codeRe.MatchString(code) || len(code) > 64 {
		return fmt.Errorf("%w: %q (use minúsculas, dígitos e _, começando por letra; máx. 64)", ErrInvalidCode, code)
	}
	return nil
}

// CreateWidget materializa o template em dir (a pasta do widget, que não pode
// existir) e devolve os caminhos relativos criados, na ordem da árvore.
func CreateWidget(dir string, opt Options) ([]string, error) {
	if err := ValidateCode(opt.Code); err != nil {
		return nil, err
	}
	data := newWidgetData(opt)
	tplName := opt.Template
	if tplName == "" {
		tplName = "classic"
	}
	layers, ok := templateLayers[tplName]
	if !ok {
		return nil, fmt.Errorf("%w: %q (disponíveis: %s)", ErrUnknownTemplate, tplName, strings.Join(Templates(), ", "))
	}
	if opt.Vuetify {
		// Variante do template vue: os blocos [[if .Vuetify]] da camada vue
		// trazem deps/plugin/uso do Vuetify; a camada _vuetify sobrepõe só o
		// App.vue (o kit fluig/ continua vindo da camada vue — sem drift).
		if tplName != "vue" {
			return nil, fmt.Errorf("%w (template %q)", ErrVuetifyTemplate, tplName)
		}
		layers = append(append([]string{}, layers...), "_vuetify")
	}
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrDirExists, dir)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// Resolve as camadas primeiro (a última vence), depois escreve.
	files := map[string][]byte{}
	for _, layer := range layers {
		base := "templates/" + layer
		err := fs.WalkDir(templatesFS, base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel := strings.TrimPrefix(p, base+"/")
			rel = strings.ReplaceAll(rel, "__code__", opt.Code)
			raw, err := templatesFS.ReadFile(p)
			if err != nil {
				return err
			}
			content := raw
			if !isBinary(p) {
				content, err = render(p, raw, data)
				if err != nil {
					return err
				}
			}
			files[rel] = content
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	created := make([]string, 0, len(files))
	for rel := range files {
		created = append(created, rel)
	}
	sort.Strings(created)
	for _, rel := range created {
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			os.RemoveAll(dir) // não deixa árvore pela metade
			return nil, err
		}
		if err := os.WriteFile(dst, files[rel], 0o644); err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
	}
	return created, nil
}

func newWidgetData(opt Options) widgetData {
	title := opt.Title
	if title == "" {
		title = opt.Code
	}
	category := opt.Category
	if category == "" {
		category = "SYSTEM"
	}
	devName := opt.DeveloperName
	if devName == "" {
		devName = "fluigcli"
	}
	devCode := opt.DeveloperCode
	if devCode == "" {
		devCode = devName
	}
	return widgetData{
		Code:          opt.Code,
		CamelCode:     camelCode(opt.Code),
		Title:         title,
		TitleProp:     propEscape(title),
		Category:      category,
		Vuetify:       opt.Vuetify,
		DeveloperCode: devCode,
		DeveloperName: devName,
	}
}

func render(name string, raw []byte, data any) ([]byte, error) {
	t, err := template.New(name).Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func isBinary(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".png")
}

// camelCode converte o código para o nome do global JS: meu_widget → MeuWidget.
func camelCode(code string) string {
	var b strings.Builder
	for _, part := range strings.Split(code, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// propEscape converte runas fora do ASCII imprimível para \uXXXX — o formato
// que o Fluig espera em application.info/.properties (java.util.Properties).
func propEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			fmt.Fprintf(&b, `\u%04X`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
