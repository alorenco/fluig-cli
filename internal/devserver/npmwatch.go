package devserver

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/alorenco/fluig-cli/internal/project"
)

// Widgets SPA (templates vue/react do widget new): têm package.json na raiz
// e o bundle compilado em src/main/webapp/resources. O dev server as trata
// diferente no live reload (a fonte é compilada pelo Vite, não servida) e
// pode rodar o `npm run watch` delas (--npm-watch).

// spaWidget descreve uma widget com toolchain npm.
type spaWidget struct {
	Code string // nome da pasta (== código/context-root nos templates da CLI)
	Dir  string // caminho absoluto da widget
}

// findSPAWidgets varre wcm/widget/ atrás de widgets com package.json.
func findSPAWidgets(root string) []spaWidget {
	base := filepath.Join(root, project.WidgetsDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []spaWidget
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			out = append(out, spaWidget{Code: e.Name(), Dir: dir})
		}
	}
	return out
}

// isSPAWidgetDir informa se a pasta de uma widget tem toolchain npm.
func isSPAWidgetDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil
}

// staleBundle compara o bundle compilado com as fontes da SPA. Devolve o
// motivo do aviso ("" = em dia). Heurística por mtime: qualquer arquivo fora
// de src/main/ e node_modules/ mais novo que o js indica fonte não compilada.
func staleBundle(w spaWidget) string {
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

// warnStaleBundles avisa (na largada) sobre widgets SPA com bundle ausente ou
// desatualizado — o portal serve o js velho e a mudança "não aparece".
func (s *Server) warnStaleBundles() {
	for _, w := range findSPAWidgets(s.opts.Root) {
		if reason := staleBundle(w); reason != "" {
			s.opts.Warnf("widget %s: %s", w.Code, reason)
		}
	}
}

// npmCommand monta o comando do watch (variável para os testes trocarem).
var npmCommand = func(dir string) *exec.Cmd {
	cmd := exec.Command("npm", "run", "watch")
	cmd.Dir = dir
	return cmd
}

// startNpmWatch dá spawn no `npm run watch` de cada widget SPA e replica a
// saída no log com o prefixo da widget. Os processos morrem com o contexto.
func (s *Server) startNpmWatch(ctx context.Context) {
	widgets := findSPAWidgets(s.opts.Root)
	if len(widgets) == 0 {
		s.opts.Warnf("--npm-watch: nenhuma widget com package.json em %s", project.WidgetsDir)
		return
	}
	if _, err := exec.LookPath("npm"); err != nil {
		s.opts.Warnf("--npm-watch: npm não encontrado no PATH — instale o Node.js (ver .nvmrc da widget)")
		return
	}
	for _, w := range widgets {
		if _, err := os.Stat(filepath.Join(w.Dir, "node_modules")); err != nil {
			s.opts.Warnf("--npm-watch: widget %s sem node_modules — rode npm install em %s e reinicie", w.Code, relOrSelf(s.opts.Root, w.Dir))
			continue
		}
		go s.runNpmWatch(ctx, w)
	}
}

// runNpmWatch mantém um `npm run watch` vivo para a widget (re-lança em caso
// de queda, com um respiro para não ciclar em erro permanente).
func (s *Server) runNpmWatch(ctx context.Context, w spaWidget) {
	for {
		cmd := npmCommand(w.Dir)
		setProcGroup(cmd)
		stdout, _ := cmd.StdoutPipe()
		cmd.Stderr = cmd.Stdout // intercala no mesmo pipe
		if err := cmd.Start(); err != nil {
			s.opts.Warnf("npm watch %s: %v", w.Code, err)
			return
		}
		s.opts.Infof("npm watch %s: compilando a cada save (pid %d)", w.Code, cmd.Process.Pid)
		go s.relayNpmOutput(w.Code, stdout)

		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-ctx.Done():
			killTree(cmd)
			<-done
			return
		case err := <-done:
			if ctx.Err() != nil {
				return
			}
			s.opts.Warnf("npm watch %s terminou (%v) — reiniciando em 5s", w.Code, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// relayNpmOutput replica as linhas do vite no log, prefixadas pela widget.
func (s *Server) relayNpmOutput(code string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			s.opts.Infof("[%s] %s", code, line)
		}
	}
}
