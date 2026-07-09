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

Escolha a seção do SEU shell — cada comando serve a um shell específico.
No Windows (PowerShell), use a seção PowerShell; a linha do Bash com
"source <(...)" NÃO funciona no PowerShell.

Bash:
  # na sessão atual:
  source <(fluigcli completion bash)
  # de forma permanente (Linux; exige o pacote bash-completion):
  fluigcli completion bash | sudo tee /etc/bash_completion.d/fluigcli > /dev/null

Zsh:
  # na sessão atual:
  source <(fluigcli completion zsh)
  # de forma permanente:
  fluigcli completion zsh > "${fpath[1]}/_fluigcli"

Fish:
  # na sessão atual:
  fluigcli completion fish | source
  # de forma permanente:
  fluigcli completion fish > ~/.config/fish/completions/fluigcli.fish

PowerShell:
  # na sessão atual:
  fluigcli completion powershell | Out-String | Invoke-Expression
  # de forma permanente (adiciona ao seu perfil do PowerShell):
  fluigcli completion powershell >> $PROFILE`,
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
