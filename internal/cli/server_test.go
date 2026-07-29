package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	helperwar "github.com/alorenco/fluig-cli/helper"
	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// healthyServerStub simula um servidor saudável; helperInstalled controla o
// ping do fluigcliHelper.
func healthyServerStub(t *testing.T, helperInstalled bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":{"login":"u","fullName":"Fulano de Teste","email":"u@x","userCode":"uc"}}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		if !helperInstalled {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "pong")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func serverTestProject(t *testing.T, stubURL string) string {
	t.Helper()
	u := mustParseHostPort(t, stubURL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	s := config.Server{ID: "st-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(s, false); err != nil {
		t.Fatal(err)
	}
	return proj
}

func TestServerTestReportsHelperStatus(t *testing.T) {
	for _, installed := range []bool{true, false} {
		stub := healthyServerStub(t, installed)
		proj := serverTestProject(t, stub.URL)
		code, stdout := runMain(t, "server", "test", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("installed=%v exit=%d stdout=%s", installed, code, stdout)
		}
		var env output.Envelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("json inválido: %v", err)
		}
		data, _ := env.Data.(map[string]any)
		if got, _ := data["helperInstalled"].(bool); got != installed {
			t.Errorf("helperInstalled=%v, quer %v", got, installed)
		}
	}
}

