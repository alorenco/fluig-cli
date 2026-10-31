package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/rhino"
	"github.com/alorenco/fluig-cli/internal/testrun"
)

// unitStub troca o motor real por um falso: devolve a observação de cada caso
// a partir do "call" do plano.
func unitStub(t *testing.T, obs func(call string) string) *int {
	t.Helper()
	oldJ, oldE, oldX := unitFindJava, unitEnsureJar, unitExecutor
	t.Cleanup(func() { unitFindJava, unitEnsureJar, unitExecutor = oldJ, oldE, oldX })
	unitFindJava = func(context.Context, string) (*rhino.Java, error) {
		return &rhino.Java{Path: "/opt/java", Version: "17.0.19", Major: 17}, nil
	}
	unitEnsureJar = func(_ context.Context, explicit string) (*rhino.Jar, error) {
		if explicit != "" {
			return &rhino.Jar{Path: explicit, Source: "flag"}, nil
		}
		return &rhino.Jar{Path: "/cache/rhino-1.7.15.1.jar", Source: "cache"}, nil
	}
	runs := 0
	unitExecutor = func(*rhino.Java, *rhino.Jar) testrun.UnitExecutor {
		return func(_ context.Context, _, planPath string) error {
			runs++
			var plan struct {
				Out   string `json:"out"`
				Cases []struct {
					File  int `json:"file"`
					Case  int `json:"case"`
					Setup struct {
						Call string `json:"call"`
					} `json:"setup"`
				} `json:"cases"`
			}
			b, _ := os.ReadFile(planPath)
			json.Unmarshal(b, &plan)
			var sb strings.Builder
			for _, c := range plan.Cases {
				fmt.Fprintf(&sb, `{"file":%d,"case":%d,%s}`+"\n", c.File, c.Case, obs(c.Setup.Call))
			}
			return os.WriteFile(plan.Out, []byte(sb.String()), 0o600)
		}
	}
	return &runs
}

const unitFileBody = `{
  "description": "regras de Compras",
  "scripts": ["forms/Compras/events/validateForm.js"],
  "cases": [
    {"name": "urgente", "call": "urgencia", "expect": {"return": "S"}},
    {"name": "sem fornecedor", "event": "validateForm", "expect": {"throws": "Fornecedor"}}
  ]
}`

func unitProject(t *testing.T, body string) string {
	t.Helper()
	proj := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ev := filepath.Join(proj, "forms", "Compras", "events")
	os.MkdirAll(ev, 0o755)
	os.WriteFile(filepath.Join(ev, "validateForm.js"), []byte("function validateForm(form) {}\n"), 0o644)
	os.MkdirAll(filepath.Join(proj, "tests", "unit"), 0o755)
	if body != "" {
		os.WriteFile(filepath.Join(proj, "tests", "unit", "compras.unit.json"), []byte(body), 0o644)
	}
	return proj
}

func TestTestUnitPassaJSON(t *testing.T) {
	unitStub(t, func(call string) string {
		if call == "urgencia" {
			return `"return":"S","returnType":"js-string"`
		}
		return `"thrown":{"kind":"value","message":"<b>Campos:</b><br>- Fornecedor"}`
	})
	proj := unitProject(t, unitFileBody)
	code, stdout := runMain(t, "test", "unit", "--json", "--project", proj)
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	_, data := envelope(t, stdout)
	eng := data["engine"].(map[string]any)
	if eng["rhino"] != "1.7.15.1" || eng["source"] != "cache" || eng["languageVersion"].(float64) != 200 {
		t.Errorf("engine: %v", eng)
	}
	sm := data["summary"].(map[string]any)
	if sm["cases"].(float64) != 2 || sm["passed"].(float64) != 2 {
		t.Errorf("summary: %v", sm)
	}
}

func TestTestUnitFalhaEFiltro(t *testing.T) {
	unitStub(t, func(string) string { return `"return":"N","returnType":"js-string"` })
	proj := unitProject(t, unitFileBody)
	code, stdout := runMain(t, "test", "unit", "--json", "--project", proj, "--filter", "URGENTE")
	env, data := envelope(t, stdout)
	if code != output.ExitGeneric || env.Error == nil || env.Error.Code != output.CodeTestFailed {
		t.Fatalf("falha: exit=%d %+v", code, env.Error)
	}
	files := data["files"].([]any)
	cases := files[0].(map[string]any)["cases"].([]any)
	if len(cases) != 1 || cases[0].(map[string]any)["status"] != "failed" {
		t.Errorf("--filter deixa só o caso urgente: %v", cases)
	}
	if code, _ := runMain(t, "test", "unit", "--project", proj, "--filter", "nada"); code != output.ExitUsage {
		t.Errorf("filtro sem caso: exit=%d", code)
	}
}

