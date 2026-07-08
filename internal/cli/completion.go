package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
)

// newCompletionCmd é o comando de autocompletar, em pt-BR (substitui o padrão
// do cobra, cujos textos são em inglês). Gera o script para o shell indicado.
func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Gera o script de autocompletar para o shell indicado",
		Long: `Gera o script de autocompletar do fluigcli.

Bash:
  fluigcli completion bash | sudo tee /etc/bash_completion.d/fluigcli > /dev/null
  # ou, na sessão atual:
  source <(fluigcli completion bash)

Zsh:
  fluigcli completion zsh > "${fpath[1]}/_fluigcli"

Fish:
  fluigcli completion fish > ~/.config/fish/completions/fluigcli.fish

PowerShell:
  fluigcli completion powershell | Out-String | Invoke-Expression`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return output.Usagef("shell inválido %q (use bash, zsh, fish ou powershell)", args[0])
			}
		},
	}
}
