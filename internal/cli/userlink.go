package cli

// Comandos de vínculo pelo LADO DO USUÁRIO (ROADMAP §5.1): `user roles`,
// `user groups`, `user add-role|remove-role` e `user add-group|remove-group`.
//
// São a mesma operação que o `role add-user` e o `group add-user` já faziam,
// por outra porta. Quem administra pensa no usuário ("dá o papel X para o
// fulano"), não no papel. Por isso as mensagens e os exit codes são iguais aos
// do caminho antigo — só muda a ordem dos argumentos.

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
)

// newUserRolesCmd lista os papéis de um usuário.
func newUserRolesCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "roles <login>",
		Short: "Lista os papéis vinculados a um usuário",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			login := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			roles, err := client.ListUserRoles(ctx, login)
			if err != nil {
				return mapFluigError(err)
			}
			if len(roles) == 0 {
				p.Infof("O usuário %q não tem nenhum papel. Vincule um com: fluigcli user add-role %s <papel>", login, login)
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
			p.Done(map[string]any{"login": login, "roles": roles})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// newUserGroupsCmd lista os grupos de um usuário.
func newUserGroupsCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "groups <login>",
		Short: "Lista os grupos de que um usuário participa",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			login := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			groups, err := client.ListUserGroups(ctx, login)
			if err != nil {
				return mapFluigError(err)
			}
			if len(groups) == 0 {
				p.Infof("O usuário %q não participa de nenhum grupo. Inclua com: fluigcli user add-group %s <grupo>", login, login)
			} else {
				rows := make([][]string, 0, len(groups))
				for _, g := range groups {
					rows = append(rows, []string{g.Code, g.Description, g.Type})
				}
				p.Table(output.Table{
					Headers: []string{"Código", "Descrição", "Tipo"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"login": login, "groups": groups})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// newUserLinkCmd gera os quatro comandos de escrita. `role` escolhe a família;
// `add` escolhe o sentido.
func newUserLinkCmd(app *App, role, add bool) *cobra.Command {
	familia, oposto := "role", "role add-user"
	if !role {
		familia, oposto = "group", "group add-user"
	}
	verbo, done, acao := "remove", "removido de", "remover "+familia+" de usuário"
	if add {
		verbo, done, acao = "add", "vinculado a", "vincular "+familia+" a usuário"
	}
	if !role {
		done = "removido de"
		if add {
			done = "incluído em"
		}
	}

	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   verbo + "-" + familia + " <login> <code>",
		Short: shortDoVinculo(role, add),
		Long: longDoVinculo(role, add) + "\n\n" +
			"Este comando é a outra porta do `" + oposto + "`: escreve o MESMO vínculo,\n" +
			"com os argumentos na ordem inversa. Use o que ficar mais natural.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			login, code := args[0], args[1]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, acao)
			if err != nil {
				return err
			}
			switch {
			case role && add:
				err = client.AddUserRole(ctx, login, code)
			case role && !add:
				err = client.RemoveUserRole(ctx, login, code)
			case !role && add:
				err = client.AddUserGroup(ctx, login, code)
			default:
				err = client.RemoveUserGroup(ctx, login, code)
			}
			if err != nil {
				return mapFluigError(err)
			}
			if role {
				p.Successf("papel %q %s usuário %q", code, done, login)
				p.Done(map[string]any{"login": login, "role": code, "member": add})
				return nil
			}
			p.Successf("usuário %q %s grupo %q", login, done, code)
			p.Done(map[string]any{"login": login, "group": code, "member": add})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

func shortDoVinculo(role, add bool) string {
	switch {
	case role && add:
		return "Vincula um papel a um usuário"
	case role && !add:
		return "Remove um papel de um usuário"
	case !role && add:
		return "Inclui um usuário em um grupo"
	default:
		return "Remove um usuário de um grupo"
	}
}

func longDoVinculo(role, add bool) string {
	if role {
		if add {
			return "Vincula um papel a um usuário.\n\n" +
				"Vincular um papel que o usuário já tem não é erro: a operação é\n" +
				"idempotente e responde sucesso."
		}
		return "Remove um papel de um usuário.\n\n" +
			"Se o usuário não tem o papel, o comando responde exit 4."
	}
	if add {
		return "Inclui um usuário em um grupo.\n\n" +
			"Incluir um usuário que já é membro é recusado pelo servidor (exit 5),\n" +
			"igual ao `group add-user`."
	}
	return "Remove um usuário de um grupo.\n\n" +
		"Se o usuário não é membro, o comando responde exit 4."
}
