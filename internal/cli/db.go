package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newDbCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Consultas SQL de diagnóstico no banco do Fluig (via fluigcliHelper)",
		Long: "Executa SQL de LEITURA contra um datasource JNDI do servidor de aplicação,\n" +
			"sem acesso direto ao banco. Requer o fluigcliHelper >= 0.6.0 (instale ou\n" +
			"atualize com: fluigcli server install-helper).\n\n" +
			"O grupo é para diagnóstico: conferir permissões do login do datasource,\n" +
			"validar um SQL antes de colar num dataset, checar se um objeto existe.\n" +
			"Somente leitura. O servidor recusa tudo que não for SELECT ou WITH.",
	}
	cmd.AddCommand(newDbQueryCmd(app))
	cmd.AddCommand(newDbGrantsCmd(app))
	cmd.AddCommand(newDbDatasourcesCmd(app))
	return cmd
}

// --- db query ---

func newDbQueryCmd(app *App) *cobra.Command {
	var (
		jndi          string
		params        []string
		maxRows       int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "Executa um SELECT de diagnóstico e mostra o resultado em tabela",
		Long: "Executa uma consulta de LEITURA (SELECT ou WITH) contra um datasource do\n" +
			"servidor e mostra o resultado em tabela. É SQL cru de diagnóstico — não é o\n" +
			"mesmo que `dataset query` (que executa um dataset cadastrado).\n\n" +
			"O datasource default é " + fluig.DefaultDatasource + " (o banco do Fluig). Use\n" +
			"--jndi para apontar outro (veja os disponíveis com: fluigcli db datasources).\n" +
			"Use --param para os `?` do SQL, na ordem. O servidor recusa escrita.\n\n" +
			"Exemplos:\n" +
			"  fluigcli db query \"select suser_sname() as login, db_name() as db\"\n" +
			"  fluigcli db query \"select has_perms_by_name(?, 'OBJECT', 'INSERT') as ok\" --param dbo.MINHA_TABELA\n" +
			"  fluigcli db query \"select top 10 * from wcm_application\" --jndi /jdbc/TotvsRM",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			res, err := client.DbQuery(ctx, fluig.DbQueryOptions{
				JNDI: jndi, SQL: args[0], Params: params, MaxRows: maxRows,
			})
			if err != nil {
				return mapFluigError(err)
			}
			if len(res.Rows) == 0 {
				p.Infof("Consulta sem linhas.")
			} else {
				headers := make([]string, len(res.Columns))
				for i, c := range res.Columns {
					headers[i] = c.Name
				}
				rows := make([][]string, 0, len(res.Rows))
				for _, r := range res.Rows {
					cells := make([]string, len(r))
					for i, v := range r {
						if v == nil {
							cells[i] = "(null)"
						} else {
							cells[i] = *v
						}
					}
					rows = append(rows, cells)
				}
				p.Table(output.Table{
					Headers: headers,
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			if res.Truncated {
				p.Warnf("resultado truncado no limite de linhas — aumente com --max-rows")
			}
			p.Done(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&jndi, "jndi", "", "datasource JNDI (default "+fluig.DefaultDatasource+")")
	cmd.Flags().StringArrayVar(&params, "param", nil, "valor de um `?` do SQL, na ordem (repetível)")
	cmd.Flags().IntVar(&maxRows, "max-rows", 0, "teto de linhas (0 = default do servidor)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- db grants ---

func newDbGrantsCmd(app *App) *cobra.Command {
	var (
		jndi          string
		perms         []string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "grants <table>...",
		Short: "Confere as permissões do login do datasource nas tabelas (preflight)",
		Long: "Confere, antes de rodar, se o login do datasource tem as permissões que um\n" +
			"dataset de escrita precisa. Para cada tabela, checa SELECT, INSERT, UPDATE e\n" +
			"DELETE via HAS_PERMS_BY_NAME e mostra o veredicto em tabela.\n\n" +
			"O objetivo é evitar a surpresa em runtime: um grant faltante (ex.: INSERT\n" +
			"para o login `fluig`) hoje só aparece como erro de SQL quando o dataset roda.\n" +
			"Aqui o problema aparece antes, com o login e o banco em destaque.\n\n" +
			"É SQL Server. Use --perm para um subconjunto e --jndi para outro datasource\n" +
			"(veja os disponíveis com: fluigcli db datasources). Se faltar qualquer\n" +
			"permissão, ou se um objeto não existir, o comando termina com código 6.\n\n" +
			"Exemplos:\n" +
			"  fluigcli db grants dbo.ZMDFLANFLUIG\n" +
			"  fluigcli db grants dbo.ZMDFLANFLUIG dbo.WCM_APPLICATION --perm INSERT,UPDATE\n" +
			"  fluigcli db grants dbo.MINHA_TABELA --jndi /jdbc/TotvsRM",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			var chosen []string
			for _, raw := range splitCSV(perms) {
				up := strings.ToUpper(raw)
				if !fluig.IsGrantPerm(up) {
					return output.Usagef("permissão inválida %q (use SELECT, INSERT, UPDATE ou DELETE)", raw)
				}
				chosen = append(chosen, up)
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			res, err := client.DbGrants(ctx, fluig.GrantsOptions{JNDI: jndi, Tables: args, Perms: chosen})
			if err != nil {
				return mapFluigError(err)
			}
			p.Infof("Login do datasource: %s  ·  banco: %s", orDash(res.Login), orDash(res.Database))
			headers := append([]string{"Tabela"}, res.Perms...)
			rows := make([][]string, 0, len(res.Tables))
			for _, tg := range res.Tables {
				cells := make([]string, 0, len(res.Perms)+1)
				cells = append(cells, tg.Table)
				for _, perm := range res.Perms {
					cells = append(cells, grantMark(tg.Grants[perm]))
				}
				rows = append(rows, cells)
			}
			p.Table(output.Table{
				Headers: headers,
				Rows:    rows,
				Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
					if col == 0 || row >= len(res.Tables) {
						return padded
					}
					switch v := res.Tables[row].Grants[res.Perms[col-1]]; {
					case v == nil:
						return output.Yellow(padded)
					case *v:
						return output.Green(padded)
					default:
						return output.Red(padded)
					}
				}),
			})
			if res.OK {
				p.Done(res)
				return nil
			}
			missingObject := false
			for _, tg := range res.Tables {
				if !tg.Exists {
					missingObject = true
					break
				}
			}
			msg := "faltam permissões de banco (veja a tabela)"
			if missingObject {
				// O caso comum de tudo `?` é o datasource errado (o default
				// /jdbc/AppDS é o banco FLUIG; tabelas do RM ficam no TOTVSRM).
				msg = "um ou mais objetos não existem neste datasource (marcador `?`) — confira o --jndi " +
					"(o default é /jdbc/AppDS, banco FLUIG; tabelas do RM ficam em /jdbc/TotvsRM). " +
					"Veja os disponíveis com: fluigcli db datasources"
			}
			p.FailData(res, output.CodePartial, msg)
			return output.Partialf("%s", msg)
		},
	}
	cmd.Flags().StringVar(&jndi, "jndi", "", "datasource JNDI (default "+fluig.DefaultDatasource+")")
	cmd.Flags().StringSliceVar(&perms, "perm", nil, "permissões a checar (default SELECT,INSERT,UPDATE,DELETE)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// grantMark traduz o veredicto de uma permissão para o marcador da tabela:
// concedida = ✓, negada = ✗, indeterminada (objeto inexistente/não verificável) = ?.
func grantMark(allowed *bool) string {
	switch {
	case allowed == nil:
		return "?"
	case *allowed:
		return "✓"
	default:
		return "✗"
	}
}

// orDash devolve "-" para texto vazio (usado no cabeçalho login/banco).
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// --- db datasources ---

func newDbDatasourcesCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "datasources",
		Short: "Lista os datasources JNDI disponíveis no servidor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			ds, err := client.ListDatasources(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			if len(ds) == 0 {
				p.Infof("Nenhum datasource enumerado (o servidor pode não permitir a listagem).\n" +
					"Passe o nome direto com --jndi no db query.")
			} else {
				rows := make([][]string, 0, len(ds))
				for _, name := range ds {
					rows = append(rows, []string{name})
				}
				// Padrão de listagem (ver CLAUDE.md): o datasource default do
				// db query em verde.
				p.Table(output.Table{
					Headers: []string{"Datasource (JNDI)"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if ds[row] == fluig.DefaultDatasource {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"datasources": ds})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
