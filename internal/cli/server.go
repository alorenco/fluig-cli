package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newServerCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Gerencia os servidores Fluig cadastrados",
	}
	cmd.AddCommand(newServerAddCmd(app))
	cmd.AddCommand(newServerListCmd(app))
	cmd.AddCommand(newServerUseCmd(app))
	cmd.AddCommand(newServerUpdateCmd(app))
	cmd.AddCommand(newServerRemoveCmd(app))
	cmd.AddCommand(newServerTestCmd(app))
	cmd.AddCommand(newServerInstallHelperCmd(app))
	cmd.AddCommand(newServerLogoutCmd(app))
	return cmd
}

// --- server logout ---

func newServerLogoutCmd(app *App) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "logout [<name>]",
		Short: "Descarta a sessão em cache de um servidor (ou de todos com --all)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			cache, err := config.NewDiskSessionCache()
			if err != nil {
				return output.Genericf("cache de sessão indisponível: %v", err)
			}
			if all {
				if err := cache.Clear(""); err != nil {
					return output.Genericf("falha ao limpar as sessões: %v", err)
				}
				p.Successf("Todas as sessões em cache foram descartadas.")
				p.Done(map[string]any{"cleared": "all"})
				return nil
			}
			nameArg := ""
			if len(args) == 1 {
				nameArg = args[0]
			}
			server, err := app.resolveServer(nameArg)
			if err != nil {
				return err
			}
			p.Server = server.Name
			key, err := fluig.SessionKeyFor(server.BaseURL(), server.Username)
			if err != nil {
				return output.Usagef("%s", err.Error())
			}
			if err := cache.Clear(key); err != nil {
				return output.Genericf("falha ao limpar a sessão: %v", err)
			}
			p.Successf("Sessão em cache de %q descartada.", server.Name)
			p.Done(map[string]any{"cleared": server.Name})
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "descarta as sessões de todos os servidores")
	return cmd
}

// helperWARURL é o WAR oficial da fluiggersWidget.
const helperWARURL = "https://raw.githubusercontent.com/fluiggers/fluig-widget-helper/refs/heads/master/target/fluiggersWidget.war"

func newServerInstallHelperCmd(app *App) *cobra.Command {
	var (
		warPath       string
		warSHA256     string
		passwordStdin bool
		force         bool
	)
	cmd := &cobra.Command{
		Use:   "install-helper [<name>]",
		Short: "Instala a widget auxiliar fluiggersWidget no servidor (necessária para scripts de processo)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			nameArg := ""
			if len(args) == 1 {
				nameArg = args[0]
			}
			server, err := app.resolveServer(nameArg)
			if err != nil {
				return err
			}
			p.Server = server.Name
			if err := app.guardProdWrite(server, "instalar a widget auxiliar"); err != nil {
				return err
			}

			ctx := context.Background()
			client, err := app.authenticate(ctx, server, passwordStdin)
			if err != nil {
				return err
			}

			if !force {
				if installed, _ := client.HelperInstalled(ctx); installed {
					p.Successf("fluiggersWidget já está instalada em %s.", server.Name)
					p.Done(map[string]any{"installed": true, "action": "none"})
					return nil
				}
			}

			war, origem, err := loadHelperWAR(ctx, warPath)
			if err != nil {
				return err
			}
			// Integridade: exibe o SHA-256 e, se --war-sha256 foi passado, verifica.
			sum := sha256.Sum256(war)
			gotSHA := hex.EncodeToString(sum[:])
			if warSHA256 != "" && !strings.EqualFold(warSHA256, gotSHA) {
				return output.Genericf("SHA-256 do WAR não confere: esperado %s, obtido %s (abortado)", warSHA256, gotSHA)
			}
			p.Infof("Publicando fluiggersWidget (%d KB, de %s) — sha256=%s", len(war)/1024, origem, gotSHA)
			if warSHA256 == "" {
				p.Warnf("integridade do WAR não verificada; para fixar, use --war-sha256 <hash> (ou --war com artefato revisado)")
			}
			if err := client.UploadWidgetWAR(ctx, "fluiggersWidget.war", war); err != nil {
				return mapFluigError(err)
			}
			p.Successf("WAR enviado. A instalação da widget é assíncrona no servidor — aguarde alguns instantes e valide com um comando de workflow.")
			p.Done(map[string]any{"installed": true, "action": "uploaded"})
			return nil
		},
	}
	cmd.Flags().StringVar(&warPath, "war", "", "usa um WAR local em vez de baixá-lo da internet")
	cmd.Flags().StringVar(&warSHA256, "war-sha256", "", "verifica o SHA-256 do WAR antes de publicar (integridade)")
	cmd.Flags().BoolVar(&force, "force", false, "reenvia mesmo se a widget já estiver instalada")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// loadHelperWAR devolve o WAR: de --war (local) ou baixado do repositório.
