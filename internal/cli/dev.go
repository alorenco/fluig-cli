package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/devserver"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newDevCmd(app *App) *cobra.Command {
	var (
		port          int
		debounce      time.Duration
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Servidor de desenvolvimento com live reload (proxy autenticado do Fluig)",
		Long: "Sobe um proxy local autenticado do servidor Fluig que serve do disco os\n" +
			"arquivos que você está editando — sem publicar nada:\n\n" +
			"  • Widgets: navegue no portal real pela porta local; o JS/CSS de\n" +
			"    wcm/widget/*/src/main/webapp/resources/ é servido da sua máquina.\n" +
			"    Salvou, recarregou, mudou — sem deploy de WAR nem espera de cache.\n" +
			"    (view.ftl e .properties são renderizados no servidor: mudanças neles\n" +
			"    geram aviso pedindo o widget export.)\n" +
			"  • Formulários: preview local em /_dev/forms/ com o style guide e os\n" +
			"    datasets do servidor real (DatasetFactory funciona com dados reais).\n" +
			"  • Live reload: ao salvar em forms/ ou wcm/widget/, o navegador\n" +
			"    recarrega sozinho.\n\n" +
			"Segurança: escuta só em 127.0.0.1 — o proxy carrega a SUA sessão\n" +
			"autenticada; não exponha a porta. Só roda em servidor dev ou hml.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if app.JSON {
				return output.Usagef("dev é um modo interativo e não suporta --json")
			}

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			server, err := app.resolveServer("")
			if err != nil {
				return err
			}
			p.Server = server.Name
			switch server.Env {
			case config.EnvDev, config.EnvHml:
			case config.EnvProd:
				return output.Usagef("o dev não roda apontando para PRODUÇÃO (%q)", server.Name)
			default:
				return output.Usagef("o dev exige servidor marcado como dev ou hml; marque com: fluigcli server update %s --env hml", server.Name)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			client, err := app.authenticate(ctx, server, passwordStdin)
			if err != nil {
				return err
			}
			// Sessões rotacionadas pelo servidor durante o proxy sobrevivem
			// à execução.
			defer client.SaveSession()

			srv, err := devserver.New(devserver.Options{
				Root:     root,
				Upstream: client.BaseURL(),
				Jar:      client.SessionJar(),
				Port:     port,
				Debounce: debounce,
				Infof:    p.Infof,
				Warnf:    p.Warnf,
			})
			if err != nil {
				return output.Genericf("não consegui montar o dev server: %v", err)
			}

			p.Successf("Dev server de %q em %s — Ctrl+C para parar.", server.Name, srv.URL())
			p.Infof("Portal via proxy:       %s/portal/p/%d/home", srv.URL(), server.CompanyID)
			p.Infof("Preview de formulários: %s/_dev/forms/", srv.URL())
			if mounts := srv.Mounts(); len(mounts) > 0 {
				p.Infof("Widgets servidas do disco (%d):", len(mounts))
				for _, m := range mounts {
					p.Infof("  %s", m)
				}
			} else {
				p.Infof("Nenhuma widget local (wcm/widget/) — só proxy e preview de formulários.")
			}
			if err := srv.Run(ctx); err != nil {
				return output.Genericf("dev server: %v", err)
			}
			p.Infof("Dev server encerrado.")
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 8787, "porta local do dev server (escuta só em 127.0.0.1)")
	cmd.Flags().DurationVar(&debounce, "debounce", 500*time.Millisecond, "espera após o salvamento antes de recarregar (agrupa rajadas do editor)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
