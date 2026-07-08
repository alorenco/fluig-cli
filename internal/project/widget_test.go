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
		if got := MapWidgetEntryToLocal(entry); got != want {
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