func loadHelperWAR(ctx context.Context, warPath string) ([]byte, string, error) {
	if warPath != "" {
		data, err := os.ReadFile(warPath)
		if err != nil {
			return nil, "", output.NotFoundf("não foi possível ler o WAR %q: %v", warPath, err)
		}
		return data, warPath, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, helperWARURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", output.Genericf("falha ao baixar o WAR da fluiggersWidget: %v (use --war <arquivo> para instalar offline)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", output.Genericf("download do WAR retornou HTTP %d (use --war <arquivo>)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, "repositório oficial da widget", nil
}

// resolveServer determina o servidor alvo: argumento posicional > --server/env >
// servidor padrão (projeto > global) > único cadastrado > seleção interativa >
// erro de uso.
func (a *App) resolveServer(nameArg string) (*config.Server, error) {
	name := nameArg
	if name == "" {
		name = a.Server
	}
	store := a.Store()
	if name != "" {
		return store.Get(name)
	}

	if def, err := store.DefaultName(); err != nil {
		return nil, err
	} else if def != "" {
		server, err := store.Get(def)
		if err != nil {
			return nil, output.NotFoundf(
				"o servidor padrão %q não existe mais; escolha outro com: fluigcli server use", def)
		}
		return server, nil
	}

	servers, err := store.List()
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, output.NotFoundf("nenhum servidor cadastrado; adicione um com: fluigcli server add")
	}
	if len(servers) == 1 {
		return &servers[0], nil
	}
	if !a.Interactive() {
		return nil, output.Usagef("informe o servidor alvo com --server <nome> (ou FLUIGCLI_SERVER), " +
			"ou defina um padrão com: fluigcli server use <nome>")
	}

	server, err := pickServerInteractive(servers)
	if err != nil {
		return nil, err
	}
	// Resolve a pergunta de uma vez: oferece fixar a escolha como padrão.
	if save, err := promptYesNo(fmt.Sprintf("Definir %q como servidor padrão?", server.Name), false); err == nil && save {
		if _, err := store.SetDefault(server.Name, false); err == nil {
			fmt.Fprintf(os.Stderr, "Servidor padrão definido: %s (troque com: fluigcli server use)\n", server.Name)
		}
	}
	return server, nil
}

// pickServerInteractive lista os servidores no stderr e lê a escolha.
func pickServerInteractive(servers []config.Server) (*config.Server, error) {
	fmt.Fprintln(os.Stderr, "Servidores disponíveis:")
	for i, s := range servers {
		env := ""
		if s.Env != "" {
			env = " [" + s.Env + "]"
		}
		fmt.Fprintf(os.Stderr, "  %d) %s%s (%s)\n", i+1, s.Name, env, s.BaseURL())
	}
	n, err := promptInt("Escolha o servidor", 1)
	if err != nil {
		return nil, err
	}
	if n < 1 || n > len(servers) {
		return nil, output.Usagef("opção inválida: %d", n)
	}
	return &servers[n-1], nil
}

