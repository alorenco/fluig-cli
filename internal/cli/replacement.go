package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

const replInputDateLayout = "2006-01-02"

func newReplacementCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "replacement",
		Aliases: []string{"substitute"},
		Short:   "Substitutos de usuário: consulta e definição de substituições (requer privilégio administrativo)",
		Long: "Gerencia substituições de usuário (delegação de tarefas de workflow/GED).\n" +
			"O TITULAR é quem será substituído; o SUBSTITUTO assume as tarefas no\n" +
			"período informado. Os argumentos de usuário são LOGINS (resolvidos para\n" +
			"userCode internamente).",
	}
	cmd.AddCommand(newReplacementListCmd(app))
	cmd.AddCommand(newReplacementShowCmd(app))
	cmd.AddCommand(newReplacementCreateCmd(app))
	cmd.AddCommand(newReplacementUpdateCmd(app))
	cmd.AddCommand(newReplacementDeleteCmd(app))
	return cmd
}

func fmtReplDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(replInputDateLayout)
}

func boolMark(b *bool) string {
	if b == nil {
		return "?"
	}
	if *b {
		return "sim"
	}
	return "não"
}

// userLabel devolve o rótulo humano de um usuário da substituição (login quando
// houver; senão o userCode cru).
func userLabel(u *fluig.RequestUser) string {
	if u == nil {
		return ""
	}
	if u.Login != "" {
		return u.Login
	}
	return u.Code
}

// --- replacement list ---

func newReplacementListCmd(app *App) *cobra.Command {
	var (
		userLogin     string
		replacedBy    string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista as substituições de usuário",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			list, err := client.ListReplacements(ctx, fluig.ReplacementFilter{
				UserLogin: userLogin, ReplacedByLogin: replacedBy, Limit: limit,
			})
			if err != nil {
				return mapFluigError(err)
			}
			if len(list) == 0 {
				p.Infof("Nenhuma substituição encontrada com esses filtros.")
			} else {
				rows := make([][]string, 0, len(list))
				for _, r := range list {
					rows = append(rows, []string{
						userLabel(r.User), userLabel(r.ReplacedBy),
						fmtReplDate(r.StartDate), fmtReplDate(r.EndDate),
					})
				}
				p.Table(output.Table{
					Headers: []string{"Titular", "Substituto", "Início", "Fim"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"replacements": list})
			return nil
		},
	}
	cmd.Flags().StringVar(&userLogin, "user", "", "filtra pelo titular (login)")
	cmd.Flags().StringVar(&replacedBy, "replaced-by", "", "filtra pelo substituto (login)")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de substituições (0 = todas)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- replacement show ---

