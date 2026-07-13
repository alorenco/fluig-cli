// Package cli implementa os comandos cobra do fluigcli. Comandos e flags em
// inglês; mensagens para humanos em pt-BR.
package cli

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// Variáveis de ambiente das flags globais.
const (
	envServer         = "FLUIGCLI_SERVER"
	envProject        = "FLUIGCLI_PROJECT"
	envNonInteractive = "FLUIGCLI_NON_INTERACTIVE"
	envTimeout        = "FLUIGCLI_TIMEOUT"
	envNoSessionCache = "FLUIGCLI_NO_SESSION_CACHE"
	envNoUpdateCheck  = "FLUIGCLI_NO_UPDATE_CHECK"
)

// App carrega o estado compartilhado entre os comandos de uma execução.
type App struct {
	// Flags globais
	Server         string
	Project        string
	JSON           bool
	Yes            bool
	NonInteractive bool
	Verbose        bool
	Timeout        time.Duration
	NoSessionCache bool

	Version string
	Commit  string
	Date    string

	Keyring config.Keyring

	printer      *output.Printer
	ranCommand   bool
	projectRoot  string
	sessionCache *config.DiskSessionCache
}

// printerFor inicializa o Printer do comando corrente (chamado no início de cada RunE).
func (a *App) printerFor(cmd *cobra.Command) *output.Printer {
	a.ranCommand = true
	name := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "fluigcli"))
	a.printer = output.NewPrinter(a.JSON, name)
	a.printer.Server = a.Server
	return a.printer
}

// Interactive informa se prompts são permitidos: exige TTY e ausência de
// --non-interactive/--json.
func (a *App) Interactive() bool {
	return !a.NonInteractive && !a.JSON && output.StdoutIsTTY() && output.StdinIsTTY()
}

// ProjectRoot resolve a raiz do projeto (--project > env > descoberta automática).
func (a *App) ProjectRoot() string {
	if a.Project != "" {
		return a.Project
	}
	if a.projectRoot == "" {
		cwd, err := os.Getwd()
		if err == nil {
			a.projectRoot = project.FindRoot(cwd)
		}
	}
	return a.projectRoot
}

// Store retorna o Store de servidores com a precedência projeto > global.
func (a *App) Store() *config.Store {
	return config.NewStore(a.ProjectRoot())
}

// clientFor cria o cliente Fluig para um servidor já com senha resolvida.
func (a *App) clientFor(server *config.Server, password string) (*fluig.Client, error) {
	var sc fluig.SessionCache
	if dc := a.diskSessionCache(); dc != nil {
		sc = dc
	}
	return fluig.NewClient(fluig.Options{
		BaseURL:      server.BaseURL(),
		Username:     server.Username,
		Password:     password,
		CompanyID:    server.CompanyID,
		Timeout:      a.Timeout,
		Verbose:      a.Verbose,
		LogWriter:    os.Stderr,
		SessionCache: sc,
	})
}

// diskSessionCache devolve o cache de sessão em disco (nil se desativado por
// --no-session-cache ou se o diretório de cache não estiver acessível).
func (a *App) diskSessionCache() *config.DiskSessionCache {
	if a.NoSessionCache {
		return nil
	}
	if a.sessionCache == nil {
		c, err := config.NewDiskSessionCache()
		if err != nil {
			return nil
		}
		a.sessionCache = c
	}
	return a.sessionCache
}

// mapFluigError traduz erros do pacote fluig para erros tipados da CLI.
func mapFluigError(err error) error {
	if err == nil {
		return nil
	}
	var cliErr *output.Error
	if errors.As(err, &cliErr) {
		return err
	}
	if errors.Is(err, fluig.ErrAuthFailed) {
		return output.AuthFailedf("%s", err.Error()).WithCause(err)
	}
	if errors.Is(err, fluig.ErrNotFound) {
		return output.NotFoundf("%s", err.Error()).WithCause(err)
	}
	if errors.Is(err, fluig.ErrHelperMissing) {
		return output.MissingHelperf("%s", err.Error()).WithCause(err)
	}
	var httpErr *fluig.HTTPError
	if errors.As(err, &httpErr) {
		return output.ServerErrorf("%s", httpErr.Error()).WithCause(err)
	}
	return output.ServerErrorf("%s", err.Error()).WithCause(err)
}

// Grupos de comandos do help.
const (
	groupDev    = "dev"
	groupOps    = "ops"
	groupConfig = "config"
)

func newRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "fluigcli",
		Short: "CLI não oficial para desenvolvimento TOTVS Fluig",
		Long: "CLI não oficial para desenvolvimento TOTVS Fluig.\n\n" +
			"Versão:\n  fluigcli " + app.Version + " " + runtime.GOOS + "-" + runtime.GOARCH,
		Version:       app.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return app.applyEnvDefaults(cmd)
		},
	}
	root.SetVersionTemplate("fluigcli " + app.Version + "\n")
	root.AddGroup(
		&cobra.Group{ID: groupDev, Title: "Desenvolvimento:"},
		&cobra.Group{ID: groupOps, Title: "Operação:"},
		&cobra.Group{ID: groupConfig, Title: "Configuração:"},
	)

	pf := root.PersistentFlags()
	pf.StringVar(&app.Server, "server", "", "servidor alvo (env: FLUIGCLI_SERVER)")
	pf.StringVar(&app.Project, "project", "", "raiz do projeto Fluig (default: descoberta automática; env: FLUIGCLI_PROJECT)")
	pf.BoolVar(&app.JSON, "json", false, "saída JSON estruturada (implica modo não-interativo)")
	pf.BoolVarP(&app.Yes, "yes", "y", false, "assume \"sim\" em confirmações")
	pf.BoolVar(&app.NonInteractive, "non-interactive", false, "falha se faltarem argumentos, em vez de perguntar (env: FLUIGCLI_NON_INTERACTIVE=1)")
	pf.BoolVarP(&app.Verbose, "verbose", "v", false, "loga as requisições HTTP no stderr")
	pf.DurationVar(&app.Timeout, "timeout", 30*time.Second, "timeout por requisição (env: FLUIGCLI_TIMEOUT)")
	pf.BoolVar(&app.NoSessionCache, "no-session-cache", false, "não reaproveita a sessão entre execuções (env: FLUIGCLI_NO_SESSION_CACHE=1)")
	// A flag --version é definida aqui (em pt-BR) para o cobra não criar a dele.
	root.Flags().Bool("version", false, "mostra a versão do fluigcli")

	addToGroup := func(group string, cmds ...*cobra.Command) {
		for _, c := range cmds {
			c.GroupID = group
			root.AddCommand(c)
		}
	}
	addToGroup(groupDev,
		newDatasetCmd(app),
		newEventCmd(app),
		newMechanismCmd(app),
		newFormCmd(app),
		newWorkflowCmd(app),
		newWidgetCmd(app),
		newDiffCmd(app),
		newWatchCmd(app),
		newDevCmd(app),
	)
	addToGroup(groupOps, newRequestCmd(app))
	addToGroup(groupConfig, newServerCmd(app))
	// Sem grupo (aparecem em "Comandos adicionais:").
	root.AddCommand(newVersionCmd(app))
	root.AddCommand(newUpgradeCmd(app))
	root.AddCommand(newSkillCmd(app))
	root.AddCommand(newCompletionCmd())
	localize(root)

	// Erros de parsing de flag em pt-BR (subcomandos herdam do pai).
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return output.Usagef("%s", translateUsageMessage(err.Error()))
	})
	return root
}

// applyEnvDefaults aplica as env vars quando a flag correspondente não foi passada.
func (a *App) applyEnvDefaults(cmd *cobra.Command) error {
	flags := cmd.Flags()
	if !flags.Changed("server") {
		if v := os.Getenv(envServer); v != "" {
			a.Server = v
		}
	}
	if !flags.Changed("project") {
		if v := os.Getenv(envProject); v != "" {
			a.Project = v
		}
	}
	if !flags.Changed("non-interactive") {
		if v := os.Getenv(envNonInteractive); v == "1" || strings.EqualFold(v, "true") {
			a.NonInteractive = true
		}
	}
	if !flags.Changed("no-session-cache") {
		if v := os.Getenv(envNoSessionCache); v == "1" || strings.EqualFold(v, "true") {
			a.NoSessionCache = true
		}
	}
	if !flags.Changed("timeout") {
		if v := os.Getenv(envTimeout); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return output.Usagef("valor inválido em %s: %q (use formatos como 30s, 1m)", envTimeout, v)
			}
			a.Timeout = d
		}
	}
	return nil
}

// newKeyring cria o keyring usado pela CLI. É uma variável para permitir a
// injeção de um keyring em memória nos testes.
var newKeyring = config.SystemKeyring

// Main executa a CLI e retorna o exit code do contrato.
func Main(version, commit, date string) int {
	app := &App{
		Version: version,
		Commit:  commit,
		Date:    date,
		Keyring: newKeyring(),
	}
	root := newRootCmd(app)

	err := root.Execute()
	if err == nil {
		maybeNotifyUpdate(app)
		return output.ExitOK
	}

	// Erros antes de qualquer RunE (flag/comando desconhecido) são de uso: exit 2.
	var cliErr *output.Error
	if !errors.As(err, &cliErr) && !app.ranCommand {
		err = output.Usagef("%s", translateUsageMessage(err.Error()))
	}
	if app.printer == nil {
		// A falha pode ter ocorrido antes do parse da flag --json; para manter o
		// contrato do envelope, detecta a flag direto nos argumentos.
		jsonMode := app.JSON
		for _, arg := range os.Args[1:] {
			if arg == "--json" {
				jsonMode = true
				break
			}
		}
		app.printer = output.NewPrinter(jsonMode, "")
		app.printer.Server = app.Server
	}
	return app.printer.Fail(err)
}
