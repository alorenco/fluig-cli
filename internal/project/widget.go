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

// WARLayout é a estrutura de pastas local de um widget ou layout WCM.
type WARLayout int

const (
	// WARLayoutMaven é a estrutura do Fluig Studio atual e do `widget new`:
	// src/main/resources e src/main/webapp.
	WARLayoutMaven WARLayout = iota
	// WARLayoutFlat é a estrutura plana (WAR aberto), gerada pelo Eclipse e
	// pelo Fluig Studio antigo: WEB-INF/ e resources/ direto na raiz da pasta.
	WARLayoutFlat
)

// String devolve o nome da estrutura para mensagens.
func (l WARLayout) String() string {
	if l == WARLayoutFlat {
		return "plana (WEB-INF/ e resources/ na raiz)"
	}
	return "src/main"
}

// ClassesRel é a pasta, relativa ao widget, dos arquivos que vão para
// WEB-INF/classes (application.info, view.ftl, .properties).
func (l WARLayout) ClassesRel() string {
	if l == WARLayoutFlat {
		return filepath.Join("WEB-INF", "classes")
	}
	return filepath.Join("src", "main", "resources")
}

// WebappRel é a pasta, relativa ao widget, que corresponde à raiz do WAR.
// Na estrutura plana é a própria pasta do widget (".").
func (l WARLayout) WebappRel() string {
	if l == WARLayoutFlat {
		return "."
	}
	return filepath.Join("src", "main", "webapp")
}

// StaticRel é a pasta, relativa ao widget, dos recursos estáticos que o
// navegador carrega (css, js, imagens).
func (l WARLayout) StaticRel() string {
	return filepath.Join(l.WebappRel(), "resources")
}

// mavenSources são as pastas da estrutura src/main e o destino de cada uma no WAR.
var mavenSources = []struct{ srcRel, warPrefix string }{
	{filepath.Join("src", "main", "webapp", "WEB-INF"), "WEB-INF"},
	{filepath.Join("src", "main", "resources"), "WEB-INF/classes"},
	{filepath.Join("src", "main", "webapp", "resources"), "resources"},
}

// flatSources são as pastas da estrutura plana. O caminho no WAR é o mesmo.
var flatSources = []struct{ srcRel, warPrefix string }{
	{"WEB-INF", "WEB-INF"},
	{"resources", "resources"},
}

// DetectWARLayout descobre a estrutura de uma pasta de widget ou layout.
//
// A regra: a estrutura src/main vale sempre que ela tem pelo menos um
// arquivo empacotável. A estrutura plana vale quando src/main não tem
// arquivos e existe WEB-INF/ ou resources/ com arquivos na raiz. Pasta vazia
// ou ausente é src/main (o padrão dos artefatos novos).
//
// Uma pasta src/main só com subpastas vazias não conta: o Studio deixa essa
// sobra em widgets de estrutura plana.
func DetectWARLayout(dir string) WARLayout {
	if hasWARFiles(dir, mavenSources) {
		return WARLayoutMaven
	}
	if hasWARFiles(dir, flatSources) {
		return WARLayoutFlat
	}
	return WARLayoutMaven
}

// HasIgnoredFlatFiles informa se a pasta usa a estrutura src/main e também
// tem arquivos em WEB-INF/ ou resources/ na raiz. Esses arquivos ficam fora
// do WAR; quem publica deve avisar.
func HasIgnoredFlatFiles(dir string) bool {
	return hasWARFiles(dir, mavenSources) && hasWARFiles(dir, flatSources)
}

func hasWARFiles(dir string, sources []struct{ srcRel, warPrefix string }) bool {
	for _, m := range sources {
		found := false
		_ = filepath.WalkDir(filepath.Join(dir, m.srcRel), func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return fs.SkipAll
			}
			if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
				found = true
				return fs.SkipAll
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}

// CollectWidgetWARFiles monta o mapa de empacotamento do WAR a partir da pasta
// do widget. A estrutura vem de DetectWARLayout.
//
// Estrutura src/main:
//
//	src/main/webapp/WEB-INF/**    → WEB-INF/**
//	src/main/resources/**         → WEB-INF/classes/**
//	src/main/webapp/resources/**  → resources/**
//
// Estrutura plana:
//
//	WEB-INF/**    → WEB-INF/**
//	resources/**  → resources/**
func CollectWidgetWARFiles(widgetDir string) ([]WARFileRef, error) {
	mains := mavenSources
	if DetectWARLayout(widgetDir) == WARLayoutFlat {
		mains = flatSources
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
// à pasta do widget, na estrutura dada. Devolve "" para entradas a ignorar.
//
// Estrutura src/main:
//
//	resources/**            → src/main/webapp/resources/**
//	WEB-INF/classes/<arq>   → src/main/resources/<arq>        (arquivo no topo)
//	WEB-INF/classes/<pkg>/**→ src/main/java/<pkg>/**          (em subpasta)
//	WEB-INF/<arq>           → src/main/webapp/WEB-INF/<arq>
//	pom.xml                 → pom.xml
//
// Estrutura plana: resources/**, WEB-INF/** e pom.xml ficam no mesmo caminho.
func MapWidgetEntryToLocal(layout WARLayout, entry string) string {
	entry = strings.TrimPrefix(path.Clean(entry), "/")
	if layout == WARLayoutFlat {
		if entry == "pom.xml" || strings.HasPrefix(entry, "resources/") || strings.HasPrefix(entry, "WEB-INF/") {
			return filepath.FromSlash(entry)
		}
		return ""
	}
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
