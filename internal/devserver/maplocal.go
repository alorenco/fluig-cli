package devserver

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/alorenco/fluig-cli/internal/project"
)

// mount liga o context-root de uma widget (URL no servidor) à sua pasta
// local de recursos estáticos (src/main/webapp) e ao view.ftl (render local).
type mount struct {
	contextRoot string // ex.: /ramais (com barra inicial, sem final)
	dir         string // ex.: <root>/wcm/widget/ramais/src/main/webapp
	appCode     string // application.code do application.info (fallback: pasta)
	viewFTL     string // caminho do view.file (fallback: view.ftl); "" se não existe
}

// mountTable descobre e cacheia os map-locals das widgets do projeto. O cache
// é invalidado pelo watcher quando algo muda em wcm/widget/ (widget nova ou
// jboss-web.xml editado).
type mountTable struct {
	root string

	mu     sync.Mutex
	loaded bool
	mounts []mount
}

func newMountTable(root string) *mountTable {
	return &mountTable{root: root}
}

// invalidate força a redescoberta na próxima requisição.
func (t *mountTable) invalidate() {
	t.mu.Lock()
	t.loaded = false
	t.mu.Unlock()
}

func (t *mountTable) snapshot() []mount {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.mounts = discoverMounts(t.root)
		t.loaded = true
	}
	return t.mounts
}

// localeSuffix casa o sufixo de idioma que o portal acrescenta ao pedir
// recursos de widget (ex.: ramais_pt_BR.js) — o servidor cai para o arquivo
// base quando a variante do idioma não existe, e o map-local faz o mesmo.
var localeSuffix = regexp.MustCompile(`_[a-z]{2}(?:_[A-Z]{2})?(\.[A-Za-z0-9]+)$`)

// resolve traduz o path de uma requisição num arquivo local de widget.
// Query string (cache-busting ?v=…) já chega separada do path. Sem arquivo
// local correspondente, devolve false — e a requisição segue para o servidor
// (recursos gerados, como os bundles de i18n, só existem lá).
func (t *mountTable) resolve(reqPath string) (string, bool) {
	for _, m := range t.snapshot() {
		rel, ok := strings.CutPrefix(reqPath, m.contextRoot+"/")
		if !ok || rel == "" {
			continue
		}
		candidates := []string{rel}
		if base := localeSuffix.ReplaceAllString(rel, "$1"); base != rel {
			candidates = append(candidates, base)
		}
		for _, cand := range candidates {
			p, err := project.SafeJoin(m.dir, strings.Split(cand, "/")...)
			if err != nil {
				continue
			}
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, true
			}
		}
	}
	return "", false
}

// discoverMounts varre wcm/widget/*/ e monta a tabela. O context-root vem do
// jboss-web.xml da widget; sem ele, vale o nome da pasta (mesma convenção do
// empacotamento do WAR).
func discoverMounts(root string) []mount {
	entries, err := os.ReadDir(filepath.Join(root, project.WidgetsDir))
	if err != nil {
		return nil
	}
	var out []mount
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, project.WidgetsDir, e.Name(), "src", "main", "webapp")
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		cr := contextRootOf(dir, e.Name())
		widgetDir := filepath.Join(root, project.WidgetsDir, e.Name())
		code, viewFTL := appInfoOf(widgetDir, e.Name())
		out = append(out, mount{contextRoot: cr, dir: dir, appCode: code, viewFTL: viewFTL})
	}
	// Prefixos mais longos primeiro: um context-root que contenha outro
	// (ex.: /app e /app2 não conflitam, mas /a e /a/b sim) resolve certo.
	sort.Slice(out, func(i, j int) bool { return len(out[i].contextRoot) > len(out[j].contextRoot) })
	return out
}

// byAppCode acha o mount de uma widget pelo application.code (o `appcode` que
// o portal estampa no envelope de cada instância).
func (t *mountTable) byAppCode(code string) (mount, bool) {
	if code == "" {
		return mount{}, false
	}
	for _, m := range t.snapshot() {
		if m.appCode == code && m.viewFTL != "" {
			return m, true
		}
	}
	return mount{}, false
}

// byViewFTL acha o mount cujo view.ftl é o arquivo dado (caminho já limpo ou
// não — a comparação normaliza).
func (t *mountTable) byViewFTL(path string) (mount, bool) {
	clean := filepath.Clean(path)
	for _, m := range t.snapshot() {
		if m.viewFTL != "" && filepath.Clean(m.viewFTL) == clean {
			return m, true
		}
	}
	return mount{}, false
}

// appInfoOf lê application.code e o arquivo de view do application.info
// (fallbacks: nome da pasta e view.ftl). viewFTL volta "" quando o arquivo
// de view não existe no disco.
func appInfoOf(widgetDir, folderName string) (code, viewFTL string) {
	code, viewFile := folderName, "view.ftl"
	resources := filepath.Join(widgetDir, "src", "main", "resources")
	if b, err := os.ReadFile(filepath.Join(resources, "application.info")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(k) {
			case "application.code":
				if v = strings.TrimSpace(v); v != "" {
					code = v
				}
			case "view.file":
				if v = strings.TrimSpace(v); v != "" {
					viewFile = v
				}
			}
		}
	}
	p, err := project.SafeJoin(resources, viewFile)
	if err != nil {
		return code, ""
	}
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return code, ""
	}
	return code, p
}

// contextRootOf lê o <context-root> do jboss-web.xml (fallback: nome da pasta).
func contextRootOf(webappDir, folderName string) string {
	cr := folderName
	b, err := os.ReadFile(filepath.Join(webappDir, "WEB-INF", "jboss-web.xml"))
	if err == nil {
		var doc struct {
			ContextRoot string `xml:"context-root"`
		}
		if xml.Unmarshal(b, &doc) == nil && strings.TrimSpace(doc.ContextRoot) != "" {
			cr = strings.TrimSpace(doc.ContextRoot)
		}
	}
	cr = "/" + strings.Trim(cr, "/")
	if cr == "/" {
		cr = "/" + folderName
	}
	return cr
}
