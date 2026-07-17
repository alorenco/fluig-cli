package devserver

import (
	"bufio"
	"context"
	"errors"
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

// warnStaleBundles avisa (na largada) sobre widgets SPA com bundle ausente ou
// desatualizado — o portal serve o js velho e a mudança "não aparece".
func (s *Server) warnStaleBundles() {
	for _, w := range project.FindSPAWidgets(s.opts.Root) {
		if reason := project.StaleBundle(w); reason != "" {
			s.opts.Warnf("widget %s: %s", w.Code, reason)
		}
	}
}

// setNpmState registra o estado do `npm run watch` de uma widget, exposto no
// card de widgets SPA do dashboard.
func (s *Server) setNpmState(code, state string) {
	s.npmMu.Lock()
	defer s.npmMu.Unlock()
	if s.npmState == nil {
		s.npmState = map[string]string{}
	}
	s.npmState[code] = state
}

// npmStateOf devolve o estado registrado ("" = nenhum spawn para a widget).
func (s *Server) npmStateOf(code string) string {
	s.npmMu.Lock()
	defer s.npmMu.Unlock()
	return s.npmState[code]
}

// npmWatchOn informa se o npm watch está ligado (toggle do dashboard).
func (s *Server) npmWatchOn() bool {
	s.npmMu.Lock()
	defer s.npmMu.Unlock()
	return s.npmOn
}

// SetNpmWatch liga/desliga os `npm run watch` das widgets SPA em execução —
// o toggle do dashboard, sem reiniciar o dev. Os processos ligados aqui
// morrem tanto no desligar (cancel derivado) quanto no Ctrl+C do dev (o
// contexto deriva do Run).
func (s *Server) SetNpmWatch(enabled bool) error {
	s.npmMu.Lock()
	if enabled == s.npmOn {
		s.npmMu.Unlock()
		return nil
	}
	if enabled {
		if s.runCtx == nil {
			s.npmMu.Unlock()
			return errors.New("o dev server ainda não terminou de subir — tente de novo")
		}
		ctx, cancel := context.WithCancel(s.runCtx)
		s.npmCancel = cancel
		s.npmOn = true
		s.npmMu.Unlock()
		s.startNpmWatch(ctx)
		return nil
	}
	cancel := s.npmCancel
	s.npmCancel = nil
	s.npmOn = false
	s.npmState = map[string]string{}
	s.npmMu.Unlock()
	if cancel != nil {
		cancel()
		s.opts.Infof("npm watch desligado pelo dashboard")
	}
	return nil
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
	widgets := project.FindSPAWidgets(s.opts.Root)
	if len(widgets) == 0 {
		s.opts.Warnf("--npm-watch: nenhuma widget com package.json em %s", project.WidgetsDir)
		return
	}
	if _, err := exec.LookPath("npm"); err != nil {
		s.opts.Warnf("--npm-watch: npm não encontrado no PATH — instale o Node.js (ver .nvmrc da widget)")
		for _, w := range widgets {
			s.setNpmState(w.Code, "npm não encontrado no PATH")
		}
		return
	}
	for _, w := range widgets {
		if _, err := os.Stat(filepath.Join(w.Dir, "node_modules")); err != nil {
			s.opts.Warnf("npm watch: widget %s sem node_modules — rode npm install em %s e religue o npm watch no dashboard", w.Code, relOrSelf(s.opts.Root, w.Dir))
			s.setNpmState(w.Code, "sem node_modules — rode npm install e religue o npm watch")
			continue
		}
		go s.runNpmWatch(ctx, w)
	}
}

// runNpmWatch mantém um `npm run watch` vivo para a widget (re-lança em caso
// de queda, com um respiro para não ciclar em erro permanente).
func (s *Server) runNpmWatch(ctx context.Context, w project.SPAWidget) {
	for {
		cmd := npmCommand(w.Dir)
		setProcGroup(cmd)
		stdout, _ := cmd.StdoutPipe()
		cmd.Stderr = cmd.Stdout // intercala no mesmo pipe
		if err := cmd.Start(); err != nil {
			s.opts.Warnf("npm watch %s: %v", w.Code, err)
			s.setNpmState(w.Code, "falhou ao iniciar: "+err.Error())
			return
		}
		s.opts.Infof("npm watch %s: compilando a cada save (pid %d)", w.Code, cmd.Process.Pid)
		s.setNpmState(w.Code, "rodando")
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
			s.setNpmState(w.Code, "reiniciando após queda")
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
