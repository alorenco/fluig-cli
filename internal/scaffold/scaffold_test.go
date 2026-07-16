package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/project"
)

func TestTemplatesInclusiClassic(t *testing.T) {
	names := Templates()
	found := false
	for _, n := range names {
		if n == "classic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("template classic ausente: %v", names)
	}
}

func TestValidateCode(t *testing.T) {
	valid := []string{"a", "meu_widget", "painel2", "a1_b2"}
	for _, c := range valid {
		if err := ValidateCode(c); err != nil {
			t.Errorf("code %q deveria ser válido: %v", c, err)
		}
	}
	invalid := []string{"", "Meu", "1abc", "com-hifen", "com espaço", "acentuadá", "_priv", strings.Repeat("a", 65)}
	for _, c := range invalid {
		if err := ValidateCode(c); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("code %q deveria ser inválido, err=%v", c, err)
		}
	}
}

func TestCamelCode(t *testing.T) {
	cases := map[string]string{
		"meu_widget":              "MeuWidget",
		"alertas_administrativos": "AlertasAdministrativos",
		"painel":                  "Painel",
		"a1_b2":                   "A1B2",
	}
	for in, want := range cases {
		if got := camelCode(in); got != want {
			t.Errorf("camelCode(%q) = %q, quero %q", in, got, want)
		}
	}
}

func TestPropEscape(t *testing.T) {
	// Mesmo formato dos samples oficiais: não-ASCII vira \uXXXX (o deployer
	// lê os properties no formato java.util.Properties, não UTF-8).
	if got := propEscape("Gráfico usuários"); got != `Gr\u00E1fico usu\u00E1rios` {
		t.Errorf("propEscape = %q", got)
	}
	if got := propEscape("ascii only"); got != "ascii only" {
		t.Errorf("ascii não deveria mudar: %q", got)
	}
}

