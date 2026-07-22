package cli

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// EnvNewUserPassword é a env var da senha do usuário NOVO/alterado — a senha
// nunca vem por flag (regra de segurança do projeto).
const envNewUserPassword = "FLUIGCLI_NEW_USER_PASSWORD"

func newUserCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Usuários da plataforma: consulta e gestão (requer privilégio administrativo)",
	}
	cmd.AddCommand(newUserListCmd(app))
	cmd.AddCommand(newUserShowCmd(app))
	cmd.AddCommand(newUserCreateCmd(app))
	cmd.AddCommand(newUserUpdateCmd(app))
	cmd.AddCommand(newUserActivateCmd(app, true))
	cmd.AddCommand(newUserActivateCmd(app, false))
	cmd.AddCommand(newUserAuditCmd(app))
	return cmd
}

// resolveNewUserPassword obtém a senha do usuário novo/alterado por env var
// (FLUIGCLI_NEW_USER_PASSWORD) ou, em modo interativo, por prompt oculto —
// nunca por argumento de linha de comando (regra 5 do projeto).
func (a *App) resolveNewUserPassword(confirm bool) (string, error) {
	if pw := os.Getenv(envNewUserPassword); pw != "" {
		return pw, nil
	}
	if !a.Interactive() {
		return "", output.Usagef("defina a senha do usuário na variável %s (nunca em argumento de linha de comando)", envNewUserPassword)
	}
	pw, err := promptPassword("Senha do usuário")
	if err != nil {
		return "", err
	}
	if confirm {
		again, err := promptPassword("Confirme a senha")
		if err != nil {
			return "", err
		}
		if again != pw {
			return "", output.Usagef("as senhas não conferem")
		}
	}
	return pw, nil
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

// --- user create ---

func newUserCreateCmd(app *App) *cobra.Command {
	var (
		email         string
		firstName     string
		lastName      string
		fullName      string
		code          string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <login>",
		Short: "Cria um usuário na plataforma",
		Long: "Cria um usuário. A senha vem da variável " + envNewUserPassword + " ou,\n" +
			"em modo interativo, de um prompt oculto — nunca por argumento de linha de\n" +
			"comando. O código (userCode) usa o login quando não informado.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			login := args[0]
			if email == "" || firstName == "" || lastName == "" {
				return output.Usagef("informe --email, --first-name e --last-name")
			}
			password, err := app.resolveNewUserPassword(true)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar usuário")
			if err != nil {
				return err
			}
			u, err := client.CreateAdminUser(ctx, fluig.AdminUserCreate{
				Login: login, Code: code, Email: email,
				FirstName: firstName, LastName: lastName, FullName: fullName,
				Password: password,
			})
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("usuário %q criado (%s, %s)", u.Login, u.FullName, u.State)
			p.Done(map[string]any{"user": u})
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "e-mail (obrigatório)")
	cmd.Flags().StringVar(&firstName, "first-name", "", "primeiro nome (obrigatório)")
	cmd.Flags().StringVar(&lastName, "last-name", "", "sobrenome (obrigatório)")
	cmd.Flags().StringVar(&fullName, "full-name", "", "nome completo (default: primeiro + sobrenome)")
	cmd.Flags().StringVar(&code, "code", "", "código do usuário / userCode (default: o login)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- user update ---

func newUserUpdateCmd(app *App) *cobra.Command {
	var (
		email         string
		firstName     string
		lastName      string
		fullName      string
		setPassword   bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <login>",
		Short: "Atualiza os dados de um usuário (mescla os campos informados)",
		Long: "Atualiza só os campos informados (os demais são preservados). Com\n" +
			"--set-password, redefine a senha lendo de " + envNewUserPassword + " ou\n" +
			"do prompt oculto — nunca por argumento.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			login := args[0]
			fields := map[string]string{}
			if cmd.Flags().Changed("email") {
				fields["email"] = email
			}
			if cmd.Flags().Changed("first-name") {
				fields["firstName"] = firstName
			}
			if cmd.Flags().Changed("last-name") {
				fields["lastName"] = lastName
			}
			if cmd.Flags().Changed("full-name") {
				fields["fullName"] = fullName
			}
			if setPassword {
				pw, err := app.resolveNewUserPassword(true)
				if err != nil {
					return err
				}
				fields["password"] = pw
			}
			if len(fields) == 0 {
				return output.Usagef("informe ao menos um campo a alterar (--email, --first-name, --last-name, --full-name ou --set-password)")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "atualizar usuário")
			if err != nil {
				return err
			}
			u, err := client.UpdateAdminUser(ctx, login, fields)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("usuário %q atualizado", u.Login)
			p.Done(map[string]any{"user": u})
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "novo e-mail")
	cmd.Flags().StringVar(&firstName, "first-name", "", "novo primeiro nome")
	cmd.Flags().StringVar(&lastName, "last-name", "", "novo sobrenome")
	cmd.Flags().StringVar(&fullName, "full-name", "", "novo nome completo")
	cmd.Flags().BoolVar(&setPassword, "set-password", false, "redefine a senha (lida de "+envNewUserPassword+" ou do prompt)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- user activate / deactivate ---

func newUserActivateCmd(app *App, activate bool) *cobra.Command {
	use, short, action, done := "deactivate <login>",
		"Desativa um usuário (state = BLOCKED)", "desativar usuário", "desativado"
	if activate {
		use, short, action, done = "activate <login>",
			"Reativa um usuário desativado", "ativar usuário", "ativado"
	}
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			login := args[0]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, action)
			if err != nil {
				return err
			}
			if err := client.SetAdminUserActive(ctx, login, activate); err != nil {
				return mapFluigError(err)
			}
			p.Successf("usuário %q %s", login, done)
			p.Done(map[string]any{"login": login, "active": activate})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}
