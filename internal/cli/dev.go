package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/devserver"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newDevCmd(app *App) *cobra.Command {
	var (
		listen        string
		port          int
		debounce      time.Duration
		npmWatch      bool
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
			"    recarrega sozinho.\n" +
			"  • Dashboard na raiz (/): acessos rápidos, watch integrado (publicar\n" +
			"    ao salvar, por tipo de artefato) e configurações do live reload.\n\n" +
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
			watch := newDevWatch(app, client, server, root, debounce)
			srv, err := devserver.New(devserver.Options{
				Root:          root,
				Upstream:      client.BaseURL(),
				Jar:           client.SessionJar(),
				Host:          listen,
				Port:          port,
				Debounce:      debounce,
				NpmWatch:      npmWatch,
				Infof:         p.Infof,
				Warnf:         p.Warnf,
				Client:        client,
				FormScope:     server.FormScopeKey(),
				CompanyID:     server.CompanyID,
				DeployServers: deployServers,
				DeployConnect: deployConnect,
				ServerName:    server.Name,
				ServerEnv:     server.Env,
				Username:      server.Username,
				Watch:         watch,
			})
			if err != nil {
				return output.Genericf("não consegui montar o dev server: %v", err)
			}

			watch.start(ctx)
			// Saída enxuta (pedido do mantenedor, 2026-07-11): status, o link
			// do dashboard e um resumo de uma linha — portal, preview,
			// widgets e configurações vivem no dashboard.
			env := ""
			if server.Env != "" {
				env = " (" + server.Env + ")"
			}
			p.Successf("Dev server de %q%s no ar — Ctrl+C para parar.", server.Name, env)
			p.Infof("Dashboard: %s/", srv.URL())
			p.Infof("%s", devSummaryLine(root, srv.Mounts(), watch.Status()))
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
	cmd.Flags().BoolVar(&npmWatch, "npm-watch", false, "roda o `npm run watch` das widgets SPA (vue/react) junto com o dev server")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// devSummaryLine resume o ambiente numa linha: widgets do disco, formulários
// do projeto e o estado do watch integrado (que publica sozinho — merece
// destaque quando ligado).
func devSummaryLine(root string, mounts []string, watch devserver.WatchStatus) string {
	forms := 0
	if entries, err := os.ReadDir(filepath.Join(root, project.FormsDirName)); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				forms++
			}
		}
	}
	parts := []string{
		fmt.Sprintf("%d widget(s) do disco", len(mounts)),
		fmt.Sprintf("%d formulário(s)", forms),
	}
	if watch.Enabled && len(watch.Types) > 0 {
		parts = append(parts, "watch LIGADO ("+strings.Join(watch.Types, ", ")+")")
	} else {
		parts = append(parts, "watch desligado")
	}
	return strings.Join(parts, " · ") + " — gerencie no dashboard"
}

// --- watch integrado do dashboard ---

// devWatchConfig é o estado persistido em .fluigcli/dev.json (git-ignorado).
type devWatchConfig struct {
	Watch struct {
		Enabled bool     `json:"enabled"`
		Types   []string `json:"types"`
	} `json:"watch"`
}

func devWatchConfigPath(root string) string {
	return filepath.Join(root, ".fluigcli", "dev.json")
}

// devWatch é o watch integrado ao dev: o mesmo loop/publicação do `fluigcli
// watch` (classifyWatchPath + watchSession.publishOutcome), mas ligável por
// tipo de artefato pelo dashboard, com feed das últimas publicações.
type devWatch struct {
	app      *App
	root     string
	debounce time.Duration
	session  *watchSession

	mu        sync.Mutex
	enabled   bool
	types     map[string]bool
	recent    []string
	available bool
}

func newDevWatch(app *App, client *fluig.Client, server *config.Server, root string, debounce time.Duration) *devWatch {
	dw := &devWatch{
		app: app, root: root, debounce: debounce,
		types: map[string]bool{},
		session: &watchSession{
			app: app, client: client, root: root,
			formScope: server.FormScopeKey(), published: map[string]string{},
		},
	}
	// Restaura a escolha persistida da última execução.
	if b, err := os.ReadFile(devWatchConfigPath(root)); err == nil {
		var cfg devWatchConfig
		if json.Unmarshal(b, &cfg) == nil {
			dw.enabled = cfg.Watch.Enabled
			for _, t := range cfg.Watch.Types {
				dw.types[t] = true
			}
		}
	}
	return dw
}

