package cli

import (
	"context"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newGroupCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Grupos da plataforma: consulta, gestão e membros (requer privilégio administrativo)",
	}
	cmd.AddCommand(newGroupListCmd(app))
	cmd.AddCommand(newGroupShowCmd(app))
	cmd.AddCommand(newGroupCreateCmd(app))
	cmd.AddCommand(newGroupUpdateCmd(app))
	cmd.AddCommand(newGroupDeleteCmd(app))
	cmd.AddCommand(newGroupUsersCmd(app))
	cmd.AddCommand(newGroupMemberCmd(app, true))
	cmd.AddCommand(newGroupMemberCmd(app, false))
	return cmd
}

// validGroupType normaliza e valida o filtro/tipo de grupo.
func validGroupType(t string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return "", nil
	case "user":
		return "user", nil
	case "community":
		return "community", nil
	default:
		return "", output.Usagef("tipo inválido %q (use user ou community)", t)
	}
}

// usersTable imprime a tabela de usuários (mesma do `user list`).
func usersTable(p *output.Printer, users []fluig.AdminUser) {
	rows := make([][]string, 0, len(users))
	for _, u := range users {
		rows = append(rows, []string{u.Login, u.FullName, u.Email, u.State})
	}
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

// --- group list ---

func newGroupListCmd(app *App) *cobra.Command {
	var (
		groupType     string
		search        string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os grupos da plataforma",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			typ, err := validGroupType(groupType)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			groups, err := client.ListGroups(ctx, fluig.GroupFilter{Type: typ, Search: search, Limit: limit})
			if err != nil {
				return mapFluigError(err)
			}
			if len(groups) == 0 {
				p.Infof("Nenhum grupo encontrado com esses filtros.")
			} else {
				rows := make([][]string, 0, len(groups))
				for _, g := range groups {
					rows = append(rows, []string{g.Code, g.Description, g.Type})
				}
				// Padrão de listagem (ver CLAUDE.md): grupos de usuário em verde
				// (os que você administra; os "community" são automáticos).
				p.Table(output.Table{
					Headers: []string{"Código", "Descrição", "Tipo"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 2 && groups[row].Type == "user" {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"groups": groups})
			return nil
		},
	}
	cmd.Flags().StringVar(&groupType, "type", "", "filtra por tipo: user ou community (filtro no cliente)")
	cmd.Flags().StringVar(&search, "search", "", "busca por substring em código ou descrição (filtro no cliente)")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de grupos (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- group show ---

func newGroupShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <code>",
		Short: "Mostra um grupo com os membros",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			g, err := client.GetGroup(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			users, err := client.ListGroupUsers(ctx, code, 0)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("%s (%s) — tipo %s", g.Description, g.Code, g.Type)
			if len(users) > 0 {
				logins := make([]string, 0, len(users))
				for _, u := range users {
					logins = append(logins, u.Login)
				}
				sort.Strings(logins)
				p.Successf("Membros (%d): %s", len(logins), strings.Join(logins, ", "))
			} else {
				p.Successf("Membros: nenhum")
			}
			p.Done(map[string]any{"group": g, "users": users})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- group create ---

func newGroupCreateCmd(app *App) *cobra.Command {
	var (
		description   string
		groupType     string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <code> --description <texto>",
		Short: "Cria um grupo na plataforma",
		Long: "Cria um grupo. A descrição é obrigatória. O tipo default é \"user\"\n" +
			"(grupo administrado); \"community\" é reservado aos grupos automáticos\n" +
			"das comunidades.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			if description == "" {
				return output.Usagef("informe --description (obrigatória)")
			}
			typ, err := validGroupType(groupType)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar grupo")
			if err != nil {
				return err
			}
			g, err := client.CreateGroup(ctx, code, description, typ)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("grupo %q criado (%s, tipo %s)", g.Code, g.Description, g.Type)
			p.Done(map[string]any{"group": g})
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "descrição do grupo (obrigatória)")
	cmd.Flags().StringVar(&groupType, "type", "", "tipo do grupo: user (default) ou community")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- group update ---

func newGroupUpdateCmd(app *App) *cobra.Command {
	var (
		description   string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <code> --description <texto>",
		Short: "Atualiza a descrição de um grupo",
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
			_, client, err := app.connectWrite(ctx, passwordStdin, "atualizar grupo")
			if err != nil {
				return err
			}
			g, err := client.UpdateGroup(ctx, code, fields)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("grupo %q atualizado", g.Code)
			p.Done(map[string]any{"group": g})
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "nova descrição")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- group delete ---

func newGroupDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <code>",
		Short: "Exclui um grupo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir grupo")
			if err != nil {
				return err
			}
			if err := client.DeleteGroup(ctx, code); err != nil {
				return mapFluigError(err)
			}
			p.Successf("grupo %q excluído", code)
			p.Done(map[string]any{"code": code, "deleted": true})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- group users ---

func newGroupUsersCmd(app *App) *cobra.Command {
	var (
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "users <code>",
		Short: "Lista os usuários membros de um grupo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			// Valida o grupo antes (404 limpo em vez de "0 membros" enganoso).
			if _, err := client.GetGroup(ctx, code); err != nil {
				return mapFluigError(err)
			}
			users, err := client.ListGroupUsers(ctx, code, limit)
			if err != nil {
				return mapFluigError(err)
			}
			if len(users) == 0 {
				p.Infof("O grupo %q não tem membros.", code)
			} else {
				usersTable(p, users)
			}
			p.Done(map[string]any{"users": users})
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "número máximo de membros (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- group add-user / remove-user ---

func newGroupMemberCmd(app *App, add bool) *cobra.Command {
	use, short, action, done := "remove-user <code> <login>",
		"Remove um usuário do grupo", "remover usuário do grupo", "removido do"
	if add {
		use, short, action, done = "add-user <code> <login>",
			"Adiciona um usuário ao grupo", "adicionar usuário ao grupo", "adicionado ao"
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
				err = client.AddGroupUser(ctx, code, login)
			} else {
				err = client.RemoveGroupUser(ctx, code, login)
			}
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("usuário %q %s grupo %q", login, done, code)
			p.Done(map[string]any{"group": code, "login": login, "member": add})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}
