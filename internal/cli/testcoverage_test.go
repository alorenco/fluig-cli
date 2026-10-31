package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/output"
)

// histCompras é o histórico do caminho 4 → 76 → 45 → 50 → 5 da fixture
// compras_entrada_documento v25 (o caminho que a suiteCompras percorre).
func histCompras(versao string) [][]string {
	return [][]string{
		{"1", "0", "4", "0", versao, "compras_entrada_documento"},
		{"2", "1", "76", "0", versao, "compras_entrada_documento"},
		{"3", "2", "45", "0", versao, "compras_entrada_documento"},
		{"4", "3", "50", "0", versao, "compras_entrada_documento"},
		{"5", "4", "5", "0", versao, "compras_entrada_documento"},
	}
}

func coverageStub() *procStub {
	return &procStub{history: map[int][][]string{
		700001: histCompras("25"),
		700005: histCompras("24"),
		700009: {{"1", "0", "4", "0", "25", "outro_processo"}},
	}}
}

func coverageData(t *testing.T, stdout string) (output.Envelope, map[string]any) {
	t.Helper()
	env, data := envelope(t, stdout)
	if data == nil {
		t.Fatalf("sem data: %s", stdout)
	}
	return env, data
}

func coveredEdges(data map[string]any) []string {
	var out []string
	for _, e := range data["edges"].([]any) {
		m := e.(map[string]any)
		if m["covered"] == true {
			out = append(out, strconv.Itoa(int(m["from"].(float64)))+"→"+strconv.Itoa(int(m["to"].(float64))))
		}
	}
	sort.Strings(out)
	return out
}

