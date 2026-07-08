package cli

import (
	"github.com/spf13/cobra"
)

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Exibe a versão do fluigcli",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			p.Successf("fluigcli %s (commit %s, build %s)", app.Version, app.Commit, app.Date)
			p.Done(map[string]string{
				"version": app.Version,
				"commit":  app.Commit,
				"date":    app.Date,
			})
			return nil
		},
	}
}
