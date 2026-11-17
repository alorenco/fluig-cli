package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapWidgetEntryToLocal(t *testing.T) {
	cases := map[string]string{
		"resources/js/app.js":          filepath.FromSlash("src/main/webapp/resources/js/app.js"),
		"WEB-INF/classes/app.info":     filepath.FromSlash("src/main/resources/app.info"),
		"WEB-INF/classes/com/x/A.java": filepath.FromSlash("src/main/java/com/x/A.java"),
		"WEB-INF/web.xml":              filepath.FromSlash("src/main/webapp/WEB-INF/web.xml"),
		"pom.xml":                      "pom.xml",
		"algo/desconhecido.txt":        "",
	}
	for entry, want := range cases {
		if got := MapWidgetEntryToLocal(WARLayoutMaven, entry); got != want {
			t.Errorf("MapWidgetEntryToLocal(%q) = %q, quer %q", entry, got, want)
		}
	}
}

func TestCollectWidgetWARFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/main/webapp/WEB-INF/application.xml")
	write("src/main/resources/app.info")
	write("src/main/webapp/resources/js/app.js")
	write("src/main/webapp/resources/img/logo.png")

	refs, err := CollectWidgetWARFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range refs {
		got[r.WARPath] = true
	}
	want := []string{
		"WEB-INF/application.xml",
		"WEB-INF/classes/app.info",
		"resources/js/app.js",
		"resources/img/logo.png",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("faltou %q no WAR; veio %v", w, got)
		}
	}
	if len(refs) != len(want) {
		t.Errorf("esperava %d arquivos, veio %d", len(want), len(refs))
	}
}

