package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newDocumentCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "document",
		Short: "GED: navegar, baixar e publicar documentos",
	}
	cmd.AddCommand(newDocumentListCmd(app))
	cmd.AddCommand(newDocumentDownloadCmd(app))
	cmd.AddCommand(newDocumentUploadCmd(app))
	cmd.AddCommand(newDocumentMkdirCmd(app))
	cmd.AddCommand(newDocumentDeleteCmd(app))
	return cmd
}

// gedTypeLabel traduz o tipo para a tabela humana.
func gedTypeLabel(t string) string {
	switch t {
	case "folder":
		return "pasta"
	case "file":
		return "arquivo"
	case "article":
		return "artigo"
	default:
		return t
	}
}

// --- document list ---

func newDocumentListCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "list [<folderId>]",
		Short: "Lista as pastas raiz do GED ou o conteúdo de uma pasta",
		Long: "Sem argumento, lista as pastas raiz do GED. Com um folderId, lista o\n" +
			"conteúdo da pasta: subpastas, arquivos e artigos, com id, versão,\n" +
			"tamanho e autor. Navegue descendo pelos ids.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}

			// Sem argumento: pastas raiz (SOAP ECMFolderService — não há rota
			// REST para as raízes).
			if len(args) == 0 {
				folders, err := client.ListGEDFolders(ctx, 0)
				if err != nil {
					return mapFluigError(err)
				}
				if len(folders) == 0 {
					p.Infof("Nenhuma pasta raiz visível para o seu usuário no GED.")
				} else {
					rows := make([][]string, 0, len(folders))
					for _, f := range folders {
						rows = append(rows, []string{strconv.Itoa(f.ID), f.Name})
					}
					p.Table(output.Table{
						Headers: []string{"ID", "Pasta"},
						Rows:    rows,
						Style:   output.BoldHeaderStyle(nil),
					})
				}
				p.Done(map[string]any{"folders": folders})
				return nil
			}

			folderID, err := strconv.Atoi(args[0])
			if err != nil || folderID <= 0 {
				return output.Usagef("folderId inválido %q", args[0])
			}
			docs, err := client.ListGEDDocuments(ctx, folderID)
			if err != nil {
				return mapFluigError(err)
			}
			if len(docs) == 0 {
				p.Infof("A pasta %d está vazia (ou nada é visível para o seu usuário).", folderID)
			} else {
				rows := make([][]string, 0, len(docs))
				for _, d := range docs {
					size := ""
					if d.Type == "file" {
						size = fmt.Sprintf("%.1f KB", d.SizeMB*1024)
					}
					rows = append(rows, []string{
						strconv.FormatInt(d.ID, 10), gedTypeLabel(d.Type), d.Description,
						strconv.Itoa(d.Version), size, d.Publisher, fmtRequestTime(d.UpdatedAt),
					})
				}
				// Padrão de listagem (ver CLAUDE.md): pastas em verde — são o
				// próximo passo da navegação.
				p.Table(output.Table{
					Headers: []string{"ID", "Tipo", "Nome", "Versão", "Tamanho", "Autor", "Modificado"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 1 && docs[row].Type == "folder" {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"items": docs})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- document download ---

func newDocumentDownloadCmd(app *App) *cobra.Command {
	var (
		dir           string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "download <id>...",
		Short: "Baixa documentos do GED pelo id",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais ids de documento")
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, arg := range args {
				id, aerr := strconv.Atoi(arg)
				if aerr != nil || id <= 0 {
					return output.Usagef("id de documento inválido %q", arg)
				}
				name := arg
				content, derr := func() ([]byte, error) {
					info, gerr := client.GetGEDDocument(ctx, id)
					if gerr != nil {
						return nil, gerr
					}
					if info.Description != "" {
						name = info.Description
					}
					return client.DownloadGEDDocument(ctx, id)
				}()
				var path string
				if derr == nil {
					if path, derr = project.SafeJoin(dir, name); derr == nil {
						derr = os.WriteFile(path, content, 0o644)
					}
				}
				if derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: arg, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("documento %s: %s", arg, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: name, Action: "downloaded", Success: true})
				p.Successf("documento %d salvo como %q (%d bytes)", id, name, len(content))
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "diretório de destino dos downloads")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- document upload ---

func newDocumentUploadCmd(app *App) *cobra.Command {
	var (
		folder        int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "upload <file>... --folder <id>",
		Short: "Publica arquivos locais numa pasta do GED",
		Long: "Publica arquivos numa pasta do GED (upload + publish em uma etapa).\n" +
			"Descubra o id da pasta navegando com document list.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais arquivos")
			}
			if folder <= 0 {
				return output.Usagef("informe a pasta de destino com --folder <id> (veja document list)")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar documentos no GED")
			if err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, file := range args {
				name := filepath.Base(file)
				content, rerr := os.ReadFile(file)
				if rerr != nil {
					failures++
					msg := rerr.Error()
					if os.IsNotExist(rerr) {
						msg = fmt.Sprintf("arquivo %q não encontrado", file)
					}
					lastErr = output.NotFoundf("%s", msg)
					results = append(results, itemResult{ID: name, Action: "failed", Success: false, Error: msg})
					p.Warnf("%s", msg)
					continue
				}
				info, uerr := client.UploadGEDDocument(ctx, folder, name, content)
				if uerr != nil {
					failures++
					lastErr = mapFluigError(uerr)
					results = append(results, itemResult{ID: name, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("arquivo %q: %s", name, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: strconv.FormatInt(info.ID, 10), Action: "published", Success: true})
				p.Successf("documento %d publicado (%q na pasta %d)", info.ID, name, folder)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().IntVar(&folder, "folder", 0, "id da pasta de destino no GED")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- document mkdir ---

func newDocumentMkdirCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "mkdir <parentId> <nome>",
		Short: "Cria uma pasta no GED",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			parentID, err := strconv.Atoi(args[0])
			if err != nil || parentID <= 0 {
				return output.Usagef("parentId inválido %q (veja document list)", args[0])
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar pasta no GED")
			if err != nil {
				return err
			}
			info, err := client.CreateGEDFolder(ctx, parentID, args[1])
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("pasta %d criada (%q dentro de %d)", info.ID, args[1], parentID)
			p.Done(map[string]any{"folderId": info.ID, "parentId": parentID, "name": args[1]})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- document delete ---

func newDocumentDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <id>...",
		Short: "Envia documentos ou pastas do GED para a lixeira",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais ids")
			}
			if err := app.confirm(fmt.Sprintf("Enviar %d item(ns) do GED para a lixeira?", len(args))); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir documentos do GED")
			if err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, arg := range args {
				id, aerr := strconv.Atoi(arg)
				if aerr != nil || id <= 0 {
					return output.Usagef("id inválido %q", arg)
				}
				if derr := client.DeleteGEDDocument(ctx, id); derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: arg, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("documento %s: %s", arg, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: arg, Action: "deleted", Success: true})
				p.Successf("documento %s enviado para a lixeira", arg)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
