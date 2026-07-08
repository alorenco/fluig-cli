package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// projWithServers cria um projeto com servidores cadastrados e isola a
// configuração global num diretório temporário.
func projWithServers(t *testing.T, servers ...config.Server) string {
	t.Helper()
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := config.NewStore(proj)
	for _, s := range servers {
		if err := st.Add(s, false); err != nil {
			t.Fatal(err)
		}
	}
	return proj
}

func doisServidores() []config.Server {
	return []config.Server{
		{ID: "1", Name: "hml", Host: "h1", Port: 80, Username: "u", CompanyID: 1, Env: config.EnvHml},
		{ID: "2", Name: "producao", Host: "h2", Port: 80, Username: "u", CompanyID: 1, Env: config.EnvProd},
	}
}

func TestResolveServerUsaPadrao(t *testing.T) {
	proj := projWithServers(t, doisServidores()...)
	if _, err := config.NewStore(proj).SetDefault("producao", false); err != nil {
		t.Fatal(err)
	}

	app := &App{Project: proj}
	srv, err := app.resolveServer("")
	if err != nil {
		t.Fatalf("resolveServer: %v", err)
	}
	if srv.Name != "producao" {
		t.Errorf("resolveu %q, quer o padrão \"producao\"", srv.Name)
	}

	// --server continua vencendo o padrão.
	app = &App{Project: proj, Server: "hml"}
	srv, err = app.resolveServer("")
	if err != nil {
		t.Fatalf("resolveServer com --server: %v", err)
	}
	if srv.Name != "hml" {
		t.Errorf("--server resolveu %q, quer hml", srv.Name)
	}
}

func TestResolveServerSemPadraoNaoInterativo(t *testing.T) {
	proj := projWithServers(t, doisServidores()...)
	app := &App{Project: proj}
	_, err := app.resolveServer("")
	if err == nil {
		t.Fatal("dois servidores sem padrão em modo não-interativo deveria falhar")
	}
	if output.ExitCodeFor(err) != output.ExitUsage {
		t.Errorf("exit = %d, quer %d", output.ExitCodeFor(err), output.ExitUsage)
	}
	if !strings.Contains(err.Error(), "server use") {
		t.Errorf("mensagem deveria sugerir o server use: %q", err.Error())
	}
}

func TestResolveServerPadraoOrfao(t *testing.T) {
	proj := projWithServers(t, doisServidores()...)
	if _, err := config.NewStore(proj).SetDefault("producao", false); err != nil {
		t.Fatal(err)
	}
	// Simula edição manual: o padrão aponta para um nome que não existe mais.
	if _, err := config.NewStore(proj).Remove("producao"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.NewStore(proj).SetDefault("hml", false); err != nil {
		t.Fatal(err)
	}
	// Remove limpou o órfão e SetDefault reapontou — resolve para hml.
	app := &App{Project: proj}
	srv, err := app.resolveServer("")
	if err != nil || srv.Name != "hml" {
		t.Fatalf("resolveServer = (%v, %v), quer hml", srv, err)
	}
}

func TestServerUse(t *testing.T) {
	proj := projWithServers(t, doisServidores()...)

	code, stdout := runMain(t, "server", "use", "hml", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d, stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["default"] != "hml" {
		t.Errorf("data = %#v, quer default=hml", data)
	}
	def, _ := config.NewStore(proj).DefaultName()
	if def != "hml" {
		t.Errorf("padrão persistido = %q, quer hml", def)
	}

	// Servidor desconhecido → NOT_FOUND.
	code, _ = runMain(t, "server", "use", "nao-existe", "--json", "--project", proj)
	if code != output.ExitNotFound {
		t.Errorf("exit para desconhecido = %d, quer %d", code, output.ExitNotFound)
	}
}

func TestServerAddPrimeiroViraPadrao(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	code, stdout := runMain(t, "server", "add", "--name", "hml", "--host", "h", "--username", "u",
		"--env", "homolog", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d, stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["default"] != true {
		t.Errorf("primeiro servidor deveria virar padrão: %#v", data)
	}
	srv, _ := data["server"].(map[string]any)
	if srv["env"] != "hml" {
		t.Errorf("env normalizado = %v, quer hml (apelido homolog)", srv["env"])
	}

	// Segundo servidor não rouba o padrão…
	code, stdout = runMain(t, "server", "add", "--name", "prod", "--host", "h2", "--username", "u",
		"--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d, stdout=%s", code, stdout)
	}
	if def, _ := config.NewStore(proj).DefaultName(); def != "hml" {
		t.Errorf("padrão após segundo add = %q, quer hml", def)
	}

	// …a menos que peça com --default.
	code, _ = runMain(t, "server", "add", "--name", "prod2", "--host", "h3", "--username", "u",
		"--default", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if def, _ := config.NewStore(proj).DefaultName(); def != "prod2" {
		t.Errorf("padrão após --default = %q, quer prod2", def)
	}
}

func TestServerAddEnvInvalido(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _ := runMain(t, "server", "add", "--name", "x", "--host", "h", "--username", "u",
		"--env", "banana", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit = %d, quer %d (uso)", code, output.ExitUsage)
	}
}

func TestGuardProdWrite(t *testing.T) {
	prod := &config.Server{Name: "p", Env: config.EnvProd}

	appYes := &App{Yes: true}
	if err := appYes.guardProdWrite(prod, "publicar"); err != nil {
		t.Errorf("--yes deveria liberar produção: %v", err)
	}

	appNo := &App{} // sem TTY nos testes → modo não-interativo
	err := appNo.guardProdWrite(prod, "publicar")
	if err == nil {
		t.Fatal("produção sem --yes deveria bloquear")
	}
	if output.ExitCodeFor(err) != output.ExitUsage {
		t.Errorf("exit = %d, quer %d", output.ExitCodeFor(err), output.ExitUsage)
	}

	if err := appNo.guardProdWrite(&config.Server{Env: config.EnvHml}, "publicar"); err != nil {
		t.Errorf("hml não deveria pedir confirmação: %v", err)
	}
	if err := appNo.guardProdWrite(&config.Server{}, "publicar"); err != nil {
		t.Errorf("sem env não deveria pedir confirmação: %v", err)
	}
}

func TestInstallHelperBloqueadoEmProd(t *testing.T) {
	proj := projWithServers(t, doisServidores()...)
	code, stdout := runMain(t, "server", "install-helper", "producao", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Fatalf("exit = %d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || !strings.Contains(env.Error.Message, "PRODUÇÃO") {
		t.Errorf("erro deveria citar produção: %+v", env.Error)
	}
}

func TestServerUpdate(t *testing.T) {
	proj := projWithServers(t, config.Server{ID: "1", Name: "hml", Host: "h", Port: 80, Username: "u", CompanyID: 1})

	code, stdout := runMain(t, "server", "update", "hml", "--env", "prod", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d, stdout=%s", code, stdout)
	}
	got, err := config.NewStore(proj).Get("hml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Env != config.EnvProd {
		t.Errorf("env persistido = %q, quer prod", got.Env)
	}

	// Sem nenhuma flag → erro de uso.
	code, _ = runMain(t, "server", "update", "hml", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("update sem flags: exit = %d, quer %d", code, output.ExitUsage)
	}
}

func TestServerListMostraPadrao(t *testing.T) {
	proj := projWithServers(t, doisServidores()...)
	if _, err := config.NewStore(proj).SetDefault("hml", false); err != nil {
		t.Fatal(err)
	}
	code, stdout := runMain(t, "server", "list", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["default"] != "hml" {
		t.Errorf("data.default = %v, quer hml", data["default"])
	}
}
