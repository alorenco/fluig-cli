package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// logServerStub simula o Fluig com o fluigcliHelper 0.3.0 (rotas de log).
func logServerStub(t *testing.T) *httptest.Server {
	readTD := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.3.0"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Write(readTD("helper_logs.json"))
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/tail", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("grep") {
		case "Dataset query:":
			w.Write(readTD("helper_log_tail_multiline.json"))
		case "formatos":
			// Cabeçalhos variados + uma entrada sem cabeçalho (ROADMAP §2.10-D).
			w.Write(readTD("helper_log_tail_formatos.json"))
		default:
			w.Write(readTD("helper_log_tail.json"))
		}
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/download", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "linha 1\nlinha 2\n")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLogFilesTabela(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "log", "files", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Arquivo", "Tamanho", "Modificado", "server.log", "server.log.2026-07-17", "2.0 MB", "7.0 MB"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	// O CSV de monitoramento da fixture NÃO entra por padrão (ROADMAP §2.10-G).
	if strings.Contains(stdout, ".csv") {
		t.Errorf("CSV de monitoramento não deveria aparecer por padrão:\n%s", stdout)
	}
	if !strings.Contains(stdout, "1 fora da listagem") {
		t.Errorf("a saída precisa dizer quantos arquivos ficaram de fora:\n%s", stdout)
	}

	code, stdout = runMain(t, "log", "files", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	files, _ := data["files"].([]any)
	if len(files) != 2 {
		t.Errorf("files=%d, quer 2 (o CSV fica fora)", len(files))
	}
	// O contrato ganhou a contagem do que ficou de fora — nada é omitido em silêncio.
	if data["total"].(float64) != 3 || data["omitted"].(float64) != 1 {
		t.Errorf("total/omitted inesperados: %+v", data)
	}
	// Ordem por recência: o log corrente primeiro (é o que se quer no tail).
	first, _ := files[0].(map[string]any)
	if first["name"] != "server.log" || first["lastModified"] == nil {
		t.Errorf("primeiro arquivo inesperado: %v", first)
	}
}