func newReplacementShowCmd(app *App) *cobra.Command {
	var (
		validOnly     bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "show <user-login>",
		Short: "Mostra as substituições de um usuário (com escopo workflow/GED)",
		Long: "Lista as substituições de um usuário (titular), incluindo as flags de\n" +
			"escopo (tarefas de workflow e de GED). Com --valid-only, só as vigentes\n" +
			"na data atual.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			list, err := client.GetUserReplacements(ctx, args[0], validOnly)
			if err != nil {
				return mapFluigError(err)
			}
			if len(list) == 0 {
				if validOnly {
					p.Infof("O usuário %q não tem substituição vigente hoje.", args[0])
				} else {
					p.Infof("O usuário %q não tem substituições cadastradas.", args[0])
				}
			} else {
				rows := make([][]string, 0, len(list))
				for _, r := range list {
					rows = append(rows, []string{
						userLabel(r.ReplacedBy),
						fmtReplDate(r.StartDate), fmtReplDate(r.EndDate),
						boolMark(r.WorkflowTasks), boolMark(r.GEDTasks),
					})
				}
				p.Table(output.Table{
					Headers: []string{"Substituto", "Início", "Fim", "Workflow", "GED"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"replacements": list})
			return nil
		},
	}
	cmd.Flags().BoolVar(&validOnly, "valid-only", false, "mostra só as substituições vigentes hoje")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- replacement create ---

func newReplacementCreateCmd(app *App) *cobra.Command {
	var (
		startStr      string
		endStr        string
		workflowTasks bool
		gedTasks      bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <titular-login> <substituto-login>",
		Short: "Define um substituto para um usuário",
		Long: "Define um substituto (delegação de tarefas) para um usuário no período\n" +
			"informado. --start assume a data de hoje quando omitido; --end é\n" +
			"obrigatório. Um par (titular, substituto) para o mesmo período não pode\n" +
			"ser duplicado.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			start, end, err := parseReplPeriod(startStr, endStr, true)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "definir substituto")
			if err != nil {
				return err
			}
			r, err := client.CreateReplacement(ctx, args[0], args[1], fluig.ReplacementInput{
				Start: start, End: end, WorkflowTasks: workflowTasks, GEDTasks: gedTasks,
			})
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("substituto %q definido para %q (%s a %s)",
				userLabel(r.ReplacedBy), userLabel(r.User), fmtReplDate(r.StartDate), fmtReplDate(r.EndDate))
			p.Done(map[string]any{"replacement": r})
			return nil
		},
	}
	cmd.Flags().StringVar(&startStr, "start", "", "início da vigência (YYYY-MM-DD; default: hoje)")
	cmd.Flags().StringVar(&endStr, "end", "", "fim da vigência (YYYY-MM-DD; obrigatório)")
	cmd.Flags().BoolVar(&workflowTasks, "workflow-tasks", true, "o substituto assume as tarefas de workflow")
	cmd.Flags().BoolVar(&gedTasks, "ged-tasks", false, "o substituto assume as tarefas de GED")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- replacement update ---

func newReplacementUpdateCmd(app *App) *cobra.Command {
	var (
		startStr      string
		endStr        string
		workflowTasks bool
		gedTasks      bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <titular-login> <substituto-login>",
		Short: "Altera uma substituição existente (período e/ou escopo)",
		Long: "Altera os campos informados de uma substituição existente (merge: os\n" +
			"campos não informados são preservados). Identifica a substituição pelo\n" +
			"par (titular, substituto).",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ch := fluig.ReplacementChanges{}
			if cmd.Flags().Changed("start") {
				t, err := parseReplDate(startStr, "start")
				if err != nil {
					return err
				}
				ch.Start = &t
			}
			if cmd.Flags().Changed("end") {
				t, err := parseReplDate(endStr, "end")
				if err != nil {
					return err
				}
				ch.End = &t
			}
			if cmd.Flags().Changed("workflow-tasks") {
				ch.WorkflowTasks = &workflowTasks
			}
			if cmd.Flags().Changed("ged-tasks") {
				ch.GEDTasks = &gedTasks
			}
			if ch.Start == nil && ch.End == nil && ch.WorkflowTasks == nil && ch.GEDTasks == nil {
				return output.Usagef("informe ao menos um campo: --start, --end, --workflow-tasks ou --ged-tasks")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar substituição")
			if err != nil {
				return err
			}
			r, err := client.UpdateReplacement(ctx, args[0], args[1], ch)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("substituição de %q por %q atualizada (%s a %s)",
				userLabel(r.User), userLabel(r.ReplacedBy), fmtReplDate(r.StartDate), fmtReplDate(r.EndDate))
			p.Done(map[string]any{"replacement": r})
			return nil
		},
	}
	cmd.Flags().StringVar(&startStr, "start", "", "novo início da vigência (YYYY-MM-DD)")
	cmd.Flags().StringVar(&endStr, "end", "", "novo fim da vigência (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&workflowTasks, "workflow-tasks", true, "o substituto assume as tarefas de workflow")
	cmd.Flags().BoolVar(&gedTasks, "ged-tasks", false, "o substituto assume as tarefas de GED")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- replacement delete ---

func newReplacementDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <titular-login> <substituto-login>",
		Short: "Remove uma substituição",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "remover substituição")
			if err != nil {
				return err
			}
			if err := client.DeleteReplacement(ctx, args[0], args[1]); err != nil {
				return mapFluigError(err)
			}
			p.Successf("substituição de %q por %q removida", args[0], args[1])
			p.Done(map[string]any{"user": args[0], "replacedBy": args[1], "deleted": true})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// parseReplDate interpreta uma data YYYY-MM-DD (meia-noite, sem fuso).
func parseReplDate(s, flag string) (time.Time, error) {
	t, err := time.Parse(replInputDateLayout, s)
	if err != nil {
		return time.Time{}, output.Usagef("--%s inválido %q (use YYYY-MM-DD)", flag, s)
	}
	return t, nil
}

// parseReplPeriod resolve o par início/fim do create. --start default = hoje.
func parseReplPeriod(startStr, endStr string, endRequired bool) (time.Time, time.Time, error) {
	start := time.Now().UTC().Truncate(24 * time.Hour)
	if startStr != "" {
		t, err := parseReplDate(startStr, "start")
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start = t
	}
	if endStr == "" {
		if endRequired {
			return time.Time{}, time.Time{}, output.Usagef("--end é obrigatório (fim da vigência, YYYY-MM-DD)")
		}
		return start, time.Time{}, nil
	}
	end, err := parseReplDate(endStr, "end")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, output.Usagef("--end (%s) é anterior a --start (%s)", endStr, start.Format(replInputDateLayout))
	}
	return start, end, nil
}
