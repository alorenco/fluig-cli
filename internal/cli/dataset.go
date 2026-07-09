package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newDatasetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dataset",
		Short: "Lista, importa, exporta e consulta datasets (import = servidor→local; export = local→servidor)",
	}
	cmd.AddCommand(newDatasetListCmd(app))
	cmd.AddCommand(newDatasetImportCmd(app))
	cmd.AddCommand(newDatasetExportCmd(app))
	cmd.AddCommand(newDatasetQueryCmd(app))
	return cmd
}

// itemResult é uma linha de resultado de operação em lote.
type itemResult struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// projectRootForFiles devolve a raiz do projeto para leitura/escrita de
// arquivos; se nenhuma foi descoberta, usa o diretório atual.
func (a *App) projectRootForFiles() (string, error) {
	if root := a.ProjectRoot(); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", output.Genericf("não foi possível determinar o diretório atual: %v", err)
	}
	return cwd, nil
}

// --- dataset list ---

func newDatasetListCmd(app *App) *cobra.Command {
	var (
		customOnly bool
		search     string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os datasets do servidor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			datasets, err := client.ListDatasets(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			needle := strings.ToLower(search)
			shown := datasets[:0]
			rows := make([][]string, 0, len(datasets))
			for _, d := range datasets {
				if customOnly && !d.Custom {
					continue
				}
				if needle != "" && !strings.Contains(strings.ToLower(d.ID), needle) &&
					!strings.Contains(strings.ToLower(d.Description), needle) {
					continue
				}
				shown = append(shown, d)
				ativo := "não"
				if d.Active {
					ativo = "sim"
				}
				rows = append(rows, []string{d.ID, d.Type, d.Description, ativo})
			}
			if len(shown) == 0 {
				if search != "" {
					p.Infof("Nenhum dataset casa com %q.", search)
				} else {
					p.Infof("Nenhum dataset encontrado no servidor.")
				}
			} else {
				// Padrão de listagem (ver CLAUDE.md): tabela com cabeçalho em
				// negrito; CUSTOM em verde — são os datasets que a CLI edita.
				p.Table(output.Table{
					Headers: []string{"ID", "Tipo", "Descrição", "Ativo"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 1 && shown[row].Custom {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"datasets": shown})
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "filtra por texto no id ou na descrição")
	cmd.Flags().BoolVar(&customOnly, "custom-only", false, "lista apenas datasets customizados")
	return cmd
}

// --- dataset import (servidor → local) ---

func newDatasetImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <id>... | --all",
		Short: "Baixa datasets do servidor para arquivos locais (servidor → local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais ids de dataset ou use --all")
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

			ids := args
			if all {
				datasets, err := client.ListDatasets(ctx)
				if err != nil {
					return mapFluigError(err)
				}
				ids = nil
				for _, d := range datasets {
					if d.Custom {
						ids = append(ids, d.ID)
					}
				}
				if len(ids) == 0 {
					p.Infof("Nenhum dataset customizado encontrado no servidor.")
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, id := range ids {
				action, err := app.importOneDataset(ctx, client, root, id)
				if err != nil {
					failures++
					lastErr = mapFluigError(err)
					results = append(results, itemResult{ID: id, Action: action, Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("dataset %q: %s", id, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: action, Success: true})
				p.Successf("dataset %q %s", id, action)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(ids))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os datasets customizados do servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// importOneDataset baixa um dataset e grava no arquivo local certo: sobrescreve
// o existente (glob datasets/**/<id>.js) ou cria datasets/<id>.js.
func (a *App) importOneDataset(ctx context.Context, client *fluig.Client, root, id string) (action string, err error) {
	ds, err := client.LoadDataset(ctx, id)
	if err != nil {
		return "failed", err
	}
	path, matches, err := project.FindArtifactFile(root, project.DatasetsDirName, id)
	if err != nil {
		return "failed", err
	}
	action = "updated"
	if path == "" {
		path, err = project.DefaultArtifactPath(root, project.DatasetsDirName, id)
		if err != nil {
			return "failed", err
		}
		action = "created"
	} else if len(matches) > 1 {
		a.printer.Warnf("dataset %q: %d arquivos com esse nome; sobrescrevendo %s", id, len(matches), path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "failed", err
	}
	if err := os.WriteFile(path, []byte(ds.Impl), 0o644); err != nil {
		return "failed", err
	}
	return action, nil
}

// --- dataset export (local → servidor) ---

func newDatasetExportCmd(app *App) *cobra.Command {
	var (
		description   string
		markNew       bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "export <file>...",
		Short: "Envia datasets locais para o servidor (local → servidor)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais arquivos .js de dataset")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar datasets")
			if err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, file := range args {
				id := project.ArtifactName(file)
				action, err := app.exportOneDataset(ctx, client, file, id, description, markNew)
				if err != nil {
					failures++
					lastErr = mapFluigError(err)
					results = append(results, itemResult{ID: id, Action: action, Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("dataset %q: %s", id, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: action, Success: true})
				p.Successf("dataset %q %s", id, action)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "descrição ao criar um dataset novo (default: o nome)")
	cmd.Flags().BoolVar(&markNew, "new", false, "confirma a criação de um dataset que ainda não existe no servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// exportOneDataset envia um arquivo local: atualiza se o dataset já existe,
// cria caso contrário (criação exige --new ou confirmação).
func (a *App) exportOneDataset(ctx context.Context, client *fluig.Client, file, id, description string, markNew bool) (action string, err error) {
	content, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return "failed", output.NotFoundf("arquivo %q não encontrado", file)
		}
		return "failed", err
	}

	loaded, err := client.LoadDataset(ctx, id)
	switch {
	case err == nil:
		if uerr := client.UpdateDataset(ctx, loaded, string(content)); uerr != nil {
			return "failed", uerr
		}
		return "updated", nil
	case errors.Is(err, fluig.ErrNotFound):
		if !a.confirmCreate(id, markNew) {
			return "failed", output.Usagef("dataset %q não existe no servidor; use --new para criá-lo", id)
		}
		desc := description
		if desc == "" {
			desc = id
		}
		if cerr := client.CreateDataset(ctx, id, desc, string(content)); cerr != nil {
			return "failed", cerr
		}
		return "created", nil
	default:
		return "failed", err
	}
}

// confirmCreate decide se um dataset novo pode ser criado: --new pula a
// pergunta; em modo interativo, pergunta; em não-interativo sem --new, recusa.
func (a *App) confirmCreate(id string, markNew bool) bool {
	if markNew {
		return true
	}
	if !a.Interactive() {
		return false
	}
	ok, err := promptYesNo(fmt.Sprintf("Dataset %q não existe no servidor. Criar?", id), false)
	return err == nil && ok
}

// --- dataset query ---

func newDatasetQueryCmd(app *App) *cobra.Command {
	var (
		fields        []string
		constraints   []string
		order         []string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "query <id>",
		Short: "Consulta os dados de um dataset (nativo, REST v2)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}

			cons, err := parseConstraints(constraints)
			if err != nil {
				return err
			}
			// A API aceita um único campo de ordenação (sufixo _ASC/_DESC
			// opcional) — mais de um faz o servidor devolver resposta nula.
			orderFields := splitCSV(order)
			if len(orderFields) > 1 {
				return output.Usagef("a ordenação aceita um único campo (recebi %d); use --order campo ou campo_DESC", len(orderFields))
			}
			orderBy := ""
			if len(orderFields) == 1 {
				orderBy = orderFields[0]
			}
			res, err := client.QueryDataset(ctx, args[0], fluig.DatasetQuery{
				Fields:      splitCSV(fields),
				Constraints: cons,
				OrderBy:     orderBy,
				Limit:       limit,
			})
			if err != nil {
				return mapFluigError(err)
			}

			// Impressão humana: cabeçalho + linhas separadas por tab.
			if len(res.Columns) > 0 {
				p.Successf("%s", strings.Join(res.Columns, "\t"))
			}
			jsonRows := make([]map[string]any, 0, len(res.Rows))
			for _, r := range res.Rows {
				cells := make([]string, len(res.Columns))
				obj := make(map[string]any, len(res.Columns))
				for i, col := range res.Columns {
					if v, ok := r[col]; ok && v != nil {
						cells[i] = *v
						obj[col] = *v
					} else {
						obj[col] = nil
					}
				}
				p.Successf("%s", strings.Join(cells, "\t"))
				jsonRows = append(jsonRows, obj)
			}
			p.Done(map[string]any{"columns": res.Columns, "rows": jsonRows, "count": len(jsonRows)})
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "campos a retornar (separados por vírgula)")
	cmd.Flags().StringArrayVar(&constraints, "constraint", nil, "filtro campo=valor (pode repetir)")
	cmd.Flags().StringSliceVar(&order, "order", nil, "campo de ordenação (um só; sufixo _DESC inverte)")
	cmd.Flags().IntVar(&limit, "limit", 0, "número máximo de linhas (0 = sem limite)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// parseConstraints converte "campo=valor" em filtros de igualdade.
func parseConstraints(raw []string) ([]fluig.DatasetConstraint, error) {
	var out []fluig.DatasetConstraint
	for _, c := range raw {
		k, v, ok := strings.Cut(c, "=")
		if !ok || k == "" {
			return nil, output.Usagef("constraint inválida %q (use campo=valor)", c)
		}
		out = append(out, fluig.DatasetConstraint{Field: k, Initial: v, Final: v})
	}
	return out, nil
}

func splitCSV(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// finishBatch decide o exit code de uma operação em lote:
// tudo ok → 0; falha em alvo único → erro real; falhas em lote → 6.
func finishBatch(p *output.Printer, single error, data map[string]any, failures, total int) error {
	if failures == 0 {
		p.Done(data)
		return nil
	}
	if total == 1 && single != nil {
		return single
	}
	p.Partial(data)
	return output.Partialf("%d de %d itens falharam", failures, total)
}
