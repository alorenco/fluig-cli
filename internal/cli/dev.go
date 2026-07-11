package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/devserver"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newDevCmd(app *App) *cobra.Command {
	var (
		listen        string
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
			"    wcm/widget/*/src/main/webapp/resources/ é servido da sua máquina, e\n" +
			"    o markup do view.ftl é rerenderizado do arquivo local direto na\n" +
			"    página (quando o template só usa ${instanceId} — FreeMarker real\n" +
			"    mantém o render do servidor, com aviso). Salvou, recarregou, mudou —\n" +
			"    sem deploy de WAR nem espera de cache. (edit.ftl, .properties e\n" +
			"    application.info seguem exigindo o widget export.)\n" +
			"  • Formulários: preview local em /_dev/forms/ com o style guide e os\n" +
			"    datasets do servidor real (DatasetFactory funciona com dados reais).\n" +
			"    Formulário de processo ganha um painel de simulação: o\n" +
			"    events/displayFields.js local roda no navegador com WKNumState,\n" +
			"    WKUser e modo escolhidos no painel — com o form vinculado\n" +
			"    (fluigcli form link), o processo é detectado e as etapas reais\n" +
			"    aparecem pelo nome; sem vínculo, digite o número da etapa.\n" +
			"  • Live reload: ao salvar em forms/ ou wcm/widget/, o navegador\n" +
			"    recarrega sozinho.\n\n" +
			"Segurança: por padrão escuta só em 127.0.0.1 — o proxy carrega a SUA\n" +
			"sessão autenticada; quem acessa a porta age no Fluig como você. Em\n" +
			"servidor de desenvolvimento remoto, use --listen com um endereço de\n" +
			"rede privada sua (ex.: o IP da máquina na tailnet) — nunca um IP\n" +
			"público. Só roda em servidor dev ou hml.",
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

			deployServers, deployConnect := app.deployBridge(server)
			srv, err := devserver.New(devserver.Options{
				Root:          root,
				Upstream:      client.BaseURL(),
				Jar:           client.SessionJar(),
				Host:          listen,
				Port:          port,
				Debounce:      debounce,
				Infof:         p.Infof,
				Warnf:         p.Warnf,
				Client:        client,
				FormScope:     server.FormScopeKey(),
				CompanyID:     server.CompanyID,
				DeployServers: deployServers,
				DeployConnect: deployConnect,
			})
			if err != nil {
				return output.Genericf("não consegui montar o dev server: %v", err)
			}

			p.Successf("Dev server de %q em %s — Ctrl+C para parar.", server.Name, srv.URL())
			if srv.ListensBeyondLoopback() {
				p.Warnf("escutando fora do loopback (%s): quem alcança essa porta age no Fluig com a SUA sessão — use só em rede privada (tailnet/VPN), nunca em IP público", listen)
			}
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
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1", "endereço de escuta — mude para um IP de rede privada (ex.: tailnet) ao desenvolver em servidor remoto")
	cmd.Flags().IntVar(&port, "port", 8787, "porta do dev server")
	cmd.Flags().DurationVar(&debounce, "debounce", 500*time.Millisecond, "espera após o salvamento antes de recarregar (agrupa rajadas do editor)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// deployBridge monta a ponte de publicação da barra do dev: a lista de
// servidores cadastrados e a conexão NÃO-interativa a qualquer um deles
// (sessão em cache → senha explícita do diálogo → keyring/env). Sem
// credencial disponível devolve devserver.ErrDeployNeedsPassword — o diálogo
// pede a senha (que trafega só do navegador ao dev server local; decisão do
// mantenedor em 2026-07-11, produção incluída com confirmação digitada).
func (a *App) deployBridge(current *config.Server) ([]devserver.DeployServerInfo, func(ctx context.Context, name, password string) (*fluig.Client, string, error)) {
	store := a.Store()
	list, err := store.List()
	if err != nil {
		list = nil
	}
	defName, _ := store.DefaultName()
	infos := make([]devserver.DeployServerInfo, 0, len(list))
	for _, s := range list {
		infos = append(infos, devserver.DeployServerInfo{
			Name:    s.Name,
			Env:     s.Env,
			URL:     s.BaseURL(),
			Default: s.Name == defName,
			Current: s.Name == current.Name,
		})
	}

	connect := func(ctx context.Context, name, password string) (*fluig.Client, string, error) {
		target, err := store.Get(name)
		if err != nil {
			return nil, "", err
		}
		// Identidade sem prompt: o request HTTP não pode travar no terminal.
		if target.Username == "" {
			if v := os.Getenv(config.EnvUsername); v != "" {
				target.Username, target.UserCode = v, v
			} else {
				return nil, "", fmt.Errorf(
					"o servidor %q não tem usuário definido — rode uma vez no terminal: fluigcli server test %s", name, name)
			}
		} else if target.UserCode == "" {
			target.UserCode = target.Username
		}
		// Sessão em cache vale como credencial (igual ao authenticate da CLI).
		if password == "" && !a.NoSessionCache {
			if client, err := a.clientFor(target, ""); err == nil && client.RestoreSession(ctx) {
				return client, target.FormScopeKey(), nil
			}
		}
		pw := password
		if pw == "" {
			res, err := (config.PasswordSource{Getenv: os.Getenv, Keyring: a.Keyring}).Resolve(target)
			if err != nil {
				return nil, "", devserver.ErrDeployNeedsPassword
			}
			pw = res.Password
		}
		client, err := a.clientFor(target, pw)
		if err != nil {
			return nil, "", err
		}
		if err := client.EnsureSession(ctx); err != nil {
			if errors.Is(err, fluig.ErrAuthFailed) {
				return nil, "", fmt.Errorf("%w (autenticação recusada em %q)", devserver.ErrDeployNeedsPassword, name)
			}
			return nil, "", err
		}
		client.SaveSession()
		return client, target.FormScopeKey(), nil
	}
	return infos, connect
}