// passwordSource monta a cadeia de resolução de senha.
func (a *App) passwordSource(passwordStdin bool) config.PasswordSource {
	src := config.PasswordSource{
		Getenv:  os.Getenv,
		Keyring: a.Keyring,
	}
	if passwordStdin {
		src.Stdin = os.Stdin
	}
	if a.Interactive() {
		keyringOK := a.Keyring != nil && a.Keyring.Available()
		src.Prompt = func(server *config.Server) (string, bool, error) {
			pw, err := promptPassword(fmt.Sprintf("Senha de %s em %s", server.Username, server.Name))
			if err != nil {
				return "", false, err
			}
			if pw == "" {
				return "", false, output.AuthFailedf("senha vazia")
			}
			// Só oferece salvar quando há keyring disponível (senão não faria nada).
			if !keyringOK {
				return pw, false, nil
			}
			save, err := promptYesNo("Salvar a senha no keyring do sistema?", true)
			if err != nil {
				return "", false, err
			}
			return pw, save, nil
		}
	}
	return src
}

// --- server add ---

func newServerAddCmd(app *App) *cobra.Command {
	var (
		name, host, username string
		port, companyID      int
		ssl                  bool
		passwordStdin        bool
		global               bool
		env                  string
		makeDefault          bool
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Cadastra um servidor Fluig (a senha vai para o keyring, nunca para arquivo)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)

			interactive := app.Interactive()
			var err error
			if name == "" {
				if !interactive {
					return output.Usagef("--name é obrigatória em modo não-interativo")
				}
				if name, err = promptLine("Nome do servidor (ex.: homolog)", ""); err != nil {
					return err
				}
			}
			if host == "" {
				if !interactive {
					return output.Usagef("--host é obrigatória em modo não-interativo")
				}
				if host, err = promptLine("Host (sem esquema, ex.: fluig.empresa.com.br)", ""); err != nil {
					return err
				}
			}
			if username == "" {
				if !interactive {
					return output.Usagef("--username é obrigatória em modo não-interativo")
				}
				if username, err = promptLine("Usuário", ""); err != nil {
					return err
				}
			}
			if name == "" || host == "" || username == "" {
				return output.Usagef("nome, host e usuário são obrigatórios")
			}
			host = stripScheme(host)

			if interactive {
				if !cmd.Flags().Changed("ssl") {
					if ssl, err = promptYesNo("Usar HTTPS?", true); err != nil {
						return err
					}
				}
				if !cmd.Flags().Changed("port") {
					def := 443
					if !ssl {
						def = 80
					}
					if port, err = promptInt("Porta", def); err != nil {
						return err
					}
				}
				if !cmd.Flags().Changed("company-id") {
					if companyID, err = promptInt("Company ID", 1); err != nil {
						return err
					}
				}
				if !cmd.Flags().Changed("env") {
					if env, err = promptLine("Ambiente (dev/hml/prod; Enter para pular)", ""); err != nil {
						return err
					}
				}
			}
			if env, err = config.NormalizeEnv(env); err != nil {
				return output.Usagef("%s", err.Error())
			}

			server := config.Server{
				Name:      name,
				Host:      host,
				Port:      port,
				SSL:       ssl,
				Username:  username,
				UserCode:  username,
				CompanyID: companyID,
				Env:       env,
			}

			// Senha: só é persistida no keyring do SO. Sem keyring (ex.: Linux
			// headless), a senha é informada por FLUIGCLI_PASSWORD/--password-stdin
			// a cada chamada — não há onde guardar.
			keyringOK := app.Keyring != nil && app.Keyring.Available()
			switch {
			case !keyringOK:
				p.Infof("keyring do sistema indisponível — informe a senha via FLUIGCLI_PASSWORD ou --password-stdin nas chamadas")
			case passwordStdin:
				pw, err := config.PasswordSource{Stdin: os.Stdin}.Resolve(&server)
				if err != nil {
					return err
				}
				if err := app.Keyring.Set(server.KeyringKey(), pw.Password); err != nil {
					p.Warnf("não foi possível salvar a senha no keyring: %v", err)
				}
			case interactive:
				pw, err := promptPassword("Senha (Enter para não salvar agora)")
				if err != nil {
					return err
				}
				if pw != "" {
					if err := app.Keyring.Set(server.KeyringKey(), pw); err != nil {
						p.Warnf("não foi possível salvar a senha no keyring: %v", err)
					}
				}
			default:
				p.Infof("nenhuma senha informada; use FLUIGCLI_PASSWORD ou --password-stdin nas próximas chamadas")
			}

			if err := app.Store().Add(server, global); err != nil {
				return err
			}

			// Em projeto, a identidade foi para o overlay pessoal (git-ignorado)
			// e só a conexão ao servers.json versionável — garante o .gitignore.
			if !global && app.ProjectRoot() != "" {
				if err := ensureLocalGitignore(app.ProjectRoot()); err != nil {
					p.Warnf("não foi possível atualizar o .gitignore: %v", err)
				}
				p.Infof("Conexão salva em .fluigcli/servers.json (versionável); seu usuário em .fluigcli/servers.local.json (git-ignorado).")
			}

			// Primeiro servidor cadastrado vira padrão sem perguntar; nos
			// demais, --default decide (ou a pergunta, no modo interativo).
			isDefault := makeDefault
			if !isDefault {
				if all, listErr := app.Store().List(); listErr == nil && len(all) == 1 {
					isDefault = true
				} else if interactive {
					isDefault, _ = promptYesNo(fmt.Sprintf("Definir %q como servidor padrão?", server.Name), false)
				}
			}
			if isDefault {
				if _, err := app.Store().SetDefault(server.Name, global); err != nil {
					p.Warnf("não foi possível definir o servidor padrão: %v", err)
					isDefault = false
				}
			}

			suffix := ""
			if isDefault {
				suffix = " — definido como padrão"
			}
			p.Successf("Servidor %q cadastrado (%s)%s.", server.Name, server.BaseURL(), suffix)
			p.Done(map[string]any{"server": server, "default": isDefault})
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "nome do servidor (ex.: homolog)")
	f.StringVar(&host, "host", "", "host do Fluig, sem esquema (ex.: fluig.empresa.com.br)")
	f.IntVar(&port, "port", 443, "porta do servidor")
	f.BoolVar(&ssl, "ssl", true, "usar HTTPS")
	f.StringVar(&username, "username", "", "usuário de login")
	f.IntVar(&companyID, "company-id", 1, "companyId do tenant")
	f.StringVar(&env, "env", "", "ambiente do servidor: dev, hml ou prod (prod pede confirmação para escrever)")
	f.BoolVar(&makeDefault, "default", false, "define este servidor como o padrão")
	f.BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin e grava no keyring")
	f.BoolVar(&global, "global", false, "grava na configuração global em vez da do projeto")
	return cmd
}

