package cli

import (
	"context"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newUserCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Usuários da plataforma: consulta (requer privilégio administrativo)",
	}
	cmd.AddCommand(newUserListCmd(app))
	cmd.AddCommand(newUserShowCmd(app))
	return cmd
}

// --- user list ---

func newUserListCmd(app *App) *cobra.Command {
	var (
		search        string
		role          string
		inactive      bool
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os usuários da plataforma",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			users, err := client.ListAdminUsers(ctx, fluig.AdminUserFilter{
				Pattern:      search,
				Role:         role,
				ShowInactive: inactive,
				Limit:        limit,
			})
			if err != nil {
				return mapFluigError(err)
			}
			if len(users) == 0 {
				p.Infof("Nenhum usuário encontrado com esses filtros.")
			} else {
				rows := make([][]string, 0, len(users))
				for _, u := range users {
					rows = append(rows, []string{u.Login, u.FullName, u.Email, u.State})
				}
				// Padrão de listagem (ver CLAUDE.md): ativos em verde.
				p.Table(output.Table{
					Headers: []string{"Login", "Nome", "E-mail", "Estado"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 3 && users[row].State == "ACTIVE" {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"users": users})
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "busca textual (login, nome ou e-mail)")
	cmd.Flags().StringVar(&role, "role", "", "filtra pelos usuários com o papel")
	cmd.Flags().BoolVar(&inactive, "inactive", false, "inclui usuários desativados")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de usuários (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- user show ---

func newUserShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <login>",
		Short: "Mostra um usuário com os papéis e grupos",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			u, err := client.GetAdminUser(ctx, args[0])
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("%s (%s) — %s", u.FullName, u.Login, u.State)
			p.Successf("E-mail: %s", u.Email)
			p.Successf("Código: %s", u.Code)
			if u.UpdatedAt != nil {
				p.Successf("Atualizado em: %s", u.UpdatedAt.Format("2006-01-02 15:04"))
			}
			if len(u.Roles) > 0 {
				roles := append([]string(nil), u.Roles...)
				sort.Strings(roles)
				p.Successf("Papéis (%d): %s", len(roles), strings.Join(roles, ", "))
			}
			if len(u.Groups) > 0 {
				groups := append([]string(nil), u.Groups...)
				sort.Strings(groups)
				p.Successf("Grupos (%d): %s", len(groups), strings.Join(groups, ", "))
			}
			p.Done(map[string]any{"user": u})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