// writeTree grava arquivos de teste (conteúdo fixo) sob dir.
func writeTree(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func warPaths(t *testing.T, dir string) map[string]string {
	t.Helper()
	refs, err := CollectWidgetWARFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range refs {
		rel, _ := filepath.Rel(dir, r.LocalPath)
		got[r.WARPath] = filepath.ToSlash(rel)
	}
	return got
}

// Estrutura plana (issue #7): WEB-INF/ e resources/ na raiz da pasta, com a
// sobra vazia de src/main que o Studio deixa. O WAR sai da raiz.
func TestCollectWidgetWARFilesEstruturaPlana(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir,
		"resources/css/relatorio.css",
		"resources/js/relatorio.js",
		"WEB-INF/classes/application.info",
		"WEB-INF/classes/view.ftl",
		"WEB-INF/classes/com/acme/Rest.class",
		"WEB-INF/lib/util.jar",
		"WEB-INF/jboss-web.xml",
		"WEB-INF/web.xml",
		// Fora do WAR: arquivos da raiz e pastas que não são do WAR.
		"pom.xml",
		".project",
		"docs/leia.txt",
		"resources/.DS_Store",
	)
	if err := os.MkdirAll(filepath.Join(dir, "src", "main", "webapp", "resources"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := DetectWARLayout(dir); got != WARLayoutFlat {
		t.Fatalf("DetectWARLayout=%v, quer a estrutura plana", got)
	}
	got := warPaths(t, dir)
	want := []string{
		"resources/css/relatorio.css",
		"resources/js/relatorio.js",
		"WEB-INF/classes/application.info",
		"WEB-INF/classes/view.ftl",
		"WEB-INF/classes/com/acme/Rest.class",
		"WEB-INF/lib/util.jar",
		"WEB-INF/jboss-web.xml",
		"WEB-INF/web.xml",
	}
	for _, w := range want {
		if got[w] != w {
			t.Errorf("entrada %q veio de %q; quer o mesmo caminho", w, got[w])
		}
	}
	if len(got) != len(want) {
		t.Errorf("esperava %d arquivos, veio %d: %v", len(want), len(got), got)
	}
}

// Só uma das duas pastas da raiz já caracteriza a estrutura plana.
func TestDetectWARLayoutPlanaParcial(t *testing.T) {
	for _, rel := range []string{"WEB-INF/classes/application.info", "resources/js/a.js"} {
		dir := t.TempDir()
		writeTree(t, dir, rel)
		if got := DetectWARLayout(dir); got != WARLayoutFlat {
			t.Errorf("só %s: DetectWARLayout=%v, quer plana", rel, got)
		}
	}
}

// As duas estruturas com arquivos: o src/main vence (comportamento de sempre)
// e os arquivos da raiz ficam fora do WAR. HasIgnoredFlatFiles sinaliza o caso.
func TestDetectWARLayoutDuasEstruturas(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir,
		"src/main/resources/application.info",
		"src/main/webapp/resources/js/novo.js",
		"resources/js/antigo.js",
	)
	if got := DetectWARLayout(dir); got != WARLayoutMaven {
		t.Fatalf("DetectWARLayout=%v, quer src/main", got)
	}
	if !HasIgnoredFlatFiles(dir) {
		t.Error("HasIgnoredFlatFiles deveria sinalizar os arquivos da raiz")
	}
	got := warPaths(t, dir)
	if _, ok := got["resources/js/antigo.js"]; ok {
		t.Errorf("arquivo da raiz entrou no WAR: %v", got)
	}
	if got["resources/js/novo.js"] == "" || got["WEB-INF/classes/application.info"] == "" {
		t.Errorf("o WAR não saiu do src/main: %v", got)
	}
}

// Pasta vazia, pasta ausente e src/main normal: estrutura src/main, sem aviso.
func TestDetectWARLayoutPadrao(t *testing.T) {
	vazia := t.TempDir()
	if got := DetectWARLayout(vazia); got != WARLayoutMaven {
		t.Errorf("pasta vazia: %v", got)
	}
	if got := DetectWARLayout(filepath.Join(vazia, "nao_existe")); got != WARLayoutMaven {
		t.Errorf("pasta ausente: %v", got)
	}
	dir := t.TempDir()
	writeTree(t, dir, "src/main/resources/application.info")
	if DetectWARLayout(dir) != WARLayoutMaven || HasIgnoredFlatFiles(dir) {
		t.Error("src/main puro não é plano nem tem arquivos ignorados")
	}
	plana := t.TempDir()
	writeTree(t, plana, "WEB-INF/web.xml")
	if HasIgnoredFlatFiles(plana) {
		t.Error("estrutura plana pura não tem arquivos ignorados")
	}
}

// Na estrutura plana o import grava a entrada do WAR no mesmo caminho.
func TestMapWidgetEntryToLocalPlana(t *testing.T) {
	cases := map[string]string{
		"resources/js/app.js":           filepath.FromSlash("resources/js/app.js"),
		"WEB-INF/classes/app.info":      filepath.FromSlash("WEB-INF/classes/app.info"),
		"WEB-INF/classes/com/x/A.class": filepath.FromSlash("WEB-INF/classes/com/x/A.class"),
		"WEB-INF/web.xml":               filepath.FromSlash("WEB-INF/web.xml"),
		"pom.xml":                       "pom.xml",
		"META-INF/MANIFEST.MF":          "",
		"algo/desconhecido.txt":         "",
	}
	for entry, want := range cases {
		if got := MapWidgetEntryToLocal(WARLayoutFlat, entry); got != want {
			t.Errorf("MapWidgetEntryToLocal(plana, %q) = %q, quer %q", entry, got, want)
		}
	}
}

func TestWARLayoutPastas(t *testing.T) {
	if got := filepath.ToSlash(WARLayoutMaven.StaticRel()); got != "src/main/webapp/resources" {
		t.Errorf("StaticRel src/main = %q", got)
	}
	if got := filepath.ToSlash(WARLayoutFlat.StaticRel()); got != "resources" {
		t.Errorf("StaticRel plana = %q", got)
	}
	if got := filepath.ToSlash(WARLayoutFlat.ClassesRel()); got != "WEB-INF/classes" {
		t.Errorf("ClassesRel plana = %q", got)
	}
}