// O template classic gera a árvore completa, com os placeholders resolvidos e
// o application.info exatamente no formato esperado pelo deployer.
func TestCreateWidgetClassic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "painel_teste")
	files, err := CreateWidget(dir, Options{
		Code:          "painel_teste",
		Title:         "Painel de Testes Automáticos",
		DeveloperName: "tester",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"README.md",
		"src/main/resources/application.info",
		"src/main/resources/edit.ftl",
		"src/main/resources/painel_teste.properties",
		"src/main/resources/painel_teste_en_US.properties",
		"src/main/resources/painel_teste_es.properties",
		"src/main/resources/painel_teste_pt_BR.properties",
		"src/main/resources/view.ftl",
		"src/main/webapp/WEB-INF/jboss-web.xml",
		"src/main/webapp/WEB-INF/web.xml",
		"src/main/webapp/resources/css/painel_teste.css",
		"src/main/webapp/resources/images/icon.png",
		"src/main/webapp/resources/js/painel_teste.js",
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("arquivo esperado não gerado: %s (gerados: %v)", w, files)
		}
	}
	if len(files) != len(want) {
		t.Errorf("gerados %d arquivos, esperava %d: %v", len(files), len(want), files)
	}

	info, err := os.ReadFile(filepath.Join(dir, "src/main/resources/application.info"))
	if err != nil {
		t.Fatal(err)
	}
	wantInfo := `application.type=widget
application.code=painel_teste
application.title=Painel de Testes Autom\u00E1ticos
application.description=Painel de Testes Autom\u00E1ticos
application.category=SYSTEM
application.renderer=freemarker
developer.code=tester
developer.name=tester
developer.url=http://www.fluig.com
application.uiwidget=true
application.mobileapp=false
application.version=1.0.0
view.file=view.ftl
edit.file=edit.ftl
locale.file.base.name=painel_teste
application.resource.js.1=/resources/js/painel_teste.js
application.resource.css.2=/resources/css/painel_teste.css
`
	if string(info) != wantInfo {
		t.Errorf("application.info difere.\n--- got ---\n%s\n--- want ---\n%s", info, wantInfo)
	}

	view, err := os.ReadFile(filepath.Join(dir, "src/main/resources/view.ftl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		`id="PainelTeste_${instanceId}"`,
		`data-params="PainelTeste.instance()"`,
		"super-widget wcm-widget-class fluig-style-guide",
		"${i18n.getTranslation('application.title')}",
	} {
		if !strings.Contains(string(view), frag) {
			t.Errorf("view.ftl sem o fragmento %q", frag)
		}
	}

	js, err := os.ReadFile(filepath.Join(dir, "src/main/webapp/resources/js/painel_teste.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "var PainelTeste = SuperWidget.extend({") {
		t.Errorf("js sem o global SuperWidget: %s", js)
	}

	// O PNG deve ser copiado intacto (binário, sem passar pelo template).
	icon, err := os.ReadFile(filepath.Join(dir, "src/main/webapp/resources/images/icon.png"))
	if err != nil {
		t.Fatal(err)
	}
	orig, err := templatesFS.ReadFile("templates/classic/src/main/webapp/resources/images/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if string(icon) != string(orig) {
		t.Errorf("icon.png foi alterado no scaffold")
	}
}

// A widget gerada precisa ser empacotável pelo widget export: as três
// subárvores src/main entram no WAR e o README fica de fora.
func TestCreateWidgetEmpacotavel(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "meu_widget")
	if _, err := CreateWidget(dir, Options{Code: "meu_widget"}); err != nil {
		t.Fatal(err)
	}
	refs, err := project.CollectWidgetWARFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	inWAR := map[string]bool{}
	for _, r := range refs {
		inWAR[r.WARPath] = true
	}
	for _, w := range []string{
		"WEB-INF/classes/application.info",
		"WEB-INF/classes/view.ftl",
		"WEB-INF/jboss-web.xml",
		"resources/js/meu_widget.js",
		"resources/images/icon.png",
	} {
		if !inWAR[w] {
			t.Errorf("entrada esperada no WAR ausente: %s (WAR: %v)", w, inWAR)
		}
	}
	for p := range inWAR {
		if strings.Contains(p, "README") {
			t.Errorf("README não deveria entrar no WAR: %s", p)
		}
	}
}

func TestCreateWidgetErros(t *testing.T) {
	base := t.TempDir()

	if _, err := CreateWidget(filepath.Join(base, "X"), Options{Code: "Maiusculo"}); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("code inválido: err=%v", err)
	}

	if _, err := CreateWidget(filepath.Join(base, "w"), Options{Code: "w", Template: "naoexiste"}); !errors.Is(err, ErrUnknownTemplate) {
		t.Errorf("template inexistente: err=%v", err)
	}

	dir := filepath.Join(base, "duplicado")
	if _, err := CreateWidget(dir, Options{Code: "duplicado"}); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateWidget(dir, Options{Code: "duplicado"}); !errors.Is(err, ErrDirExists) {
		t.Errorf("pasta existente: err=%v", err)
	}
}

// O template vue compõe o núcleo _spa_core com a camada do framework; o
// toolchain fica fora de src/main (e portanto fora do WAR).
func TestCreateWidgetVue(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "painel_vue")
	files, err := CreateWidget(dir, Options{Code: "painel_vue", Template: "vue", DeveloperName: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, w := range []string{
		// camada vue (toolchain + SPA)
		"README.md", "package.json", "vite.config.ts", "tsconfig.json", "index.html",
		".nvmrc", ".gitignore",
		"src/vue/main.ts", "src/vue/App.vue", "src/vue/vite-env.d.ts",
		"src/vue/fluig/dataset.ts", "src/vue/fluig/fluigc.ts", "src/vue/fluig/i18n.ts",
		"src/vue/composables/useDataset.ts",
		// núcleo _spa_core (casca Fluig)
		"src/main/resources/application.info",
		"src/main/resources/view.ftl",
		"src/main/resources/edit.ftl",
		"src/main/resources/painel_vue.properties",
		"src/main/webapp/WEB-INF/jboss-web.xml",
		"src/main/webapp/resources/images/icon.png",
	} {
		if !got[w] {
			t.Errorf("arquivo esperado não gerado: %s", w)
		}
	}

	// Placeholders resolvidos nos pontos críticos da ponte.
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if s := read("vite.config.ts"); !strings.Contains(s, "const widgetCode = 'painel_vue'") {
		t.Errorf("vite.config.ts sem o código: %s", s)
	}
	if s := read("src/vue/main.ts"); !strings.Contains(s, "win.PainelVue = win.SuperWidget.extend({") ||
		!strings.Contains(s, "`painel_vue-root-${instanceId}`") {
		t.Errorf("main.ts sem a ponte parametrizada")
	}
	if s := read("src/main/resources/view.ftl"); !strings.Contains(s, `id="painel_vue-root-${instanceId}"`) ||
		!strings.Contains(s, "widgetSettings") {
		t.Errorf("view.ftl sem o div de montagem/configs: %s", s)
	}
	if s := read("src/main/resources/edit.ftl"); !strings.Contains(s, "data-save-settings") {
		t.Errorf("edit.ftl sem o botão de salvar")
	}

	// O WAR não pode conter o toolchain.
	refs, err := project.CollectWidgetWARFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		if strings.Contains(r.WARPath, "package.json") || strings.Contains(r.WARPath, "vite") ||
			strings.Contains(r.WARPath, "node_modules") || strings.Contains(r.WARPath, "src/vue") {
			t.Errorf("toolchain vazou para o WAR: %s", r.WARPath)
		}
	}
}

// Núcleos (prefixo _) não são selecionáveis como template.
func TestSpaCoreNaoSelecionavel(t *testing.T) {
	for _, n := range Templates() {
		if strings.HasPrefix(n, "_") {
			t.Errorf("núcleo %q não deveria ser selecionável", n)
		}
	}
	if _, err := CreateWidget(filepath.Join(t.TempDir(), "w"), Options{Code: "w", Template: "_spa_core"}); !errors.Is(err, ErrUnknownTemplate) {
		t.Errorf("_spa_core deveria ser template desconhecido, err=%v", err)
	}
}

// Guarda anti-drift da B2: os helpers puros do kit (sem framework) precisam
// ser byte a byte idênticos entre as variantes vue e react — uma correção em
// um sem o outro é drift.
func TestKitSemDriftEntreVariantes(t *testing.T) {
	for _, f := range []string{"dataset.ts", "fluigc.ts", "i18n.ts"} {
		vue, err := templatesFS.ReadFile("templates/vue/src/vue/fluig/" + f)
		if err != nil {
			t.Fatal(err)
		}
		react, err := templatesFS.ReadFile("templates/react/src/react/fluig/" + f)
		if err != nil {
			t.Fatal(err)
		}
		if string(vue) != string(react) {
			t.Errorf("fluig/%s divergiu entre vue e react — sincronize as duas cópias", f)
		}
	}
}

// O template react compõe o mesmo núcleo _spa_core da vue.
func TestCreateWidgetReact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "painel_react")
	files, err := CreateWidget(dir, Options{Code: "painel_react", Template: "react", DeveloperName: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, w := range []string{
		"README.md", "package.json", "vite.config.ts", "tsconfig.json", "index.html",
		"src/react/main.tsx", "src/react/App.tsx", "src/react/app.css",
		"src/react/hooks/useDataset.ts", "src/react/fluig/dataset.ts",
		"src/main/resources/application.info",
		"src/main/resources/view.ftl",
		"src/main/webapp/resources/css/painel_react.css",
	} {
		if !got[w] {
			t.Errorf("arquivo esperado não gerado: %s", w)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "src/react/main.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "win.PainelReact = win.SuperWidget.extend({") ||
		!strings.Contains(string(b), "`painel_react-root-${instanceId}`") {
		t.Errorf("main.tsx sem a ponte parametrizada")
	}
}