// --all, --pattern e --limit: os escapes da listagem enxuta.
func TestLogFilesFiltros(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)

	// --all inclui os CSVs de monitoramento.
	code, stdout := runMain(t, "log", "files", "--all", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	json.Unmarshal([]byte(stdout), &env)
	data, _ := env.Data.(map[string]any)
	if files, _ := data["files"].([]any); len(files) != 3 || data["omitted"].(float64) != 0 {
		t.Errorf("--all deveria trazer tudo: %+v", data)
	}

	// --pattern manda no filtro, inclusive para pegar um CSV de propósito.
	code, stdout = runMain(t, "log", "files", "--pattern", "*.csv", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	json.Unmarshal([]byte(stdout), &env)
	data, _ = env.Data.(map[string]any)
	files, _ := data["files"].([]any)
	primeiro, _ := files[0].(map[string]any)
	if len(files) != 1 || !strings.HasSuffix(primeiro["name"].(string), ".csv") {
		t.Errorf("--pattern '*.csv' deveria achar só o CSV: %+v", data)
	}

	// --limit corta e a saída humana diz o que ficou de fora.
	code, stdout = runMain(t, "log", "files", "--limit", "1", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "server.log ") || strings.Contains(stdout, "server.log.2026-07-17") {
		t.Errorf("--limit 1 deveria mostrar só o mais recente:\n%s", stdout)
	}
	if !strings.Contains(stdout, "2 fora da listagem") {
		t.Errorf("--limit precisa reportar os omitidos:\n%s", stdout)
	}

	// Filtro que não casa com nada: mensagem clara, não tabela vazia.
	code, stdout = runMain(t, "log", "files", "--pattern", "nada-*", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "Nenhum arquivo de log casa com o filtro") {
		t.Errorf("saída inesperada:\n%s", stdout)
	}
}

func TestSortLogFilesByRecency(t *testing.T) {
	t1 := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	files := []fluig.ServerLogFile{
		{Name: "sem-data.log"},
		{Name: "antigo.log", LastModified: &t1},
		{Name: "novo.log", LastModified: &t2},
	}
	sortLogFilesByRecency(files)
	if files[0].Name != "novo.log" || files[1].Name != "antigo.log" || files[2].Name != "sem-data.log" {
		t.Errorf("ordem inesperada: %+v", files)
	}
}

func TestIsLogFileName(t *testing.T) {
	casos := map[string]bool{
		"server.log":            true,
		"server.log.2026-07-17": true,
		"chrono.log":            true,
		"audit.log":             true,
		"CustomizationManagerImpl.invokeFunction.batch.csv": false,
		// CSV com "log" no MEIO do nome não é log (caso real da homologação).
		"FreemarkerPageRenderer.renderWidget.logcontrol.csv": false,
		"logurl.log": true,
	}
	for nome, quer := range casos {
		if got := isLogFileName(nome); got != quer {
			t.Errorf("isLogFileName(%q)=%v, quer %v", nome, got, quer)
		}
	}
}

func TestLogTail(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)

	// Modo humano: as entradas saem na íntegra (fixture real da homolog).
	code, stdout := runMain(t, "log", "tail", "-n", "3", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Registered web context", "Redeployed", "Replaced deployment"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}

	code, stdout = runMain(t, "log", "tail", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d", code)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	entries, _ := data["entries"].([]any)
	if data["file"] != "server.log" || len(entries) != 3 || data["truncated"] != false {
		t.Errorf("envelope inesperado: file=%v entries=%d truncated=%v", data["file"], len(entries), data["truncated"])
	}
	// `entries` (texto cru) continua no contrato; `records` é o campo novo,
	// com a entrada decomposta e o MESMO índice (ROADMAP §2.10-D).
	records, _ := data["records"].([]any)
	if len(records) != len(entries) {
		t.Fatalf("records=%d, entries=%d — os índices têm de casar", len(records), len(entries))
	}
	first, _ := records[0].(map[string]any)
	if first["timestamp"] != "2026-07-18T15:39:31.759" || first["level"] != "INFO" ||
		first["logger"] != "org.wildfly.extension.undertow" ||
		!strings.Contains(first["message"].(string), "Registered web context") {
		t.Errorf("record[0] inesperado: %+v", first)
	}
	if _, temRaw := first["raw"]; temRaw {
		t.Errorf("cabeçalho reconhecido não deveria trazer raw: %+v", first)
	}
}

// Entradas estruturadas: cabeçalhos variados viram campos e a entrada sem
// cabeçalho vem em `raw`, sem inventar campo nenhum.
func TestLogTailRecords(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "tail", "--grep", "formatos", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	records, _ := data["records"].([]any)
	if len(records) != 5 {
		t.Fatalf("records=%d, quer 5", len(records))
	}
	comStack, _ := records[1].(map[string]any)
	if comStack["level"] != "ERROR" || comStack["thread"] != "default task-22" {
		t.Errorf("record com stack: %+v", comStack)
	}
	if stack, _ := comStack["stack"].(string); !strings.Contains(stack, "at com.fluig.auth//") {
		t.Errorf("stack não separado do cabeçalho: %+v", comStack)
	}
	// Thread com parênteses dentro do nome (ActiveMQ).
	activeMQ, _ := records[3].(map[string]any)
	if activeMQ["thread"] != "Thread-2039 (ActiveMQ-client-global-threads)" {
		t.Errorf("thread com parênteses: %+v", activeMQ)
	}
	// Entrada sem cabeçalho: só `raw`.
	semCabecalho, _ := records[4].(map[string]any)
	if semCabecalho["raw"] == nil || semCabecalho["level"] != nil || semCabecalho["timestamp"] != nil {
		t.Errorf("entrada sem cabeçalho deveria vir só em raw: %+v", semCabecalho)
	}
}

// Entrada multi-linha (fixture real) sai com as continuações em linhas
// próprias no modo humano.
func TestLogTailMultilinha(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "tail", "--grep", "Dataset query:", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "Dataset query:\n((UT.USER_CODE") {
		t.Errorf("continuação não saiu em linha própria:\n%s", stdout)
	}
}

// --follow é contínuo: com --json é recusado (contrato do envelope único).
func TestLogTailFollowRecusaJSON(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _ := runMain(t, "log", "tail", "--follow", "--json", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d (usage)", code, output.ExitUsage)
	}
}

func TestLogTailLevelInvalido(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _ := runMain(t, "log", "tail", "--level", "gigante", "--project", proj)
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d (usage)", code, output.ExitUsage)
	}
}