// --- server use ---

func newServerUseCmd(app *App) *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "use [<name>]",
		Short: "Define o servidor padrão (usado quando --server não é informado)",
		Long: "Grava o servidor padrão no servers.local.json do projeto — pessoal e\n" +
			"git-ignorado, então seu padrão não vaza para o time — ou no global com\n" +
			"--global. O padrão do projeto vence o global; --server e FLUIGCLI_SERVER\n" +
			"continuam vencendo tudo, por execução.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			store := app.Store()

			name := ""
			if len(args) == 1 {
				name = args[0]
			} else {
				servers, err := store.List()
				if err != nil {
					return err
				}
				if len(servers) == 0 {
					return output.NotFoundf("nenhum servidor cadastrado; adicione um com: fluigcli server add")
				}
				if !app.Interactive() {
					return output.Usagef("informe o servidor: fluigcli server use <nome>")
				}
				s, err := pickServerInteractive(servers)
				if err != nil {
					return err
				}
				name = s.Name
			}

			path, err := store.SetDefault(name, global)
			if err != nil {
				return err
			}
			p.Server = name
			p.Successf("Servidor padrão definido: %s (em %s)", name, path)
			p.Done(map[string]any{"default": name, "path": path})
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "grava na configuração global em vez da do projeto")
	return cmd
}