// Status implementa devserver.WatchBridge.
func (dw *devWatch) Status() devserver.WatchStatus {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	types := make([]string, 0, len(dw.types))
	for t, on := range dw.types {
		if on {
			types = append(types, t)
		}
	}
	sort.Strings(types)
	recent := make([]string, len(dw.recent))
	copy(recent, dw.recent)
	return devserver.WatchStatus{Available: dw.available, Enabled: dw.enabled, Types: types, Recent: recent}
}

// Set implementa devserver.WatchBridge: aplica e persiste a escolha.
func (dw *devWatch) Set(enabled bool, types []string) error {
	dw.mu.Lock()
	dw.enabled = enabled
	dw.types = map[string]bool{}
	for _, t := range types {
		dw.types[t] = true
	}
	dw.mu.Unlock()

	var cfg devWatchConfig
	cfg.Watch.Enabled = enabled
	cfg.Watch.Types = append([]string{}, types...)
	sort.Strings(cfg.Watch.Types)
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.MkdirAll(filepath.Dir(devWatchConfigPath(dw.root)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(devWatchConfigPath(dw.root), append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("não consegui gravar .fluigcli/dev.json: %v", err)
	}
	if err := ensureGitignoreEntry(dw.root, ".fluigcli/dev.json", "preferências locais do dev server (não versionar)"); err != nil {
		dw.app.printer.Warnf("não foi possível atualizar o .gitignore: %v", err)
	}
	if enabled && len(types) > 0 {
		dw.app.printer.Infof("watch integrado ligado (%s)", strings.Join(cfg.Watch.Types, ", "))
	} else {
		dw.app.printer.Infof("watch integrado desligado")
	}
	return nil
}

// allowed decide se a unidade salva deve ser publicada agora.
func (dw *devWatch) allowed(typ string) bool {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	return dw.enabled && dw.types[typ]
}

// record alimenta o feed do dashboard (mais novo primeiro, máx. 8).
func (dw *devWatch) record(msg string) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	dw.recent = append([]string{msg}, dw.recent...)
	if len(dw.recent) > 8 {
		dw.recent = dw.recent[:8]
	}
}

// start sobe o observador em goroutine (mesma mecânica do runWatch: debounce
// por unidade; pastas novas entram na observação). O filtro por tipo é
// avaliado NO DISPARO — mudar a config no dashboard vale na hora.
func (dw *devWatch) start(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		dw.app.printer.Warnf("watch integrado indisponível: %v", err)
		return
	}
	watchedAny := false
	for _, dir := range watchDirs {
		base := filepath.Join(dw.root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		if watchRecursive(w, base) == nil {
			watchedAny = true
		}
	}
	dw.mu.Lock()
	dw.available = watchedAny
	dw.mu.Unlock()
	if !watchedAny {
		_ = w.Close()
		return
	}

	go func() {
		defer w.Close()
		pending := map[string]*time.Timer{}
		fire := make(chan watchUnit, 32)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
					continue
				}
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = watchRecursive(w, ev.Name)
					continue
				}
				unit, ok := classifyWatchPath(dw.root, ev.Name)
				if !ok {
					continue
				}
				if t, ok := pending[unit.path]; ok {
					t.Stop()
				}
				u := unit
				pending[u.path] = time.AfterFunc(dw.debounce, func() {
					select {
					case fire <- u:
					case <-ctx.Done():
					}
				})
			case u := <-fire:
				delete(pending, u.path)
				if !dw.allowed(u.typ) {
					continue
				}
				if _, err := os.Stat(u.path); err != nil {
					continue
				}
				level, msg := dw.session.publishOutcome(ctx, u)
				dw.record(msg)
				switch level {
				case "success":
					dw.app.printer.Successf("%s", msg)
				case "warn":
					dw.app.printer.Warnf("%s", msg)
				default:
					dw.app.printer.Infof("%s", msg)
				}
			case werr, ok := <-w.Errors:
				if !ok {
					return
				}
				dw.app.printer.Warnf("watch integrado: %v", werr)
			}
		}
	}()
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