func TestLogDownload(t *testing.T) {
	stub := logServerStub(t)
	proj := serverTestProject(t, stub.URL)
	dest := filepath.Join(t.TempDir(), "logs", "server.log")

	code, stdout := runMain(t, "log", "download", "-o", dest, "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	b, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "linha 1\nlinha 2\n" {
		t.Errorf("conteúdo baixado: %q", b)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["file"] != "server.log" || data["path"] != dest || data["size"].(float64) != 16 {
		t.Errorf("envelope inesperado: %v", data)
	}
}

// Sem o helper (ping 404), os comandos de log orientam a instalação (exit 7).
func TestLogFilesSemHelper(t *testing.T) {
	stub := healthyServerStub(t, false)
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "files", "--json", "--project", proj)
	if code != output.ExitMissingHelper {
		t.Fatalf("exit=%d stdout=%s, quer %d", code, stdout, output.ExitMissingHelper)
	}
}

// --- log tail --since/--until (ROADMAP §2.10-E-1) ---

// logRangeStub simula o helper 0.5.0 (rota /range) informando o fuso do
// servidor (0.4.0+), como o painel do dev já consome.
func logRangeStub(t *testing.T, gotQuery *url.Values) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.5.0","zoneId":"America/Sao_Paulo","zoneOffsetMinutes":-180}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/range", func(w http.ResponseWriter, r *http.Request) {
		*gotQuery = r.URL.Query()
		io.WriteString(w, `{"file":"server.log","from":"`+r.URL.Query().Get("from")+`","to":"`+r.URL.Query().Get("to")+`",`+
			`"entries":["2026-07-24 18:20:11,004 ERROR [stderr] (default task-1) falha na integração"],"truncated":false}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A tradução dos limites é lógica pura: hora do SERVIDOR, formato da API.
func TestResolveLogBound(t *testing.T) {
	// "Agora" no fuso do servidor (América/São Paulo), não no desta máquina.
	zone := time.FixedZone("America/Sao_Paulo", -3*3600)
	now := time.Date(2026, 7, 25, 9, 30, 0, 0, zone)

	cases := []struct {
		nome        string
		valor       string
		end         bool
		quer        string
		querUsouNow bool
	}{
		{"vazio não limita", "", false, "", false},
		{"duração volta no tempo", "30m", false, "2026-07-25T09:00:00", true},
		{"duração composta", "1h30m", false, "2026-07-25T08:00:00", true},
		{"hora de hoje usa o dia do servidor", "18:19", false, "2026-07-25T18:19", true},
		{"hora com segundos", "18:19:05", false, "2026-07-25T18:19:05", true},
		{"data sem hora começa 00:00", "2026-07-24", false, "2026-07-24T00:00", false},
		{"data sem hora termina no fim do dia", "2026-07-24", true, "2026-07-24T23:59", false},
		{"data e hora com T", "2026-07-24T18:19", false, "2026-07-24T18:19", false},
		{"data e hora com espaço", "2026-07-24 18:19:30", false, "2026-07-24T18:19:30", false},
		// Ambiguidade resolvida a favor da duração: "18h" são 18 HORAS ATRÁS,
		// não 18:00. Quem quer a hora do dia escreve "18:00".
		{"18h é duração, não hora do dia", "18h", false, "2026-07-24T15:30:00", true},
		{"18:00 é hora do dia", "18:00", false, "2026-07-25T18:00", true},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			got, usouNow, err := resolveLogBound("--since", tc.valor, tc.end, now)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != tc.quer {
				t.Errorf("resolveu %q, quer %q", got, tc.quer)
			}
			if usouNow != tc.querUsouNow {
				t.Errorf("usouNow=%v, quer %v (decide o aviso de fuso)", usouNow, tc.querUsouNow)
			}
		})
	}

	for _, ruim := range []string{"ontem", "meio-dia", "2026-13-45", "-30m", "0s", "18:99"} {
		if _, _, err := resolveLogBound("--since", ruim, false, now); err == nil {
			t.Errorf("valor %q deveria ser recusado", ruim)
		} else if output.AsError(err).Exit != output.ExitUsage {
			t.Errorf("valor %q: exit=%d, quer %d", ruim, output.AsError(err).Exit, output.ExitUsage)
		}
	}
}

// Janela de tempo ponta a ponta: os limites vão na query da rota /range e o
// envelope troca `size` por `from`/`to`.
func TestLogTailJanela(t *testing.T) {
	var q url.Values
	stub := logRangeStub(t, &q)
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "log", "tail", "--since", "2026-07-24T18:19", "--until", "2026-07-24T18:30",
		"--level", "error", "--grep", "integração", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if q.Get("from") != "2026-07-24T18:19" || q.Get("to") != "2026-07-24T18:30" {
		t.Errorf("limites na query: from=%q to=%q", q.Get("from"), q.Get("to"))
	}
	if q.Get("level") != "ERROR" || q.Get("grep") != "integração" {
		t.Errorf("level/grep não repassados: %v", q)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	entries, _ := data["entries"].([]any)
	if data["from"] != "2026-07-24T18:19" || data["to"] != "2026-07-24T18:30" || len(entries) != 1 {
		t.Errorf("envelope inesperado: %+v", data)
	}
	records, _ := data["records"].([]any)
	rec, _ := records[0].(map[string]any)
	if len(records) != 1 || rec["level"] != "ERROR" || rec["logger"] != "stderr" {
		t.Errorf("a janela também traz records decompostos: %+v", records)
	}
	if _, temSize := data["size"]; temSize {
		t.Errorf("a janela não tem offset de arquivo — `size` não deveria aparecer: %+v", data)
	}

	// Modo humano: a entrada sai na íntegra.
	code, stdout = runMain(t, "log", "tail", "--since", "30m", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("modo humano exit=%d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, "falha na integração") {
		t.Errorf("entrada não saiu:\n%s", stdout)
	}
	// A duração é resolvida no fuso do SERVIDOR (-03:00), não no desta máquina.
	querDia := time.Now().In(time.FixedZone("srv", -3*3600)).Add(-30 * time.Minute).Format("2006-01-02")
	if !strings.HasPrefix(q.Get("from"), querDia) {
		t.Errorf("from=%q não começa com o dia do servidor (%s)", q.Get("from"), querDia)
	}
}

// A janela é um recorte fechado: não combina com --follow nem com as flags de
// contagem das últimas entradas.
func TestLogTailJanelaRecusas(t *testing.T) {
	var q url.Values
	stub := logRangeStub(t, &q)
	proj := serverTestProject(t, stub.URL)
	for _, args := range [][]string{
		{"log", "tail", "--since", "30m", "--follow"},
		{"log", "tail", "--since", "30m", "-n", "10"},
		{"log", "tail", "--until", "18:19", "--skip", "5"},
		{"log", "tail", "--since", "ontem"},
	} {
		code, stdout := runMain(t, append(args, "--project", proj)...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d\n%s", args, code, output.ExitUsage, stdout)
		}
	}
}

// Helper antigo (0.3.0, sem a rota /range): erro de dependência, não 404 solto.
func TestLogTailJanelaHelperAntigo(t *testing.T) {
	stub := logServerStub(t) // versão 0.3.0
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "tail", "--since", "30m", "--json", "--project", proj)
	if code != output.ExitMissingHelper {
		t.Fatalf("exit=%d, quer %d (helper sem /range)\n%s", code, output.ExitMissingHelper, stdout)
	}
}

// --- log tail --follow --ndjson + condições de parada (ROADMAP §2.10-F) ---

// logFollowStub serve /read?from= com uma sequência roteirizada de trechos: é
// assim que o polling do --follow vê o log crescer. Trecho vazio = log em
// silêncio (o que fecha a entrada pendente).
func logFollowStub(t *testing.T, chunks []string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/portal/api/servlet/login.do", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONIDSSO", Value: "ok", Path: "/"})
	})
	mux.HandleFunc("/portal/p/api/servlet/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"message":"pong"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/ping", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "pong")
	})
	mux.HandleFunc("/fluigcliHelper/api/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"name":"fluigcliHelper","version":"0.7.0"}`)
	})
	// O tail inicial só posiciona o offset: o --follow começa do fim do arquivo.
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/tail", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"file":"server.log","size":100,"entries":[],"truncated":false}`)
	})
	var mu sync.Mutex
	call, size := 0, 100 // o arquivo só CRESCE (encolher significaria rotação)
	mux.HandleFunc("/fluigcliHelper/api/logs/server.log/read", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		content := ""
		if call < len(chunks) {
			content = chunks[call]
		}
		call++
		from := size
		size += len(content)
		to := size
		mu.Unlock()
		body, _ := json.Marshal(map[string]any{
			"file": "server.log", "from": from, "to": to, "size": to, "content": content,
		})
		w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// O polling entrega LINHAS; o acompanhamento emite ENTRADAS. Uma entrada só
// fecha quando a próxima começa (ou quando o log silencia).
func TestLogEntryAccumulator(t *testing.T) {
	acc := &logEntryAccumulator{}

	// Primeiro trecho: uma entrada completa + o começo da segunda.
	done := acc.add([]string{
		"2026-07-25 10:00:00,000 INFO  [a] (t) primeira",
		"2026-07-25 10:00:01,000 ERROR [b] (t) segunda",
		"\tat foo.Bar(Bar.java:1)",
	})
	if len(done) != 1 || done[0] != "2026-07-25 10:00:00,000 INFO  [a] (t) primeira" {
		t.Fatalf("a primeira entrada deveria fechar ao começar a segunda: %q", done)
	}

	// A segunda continua pendente: o stack pode ter mais linhas.
	if done := acc.add([]string{"\tat foo.Baz(Baz.java:2)"}); len(done) != 0 {
		t.Errorf("entrada com stack não deveria fechar sem cabeçalho novo: %q", done)
	}

	// Silêncio do log fecha a pendente, com o stack inteiro junto.
	entry := acc.flush()
	if !strings.HasPrefix(entry, "2026-07-25 10:00:01,000 ERROR") ||
		!strings.Contains(entry, "Bar.java:1") || !strings.Contains(entry, "Baz.java:2") {
		t.Errorf("flush deveria devolver a entrada inteira: %q", entry)
	}
	if acc.flush() != "" {
		t.Error("flush duas vezes não deveria repetir a entrada")
	}
}

// Filtro do --follow com vários padrões: OU, igual ao do servidor.
func TestLogEntryFilterVariosGreps(t *testing.T) {
	f := newLogEntryFilter("", []string{"widget", "TIMEOUT"})
	if !f.match("2026-07-25 10:00:00,000 INFO  [a] (t) deploy da widget") {
		t.Error("deveria casar com o primeiro padrão")
	}
	if !f.match("2026-07-25 10:00:00,000 ERROR [a] (t) timeout no dataset") {
		t.Error("deveria casar com o segundo padrão (case-insensitive)")
	}
	if f.match("2026-07-25 10:00:00,000 INFO  [a] (t) nada a ver") {
		t.Error("não deveria casar sem nenhum dos padrões")
	}
	// Nível + OU de texto continuam somando (E entre os critérios).
	f = newLogEntryFilter("ERROR", []string{"widget", "timeout"})
	if f.match("2026-07-25 10:00:00,000 INFO  [a] (t) deploy da widget") {
		t.Error("o nível mínimo tem de valer junto com o OU do texto")
	}
}

// Vários --grep exigem helper >= 0.8.0: a CLI recusa em vez de filtrar no
// cliente (filtrar depois do teto de 2 MB perderia entradas em silêncio).
func TestLogTailVariosGrepsHelperAntigo(t *testing.T) {
	stub := logServerStub(t) // versão 0.3.0
	proj := serverTestProject(t, stub.URL)
	code, stdout := runMain(t, "log", "tail", "--grep", "a", "--grep", "b", "--json", "--project", proj)
	if code != output.ExitMissingHelper {
		t.Fatalf("exit=%d, quer %d (helper sem OU)\n%s", code, output.ExitMissingHelper, stdout)
	}
	// Um padrão só continua funcionando no helper antigo.
	code, _ = runMain(t, "log", "tail", "--grep", "a", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Errorf("um grep só não deveria exigir helper novo: exit=%d", code)
	}
}

func TestLogEntryFilter(t *testing.T) {
	entry := "2026-07-25 10:00:00,000 ERROR [a.B] (t) falhou\n\tat foo.Bar(Bar.java:1)"
	info := "2026-07-25 10:00:00,000 INFO  [a.B] (t) subiu"

	if f := newLogEntryFilter("WARN", nil); !f.match(entry) || f.match(info) {
		t.Error("--level filtra pela severidade da linha de cabeçalho")
	}
	// O texto é procurado na entrada INTEIRA: acha dentro do stack trace.
	if f := newLogEntryFilter("", []string{"bar.java"}); !f.match(entry) {
		t.Error("--grep deveria alcançar o stack trace")
	}
	if f := newLogEntryFilter("", []string{"inexistente"}); f.match(entry) {
		t.Error("--grep sem casar deveria filtrar fora")
	}
	if f := newLogEntryFilter("", nil); !f.match(entry) || f.match("") {
		t.Error("sem filtro passa tudo, menos entrada vazia")
	}
}

// --follow --ndjson: uma linha por entrada, e o --until-match encerra com 0.
func TestLogTailFollowNDJSON(t *testing.T) {
	logFollowInterval = 5 * time.Millisecond
	t.Cleanup(func() { logFollowInterval = 2 * time.Second })

	stub := logFollowStub(t, []string{
		"2026-07-25 10:00:00,000 INFO  [a.B] (task-1) subiu\n",
		"2026-07-25 10:00:01,000 ERROR [a.B] (task-2) falha na integração\n\tat foo.Bar(Bar.java:1)\n",
		"", // silêncio: fecha a entrada pendente e o --until-match casa
	})
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "log", "tail", "--follow", "--ndjson",
		"--until-match", "falha na integração", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d, quer 0 (o texto apareceu)\n%s", code, stdout)
	}
	linhas := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(linhas) != 2 {
		t.Fatalf("esperava 2 linhas NDJSON, veio %d:\n%s", len(linhas), stdout)
	}
	// Cada linha é um JSON completo e independente (não um envelope).
	var primeira, segunda fluig.LogEntry
	if err := json.Unmarshal([]byte(linhas[0]), &primeira); err != nil {
		t.Fatalf("linha 1 não é JSON: %v (%q)", err, linhas[0])
	}
	if err := json.Unmarshal([]byte(linhas[1]), &segunda); err != nil {
		t.Fatalf("linha 2 não é JSON: %v (%q)", err, linhas[1])
	}
	if primeira.Level != "INFO" || primeira.Logger != "a.B" || primeira.Message != "subiu" {
		t.Errorf("entrada 1: %+v", primeira)
	}
	if segunda.Level != "ERROR" || segunda.Thread != "task-2" ||
		segunda.Message != "falha na integração" || !strings.Contains(segunda.Stack, "Bar.java:1") {
		t.Errorf("entrada 2 (com stack): %+v", segunda)
	}
	if strings.Contains(stdout, `"ok"`) {
		t.Errorf("o streaming não emite envelope:\n%s", stdout)
	}
}

