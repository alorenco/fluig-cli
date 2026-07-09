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
// local de recursos estáticos (src/main/webapp).
type mount struct {
	contextRoot string // ex.: /ramais (com barra inicial, sem final)
	dir         string // ex.: <root>/wcm/widget/ramais/src/main/webapp
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
		out = append(out, mount{contextRoot: cr, dir: dir})
	}
	// Prefixos mais longos primeiro: um context-root que contenha outro
	// (ex.: /app e /app2 não conflitam, mas /a e /a/b sim) resolve certo.
	sort.Slice(out, func(i, j int) bool { return len(out[i].contextRoot) > len(out[j].contextRoot) })
	return out
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