// --- server update ---

func newServerUpdateCmd(app *App) *cobra.Command {
	var (
		host, username, env string
		port, companyID     int
		ssl                 bool
	)
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Altera dados de um servidor cadastrado (ex.: --env prod)",
		Long: "Atualiza campos do cadastro sem remover o servidor (a senha no keyring é\n" +
			"preservada). O nome não muda — ele é a chave do cadastro; para renomear,\n" +
			"remova e cadastre de novo.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			flags := cmd.Flags()
			changed := false
			for _, f := range []string{"host", "port", "ssl", "username", "company-id", "env"} {
				if flags.Changed(f) {
					changed = true
					break
				}
			}
			if !changed {
				return output.Usagef("nada a alterar: informe ao menos uma flag (--host, --port, --ssl, --username, --company-id, --env)")
			}

			normEnv := ""
			if flags.Changed("env") {
				var err error
				if normEnv, err = config.NormalizeEnv(env); err != nil {
					return output.Usagef("%s", err.Error())
				}
			}

			server, err := app.Store().Update(args[0], func(s *config.Server) {
				if flags.Changed("host") {
					s.Host = stripScheme(host)
				}
				if flags.Changed("port") {
					s.Port = port
				}
				if flags.Changed("ssl") {
					s.SSL = ssl
				}
				if flags.Changed("username") {
					s.Username = username
					s.UserCode = username
				}
				if flags.Changed("company-id") {
					s.CompanyID = companyID
				}
				if flags.Changed("env") {
					s.Env = normEnv
				}
			})
			if err != nil {
				return err
			}
			p.Server = server.Name
			p.Successf("Servidor %q atualizado (%s).", server.Name, server.BaseURL())
			p.Done(map[string]any{"server": server})
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&host, "host", "", "host do Fluig, sem esquema")
	f.IntVar(&port, "port", 443, "porta do servidor")
	f.BoolVar(&ssl, "ssl", true, "usar HTTPS")
	f.StringVar(&username, "username", "", "usuário de login")
	f.IntVar(&companyID, "company-id", 1, "companyId do tenant")
	f.StringVar(&env, "env", "", "ambiente: dev, hml, prod ou \"\" para limpar")
	return cmd
}

// ensureLocalGitignore garante que .fluigcli/servers.local.json (identidade
// pessoal) esteja no .gitignore do projeto — para não ser versionado por engano.
// Idempotente: não duplica a entrada nem sobrescreve o resto do arquivo.
func ensureLocalGitignore(projectRoot string) error {
	const entry = ".fluigcli/servers.local.json"
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil // já ignorado
		}
	}
	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	block := prefix + "\n# fluigcli: identidade pessoal (não versionar)\n" + entry + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(block)
	return err
}

// stripScheme remove http(s):// e barras finais de um host informado por engano.
func stripScheme(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return strings.TrimRight(host, "/")
}

// --- server list ---

func newServerListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os servidores cadastrados (projeto e global)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			servers, err := app.Store().List()
			if err != nil {
				return err
			}
			if servers == nil {
				servers = []config.Server{}
			}
			def, err := app.Store().DefaultName()
			if err != nil {
				return err
			}
			if len(servers) == 0 {
				p.Infof("Nenhum servidor cadastrado. Adicione com: fluigcli server add")
			} else {
				renderServerTable(p, servers, def)
			}
			p.Done(map[string]any{"servers": servers, "default": def})
			return nil
		},
	}
}

