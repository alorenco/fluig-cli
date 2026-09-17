package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// observeStub simula o Fluig com o fluigcliHelper e as rotas de observação.
type observeStub struct {
	version   string         // "" = 0.11.0
	hasRoutes bool           // false = helper sem a rota (404)
	postCode  int            // 0 = 201
	postError string         // corpo do erro do POST
	getBody   string         // "" = uma observação
	getCode   int            // 0 = 200
	getError  string         // corpo do erro do GET
	lastBody  map[string]any // corpo recebido no POST
	lastQuery string
}

const observeCriada = `{"id":123456,"processInstanceId":235189,"stateSequence":34,"movementSequence":5,` +
	`"threadSequence":0,"colleagueId":"sabia","observationDate":"2026-09-17T14:03:11.000-04:00",` +
	`"observation":"<b>teste helper</b> observação <i>sem</i> move"}`

func (s *observeStub) server(t *testing.T) *httptest.Server {
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
		v := s.version
		if v == "" {
			v = "0.11.0"
		}
		io.WriteString(w, `{"name":"fluigcliHelper","version":"`+v+`"}`)
	})
	mux.HandleFunc("/fluigcliHelper/api/workflows/235189/observations", func(w http.ResponseWriter, r *http.Request) {
		if !s.hasRoutes {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			s.lastBody = nil
			_ = json.Unmarshal(body, &s.lastBody)
			if s.postCode != 0 {
				w.WriteHeader(s.postCode)
				io.WriteString(w, s.postError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, observeCriada)
		case http.MethodGet:
			s.lastQuery = r.URL.RawQuery
			if s.getCode != 0 {
				w.WriteHeader(s.getCode)
				io.WriteString(w, s.getError)
				return
			}
			if s.getBody == "" {
				io.WriteString(w, "["+observeCriada+"]")
				return
			}
			io.WriteString(w, s.getBody)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRequestObserveJSON(t *testing.T) {
	stub := &observeStub{hasRoutes: true}
	proj := serverTestProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "request", "observe", "235189", "--text", "<b>teste helper</b> observação sem move", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	obs, _ := data["observation"].(map[string]any)
	if obs["id"].(float64) != 123456 || obs["colleagueId"] != "sabia" {
		t.Errorf("observação inesperada no envelope: %v", data)
	}
	if obs["observationDate"] != "2026-09-17T14:03:11.000-04:00" {
		t.Errorf("a data do --json deve ser a ISO do helper: %v", obs["observationDate"])
	}
	// Sem --state/--movement, o corpo não leva os dois (o helper resolve).
	if _, ok := stub.lastBody["stateSequence"]; ok {
		t.Errorf("stateSequence não devia ir no corpo: %v", stub.lastBody)
	}
	if stub.lastBody["observation"] != "<b>teste helper</b> observação sem move" {
		t.Errorf("texto não repassado íntegro: %v", stub.lastBody)
	}
}

func TestRequestObserveHumano(t *testing.T) {
	stub := &observeStub{hasRoutes: true}
	proj := serverTestProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "request", "observe", "235189", "--text", "ok", "--state", "34", "--movement", "5", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"123456", "235189", "etapa 34", "movimento 5", "sabia", "não foi movimentada"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("mensagem sem %q:\n%s", want, stdout)
		}
	}
	if stub.lastBody["stateSequence"] != float64(34) || stub.lastBody["movementSequence"] != float64(5) {
		t.Errorf("--state/--movement não repassados: %v", stub.lastBody)
	}
}

// --text-file -: lê o laudo do stdin (modo natural para agentes).
func TestRequestObserveTextFileStdin(t *testing.T) {
	stub := &observeStub{hasRoutes: true}
	proj := serverTestProject(t, stub.server(t).URL)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	io.WriteString(w, "<p>laudo via stdin</p>\n")
	w.Close()

	code, stdout := runMain(t, "request", "observe", "235189", "--text-file", "-", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	if stub.lastBody["observation"] != "<p>laudo via stdin</p>\n" {
		t.Errorf("texto do stdin não repassado: %v", stub.lastBody)
	}
}

func TestRequestObserveUsoIncorreto(t *testing.T) {
	stub := &observeStub{hasRoutes: true}
	proj := serverTestProject(t, stub.server(t).URL)

	cases := [][]string{
		{"request", "observe", "235189", "--project", proj},                                         // sem texto
		{"request", "observe", "235189", "--text", "a", "--text-file", "x.html", "--project", proj}, // as duas fontes
		{"request", "observe", "235189", "--text", "a", "--state", "34", "--project", proj},         // --state sem --movement
		{"request", "observe", "235189", "--text", "   ", "--project", proj},                        // vazio após trim
		{"request", "observe", "abc", "--text", "a", "--project", proj},                             // número inválido
	}
	for _, args := range cases {
		code, _, stderr := runMainStderr(t, args...)
		if code != output.ExitUsage {
			t.Errorf("%v: quer exit %d, veio %d (%s)", args[2:], output.ExitUsage, code, stderr)
		}
	}
	if stub.lastBody != nil {
		t.Errorf("nenhum caso de uso incorreto pode chegar ao servidor: %v", stub.lastBody)
	}
}

// Helper sem a rota (< 0.11.0): exit 7 orientando o install-helper --force.
func TestRequestObserveHelperAntigo(t *testing.T) {
	stub := &observeStub{version: "0.10.3", hasRoutes: false}
	proj := serverTestProject(t, stub.server(t).URL)

	code, _, stderr := runMainStderr(t, "request", "observe", "235189", "--text", "a", "--project", proj)
	if code != output.ExitMissingHelper || !strings.Contains(stderr, "desatualizado") {
		t.Errorf("quer exit %d com 'desatualizado', veio %d: %s", output.ExitMissingHelper, code, stderr)
	}
	code, _, stderr = runMainStderr(t, "request", "observations", "235189", "--project", proj)
	if code != output.ExitMissingHelper {
		t.Errorf("observations: quer exit %d, veio %d: %s", output.ExitMissingHelper, code, stderr)
	}
}

// 409 de paralelismo: a CLI completa a mensagem do helper com as flags.
func TestRequestObserveTarefasParalelas(t *testing.T) {
	stub := &observeStub{hasRoutes: true, postCode: http.StatusConflict,
		postError: "solicitação tem tarefas paralelas; informe stateSequence e movementSequence"}
	proj := serverTestProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "request", "observe", "235189", "--text", "a", "--json", "--project", proj)
	if code != output.ExitServer {
		t.Fatalf("quer exit %d, veio %d: %s", output.ExitServer, code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	if env.Error == nil || env.Error.Code != output.CodeServerError {
		t.Fatalf("erro inesperado: %+v", env.Error)
	}
	for _, want := range []string{"paralelas", "--state e --movement", "request show 235189"} {
		if !strings.Contains(env.Error.Message, want) {
			t.Errorf("mensagem sem %q: %s", want, env.Error.Message)
		}
	}
}

// 409 de estado (finalizada): exit 5 com o motivo do helper, sem a dica das flags.
func TestRequestObserveFinalizada(t *testing.T) {
	stub := &observeStub{hasRoutes: true, postCode: http.StatusConflict,
		postError: "solicitação finalizada; não aceita observação"}
	proj := serverTestProject(t, stub.server(t).URL)

	code, _, stderr := runMainStderr(t, "request", "observe", "235189", "--text", "a", "--project", proj)
	if code != output.ExitServer || !strings.Contains(stderr, "finalizada") || strings.Contains(stderr, "--state") {
		t.Errorf("quer exit %d com o motivo do helper e sem a dica das flags, veio %d: %s", output.ExitServer, code, stderr)
	}
}

// GET sem --state numa solicitação encerrada: o helper responde 409 pedindo a
// etapa e a CLI traduz para a flag (exit 5, como os demais conflitos).
func TestRequestObservationsEncerradaSemState(t *testing.T) {
	stub := &observeStub{hasRoutes: true, getCode: http.StatusConflict,
		getError: "solicitação finalizada; informe stateSequence"}
	proj := serverTestProject(t, stub.server(t).URL)

	code, _, stderr := runMainStderr(t, "request", "observations", "235189", "--project", proj)
	if code != output.ExitServer {
		t.Fatalf("quer exit %d, veio %d: %s", output.ExitServer, code, stderr)
	}
	for _, want := range []string{"finalizada", "--state", "request show 235189"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("mensagem sem %q: %s", want, stderr)
		}
	}
}

func TestRequestObservationsTabela(t *testing.T) {
	stub := &observeStub{hasRoutes: true}
	proj := serverTestProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "request", "observations", "235189", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"Data", "Autor", "Etapa", "Mov.", "Observação", "2026-09-17 14:03", "sabia", "34", "5", "teste helper observação sem move"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("tabela sem %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "<b>") {
		t.Errorf("a tabela não deve mostrar as tags HTML:\n%s", stdout)
	}
	if stub.lastQuery != "threadSequence=0" {
		t.Errorf("sem --state a query leva só a thread: %q", stub.lastQuery)
	}

	code, stdout = runMain(t, "request", "observations", "235189", "--state", "34", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("--json exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	list, _ := data["observations"].([]any)
	if data["count"].(float64) != 1 || len(list) != 1 {
		t.Errorf("envelope inesperado: %v", data)
	}
	first, _ := list[0].(map[string]any)
	if first["observation"] != "<b>teste helper</b> observação <i>sem</i> move" {
		t.Errorf("o --json deve levar o HTML íntegro: %v", first["observation"])
	}
	if stub.lastQuery != "threadSequence=0&stateSequence=34" {
		t.Errorf("query inesperada: %q", stub.lastQuery)
	}
}

func TestRequestObservationsVazia(t *testing.T) {
	stub := &observeStub{hasRoutes: true, getBody: "[]"}
	proj := serverTestProject(t, stub.server(t).URL)

	code, stdout := runMain(t, "request", "observations", "235189", "--project", proj)
	if code != output.ExitOK || !strings.Contains(stdout, "Nenhuma observação") || !strings.Contains(stdout, "request observe 235189") {
		t.Errorf("lista vazia deve orientar o observe (exit %d): %s", code, stdout)
	}
}

func TestObservationPreview(t *testing.T) {
	if got := observationPreview("<p>Laudo:</p>\n<ul><li>item  1</li></ul>", 80); got != "Laudo: item 1" {
		t.Errorf("preview inesperado: %q", got)
	}
	longo := strings.Repeat("ação ", 30)
	got := observationPreview(longo, 20)
	if len([]rune(got)) > 20 || !strings.HasSuffix(got, "…") {
		t.Errorf("deve cortar em 20 runas com reticência: %q", got)
	}
}

func TestFmtObservationTime(t *testing.T) {
	if got := fmtObservationTime("2026-09-17T14:03:11.000-04:00"); got != "2026-09-17 14:03" {
		t.Errorf("hora inesperada: %q", got)
	}
	if got := fmtObservationTime("texto estranho"); got != "texto estranho" {
		t.Errorf("texto inesperado deve sair como veio: %q", got)
	}
}