// install-helper publica o WAR embutido do fluigcliHelper; se ele já responde
// ao ping, não reenvia (action=none).
func TestServerInstallHelperEmbutido(t *testing.T) {
	for _, jaInstalado := range []bool{false, true} {
		var uploadedName string
		var uploadedSize int
		mux := http.NewServeMux()
		mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
		})
		mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"message":"pong"}`)
		})
		mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
			if !jaInstalado {
				http.NotFound(w, r)
				return
			}
			io.WriteString(w, "pong")
		})
		mux.HandleFunc("/portal/api/rest/wcmservice/rest/product/uploadfile", func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseMultipartForm(20 << 20)
			uploadedName = r.FormValue("fileName")
			if f, _, err := r.FormFile("attachment"); err == nil {
				b, _ := io.ReadAll(f)
				uploadedSize = len(b)
			}
			io.WriteString(w, `{}`)
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		proj := serverTestProject(t, srv.URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("jaInstalado=%v exit=%d stdout=%s", jaInstalado, code, stdout)
		}
		var env output.Envelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("json inválido: %v", err)
		}
		data, _ := env.Data.(map[string]any)
		if jaInstalado {
			if data["action"] != "none" || uploadedName != "" {
				t.Errorf("já instalado: action=%v upload=%q (quer none, sem upload)", data["action"], uploadedName)
			}
			continue
		}
		if data["action"] != "uploaded" || data["helper"] != fluig.HelperFluigcli {
			t.Errorf("action=%v helper=%v, quer uploaded/fluigcliHelper", data["action"], data["helper"])
		}
		if uploadedName != helperwar.Name {
			t.Errorf("nome do WAR enviado = %q, quer %q", uploadedName, helperwar.Name)
		}
		if uploadedSize != len(helperwar.WAR) || uploadedSize == 0 {
			t.Errorf("tamanho enviado %d ≠ WAR embutido %d", uploadedSize, len(helperwar.WAR))
		}
	}
}

// server status: resumo + tabela de monitores (fixtures reais da homolog).
func TestServerStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux.HandleFunc("/environment/api/v2/monitors", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("rest_monitors.json"))
	})
	mux.HandleFunc("/environment/api/v2/statistics", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("rest_statistics.json"))
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.2.0"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u := mustParseHostPort(t, srv.URL)
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(config.EnvPassword, "p")
	server := config.Server{ID: "st-srv", Name: "homolog", Host: u.host, Port: u.port, SSL: false, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(server, false); err != nil {
		t.Fatal(err)
	}

	code, stdout := runMain(t, "server", "status", "homolog", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Uptime:", "Usuários conectados: 35", "Threads: 385 (pico 454)",
		"Microsoft SQL Server", "Monitor", "LICENSE_SERVER_AVAILABILITY", "OK", "FAILURE", "100%",
		"Helper (fluigcliHelper): instalado · v0.2.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "server", "status", "homolog", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	stats, _ := data["stats"].(map[string]any)
	monitors, _ := data["monitors"].([]any)
	if stats["connectedUsers"].(float64) != 35 || len(monitors) != 8 {
		t.Errorf("envelope inesperado: stats=%v monitors=%d", stats["connectedUsers"], len(monitors))
	}
}

// helperVersionStub simula um servidor com o fluigcliHelper numa versão dada e
// captura o que for publicado no uploadfile.
type helperVersionStub struct {
	version      string // versão que o helper anuncia ("" = rota ausente)
	uploadedName string
	uploadedSize int
}

func (s *helperVersionStub) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/user/findUserByLogin", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":{"login":"u","fullName":"Fulano de Teste","email":"u@x","userCode":"uc"}}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		if s.version == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"name":"fluigcliHelper","version":"`+s.version+`"}`)
	})
	mux.HandleFunc("/portal/api/rest/wcmservice/rest/product/uploadfile", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(20 << 20)
		s.uploadedName = r.FormValue("fileName")
		if f, _, err := r.FormFile("attachment"); err == nil {
			b, _ := io.ReadAll(f)
			s.uploadedSize = len(b)
		}
		io.WriteString(w, `{}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// install-helper compara a versão do servidor com a do WAR embutido antes de
// publicar (ROADMAP §2.10-J): sem isso, um binário antigo rebaixaria o servidor
// em silêncio.
func TestServerInstallHelperComparaVersao(t *testing.T) {
	embutida := helperwar.Version()
	if embutida == "" {
		t.Fatal("o WAR embutido precisa anunciar a versão para este teste valer")
	}

	t.Run("servidor mais antigo é atualizado", func(t *testing.T) {
		stub := &helperVersionStub{version: "0.3.0"}
		proj := serverTestProject(t, stub.server(t).URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("exit=%d stdout=%s", code, stdout)
		}
		var env output.Envelope
		json.Unmarshal([]byte(stdout), &env)
		data, _ := env.Data.(map[string]any)
		if data["action"] != "uploaded" || stub.uploadedName != helperwar.Name {
			t.Errorf("deveria publicar: action=%v upload=%q", data["action"], stub.uploadedName)
		}
		if data["embeddedVersion"] != embutida || data["version"] != "0.3.0" {
			t.Errorf("o envelope deveria trazer as duas versões: %+v", data)
		}
	})

	t.Run("mesma versão não reenvia sem --force", func(t *testing.T) {
		stub := &helperVersionStub{version: embutida}
		proj := serverTestProject(t, stub.server(t).URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("exit=%d stdout=%s", code, stdout)
		}
		var env output.Envelope
		json.Unmarshal([]byte(stdout), &env)
		data, _ := env.Data.(map[string]any)
		if data["action"] != "none" || stub.uploadedName != "" {
			t.Errorf("mesma versão: action=%v upload=%q (quer none, sem upload)", data["action"], stub.uploadedName)
		}
	})

	t.Run("mesma versão com --force reenvia (reparo)", func(t *testing.T) {
		stub := &helperVersionStub{version: embutida}
		proj := serverTestProject(t, stub.server(t).URL)
		code, _ := runMain(t, "server", "install-helper", "homolog", "--force", "--json", "--project", proj)
		if code != output.ExitOK || stub.uploadedName != helperwar.Name {
			t.Errorf("--force deveria reenviar: exit=%d upload=%q", code, stub.uploadedName)
		}
	})

	// O caso que motivou o item: binário velho contra servidor novo.
	t.Run("servidor mais novo é recusado, mesmo com --force", func(t *testing.T) {
		stub := &helperVersionStub{version: "99.0.0"}
		proj := serverTestProject(t, stub.server(t).URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--force", "--json", "--project", proj)
		if code != output.ExitUsage {
			t.Fatalf("exit=%d, quer %d (rebaixamento recusado)\n%s", code, output.ExitUsage, stdout)
		}
		if stub.uploadedName != "" {
			t.Errorf("nada deveria ter sido publicado: %q", stub.uploadedName)
		}
		var env output.Envelope
		json.Unmarshal([]byte(stdout), &env)
		if env.Error == nil || !strings.Contains(env.Error.Message, "99.0.0") ||
			!strings.Contains(env.Error.Message, "--allow-downgrade") {
			t.Errorf("a mensagem deveria citar a versão do servidor e a saída: %+v", env.Error)
		}
	})

	t.Run("--allow-downgrade permite rebaixar", func(t *testing.T) {
		stub := &helperVersionStub{version: "99.0.0"}
		proj := serverTestProject(t, stub.server(t).URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--allow-downgrade", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("exit=%d stdout=%s", code, stdout)
		}
		if stub.uploadedName != helperwar.Name {
			t.Errorf("com --allow-downgrade deveria publicar: %q", stub.uploadedName)
		}
	})

	// Helper antigo sem a rota /version: sem versão para comparar, vale a regra
	// de antes (não reenvia sem --force).
	t.Run("sem rota de versão mantém o comportamento antigo", func(t *testing.T) {
		stub := &helperVersionStub{version: ""}
		proj := serverTestProject(t, stub.server(t).URL)
		code, stdout := runMain(t, "server", "install-helper", "homolog", "--json", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("exit=%d stdout=%s", code, stdout)
		}
		var env output.Envelope
		json.Unmarshal([]byte(stdout), &env)
		data, _ := env.Data.(map[string]any)
		if data["action"] != "none" {
			t.Errorf("action=%v, quer none", data["action"])
		}
	})
}

// server test passa a reportar a versão do helper e a do WAR do binário.
func TestServerTestReportaVersoesDoHelper(t *testing.T) {
	stub := &helperVersionStub{version: "0.3.0"}
	proj := serverTestProject(t, stub.server(t).URL)
	code, stdout := runMain(t, "server", "test", "homolog", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if data["helperVersion"] != "0.3.0" || data["cliHelperWAR"] != helperwar.Version() {
		t.Errorf("versões no envelope: %+v", data)
	}
}

func TestCompareHelperVersions(t *testing.T) {
	casos := []struct {
		a, b string
		quer int
	}{
		{"0.7.0", "0.8.0", -1},
		{"0.8.0", "0.8.0", 0},
		{"0.8.1", "0.8.0", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.10.0", "0.9.0", 1}, // comparação numérica, não alfabética
		{"0.8", "0.8.0", 0},    // sem patch = patch 0
		{"0.8.0-SNAPSHOT", "0.8.0", 0},
		{"", "0.8.0", -1},
		{"lixo", "0.0.0", 0}, // formato irreconhecível não trava o install
	}
	for _, tc := range casos {
		if got := compareHelperVersions(tc.a, tc.b); got != tc.quer {
			t.Errorf("compareHelperVersions(%q,%q)=%d, quer %d", tc.a, tc.b, got, tc.quer)
		}
	}
}

// Rodar de fora do projeto: a CLI tem de dizer que não achou PROJETO, não que o
// servidor precisa ser cadastrado (ROADMAP2 §3.4). O servidor existe — está no
// .fluigcli/servers.json do projeto — e "cadastre de novo" levaria o usuário a
// criar um segundo .fluigcli/ no diretório em que ele estiver.
func TestServerNaoEncontradoForaDoProjetoCitaProjeto(t *testing.T) {
	// Projeto com um servidor, mas SEM passar --project: o cwd do teste não é a
	// raiz dele, então a descoberta não acha projeto nenhum.
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := config.Server{Name: "homologacao", Host: "h.test", Port: 8080, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(s, false); err != nil {
		t.Fatal(err)
	}

	code, stdout := runMain(t, "dataset", "list", "--server", "homologacao", "--json")
	if code != output.ExitNotFound {
		t.Fatalf("exit = %d, quer %d; stdout=%s", code, output.ExitNotFound, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope inválido: %v\n%s", err, stdout)
	}
	if env.Error == nil {
		t.Fatal("envelope sem erro")
	}
	for _, want := range []string{"Nenhum projeto Fluig foi descoberto", "--project"} {
		if !strings.Contains(env.Error.Message, want) {
			t.Errorf("mensagem sem %q: %s", want, env.Error.Message)
		}
	}

	// Com --project apontando a raiz, o mesmo comando resolve o servidor (o erro
	// deixa de ser NOT_FOUND de cadastro e passa a ser de rede/servidor).
	code, stdout = runMain(t, "dataset", "list", "--server", "homologacao", "--json", "--project", proj)
	json.Unmarshal([]byte(stdout), &env)
	if env.Error != nil && strings.Contains(env.Error.Message, "não encontrado; cadastre com") {
		t.Errorf("com --project o servidor devia ter sido encontrado: %s", env.Error.Message)
	}
	_ = code
}

// Sem --server e sem projeto descoberto: a lista vazia também não pode mandar
// cadastrar como se nada existisse.
func TestSemServidorEForaDoProjetoCitaProjeto(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := config.Server{Name: "homologacao", Host: "h.test", Port: 8080, Username: "u", CompanyID: 1}
	if err := config.NewStore(proj).Add(s, false); err != nil {
		t.Fatal(err)
	}

	code, stdout := runMain(t, "dataset", "list", "--json")
	if code != output.ExitNotFound {
		t.Fatalf("exit = %d, quer %d; stdout=%s", code, output.ExitNotFound, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	if env.Error == nil || !strings.Contains(env.Error.Message, "Nenhum projeto Fluig foi descoberto") {
		t.Errorf("mensagem não cita a ausência de projeto: %+v", env.Error)
	}
}

// --- server logout: aviso quando não sobra credencial (ROADMAP2 §3.10) ---

// logoutApp monta um App de teste com keyring em memória e um Printer com
// buffers, para inspecionar o aviso.
func logoutApp(t *testing.T, kr config.Keyring, servers ...config.Server) (*App, *strings.Builder) {
	t.Helper()
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store := config.NewStore(proj)
	for _, s := range servers {
		if err := store.Add(s, false); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{Project: proj, Keyring: kr, NonInteractive: true}
	var stderr strings.Builder
	p := output.NewPrinter(false, "server logout")
	p.Stdout = &strings.Builder{}
	p.Stderr = &stderr
	app.printer = p
	return app, &stderr
}

// srvLogout monta um servidor de teste. O host acompanha o nome porque a chave
// do keyring é baseURL|usuário: dois cadastros com a mesma URL e o mesmo usuário
// compartilham a MESMA credencial.
func srvLogout(name string) config.Server {
	return config.Server{Name: name, Host: name + ".test", Port: 8080, Username: "u-" + name, CompanyID: 1}
}

// Sem senha em nenhuma fonte: avisa (e não bloqueia — automação existente não
// pode quebrar por causa disto).
func TestLogoutAvisaSemSenha(t *testing.T) {
	t.Setenv(config.EnvPassword, "")
	s := srvLogout("homolog")
	app, stderr := logoutApp(t, &memKeyring{m: map[string]string{}}, s)

	if err := app.guardLogoutSemSenha(app.printer, &s); err != nil {
		t.Fatalf("não pode bloquear em modo não-interativo: %v", err)
	}
	msg := stderr.String()
	for _, want := range []string{"não há senha disponível", "homolog", config.EnvPassword} {
		if !strings.Contains(msg, want) {
			t.Errorf("aviso sem %q: %q", want, msg)
		}
	}
}

// Senha no keyring: nada a avisar — a próxima execução se autentica sozinha.
func TestLogoutSilenciosoComSenhaNoKeyring(t *testing.T) {
	t.Setenv(config.EnvPassword, "")
	s := srvLogout("homolog")
	kr := &memKeyring{m: map[string]string{s.KeyringKey(): "segredo"}}
	app, stderr := logoutApp(t, kr, s)

	if err := app.guardLogoutSemSenha(app.printer, &s); err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "" {
		t.Errorf("não devia avisar com senha no keyring: %q", stderr.String())
	}
}

// Senha na env var: também dispensa o aviso (é o caso de CI).
func TestLogoutSilenciosoComEnvVar(t *testing.T) {
	s := srvLogout("homolog")
	app, stderr := logoutApp(t, &memKeyring{m: map[string]string{}}, s)
	t.Setenv(config.EnvPassword, "segredo")

	if err := app.guardLogoutSemSenha(app.printer, &s); err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "" {
		t.Errorf("não devia avisar com %s definida: %q", config.EnvPassword, stderr.String())
	}
}

// Keyring indisponível (Linux headless, o caso deste servidor de dev): conta
// como "sem senha".
func TestLogoutAvisaComKeyringIndisponivel(t *testing.T) {
	t.Setenv(config.EnvPassword, "")
	s := srvLogout("homolog")
	app, stderr := logoutApp(t, unavailableKeyring{}, s)

	if err := app.guardLogoutSemSenha(app.printer, &s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "não há senha disponível") {
		t.Errorf("keyring indisponível devia gerar aviso: %q", stderr.String())
	}
}

// --all avisa uma vez, no plural, com a contagem.
func TestLogoutAllAvisaNoPlural(t *testing.T) {
	t.Setenv(config.EnvPassword, "")
	a, b := srvLogout("homolog"), srvLogout("producao")
	kr := &memKeyring{m: map[string]string{a.KeyringKey(): "segredo"}}
	app, stderr := logoutApp(t, kr, a, b)

	if err := app.guardLogoutSemSenha(app.printer, nil); err != nil {
		t.Fatal(err)
	}
	msg := stderr.String()
	// homolog tem senha no keyring; producao não. O aviso sai uma vez, no plural,
	// sem varrer servidor por servidor.
	if !strings.Contains(msg, "1 servidor(es) ficam sem senha") {
		t.Errorf("aviso plural ausente: %q", msg)
	}
	if strings.Count(msg, "aviso:") != 1 {
		t.Errorf("o --all devia avisar UMA vez, veio: %q", msg)
	}
}

// O comando segue funcionando: a sessão é descartada e o exit é 0.
func TestLogoutLimpaSessaoMesmoSemSenha(t *testing.T) {
	t.Setenv(config.EnvPassword, "")
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.NewStore(proj).Add(srvLogout("homolog"), false); err != nil {
		t.Fatal(err)
	}
	code, stdout := runMain(t, "server", "logout", "homolog", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d, quer 0; stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, `"cleared":"homolog"`) {
		t.Errorf("envelope inesperado: %s", stdout)
	}
}

// unavailableKeyring simula o keyring ausente (servidor headless sem D-Bus).
type unavailableKeyring struct{}

func (unavailableKeyring) Get(string) (string, error) { return "", config.ErrKeyringNotFound }
func (unavailableKeyring) Set(string, string) error   { return config.ErrKeyringNotFound }
func (unavailableKeyring) Delete(string) error        { return config.ErrKeyringNotFound }
func (unavailableKeyring) Available() bool            { return false }