// --until-match que não aparece = resultado NEGATIVO: exit 4, para o agente
// distinguir "vi o que esperava" de "desisti".
func TestLogTailFollowUntilMatchNaoAparece(t *testing.T) {
	logFollowInterval = 5 * time.Millisecond
	t.Cleanup(func() { logFollowInterval = 2 * time.Second })

	stub := logFollowStub(t, []string{"2026-07-25 10:00:00,000 INFO  [a.B] (t) subiu\n", "", ""})
	proj := serverTestProject(t, stub.URL)

	code, stdout := runMain(t, "log", "tail", "--follow", "--ndjson",
		"--until-match", "nunca vai aparecer", "--for", "60ms", "--project", proj)
	if code != output.ExitNotFound {
		t.Fatalf("exit=%d, quer %d (esperou e não viu)\n%s", code, output.ExitNotFound, stdout)
	}
	// As entradas vistas no caminho continuam saindo — o monitor não perde dado.
	if !strings.Contains(stdout, `"subiu"`) {
		t.Errorf("as entradas do período deveriam sair mesmo sem casar:\n%s", stdout)
	}
}

// --max-entries e --idle-timeout encerram com 0 (não há veredicto a dar).
func TestLogTailFollowCondicoesDeParada(t *testing.T) {
	logFollowInterval = 5 * time.Millisecond
	t.Cleanup(func() { logFollowInterval = 2 * time.Second })

	chunks := []string{
		"2026-07-25 10:00:00,000 INFO  [a] (t) um\n2026-07-25 10:00:01,000 INFO  [a] (t) dois\n",
		"2026-07-25 10:00:02,000 INFO  [a] (t) tres\n",
		"", "", "",
	}

	t.Run("max-entries", func(t *testing.T) {
		proj := serverTestProject(t, logFollowStub(t, chunks).URL)
		code, stdout := runMain(t, "log", "tail", "--follow", "--ndjson", "--max-entries", "2", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("exit=%d\n%s", code, stdout)
		}
		if n := len(strings.Split(strings.TrimSpace(stdout), "\n")); n != 2 {
			t.Errorf("emitiu %d linhas, quer 2:\n%s", n, stdout)
		}
	})

	t.Run("idle-timeout", func(t *testing.T) {
		proj := serverTestProject(t, logFollowStub(t, chunks).URL)
		code, stdout := runMain(t, "log", "tail", "--follow", "--ndjson", "--idle-timeout", "40ms", "--project", proj)
		if code != output.ExitOK {
			t.Fatalf("exit=%d\n%s", code, stdout)
		}
		if !strings.Contains(stdout, `"tres"`) {
			t.Errorf("a última entrada deveria ter saído antes do encerramento:\n%s", stdout)
		}
	})
}

