package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedCatalog(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Classes) < 2000 {
		t.Errorf("catálogo com poucas classes: %d", len(cat.Classes))
	}
	if !cat.HasClass("fs-bg-white") || !cat.HasClass("form-control") {
		t.Error("classes conhecidas ausentes do catálogo")
	}
	v, ok := cat.Vars["--fs-color-neutral-light-00"]
	if !ok || v.Light != "#ffffff" || v.Dark == v.Light {
		t.Errorf("variável neutra base: %+v ok=%v", v, ok)
	}
}

func TestSuggestColor(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	// Valor exato do tema → variável exata (e neutra vence empate).
	if s := cat.SuggestColor("#ffffff"); !strings.Contains(s, "--fs-color-neutral-light-00") {
		t.Errorf("#ffffff: %q", s)
	}
	if s := cat.SuggestColor("#FFF"); !strings.Contains(s, "--fs-color-neutral-light-00") {
		t.Errorf("#FFF (3 dígitos/caixa alta): %q", s)
	}
	// Cinza que não bate exato → vizinho neutro por luminância.
	if s := cat.SuggestColor("#fdfdfd"); !strings.Contains(s, "neutral") {
		t.Errorf("cinza próximo: %q", s)
	}
	// Cor saturada sem match → orientação genérica de família.
	if s := cat.SuggestColor("#ff00aa"); !strings.Contains(s, "variável do tema") {
		t.Errorf("cor fora do tema: %q", s)
	}
}

func TestNearestClass(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if got := cat.NearestClass("fs-bg-whit"); got != "fs-bg-white" {
		t.Errorf("typo próximo: %q", got)
	}
	if got := cat.NearestClass("fs-xyzzy-completamente-diferente"); got != "" {
		t.Errorf("sem vizinho: %q", got)
	}
}

func TestParseCSSSync(t *testing.T) {
	css := []byte(`:root{--fs-color-brand-01-base:#0079b8;--fs-font-size:12px}` +
		`html.theme-dark{--fs-color-brand-01-base:#3dadfa}` +
		`.fs-minha-classe{color:red}.outra{display:none}`)
	cat := ParseCSS(css, "teste")
	if !cat.HasClass("fs-minha-classe") || !cat.HasClass("outra") {
		t.Errorf("classes não extraídas: %v", cat.Classes)
	}
	v := cat.Vars["--fs-color-brand-01-base"]
	if v.Light != "#0079b8" || v.Dark != "#3dadfa" {
		t.Errorf("variável light/dark: %+v", v)
	}
}