// Modo humano: tabela com o resultado e o esperado × obtido da falha.
func TestTestUnitTabela(t *testing.T) {
	unitStub(t, func(call string) string {
		if call == "urgencia" {
			return `"return":"N","returnType":"js-string","logs":[{"level":"warn","message":"sem vencimento"}]`
		}
		return `"thrown":{"kind":"value","message":"Fornecedor"}`
	})
	proj := unitProject(t, unitFileBody)
	code, stdout, stderr := runMainStderr(t, "test", "unit", "--project", proj)
	stderr = stdout + stderr
	if code != output.ExitGeneric {
		t.Fatalf("exit=%d", code)
	}
	for _, want := range []string{"│ Caso", "urgente", "FALHOU", "passou", `return: esperado "S", obtido "N"`,
		"log.warn: sem vencimento", "Motor: Rhino 1.7.15.1 (oficial, do cache), ES6, Java 17.0.19", "Resumo: 1 passaram · 1 falharam"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("sem %q:\n%s", want, stderr)
		}
	}
}

func TestTestUnitDependencias(t *testing.T) {
	unitStub(t, nil)
	proj := unitProject(t, unitFileBody)
	unitFindJava = func(context.Context, string) (*rhino.Java, error) {
		return nil, fmt.Errorf("%w: sem java no PATH", rhino.ErrJavaNotFound)
	}
	code, stdout := runMain(t, "test", "unit", "--json", "--project", proj)
	env, _ := envelope(t, stdout)
	if code != output.ExitMissingHelper || env.Error.Code != output.CodeJavaNotFound {
		t.Errorf("sem java: exit=%d %+v", code, env.Error)
	}

	unitStub(t, nil)
	unitEnsureJar = func(context.Context, string) (*rhino.Jar, error) {
		return nil, fmt.Errorf("%w: download falhou", rhino.ErrUnavailable)
	}
	code, stdout = runMain(t, "test", "unit", "--json", "--project", proj)
	env, _ = envelope(t, stdout)
	if code != output.ExitMissingHelper || env.Error.Code != output.CodeRhinoUnavailable || !strings.Contains(env.Error.Message, "--rhino-jar") {
		t.Errorf("sem jar: exit=%d %+v", code, env.Error)
	}

	unitStub(t, nil)
	unitExecutor = func(*rhino.Java, *rhino.Jar) testrun.UnitExecutor {
		return func(context.Context, string, string) error {
			return errors.New("Could not find or load main class org.mozilla.javascript.tools.shell.Main")
		}
	}
	code, stdout = runMain(t, "test", "unit", "--json", "--project", proj, "--rhino-jar", "/x/rhino-core.jar")
	env, _ = envelope(t, stdout)
	if code != output.ExitMissingHelper || !strings.Contains(env.Error.Message, "não tem o shell do Rhino") {
		t.Errorf("jar sem shell: exit=%d %+v", code, env.Error)
	}
}

func TestTestUnitArquivos(t *testing.T) {
	runs := unitStub(t, func(string) string { return `"return":"S"` })
	// Sem tests/unit/: exit 4.
	vazio := t.TempDir()
	if code, _ := runMain(t, "test", "unit", "--project", vazio); code != output.ExitNotFound {
		t.Errorf("sem pasta: exit=%d", code)
	}
	proj := unitProject(t, `{"scripts": ["forms/Compras/events/validateForm.js"], "cases": [{"name": "a", "call": "f", "expcet": {}}]}`)
	code, stdout := runMain(t, "test", "unit", "--json", "--project", proj)
	env, _ := envelope(t, stdout)
	if code != output.ExitUsage || !strings.Contains(env.Error.Message, "chave desconhecida \"expcet\"") {
		t.Errorf("arquivo inválido: exit=%d %+v", code, env.Error)
	}
	proj = unitProject(t, `{"scripts": ["forms/nao/existe.js"], "cases": [{"name": "a", "call": "f", "expect": {"throws": true}}]}`)
	if code, _ := runMain(t, "test", "unit", "--project", proj); code != output.ExitNotFound {
		t.Errorf("script ausente: exit=%d", code)
	}
	if code, _ := runMain(t, "test", "unit", "--self-check", "x.unit.json"); code != output.ExitUsage {
		t.Errorf("--self-check com arquivo: exit=%d", code)
	}
	if *runs != 0 {
		t.Errorf("nenhum desses casos pode subir o motor: %d", *runs)
	}
}

func TestTestUnitSelfCheck(t *testing.T) {
	unitStub(t, func(string) string { return `"return":"E"` })
	code, stdout := runMain(t, "test", "unit", "--self-check", "--json")
	env, data := envelope(t, stdout)
	if code != output.ExitGeneric || !strings.Contains(env.Error.Message, "o motor diverge do Rhino do servidor") {
		t.Fatalf("self-check divergente: exit=%d %+v", code, env.Error)
	}
	if data["summary"].(map[string]any)["cases"].(float64) != 21 {
		t.Errorf("21 sondas: %v", data["summary"])
	}
}
