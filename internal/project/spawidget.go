package project

import (
	"os"
	"path/filepath"
)

// Widgets SPA (templates vue/react do `widget new`): têm package.json na raiz
// e o bundle compilado em src/main/webapp/resources. CLI (widget export) e
// dev server usam estes helpers para tratá-las.

// SPAWidget descreve uma widget com toolchain npm.
type SPAWidget struct {
	Code string // nome da pasta (== código/context-root nos templates da CLI)
	Dir  string // caminho absoluto da widget
}

// FindSPAWidgets varre wcm/widget/ atrás de widgets com package.json.
func FindSPAWidgets(root string) []SPAWidget {
	base := filepath.Join(root, WidgetsDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []SPAWidget
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		if IsSPAWidgetDir(dir) {
			out = append(out, SPAWidget{Code: e.Name(), Dir: dir})
		}
	}
	return out
}

// IsSPAWidgetDir informa se a pasta de uma widget tem toolchain npm.
func IsSPAWidgetDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil
}

// StaleBundle compara o bundle compilado com as fontes da SPA. Devolve o
// motivo do aviso ("" = em dia). Heurística por mtime: qualquer arquivo fora
// de src/main/ e node_modules/ mais novo que o js indica fonte não compilada.
func StaleBundle(w SPAWidget) string {
	bundle := filepath.Join(w.Dir, "src", "main", "webapp", "resources", "js", w.Code+".js")
	info, err := os.Stat(bundle)
	if err != nil {
		return "sem bundle compilado — rode: npm install && npm run build"
	}
	built := info.ModTime()
	stale := false
	_ = filepath.WalkDir(w.Dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || stale {
			return filepath.SkipAll
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == "main" && filepath.Base(filepath.Dir(p)) == "src" {
				return filepath.SkipDir
			}
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.ModTime().After(built) {
			stale = true
			return filepath.SkipAll
		}
		return nil
	})
	if stale {
		return "fonte mais nova que o bundle — rode: npm run build (ou use fluigcli dev --npm-watch)"
	}
	return ""
}