// projeto de teste com as violações de cada regra.
func violationProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	form := filepath.Join(root, "forms", "MeuForm")
	if err := os.MkdirAll(form, 0o755); err != nil {
		t.Fatal(err)
	}
	html := `<html><head>
<link rel="stylesheet" href="/style-guide/css/fluig-style-guide.min.css">
<link href="https://fonts.googleapis.com/css?family=Roboto" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/vue@2"></script>
<style>
.painel { background: #fff; }
</style>
</head><body>
<form name="meuform">
<div class="fs-bg-whit" style="color: #006400">verde</div>
<div class="fs-bg-white">ok ${expressao}</div>
</form>
</body></html>`
	if err := os.WriteFile(filepath.Join(form, "MeuForm.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}

	wcss := filepath.Join(root, "wcm", "widget", "minha", "src", "main", "webapp", "resources", "css")
	if err := os.MkdirAll(wcss, 0o755); err != nil {
		t.Fatal(err)
	}
	css := `/* comentário com #123456 não conta */
.a { color: rgb(255, 255, 255); }
@import url("https://cdn.exemplo.com/x.css");
.b { border-color: var(--fs-color-neutral-light-10); }`
	if err := os.WriteFile(filepath.Join(wcss, "minha.css"), []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunRegras(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(violationProject(t), nil, cat, Config{})
	if err != nil {
		t.Fatal(err)
	}
	byRule := map[string]int{}
	for _, f := range res.Findings {
		byRule[f.Rule]++
	}
	// SG001 legado ×1; SG002 externos: fonts + cdn script + @import css = 3;
	// SG003: #fff no <style>, #006400 no style=, rgb() no css = 3;
	// SG006: fs-bg-whit ×1 (fs-bg-white existe; ${expressao} ignorada).
	want := map[string]int{RuleLegacyCSS: 1, RuleExternalRes: 3, RuleHardcodedHex: 3, RuleUnknownClass: 1}
	for rule, n := range want {
		if byRule[rule] != n {
			t.Errorf("%s: %d achado(s), quero %d\n%v", rule, byRule[rule], n, res.Findings)
		}
	}
	if res.Scanned != 2 {
		t.Errorf("scanned=%d, quero 2", res.Scanned)
	}
	// Severidades e sugestões nos pontos-chave.
	for _, f := range res.Findings {
		switch f.Rule {
		case RuleLegacyCSS, RuleUnknownClass:
			if f.Severity != SeverityWarning {
				t.Errorf("%s deveria ser aviso: %+v", f.Rule, f)
			}
		default:
			if f.Severity != SeverityError {
				t.Errorf("%s deveria ser erro: %+v", f.Rule, f)
			}
		}
		if f.Rule == RuleUnknownClass && !strings.Contains(f.Suggestion, "fs-bg-white") {
			t.Errorf("typo sem sugestão do vizinho: %+v", f)
		}
		if f.Rule == RuleHardcodedHex && strings.Contains(f.Message, "rgb(255, 255, 255)") &&
			!strings.Contains(f.Suggestion, "--fs-color-neutral-light-00") {
			t.Errorf("rgb branco sem a variável exata: %+v", f)
		}
	}
}

// Exceções do audit.json, vendorado (.min./minificado) e bundle de SPA ficam
// fora — mas contam no Ignored.
func TestRunIgnorados(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	root := violationProject(t)

	// vendorado .min.css
	vend := filepath.Join(root, "wcm", "widget", "minha", "src", "main", "webapp", "resources", "css", "lib.min.css")
	if err := os.WriteFile(vend, []byte(".x{color:#123}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// minificado sem .min. (linha gigante)
	big := strings.Repeat(".y{color:#abc}", 3000)
	if err := os.WriteFile(filepath.Join(root, "forms", "MeuForm", "vendor.css"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	// bundle gerado de widget SPA (package.json na raiz da widget)
	spa := filepath.Join(root, "wcm", "widget", "spa")
	spaCSS := filepath.Join(spa, "src", "main", "webapp", "resources", "css")
	if err := os.MkdirAll(spaCSS, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(spa, "package.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(spaCSS, "spa.css"), []byte(".z{color:#000}\n"), 0o644)

	res, err := Run(root, nil, cat, Config{Ignore: []string{"forms/MeuForm/"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 2 { // só o minha.css sobrevive: rgb (SG003) + @import (SG002)
		t.Errorf("findings=%d, quero 2: %+v", len(res.Findings), res.Findings)
	}
	if len(res.Ignored) != 4 { // form (config) + form vendor.css? não: pasta inteira ignorada cobre os dois; .min.css; spa.css
		// forms/MeuForm/MeuForm.html e vendor.css (config, 2) + lib.min.css + spa.css
		t.Errorf("ignored=%d, quero 4: %v", len(res.Ignored), res.Ignored)
	}
}

// Alvo explícito restringe a varredura.
func TestRunAlvoExplicito(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	root := violationProject(t)
	res, err := Run(root, []string{"wcm/widget"}, cat, Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if strings.HasPrefix(f.File, "forms/") {
			t.Errorf("forms não deveria entrar: %+v", f)
		}
	}
	if res.Scanned != 1 {
		t.Errorf("scanned=%d, quero 1", res.Scanned)
	}
}
