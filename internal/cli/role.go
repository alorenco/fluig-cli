package cli

import (
	"context"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newRoleCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Papéis da plataforma: consulta, gestão e usuários (requer privilégio administrativo)",
	}
	cmd.AddCommand(newRoleListCmd(app))
	cmd.AddCommand(newRoleShowCmd(app))
	cmd.AddCommand(newRoleCreateCmd(app))
	cmd.AddCommand(newRoleUpdateCmd(app))
	cmd.AddCommand(newRoleDeleteCmd(app))
	cmd.AddCommand(newRoleUsersCmd(app))
	cmd.AddCommand(newRoleMemberCmd(app, true))
	cmd.AddCommand(newRoleMemberCmd(app, false))
	return cmd
}

// --- role list ---

func newRoleListCmd(app *App) *cobra.Command {
	var (
		search        string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os papéis da plataforma",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			roles, err := client.ListRoles(ctx, fluig.RoleFilter{Search: search, Limit: limit})
			if err != nil {
				return mapFluigError(err)
			}
			if len(roles) == 0 {
				p.Infof("Nenhum papel encontrado com esses filtros.")
			} else {
				rows := make([][]string, 0, len(roles))
				for _, r := range roles {
					rows = append(rows, []string{r.Code, r.Description})
				}
				p.Table(output.Table{
					Headers: []string{"Código", "Descrição"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"roles": roles})
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "busca por substring em código ou descrição (filtro no cliente)")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de papéis (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- role show ---

func newRoleShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <code>",
		Short: "Mostra um papel com os usuários vinculados",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			r, err := client.GetRole(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			users, err := client.ListRoleUsers(ctx, code, 0)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("%s (%s)", r.Description, r.Code)
			if len(users) > 0 {
				logins := make([]string, 0, len(users))
				for _, u := range users {
					logins = append(logins, u.Login)
				}
				sort.Strings(logins)
				p.Successf("Usuários (%d): %s", len(logins), strings.Join(logins, ", "))
			} else {
				p.Successf("Usuários: nenhum")
			}
			p.Done(map[string]any{"role": r, "users": users})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- role create ---

func newRoleCreateCmd(app *App) *cobra.Command {
	var (
		description   string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <code> [--description <texto>]",
		Short: "Cria um papel na plataforma",
		Long: "Cria um papel. A descrição é opcional; quando omitida, usa o próprio\n" +
			"código como descrição (evita papel com descrição em branco).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			if description == "" {
				description = code
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar papel")
			if err != nil {
				return err
			}
			r, err := client.CreateRole(ctx, code, description)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("papel %q criado (%s)", r.Code, r.Description)
			p.Done(map[string]any{"role": r})
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "descrição do papel (default: o próprio código)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- role update ---

func newRoleUpdateCmd(app *App) *cobra.Command {
	var (
		description   string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <code> --description <texto>",
		Short: "Atualiza a descrição de um papel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			fields := map[string]string{}
			if cmd.Flags().Changed("description") {
				fields["description"] = description
			}
			if len(fields) == 0 {
				return output.Usagef("informe --description")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "atualizar papel")
			if err != nil {
				return err
			}
			r, err := client.UpdateRole(ctx, code, fields)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("papel %q atualizado", r.Code)
			p.Done(map[string]any{"role": r})
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "nova descrição")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- role delete ---

func newRoleDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <code>",
		Short: "Exclui um papel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir papel")
			if err != nil {
				return err
			}
			if err := client.DeleteRole(ctx, code); err != nil {
				return mapFluigError(err)
			}
			p.Successf("papel %q excluído", code)
			p.Done(map[string]any{"code": code, "deleted": true})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- role users ---

func newRoleUsersCmd(app *App) *cobra.Command {
	var (
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "users <code>",
		Short: "Lista os usuários vinculados a um papel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			if _, err := client.GetRole(ctx, code); err != nil {
				return mapFluigError(err)
			}
			users, err := client.ListRoleUsers(ctx, code, limit)
			if err != nil {
				return mapFluigError(err)
			}
			if len(users) == 0 {
				p.Infof("O papel %q não tem usuários vinculados.", code)
			} else {
				usersTable(p, users)
			}
			p.Done(map[string]any{"users": users})
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "número máximo de usuários (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- role add-user / remove-user ---

func newRoleMemberCmd(app *App, add bool) *cobra.Command {
	use, short, action, done := "remove-user <code> <login>",
		"Remove o papel de um usuário", "remover papel de usuário", "removido de"
	if add {
		use, short, action, done = "add-user <code> <login>",
			"Vincula um papel a um usuário", "vincular papel a usuário", "vinculado a"
	}
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code, login := args[0], args[1]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, action)
			if err != nil {
				return err
			}
			if add {
				err = client.AddRoleUser(ctx, code, login)
			} else {
				err = client.RemoveRoleUser(ctx, code, login)
			}
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("papel %q %s usuário %q", code, done, login)
			p.Done(map[string]any{"role": code, "login": login, "member": add})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}