// Fonte "executar": roda a suíte e mede a solicitação criada.
func TestTestCoverageRunJSON(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout := runMain(t, "test", "coverage", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	_, data := coverageData(t, stdout)
	if data["process"] != "compras_entrada_documento" || data["version"].(float64) != 25 {
		t.Errorf("processo/versão: %v %v", data["process"], data["version"])
	}
	src := data["source"].(map[string]any)
	if src["kind"] != "run" || src["requests"].([]any)[0].(float64) != 700001 {
		t.Errorf("fonte: %+v", src)
	}
	if data["run"] == nil {
		t.Error("a fonte executar traz o relatório do test run em run")
	}
	got := strings.Join(coveredEdges(data), " ")
	if got != "45→50 4→76 50→5 76→45" {
		t.Errorf("arestas cobertas: %s", got)
	}
	sum := data["summary"].(map[string]any)["main"].(map[string]any)
	if sum["covered"].(float64) != 4 {
		t.Errorf("resumo: %+v", sum)
	}
}

// Modo humano: tabela com bordas e o cenário (regra 7).
func TestTestCoverageTabela(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout := runMain(t, "test", "coverage", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, want := range []string{"│", "Origem", "Destino", "Coberta", "Cenários", "NÃO", "sim", "aprova",
		"Cobertura de compras_entrada_documento (versão 25)", "Fluxo principal: 4 de", "Gateways:",
		"Etapas humanas nunca abertas:", "Tratamento de erro:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("saída sem %q:\n%s", want, stdout)
		}
	}
	// Não cobertas primeiro.
	if strings.Index(stdout, "NÃO") > strings.Index(stdout, " sim ") {
		t.Error("as não cobertas vêm primeiro")
	}
}

func TestTestCoverageMinimo(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout := runMain(t, "test", "coverage", "--min", "90", "--json", "--project", proj, "--server", "homolog")
	env, data := coverageData(t, stdout)
	if code != output.ExitGeneric || env.Error.Code != output.CodeCoverageBelowMin || data["summary"] == nil {
		t.Fatalf("exit=%d env=%+v", code, env.Error)
	}
	// Com mínimo atingível: exit 0.
	code, _ = runMain(t, "test", "coverage", "--min", "1", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Errorf("--min 1: exit=%d", code)
	}
}

// TEST_FAILED vence COVERAGE_BELOW_MIN.
func TestTestCoverageTestFailedVence(t *testing.T) {
	stub := coverageStub()
	suite := strings.Replace(suiteCompras, `"state":5,`, `"state":99,`, 1)
	proj := testProject(t, stub.server(t).URL, "hml", suite)
	code, stdout := runMain(t, "test", "coverage", "--min", "90", "--json", "--project", proj, "--server", "homolog")
	env, _ := coverageData(t, stdout)
	if code != output.ExitGeneric || env.Error.Code != output.CodeTestFailed {
		t.Fatalf("exit=%d env=%+v", code, env.Error)
	}
}

// Fonte "lista": só leitura; versão diferente e inexistente viram aviso.
func TestTestCoverageRequests(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, stdout, stderr := runMainStderr(t, "test", "coverage", "--process", "compras_entrada_documento",
		"--requests", "700005, 700404", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	_, data := coverageData(t, stdout)
	avisos := strings.Join(func() []string {
		var out []string
		for _, w := range data["warnings"].([]any) {
			out = append(out, w.(string))
		}
		return out
	}(), "\n")
	for _, want := range []string{"a solicitação 700404 não existe", "a 700005 rodou na versão 24; a referência é a 25"} {
		if !strings.Contains(avisos, want) || !strings.Contains(stderr, want) {
			t.Errorf("aviso sem %q:\njson: %s\nstderr: %s", want, avisos, stderr)
		}
	}
	if stub.moves != nil || stub.canceled {
		t.Error("a fonte lista não escreve")
	}
}

// Fonte "resultado": lê o envelope de um test run anterior.
func TestTestCoverageFromResult(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, runOut := runMain(t, "test", "run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("run: exit=%d", code)
	}
	arq := filepath.Join(t.TempDir(), "r.json")
	os.WriteFile(arq, []byte(runOut), 0o644)
	movesAntes := len(stub.moves)
	code, stdout := runMain(t, "test", "coverage", "--from-result", arq, "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	_, data := coverageData(t, stdout)
	if data["source"].(map[string]any)["kind"] != "result" || strings.Join(coveredEdges(data), " ") != "45→50 4→76 50→5 76→45" {
		t.Errorf("fonte resultado: %+v", data["source"])
	}
	if len(stub.moves) != movesAntes {
		t.Error("a fonte resultado não pode executar nada")
	}
}

func TestTestCoverageUso(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	casos := [][]string{
		{"--requests", "1", "--from-result", "x.json"},
		{"tests/compras", "--requests", "1"},
		{"--requests", "700001"},
		{"--min", "150"},
		{"--process", "p", "--requests", "1", "--scenario", "a"},
		{"--process", "p", "--requests", "abc"},
	}
	for _, c := range casos {
		args := append([]string{"test", "coverage", "--json", "--project", proj, "--server", "homolog"}, c...)
		if code, out := runMain(t, args...); code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer 2\n%s", c, code, out)
		}
	}
	// Solicitação de outro processo.
	code, stdout := runMain(t, "test", "coverage", "--process", "compras_entrada_documento", "--requests", "700009",
		"--json", "--project", proj, "--server", "homolog")
	env, _ := envelope(t, stdout)
	if code != output.ExitUsage || !strings.Contains(env.Error.Message, `700009 é do processo "outro_processo"`) {
		t.Errorf("outro processo: exit=%d %+v", code, env.Error)
	}
	// Suítes de processos diferentes.
	os.MkdirAll(filepath.Join(proj, "tests", "outro"), 0o755)
	os.WriteFile(filepath.Join(proj, "tests", "outro", "s.json"),
		[]byte(`{"process":"outro","scenarios":[{"name":"a","steps":[{"start":true}]}]}`), 0o644)
	code, stdout = runMain(t, "test", "coverage", "--json", "--project", proj, "--server", "homolog")
	env, _ = envelope(t, stdout)
	if code != output.ExitUsage || !strings.Contains(env.Error.Message, "um processo por vez") {
		t.Errorf("processos misturados: exit=%d %+v", code, env.Error)
	}
}

func TestTestCoverageNaoEncontrado(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, _ := runMain(t, "test", "coverage", "--process", "compras_entrada_documento", "--requests", "700404,700405",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("todas inexistentes: exit=%d", code)
	}
	code, _ = runMain(t, "test", "coverage", "--process", "compras_entrada_documento", "--requests", "700001",
		"--version", "99", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("versão inexistente: exit=%d", code)
	}
	code, _ = runMain(t, "test", "coverage", "--process", "inexistente", "--requests", "700001",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitNotFound {
		t.Errorf("processo inexistente: exit=%d", code)
	}
}

func TestTestCoverageSemHelper(t *testing.T) {
	stub := coverageStub()
	stub.noHelper = true
	proj := testProject(t, stub.server(t).URL, "hml", suiteCompras)
	code, _ := runMain(t, "test", "coverage", "--process", "compras_entrada_documento", "--requests", "700001",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitMissingHelper {
		t.Errorf("sem helper: exit=%d, quer 7", code)
	}
}

// Produção: a fonte executar recusa; as fontes de leitura aceitam com aviso.
func TestTestCoverageProducao(t *testing.T) {
	stub := coverageStub()
	proj := testProject(t, stub.server(t).URL, config.EnvProd, suiteCompras)
	code, _ := runMain(t, "test", "coverage", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage || stub.hits != 0 {
		t.Fatalf("executar em produção: exit=%d hits=%d", code, stub.hits)
	}
	code, _, stderr := runMainStderr(t, "test", "coverage", "--process", "compras_entrada_documento", "--requests", "700001",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || !strings.Contains(stderr, "PRODUÇÃO") {
		t.Errorf("lista em produção: exit=%d stderr=%s", code, stderr)
	}
}
