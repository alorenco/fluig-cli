package devserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/alorenco/fluig-cli/internal/project"
)

// monta uma widget SPA falsa: package.json + fonte + (opcional) bundle.
func fakeSPAWidget(t *testing.T, root, code string, withBundle bool) string {
	t.Helper()
	dir := filepath.Join(root, "wcm", "widget", code)
	write := func(rel, content string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", "{}")
	write("src/vue/App.vue", "<template/>")
	write("node_modules/algum/pacote.js", "x")
	if withBundle {
		write("src/main/webapp/resources/js/"+code+".js", "bundle")
	}
	return dir
}

func TestFindSPAWidgets(t *testing.T) {
	root := t.TempDir()
	fakeSPAWidget(t, root, "spa_um", true)
	// widget classic (sem package.json) não conta
	os.MkdirAll(filepath.Join(root, "wcm", "widget", "classica", "src"), 0o755)

	ws := project.FindSPAWidgets(root)
	if len(ws) != 1 || ws[0].Code != "spa_um" {
		t.Fatalf("findSPAWidgets = %+v", ws)
	}
}

func TestStaleBundle(t *testing.T) {
	root := t.TempDir()

	// Sem bundle: precisa avisar.
	w := project.SPAWidget{Code: "sem_bundle", Dir: fakeSPAWidget(t, root, "sem_bundle", false)}
	if got := project.StaleBundle(w); got == "" {
		t.Errorf("sem bundle deveria avisar")
	}

	// Bundle mais novo que a fonte: em dia.
	w2 := project.SPAWidget{Code: "em_dia", Dir: fakeSPAWidget(t, root, "em_dia", true)}
	old := time.Now().Add(-time.Hour)
	for _, rel := range []string{"package.json", "src/vue/App.vue"} {
		os.Chtimes(filepath.Join(w2.Dir, filepath.FromSlash(rel)), old, old)
	}
	if got := project.StaleBundle(w2); got != "" {
		t.Errorf("bundle em dia avisou: %q", got)
	}

	// Fonte editada depois do build: desatualizado (node_modules não conta).
	novo := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(w2.Dir, "src", "vue", "App.vue"), novo, novo)
	if got := project.StaleBundle(w2); got == "" {
		t.Errorf("fonte mais nova deveria avisar")
	}
}

func TestSpaSourceEvent(t *testing.T) {
	root := t.TempDir()
	dir := fakeSPAWidget(t, root, "minha_spa", true)

	// Fonte/toolchain (fora de src/main): evento silencioso.
	for _, rel := range []string{"src/vue/App.vue", "package.json", "vite.config.ts"} {
		if !spaSourceEvent(root, "minha_spa", filepath.Join(dir, filepath.FromSlash(rel))) {
			t.Errorf("%s deveria ser fonte de SPA", rel)
		}
	}
	// Dentro de src/main: segue o fluxo normal (reload/aviso de server-side).
	for _, rel := range []string{"src/main/resources/view.ftl", "src/main/webapp/resources/js/minha_spa.js"} {
		if spaSourceEvent(root, "minha_spa", filepath.Join(dir, filepath.FromSlash(rel))) {
			t.Errorf("%s NÃO deveria ser classificado como fonte de SPA", rel)
		}
	}
	// Widget sem package.json nunca é SPA.
	os.MkdirAll(filepath.Join(root, "wcm", "widget", "classica", "src", "js"), 0o755)
	if spaSourceEvent(root, "classica", filepath.Join(root, "wcm", "widget", "classica", "src", "js", "x.js")) {
		t.Errorf("widget sem package.json não é SPA")
	}
}

func TestInNodeModules(t *testing.T) {
	sep := string(filepath.Separator)
	if !inNodeModules("a" + sep + "node_modules" + sep + "x.js") {
		t.Errorf("caminho dentro de node_modules não detectado")
	}
	if !inNodeModules("a" + sep + "node_modules") {
		t.Errorf("a própria pasta node_modules não detectada")
	}
	if inNodeModules("a" + sep + "node_modules_meu" + sep + "x.js") {
		t.Errorf("falso positivo em node_modules_meu")
	}
}

// O watcher recursivo não pode entrar em node_modules (estouraria o limite de
// watches do SO num npm install).
func TestAddRecursivePulaNodeModules(t *testing.T) {
	root := t.TempDir()
	fakeSPAWidget(t, root, "spa_watch", true)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := addRecursive(w, filepath.Join(root, "wcm", "widget")); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.WatchList() {
		if inNodeModules(p) {
			t.Errorf("node_modules entrou no watcher: %s", p)
		}
	}
	if len(w.WatchList()) == 0 {
		t.Errorf("nada sendo observado")
	}
}
