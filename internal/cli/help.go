package cli

import "github.com/spf13/cobra"

// usageTemplatePT é o template de uso do cobra com os cabeçalhos em pt-BR
// (nomes de comandos/flags em inglês; todo o texto explicativo em pt-BR).
// É o template padrão do cobra com as strings fixas traduzidas.
const usageTemplatePT = `Uso:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [comando]{{end}}{{if gt (len .Aliases) 0}}

Apelidos:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Exemplos:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Comandos disponíveis:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Adicionais:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Flags globais:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Tópicos de ajuda adicionais:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [comando] --help" para ver mais informações sobre um comando.{{end}}
`

// localize deixa todo o texto gerado pelo cobra em pt-BR, mantendo nomes de
// comandos e flags em inglês.
func localize(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplatePT)

	// Substitui a flag de ajuda automática ("help for X") por uma em pt-BR.
	// Definir a flag "help" aqui impede o cobra de adicionar a sua própria.
	root.PersistentFlags().BoolP("help", "h", false, "mostra a ajuda deste comando")

	// Comando `help` em pt-BR (o padrão do cobra é "Help about any command").
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [comando]",
		Short: "Mostra a ajuda de qualquer comando",
		Long: "Mostra a ajuda de qualquer comando da CLI.\n\n" +
			"Exemplo: fluigcli help server",
		Run: func(c *cobra.Command, args []string) {
			target, _, err := c.Root().Find(args)
			if target == nil || err != nil {
				c.Root().Printf("Comando desconhecido: %q. Use \"fluigcli --help\".\n", args)
				return
			}
			_ = target.Help()
		},
	})

	// Desabilita o comando `completion` gerado pelo cobra (textos em inglês);
	// um comando próprio, em pt-BR, é adicionado em newRootCmd.
	root.CompletionOptions.DisableDefaultCmd = true
}
