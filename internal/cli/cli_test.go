package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// runMain executa a CLI com args e captura stdout (o envelope JSON do contrato).
func runMain(t *testing.T, args ...string) (int, string) {
	t.Helper()
	// Não escreve o cache de sessão em disco durante os testes (evita tocar o
	// ~/.cache real); o cache é testado nos pacotes fluig/config.
	t.Setenv(envNoSessionCache, "1")
	oldArgs, oldStdout := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = oldArgs, oldStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	os.Args = append([]string{"fluigcli"}, args...)

	// Drena o pipe concorrentemente: o buffer do pipe é limitado (pequeno no
	// Windows) e ler só depois de Main() retornar trava a escrita em saídas
	// maiores que o buffer (ex.: TestSkillShow, que imprime a skill inteira).
	outCh := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(r)
		outCh <- string(out)
	}()

	code := Main("test", "abc123", "2026-01-01")

	w.Close()
	return code, <-outCh
}

func TestVersionJSONEnvelope(t *testing.T) {
	code, stdout := runMain(t, "version", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit = %d, quer 0; stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é JSON: %v\n%s", err, stdout)
	}
	if !env.OK || env.Command != "version" || env.Error != nil {
		t.Errorf("envelope inesperado: %+v", env)
	}
	data, _ := env.Data.(map[string]any)
	if data["version"] != "test" {
		t.Errorf("data.version = %v", data["version"])
	}
}

// Comando ou flag desconhecidos são erro de uso: exit 2.
func TestUnknownCommandIsUsageError(t *testing.T) {
	code, _ := runMain(t, "comando-inexistente")
	if code != output.ExitUsage {
		t.Errorf("exit = %d, quer %d", code, output.ExitUsage)
	}

	code, stdout := runMain(t, "version", "--flag-inexistente", "--json")
	if code != output.ExitUsage {
		t.Errorf("exit = %d, quer %d", code, output.ExitUsage)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("falha de uso com --json também deve emitir envelope: %v\n%s", err, stdout)
	}
	if env.OK || env.Error == nil || env.Error.Code != output.CodeUsage {
		t.Errorf("envelope de erro inesperado: %+v", env)
	}
	// A mensagem deve estar em pt-BR (traduzida do cobra/pflag).
	if env.Error != nil && !strings.Contains(env.Error.Message, "flag desconhecida") {
		t.Errorf("mensagem de flag desconhecida não traduzida: %q", env.Error.Message)
	}
}

// O texto de ajuda gerado pelo cobra deve estar 100% em pt-BR: nomes de
// comandos/flags em inglês, mas nenhum texto explicativo em inglês.
func TestHelpIsPortuguese(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"server", "--help"},
		{"server", "add", "--help"},
		{"dataset", "--help"},
		{"event", "--help"},
		{"mechanism", "--help"},
		{"form", "--help"},
		{"workflow", "--help"},
		{"widget", "--help"},
		{"completion", "--help"},
	} {
		_, stdout := runMain(t, args...)
		for _, english := range []string{
			"Usage:", "Available Commands:", "Global Flags:",
			"help for", "Additional", "for more information",
			"Generate the autocompletion", "Help about any command",
		} {
			if strings.Contains(stdout, english) {
				t.Errorf("ajuda de %v contém texto em inglês %q:\n%s", args, english, stdout)
			}
		}
	}
}

// memKeyring é um keyring em memória para os testes da CLI.
type memKeyring struct{ m map[string]string }

func (k *memKeyring) Get(id string) (string, error) {
	if v, ok := k.m[id]; ok {
		return v, nil
	}
	return "", config.ErrKeyringNotFound
}
func (k *memKeyring) Set(id, pw string) error { k.m[id] = pw; return nil }
func (k *memKeyring) Delete(id string) error  { delete(k.m, id); return nil }
func (k *memKeyring) Available() bool         { return true }

// fluigLoginStub responde login.do (cookie só com a senha certa) e ping.
func fluigLoginStub(goodPassword string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("j_password") == goodPassword {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
		}
		fmt.Fprint(w, "portal")
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		if ck, err := r.Cookie("JSESSIONIDSSO"); err == nil && ck.Value == "ok" {
			fmt.Fprint(w, `{"message":"pong"}`)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	return mux
}

// Regressão do bug de validação da Fase 0: uma senha ERRADA salva no keyring
// deixava o usuário preso (a resolução de precedência a reusava e nunca perguntava de
// novo). Agora, ao falhar a autenticação, a senha ruim do keyring é removida.
func TestServerTestRemovesBadKeyringPassword(t *testing.T) {
	srv := httptest.NewServer(fluigLoginStub("senha-certa"))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "") // sem env var: força o degrau do keyring

	server := config.Server{
		Name: "homolog", Host: u.Hostname(), Port: port,
		SSL: false, Username: "user-kr", CompanyID: 1,
	}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}

	// Keyring pré-carregado com a senha ERRADA (como no cenário reportado),
	// chaveado por baseURL+usuário (a chave estável de KeyringKey).
	key := server.KeyringKey()
	kr := &memKeyring{m: map[string]string{key: "senha-errada"}}
	old := newKeyring
	newKeyring = func() config.Keyring { return kr }
	defer func() { newKeyring = old }()

	code, _ := runMain(t, "server", "test", "homolog", "--project", proj, "--non-interactive")
	if code != output.ExitAuth {
		t.Errorf("exit = %d, quer %d (AUTH_FAILED)", code, output.ExitAuth)
	}
	if _, ainda := kr.m[key]; ainda {
		t.Errorf("a senha errada deveria ter sido removida do keyring, mas continua lá")
	}
}

// server test sem servidor cadastrado, em modo não-interativo → NOT_FOUND (exit 4).
func TestServerTestWithoutServers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FLUIGCLI_NON_INTERACTIVE", "1")
	code, stdout := runMain(t, "server", "test", "--json", "--project", t.TempDir())
	if code != output.ExitNotFound {
		t.Errorf("exit = %d, quer %d; stdout=%s", code, output.ExitNotFound, stdout)
	}
}

// O help raiz mostra a versão e os grupos de comandos; --version funciona.
func TestHelpMostraVersaoEGrupos(t *testing.T) {
	code, stdout := runMain(t, "--help")
	if code != output.ExitOK {
		t.Fatalf("exit = %d, quer 0", code)
	}
	for _, want := range []string{
		"Versão:", "fluigcli test",
		"Desenvolvimento:", "Configuração:", "Adicionais:",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "--version")
	if code != output.ExitOK || !strings.Contains(stdout, "fluigcli test") {
		t.Errorf("--version: exit=%d stdout=%q", code, stdout)
	}
	if strings.Contains(stdout, "commit") {
		t.Errorf("--version não deveria mostrar commit/build: %q", stdout)
	}
}