// O streaming e as condições de parada só existem no modo contínuo, e --ndjson
// não se mistura com o envelope do --json.
func TestLogTailFollowRecusas(t *testing.T) {
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{
		{"log", "tail", "--ndjson"},
		{"log", "tail", "--until-match", "x"},
		{"log", "tail", "--for", "1m"},
		{"log", "tail", "--idle-timeout", "1m"},
		{"log", "tail", "--max-entries", "5"},
		{"log", "tail", "--follow", "--ndjson", "--json"},
	} {
		code, stdout := runMain(t, append(args, "--project", proj)...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer %d\n%s", args, code, output.ExitUsage, stdout)
		}
	}
}

// garante que o helper de projeto compartilhado segue disponível aqui
var _ = config.Server{}

// Com o servidor lento, o ping de validação da sessão estourava o tempo limite e
// a CLI caía na resolução de senha, morrendo com AUTH_FAILED "nenhuma senha
// disponível" — mandando o usuário atrás do problema errado. Agora a causa real
// aparece (ROADMAP §2.10-H).
func TestTimeoutNaValidacaoDaSessaoNaoViraAuthFailed(t *testing.T) {
	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		io.WriteString(w, `{"message":"pong"}`)
	}))
	t.Cleanup(lento.Close)

	proj := serverTestProject(t, lento.URL)
	code, stdout := runMain(t, "log", "files", "--timeout", "60ms", "--json", "--project", proj)
	if code == output.ExitAuth {
		t.Fatalf("timeout não é falha de credencial (exit %d)\n%s", code, stdout)
	}
	if code != output.ExitServer {
		t.Fatalf("exit=%d, quer %d\n%s", code, output.ExitServer, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	if env.Error == nil || env.Error.Code != output.CodeTimeout {
		t.Fatalf("esperava code %s: %+v", output.CodeTimeout, env.Error)
	}
	if strings.Contains(env.Error.Message, "senha") {
		t.Errorf("a mensagem não deveria falar de senha: %q", env.Error.Message)
	}
}
