package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newEventCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Lista, importa, exporta e exclui eventos globais (import = servidor→local; export = local→servidor)",
	}
	cmd.AddCommand(newEventListCmd(app))
	cmd.AddCommand(newEventImportCmd(app))
	cmd.AddCommand(newEventExportCmd(app))
	cmd.AddCommand(newEventDeleteCmd(app))
	return cmd
}

// --- event list ---

func newEventListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os eventos globais do servidor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			events, err := client.ListGlobalEvents(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			ids := make([]string, 0, len(events))
			rows := make([][]string, 0, len(events))
			for _, e := range events {
				ids = append(ids, e.ID)
				lines := strconv.Itoa(strings.Count(strings.TrimRight(e.Code, "\n"), "\n") + 1)
				rows = append(rows, []string{e.ID, lines})
			}
			if len(events) == 0 {
				p.Infof("Nenhum evento global no servidor.")
			} else {
				// Padrão de listagem (ver CLAUDE.md).
				p.Table(output.Table{
					Headers: []string{"ID", "Linhas"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"events": ids})
			return nil
		},
	}
}

// --- event import (servidor → local) ---

func newEventImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <id>... | --all",
		Short: "Baixa eventos globais do servidor para arquivos locais (servidor → local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais ids de evento ou use --all")
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}

			events, err := client.ListGlobalEvents(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byID := make(map[string]fluig.GlobalEvent, len(events))
			for _, e := range events {
				byID[e.ID] = e
			}

			ids := args
			if all {
				ids = nil
				for _, e := range events {
					ids = append(ids, e.ID)
				}
				if len(ids) == 0 {
					p.Infof("Nenhum evento global no servidor.")
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, id := range ids {
				ev, ok := byID[id]
				if !ok {
					failures++
					lastErr = output.NotFoundf("evento global %q não encontrado no servidor", id)
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("evento %q: não encontrado", id)
					continue
				}
				action, werr := writeArtifactFile(app, root, project.EventsDirName, id, ev.Code)
				if werr != nil {
					failures++
					lastErr = werr
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(werr).Message})
					p.Warnf("evento %q: %s", id, output.AsError(werr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: action, Success: true})
				p.Successf("evento %q %s", id, action)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(ids))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os eventos globais do servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- event export (local → servidor) ---

func newEventExportCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "export <file>...",
		Short: "Envia eventos globais locais para o servidor (local → servidor)",
		Long: "Envia eventos globais locais para o servidor.\n\n" +
			"O Fluig salva a lista completa de eventos de uma vez; a CLI busca a lista\n" +
			"atual do servidor e sobrepõe apenas os eventos informados, então exportar\n" +
			"um evento NÃO apaga os demais.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais arquivos .js de evento")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar eventos globais")
			if err != nil {
				return err
			}

			// Parte da lista atual do servidor para não apagar eventos não tocados.
			existing, err := client.ListGlobalEvents(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byID := make(map[string]fluig.GlobalEvent, len(existing))
			order := make([]string, 0, len(existing))
			for _, e := range existing {
				byID[e.ID] = e
				order = append(order, e.ID)
			}

			var results []itemResult
			failures := 0
			for _, file := range args {
				id := project.ArtifactName(file)
				content, rerr := os.ReadFile(file)
				if rerr != nil {
					failures++
					msg := rerr.Error()
					if os.IsNotExist(rerr) {
						msg = fmt.Sprintf("arquivo %q não encontrado", file)
					}
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: msg})
					p.Warnf("evento %q: %s", id, msg)
					continue
				}
				action := "updated"
				if _, ok := byID[id]; !ok {
					action = "created"
					order = append(order, id)
				}
				byID[id] = fluig.GlobalEvent{ID: id, Code: string(content)}
				results = append(results, itemResult{ID: id, Action: action, Success: true})
			}

			// Uma única gravação com o conjunto completo mesclado.
			merged := make([]fluig.GlobalEvent, 0, len(order))
			for _, id := range order {
				merged = append(merged, byID[id])
			}
			if err := client.SaveGlobalEvents(ctx, merged); err != nil {
				return mapFluigError(err)
			}
			for _, r := range results {
				if r.Success {
					p.Successf("evento %q %s", r.ID, r.Action)
				}
			}
			return finishBatch(p, nil, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- event delete ---

func newEventDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <id>...",
		Short: "Exclui eventos globais no servidor",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais ids de evento a excluir")
			}
			if err := app.confirm(fmt.Sprintf("Excluir %d evento(s) global(is) no servidor?", len(args))); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir eventos globais")
			if err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, id := range args {
				if derr := client.DeleteGlobalEvent(ctx, id); derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("evento %q: %s", id, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: "deleted", Success: true})
				p.Successf("evento %q excluído", id)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// writeArtifactFile grava o conteúdo de um artefato no lugar certo: sobrescreve
// o arquivo existente (glob) ou cria em <root>/<subdir>/<id>.js.
func writeArtifactFile(app *App, root, subdir, id, content string) (action string, err error) {
	path, matches, err := project.FindArtifactFile(root, subdir, id)
	if err != nil {
		return "failed", err
	}
	action = "updated"
	if path == "" {
		path, err = project.DefaultArtifactPath(root, subdir, id)
		if err != nil {
			return "failed", err
		}
		action = "created"
	} else if len(matches) > 1 {
		app.printer.Warnf("%q: %d arquivos com esse nome; sobrescrevendo %s", id, len(matches), path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "failed", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "failed", err
	}
	return action, nil
}
