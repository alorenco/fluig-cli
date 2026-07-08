package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newWatchCmd(app *App) *cobra.Command {
	var (
		debounce      time.Duration
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Publica automaticamente ao salvar (datasets, eventos e mecanismos)",
		Long: "Observa as pastas datasets/, events/ e mechanisms/ do projeto e publica o\n" +
			"artefato no servidor a cada salvamento — o ciclo editar→exportar→testar\n" +
			"vira só editar→testar.\n\n" +
			"Regras de segurança: só roda em servidor marcado como dev ou hml (produção\n" +
			"é recusada, sem exceção); só ATUALIZA artefatos que já existem no servidor\n" +
			"— arquivo novo gera um aviso com o comando de criação; salvamento sem\n" +
			"mudança de conteúdo não publica nada.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if app.JSON {
				return output.Usagef("watch é um modo interativo e não suporta --json; em automação, use os comandos export")
			}

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			server, err := app.resolveServer("")
			if err != nil {
				return err
			}
			p.Server = server.Name
			switch server.Env {
			case config.EnvDev, config.EnvHml:
			case config.EnvProd:
				return output.Usagef("o watch não roda em servidor de PRODUÇÃO (%q) — publique conscientemente com export", server.Name)
			default:
				return output.Usagef("o watch exige servidor marcado como dev ou hml; marque com: fluigcli server update %s --env hml", server.Name)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			client, err := app.authenticate(ctx, server, passwordStdin)
			if err != nil {
				return err
			}
			return app.runWatch(ctx, client, root, server.Name, debounce)
		},
	}
	cmd.Flags().DurationVar(&debounce, "debounce", 500*time.Millisecond, "espera após o salvamento antes de publicar (agrupa rajadas do editor)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// runWatch é o loop do watch: observa as pastas convencionais, agrupa eventos
// por arquivo (debounce) e publica cada salvamento. Erros de publicação viram
// aviso — o watch segue vivo até o contexto ser cancelado (Ctrl+C).
func (a *App) runWatch(ctx context.Context, client *fluig.Client, root, serverName string, debounce time.Duration) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return output.Genericf("não consegui iniciar o observador de arquivos: %v", err)
	}
	defer w.Close()

	var watched []string
	for _, d := range diffDirs {
		base := filepath.Join(root, d.dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		if err := watchRecursive(w, base); err != nil {
			return output.Genericf("não consegui observar %s: %v", base, err)
		}
		watched = append(watched, d.dir+"/")
	}
	if len(watched) == 0 {
		return output.Usagef("nenhuma pasta da convenção (datasets/, events/, mechanisms/) encontrada em %s", root)
	}

	a.printer.Infof("Observando %s em %q — Ctrl+C para parar.", strings.Join(watched, ", "), serverName)

	pending := map[string]*time.Timer{}
	fire := make(chan string, 32)
	for {
		select {
		case <-ctx.Done():
			a.printer.Infof("Watch encerrado.")
			return nil

		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
				continue
			}
			// Pasta nova criada dentro da árvore observada: passa a observá-la.
			if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
				_ = watchRecursive(w, ev.Name)
				continue
			}
			if !strings.HasSuffix(ev.Name, ".js") {
				continue
			}
			// Debounce por arquivo: editores salvam em rajadas (write+rename).
			if t, ok := pending[ev.Name]; ok {
				t.Stop()
			}
			name := ev.Name
			pending[name] = time.AfterFunc(debounce, func() {
				select {
				case fire <- name:
				case <-ctx.Done():
				}
			})

		case name := <-fire:
			delete(pending, name)
			target, err := classifyArtifactPath(root, name)
			if err != nil {
				continue // fora da convenção (ex.: .js solto em subpasta desconhecida)
			}
			if _, err := os.Stat(name); err != nil {
				continue // apagado/renomeado entre o evento e o disparo
			}
			a.publishWatched(ctx, client, target)

		case werr, ok := <-w.Errors:
			if !ok {
				return nil
			}
			a.printer.Warnf("observador de arquivos: %v", werr)
		}
	}
}

// watchRecursive adiciona dir e todos os subdiretórios ao observador (o
// fsnotify não é recursivo por conta própria).
func watchRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}

// publishWatched publica um artefato salvo: atualiza se existe no servidor e o
// conteúdo mudou; nunca cria (aviso com o comando certo) — assim o watch não
// gera artefatos nem versões por acidente.
func (a *App) publishWatched(ctx context.Context, client *fluig.Client, t diffTarget) {
	ts := time.Now().Format("15:04:05")
	raw, err := os.ReadFile(t.path)
	if err != nil {
		a.printer.Warnf("%s  %s %q: não consegui ler o arquivo: %v", ts, t.typ, t.id, err)
		return
	}
	content := string(raw)

	action, err := a.updateExisting(ctx, client, t, content)
	switch {
	case err != nil:
		a.printer.Warnf("%s  %s %q: %s", ts, t.typ, t.id, output.AsError(mapFluigError(err)).Message)
	case action == "missing":
		a.printer.Warnf("%s  %s %q não existe no servidor — o watch não cria; use: fluigcli %s export %s%s",
			ts, t.typ, t.id, t.typ, relTo(a.ProjectRoot(), t.path), createFlagHint(t.typ))
	case action == "unchanged":
		a.printer.Infof("· %s  %s %q sem mudança — nada a publicar", ts, t.typ, t.id)
	default:
		a.printer.Successf("✓ %s  %s %q publicado", ts, t.typ, t.id)
	}
}

// createFlagHint devolve a flag de criação do comando export do tipo (só o
// dataset export exige --new; eventos e mecanismos criam direto).
func createFlagHint(typ string) string {
	if typ == "dataset" {
		return " --new"
	}
	return ""
}

// updateExisting atualiza o artefato no servidor. Retorna "updated",
// "unchanged" (conteúdo idêntico, nada gravado) ou "missing" (não existe —
// o watch não cria).
func (a *App) updateExisting(ctx context.Context, client *fluig.Client, t diffTarget, content string) (string, error) {
	switch t.typ {
	case "dataset":
		loaded, err := client.LoadDataset(ctx, t.id)
		if errors.Is(err, fluig.ErrNotFound) {
			return "missing", nil
		}
		if err != nil {
			return "", err
		}
		if normalizeEOL(loaded.Impl) == normalizeEOL(content) {
			return "unchanged", nil
		}
		return "updated", client.UpdateDataset(ctx, loaded, content)

	case "event":
		existing, err := client.ListGlobalEvents(ctx)
		if err != nil {
			return "", err
		}
		for i := range existing {
			if existing[i].ID != t.id {
				continue
			}
			if normalizeEOL(existing[i].Code) == normalizeEOL(content) {
				return "unchanged", nil
			}
			existing[i].Code = content
			return "updated", client.SaveGlobalEvents(ctx, existing)
		}
		return "missing", nil

	case "mechanism":
		mechs, err := client.ListMechanisms(ctx)
		if err != nil {
			return "", err
		}
		for i := range mechs {
			if mechs[i].ID != t.id {
				continue
			}
			if normalizeEOL(mechs[i].Code) == normalizeEOL(content) {
				return "unchanged", nil
			}
			return "updated", client.UpdateMechanism(ctx, &mechs[i], content)
		}
		return "missing", nil
	}
	return "", output.Genericf("tipo de artefato desconhecido: %q", t.typ)
}
