package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/update"
)

const (
	// updateCheckMaxAge limita a checagem automática de versão a 1×/dia.
	updateCheckMaxAge = 24 * time.Hour
	// noticeTimeout limita quanto a checagem automática pode atrasar o comando.
	noticeTimeout = 2 * time.Second
)

// updateBaseURL é variável para os testes apontarem para um servidor fake.
var updateBaseURL = update.DefaultBaseURL

type upgradeResult struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Updated bool   `json:"updated"`
	Path    string `json:"path,omitempty"`
}

func newUpgradeCmd(app *App) *cobra.Command {
	var targetVersion string
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Atualiza o fluigcli para a última versão",
		Long: "Baixa a release do GitHub, confere o checksum e substitui o próprio binário.\n\n" +
			"Sem flags, instala a última versão publicada. Use --version para instalar uma\n" +
			"versão específica (inclusive mais antiga) e --check para só consultar.\n\n" +
			"O aviso automático de versão nova (mostrado no stderr no máximo uma vez por\n" +
			"dia) pode ser desativado com FLUIGCLI_NO_UPDATE_CHECK=1.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			current := strings.TrimPrefix(app.Version, "v")

			client := update.NewClient(app.Timeout)
			client.BaseURL = updateBaseURL

			target := strings.TrimPrefix(targetVersion, "v")
			if target == "" {
				latest, err := client.Latest(cmd.Context())
				if err != nil {
					return output.ServerErrorf("não consegui consultar a última release: %s", err).WithCause(err)
				}
				target = latest
			}

			if checkOnly {
				switch {
				case current == "dev":
					p.Successf("última release: v%s — esta cópia é build de desenvolvimento (dev)", target)
				case update.IsNewer(target, current):
					p.Successf("nova versão disponível: v%s (atual: v%s) — atualize com: fluigcli upgrade", target, current)
				default:
					p.Successf("você já está na última versão (v%s)", current)
				}
				p.Done(upgradeResult{Current: current, Latest: target, Updated: false})
				return nil
			}

			if current == "dev" {
				return output.Usagef("esta cópia do fluigcli não veio de uma release (build dev) — " +
					"atualize pelo mesmo meio da instalação (ex.: go install github.com/alorenco/fluig-cli/cmd/fluigcli@latest)")
			}
			if target == current {
				p.Successf("você já está na versão v%s — nada a fazer", current)
				p.Done(upgradeResult{Current: current, Latest: target, Updated: false})
				return nil
			}
			if targetVersion == "" && !update.IsNewer(target, current) {
				p.Successf("nada a fazer: a versão instalada (v%s) é mais nova que a última release (v%s)", current, target)
				p.Done(upgradeResult{Current: current, Latest: target, Updated: false})
				return nil
			}

			exe, err := update.CurrentExecutable()
			if err != nil {
				return output.Genericf("não consegui localizar o binário atual: %s", err).WithCause(err)
			}

			tmp, err := os.MkdirTemp("", "fluigcli-upgrade-")
			if err != nil {
				return output.Genericf("não consegui criar diretório temporário: %s", err).WithCause(err)
			}
			defer os.RemoveAll(tmp)

			p.Infof("Baixando o fluigcli v%s (%s/%s)…", target, runtime.GOOS, runtime.GOARCH)
			newBin, err := client.Download(cmd.Context(), target, runtime.GOOS, runtime.GOARCH, tmp)
			if err != nil {
				return output.ServerErrorf("download falhou: %s", err).WithCause(err)
			}
			p.Infof("Checksum conferido.")

			if err := update.ReplaceExecutable(newBin, exe); err != nil {
				if errors.Is(err, os.ErrPermission) {
					return output.Genericf("sem permissão para escrever em %s — repita com sudo", exe).WithCause(err)
				}
				return output.Genericf("não consegui substituir o binário: %s", err).WithCause(err)
			}

			p.Successf("fluigcli atualizado: v%s → v%s (%s)", current, target, exe)
			if runtime.GOOS == "windows" {
				p.Infof("O binário antigo ficou como %s.old — pode apagar quando quiser.", exe)
			}
			p.Done(upgradeResult{Current: current, Latest: target, Updated: true, Path: exe})
			return nil
		},
	}
	cmd.Flags().StringVar(&targetVersion, "version", "", "versão a instalar (padrão: última release)")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "só verifica se há versão nova, sem instalar")
	return cmd
}

// maybeNotifyUpdate avisa no stderr (no máximo 1×/dia) quando há versão nova.
// É totalmente silencioso quando não se aplica: build dev, sem TTY, desativado
// por FLUIGCLI_NO_UPDATE_CHECK, sem rede ou dentro do próprio upgrade.
func maybeNotifyUpdate(app *App) {
	if app.Version == "" || app.Version == "dev" {
		return
	}
	if v := os.Getenv(envNoUpdateCheck); v == "1" || strings.EqualFold(v, "true") {
		return
	}
	if !output.StderrIsTTY() {
		return
	}
	if app.printer == nil {
		return
	}
	if c := app.printer.Command; strings.HasPrefix(c, "upgrade") || strings.HasPrefix(c, "completion") {
		return
	}
	cachePath, err := update.NoticeCachePath()
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), noticeTimeout)
	defer cancel()
	client := update.NewClient(noticeTimeout)
	client.BaseURL = updateBaseURL

	current := strings.TrimPrefix(app.Version, "v")
	latest, newer := update.CheckForNotice(ctx, client, cachePath, current, time.Now(), updateCheckMaxAge)
	if newer {
		fmt.Fprintf(os.Stderr, "\nnova versão disponível: v%s (atual: v%s) — atualize com: fluigcli upgrade\n", latest, current)
	}
}
