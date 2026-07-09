package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newWidgetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "widget",
		Short: "Lista, importa e exporta widgets (export = local → servidor; deploy nativo)",
	}
	cmd.AddCommand(newWidgetListCmd(app))
	cmd.AddCommand(newWidgetImportCmd(app))
	cmd.AddCommand(newWidgetExportCmd(app))
	return cmd
}

// --- widget list (via fluiggersWidget) ---

func newWidgetListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os widgets do servidor (via fluiggersWidget)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			widgets, err := client.ListWidgets(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			rows := make([][]string, 0, len(widgets))
			for _, w := range widgets {
				rows = append(rows, []string{w.Code, w.Title})
			}
			if len(widgets) == 0 {
				p.Infof("Nenhum widget no servidor.")
			} else {
				// Padrão de listagem (ver CLAUDE.md).
				p.Table(output.Table{
					Headers: []string{"Código", "Título"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"widgets": widgets})
			return nil
		},
	}
}

// --- widget import (servidor → local, via fluiggersWidget) ---

func newWidgetImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <code>... | --all",
		Short: "Baixa widgets do servidor para o projeto local (servidor → local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais códigos de widget ou use --all")
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
			widgets, err := client.ListWidgets(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byCode := make(map[string]fluig.Widget, len(widgets))
			for _, w := range widgets {
				byCode[w.Code] = w
			}

			codes := args
			if all {
				codes = codes[:0]
				for _, w := range widgets {
					codes = append(codes, w.Code)
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, code := range codes {
				w, ok := byCode[code]
				if !ok {
					failures++
					lastErr = output.NotFoundf("widget %q não encontrado no servidor", code)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("widget %q: não encontrado", code)
					continue
				}
				if err := app.importOneWidget(ctx, client, root, w); err != nil {
					failures++
					lastErr = mapFluigError(err)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("widget %q: %s", code, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: code, Action: "imported", Success: true})
				p.Successf("widget %q importado em wcm/widget/%s", code, code)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(codes))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os widgets do servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// importOneWidget baixa o .war do widget e desempacota no layout local.
func (a *App) importOneWidget(ctx context.Context, client *fluig.Client, root string, w fluig.Widget) error {
	war, err := client.DownloadWidget(ctx, w.Filename)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(war), int64(len(war)))
	if err != nil {
		return err
	}
	// O código do widget vem do servidor — confina a pasta em wcm/widget/.
	widgetDir, err := project.SafeJoin(filepath.Join(root, project.WidgetsDir), w.Code)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := project.MapWidgetEntryToLocal(f.Name)
		if rel == "" {
			continue // entrada fora do mapa (ignora)
		}
		dst, err := project.SafeJoin(widgetDir, rel) // defesa extra contra zip-slip
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// --- widget export (local → servidor, deploy nativo) ---

func newWidgetExportCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "export <NomeWidget>",
		Short: "Empacota e publica um widget no servidor (deploy nativo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			name := args[0]
			widgetDir := project.WidgetDir(root, name)
			if info, err := os.Stat(widgetDir); err != nil || !info.IsDir() {
				return output.NotFoundf("widget %q não encontrado em %s", name, project.WidgetsDir)
			}

			refs, err := project.CollectWidgetWARFiles(widgetDir)
			if err != nil {
				return err
			}
			if len(refs) == 0 {
				return output.Usagef("nada para empacotar em %s (esperado src/main/...)", widgetDir)
			}
			warFiles := make([]fluig.WARFile, 0, len(refs))
			for _, ref := range refs {
				content, err := os.ReadFile(ref.LocalPath)
				if err != nil {
					return err
				}
				warFiles = append(warFiles, fluig.WARFile{Name: ref.WARPath, Content: content})
			}
			war, err := fluig.BuildWAR(warFiles)
			if err != nil {
				return err
			}

			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar a widget")
			if err != nil {
				return err
			}
			if err := client.UploadWidgetWAR(ctx, name+".war", war); err != nil {
				return mapFluigError(err)
			}
			p.Successf("widget %q enviado (%d arquivos, %d KB). A instalação é assíncrona no servidor.", name, len(warFiles), len(war)/1024)
			p.Done(map[string]any{"widget": name, "files": len(warFiles)})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
