package project

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WidgetsDir é a pasta convencional dos widgets: wcm/widget/<Nome>.
var WidgetsDir = filepath.Join("wcm", "widget")

// WidgetDir devolve o diretório de um widget.
func WidgetDir(root, name string) string {
	return filepath.Join(root, WidgetsDir, name)
}

// WARFileRef liga um caminho dentro do WAR a um arquivo local.
type WARFileRef struct {
	WARPath   string // caminho dentro do .war (ex.: resources/js/app.js)
	LocalPath string // caminho absoluto local
}

// CollectWidgetWARFiles monta o mapa de empacotamento do WAR a partir da pasta
// do widget:
//
//	src/main/webapp/WEB-INF/**    → WEB-INF/**
//	src/main/resources/**         → WEB-INF/classes/**
//	src/main/webapp/resources/**  → resources/**
func CollectWidgetWARFiles(widgetDir string) ([]WARFileRef, error) {
	mains := []struct{ srcRel, warPrefix string }{
		{filepath.Join("src", "main", "webapp", "WEB-INF"), "WEB-INF"},
		{filepath.Join("src", "main", "resources"), "WEB-INF/classes"},
		{filepath.Join("src", "main", "webapp", "resources"), "resources"},
	}
	var out []WARFileRef
	for _, m := range mains {
		base := filepath.Join(widgetDir, m.srcRel)
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return fs.SkipAll
				}
				return err
			}
			if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			rel, err := filepath.Rel(base, p)
			if err != nil {
				return err
			}
			out = append(out, WARFileRef{
				WARPath:   m.warPrefix + "/" + filepath.ToSlash(rel),
				LocalPath: p,
			})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return out, nil
}

// MapWidgetEntryToLocal traduz uma entrada do WAR para o caminho local relativo
// à pasta do widget. Devolve "" para entradas a ignorar.
//
//	resources/**            → src/main/webapp/resources/**
//	WEB-INF/classes/<arq>   → src/main/resources/<arq>        (arquivo no topo)
//	WEB-INF/classes/<pkg>/**→ src/main/java/<pkg>/**          (em subpasta)
//	WEB-INF/<arq>           → src/main/webapp/WEB-INF/<arq>
//	pom.xml                 → pom.xml
func MapWidgetEntryToLocal(entry string) string {
	entry = strings.TrimPrefix(path.Clean(entry), "/")
	switch {
	case entry == "pom.xml":
		return "pom.xml"
	case strings.HasPrefix(entry, "resources/"):
		return filepath.FromSlash("src/main/webapp/" + entry)
	case strings.HasPrefix(entry, "WEB-INF/classes/"):
		rest := strings.TrimPrefix(entry, "WEB-INF/classes/")
		if !strings.Contains(rest, "/") {
			return filepath.FromSlash("src/main/resources/" + rest)
		}
		return filepath.FromSlash("src/main/java/" + rest)
	case strings.HasPrefix(entry, "WEB-INF/"):
		return filepath.FromSlash("src/main/webapp/" + entry)
	default:
		return ""
	}
}