// renderServerTable imprime os servidores em uma tabela com bordas; marca o
// padrão com "●", ordena-o em primeiro e, em terminal, colore cabeçalho e
// marcador. Fora de terminal (pipe/redirecionamento) sai sem cores, preservando
// o texto legível.
func renderServerTable(p *output.Printer, servers []config.Server, def string) {
	// Padrão efetivo: o explícito (server use) ou, se não houver, o único
	// cadastrado — que a CLI usa implicitamente (mesma regra do resolveServer).
	effectiveDef, implicit := def, false
	if effectiveDef == "" && len(servers) == 1 {
		effectiveDef, implicit = servers[0].Name, true
	}

	// O padrão sempre aparece primeiro; os demais mantêm a ordem de cadastro.
	ordered := make([]config.Server, len(servers))
	copy(ordered, servers)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Name == effectiveDef && ordered[j].Name != effectiveDef
	})

	headers := []string{"", "Nome", "Ambiente", "URL", "Usuário", "Company"}
	rows := make([][]string, 0, len(ordered))
	defaultIdx := -1
	for i, s := range ordered {
		marker := ""
		if s.Name == effectiveDef {
			marker = "●"
			defaultIdx = i
		}
		env := s.Env
		if env == "" {
			env = "-"
		}
		rows = append(rows, []string{
			marker, s.Name, env, s.BaseURL(), s.Username, strconv.Itoa(s.CompanyID),
		})
	}

	style := output.BoldHeaderStyle(func(row, col int, padded string) string {
		if row == defaultIdx && col == 0 {
			return output.Green(padded)
		}
		return padded
	})

	p.Table(output.Table{Headers: headers, Rows: rows, Style: style})
	switch {
	case implicit:
		p.Infof("● = servidor padrão (único cadastrado) · fixe outro com: fluigcli server use")
	case effectiveDef != "":
		p.Infof("● = servidor padrão · troque com: fluigcli server use")
	default:
		// 2+ servidores e nenhum padrão definido: sem padrão, todo comando
		// precisa de --server. Orienta a fixar um.
		p.Infof("Nenhum servidor padrão definido — fixe um com: fluigcli server use <nome> (ex.: fluigcli server use %s)", ordered[0].Name)
	}
}

// --- server remove ---

func newServerRemoveCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove um servidor cadastrado (e a senha do keyring)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			name := args[0]
			if err := app.confirm(fmt.Sprintf("Remover o servidor %q?", name)); err != nil {
				return err
			}
			server, err := app.Store().Remove(name)
			if err != nil {
				return err
			}
			if err := app.Keyring.Delete(server.KeyringKey()); err != nil {
				p.Warnf("não foi possível remover a senha do keyring: %v", err)
			}
			p.Successf("Servidor %q removido.", name)
			p.Done(map[string]any{"removed": name})
			return nil
		},
	}
}

// --- server test ---

func newServerTestCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "test [<name>]",
		Short: "Testa o acesso a um servidor (login + ping + dados do usuário)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			nameArg := ""
			if len(args) == 1 {
				nameArg = args[0]
				p.Server = nameArg
			}
			server, err := app.resolveServer(nameArg)
			if err != nil {
				return err
			}
			p.Server = server.Name

			ctx := context.Background()
			client, err := app.authenticate(ctx, server, passwordStdin)
			if err != nil {
				return err
			}
			p.Infof("Login e ping ok em %s.", server.BaseURL())

			if claims, ok := client.SessionClaims(); ok {
				if claims.Tenant != 0 && claims.Tenant != server.CompanyID {
					p.Warnf("companyId configurado (%d) difere do tenant do servidor (%d)", server.CompanyID, claims.Tenant)
				}
			}

			user, err := client.FindUserByLogin(ctx, server.Username)
			if err != nil {
				return mapFluigError(err)
			}
			if user.FullName != "" {
				p.Successf("Usuário: %s <%s>", user.FullName, user.Email)
			} else {
				p.Successf("Usuário %s autenticado.", server.Username)
			}

			// Status da widget auxiliar (necessária para scripts de processo).
			helperInstalled, _ := client.HelperInstalled(ctx)
			if helperInstalled {
				p.Successf("Widget auxiliar (fluiggersWidget): instalada")
			} else {
				p.Successf("Widget auxiliar (fluiggersWidget): ausente (instale com: fluigcli server install-helper %s)", server.Name)
			}

			p.Done(map[string]any{
				"server":          server.Name,
				"url":             server.BaseURL(),
				"user":            user.Raw,
				"helperInstalled": helperInstalled,
			})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
