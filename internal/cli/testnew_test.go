package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func TestTestNewGera(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", "")
	out := filepath.Join(t.TempDir(), "esqueleto")
	code, stdout := runMain(t, "test", "new", "compras_entrada_documento", "--out", out,
		"--split-service", "integracao", "--from-request", "700001", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	_, data := envelope(t, stdout)
	cov := data["coverage"].(map[string]any)
	if cov["covered"] != cov["total"] || cov["total"].(float64) == 0 {
		t.Errorf("os caminhos cobrem o fluxo principal: %+v", cov)
	}
	if w := data["warnings"].([]any); len(w) == 0 || !strings.Contains(w[0].(string), "dados reais da solicitação 700001") {
		t.Errorf("aviso de dados reais: %v", w)
	}
	for _, f := range []string{"compras_entrada_documento.json", "integracao/compras_entrada_documento.json", "fixtures/card_base.json"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("arquivo %s não gerado", f)
		}
	}
	// A fixture é o card achatado (linhas filhas como campo___N), sem colunas
	// de controle.
	var card map[string]string
	b, _ := os.ReadFile(filepath.Join(out, "fixtures", "card_base.json"))
	json.Unmarshal(b, &card)
	if card["parcelaValor___1"] == "" || card["currentState"] != "" || card["cardid___1"] != "" {
		t.Errorf("fixture: %d campos, parcelaValor___1=%q", len(card), card["parcelaValor___1"])
	}
	// O esqueleto gerado passa no dry-run da própria CLI.
	code, stdout = runMain(t, "test", "run", out, "--dry-run", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Errorf("dry-run do esqueleto: exit=%d\n%s", code, stdout)
	}
	if stub.moves != nil || stub.canceled {
		t.Error("o test new e o dry-run não escrevem")
	}

	// Arquivo existente sem --force: exit 2, nada gravado.
	os.Remove(filepath.Join(out, "fixtures", "card_base.json"))
	code, _ = runMain(t, "test", "new", "compras_entrada_documento", "--out", out,
		"--from-request", "700001", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem --force: exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(out, "fixtures", "card_base.json")); err == nil {
		t.Error("a recusa não pode gravar nada")
	}
	code, _ = runMain(t, "test", "new", "compras_entrada_documento", "--out", out, "--force",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Errorf("com --force: exit=%d", code)
	}
}

// Sem --out, grava em tests/<processId>/ do projeto.
func TestTestNewPastaPadrao(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", "")
	code, _ := runMain(t, "test", "new", "compras_entrada_documento", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, "tests", "compras_entrada_documento", "compras_entrada_documento.json")); err != nil {
		t.Error("não gravou em tests/<processId>/")
	}
}

func TestTestNewStdoutEUso(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", "")
	code, stdout := runMain(t, "test", "new", "compras_entrada_documento", "--stdout", "--project", proj, "--server", "homolog")
	if code != output.ExitOK || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"process": "compras_entrada_documento"`) {
		t.Fatalf("--stdout: exit=%d\n%s", code, stdout)
	}
	for _, c := range [][]string{
		{"--stdout", "--split-service", "integracao"},
		{"--stdout", "--from-request", "700001"},
		{"--stdout", "--json"},
		{"--max-scenarios", "0"},
		{"--split-service", "a/b"},
	} {
		args := append([]string{"test", "new", "compras_entrada_documento", "--project", proj, "--server", "homolog"}, c...)
		if code, _ := runMain(t, args...); code != output.ExitUsage {
			t.Errorf("%v: exit=%d, quer 2", c, code)
		}
	}
	// Solicitação de outro processo.
	out := filepath.Join(t.TempDir(), "x")
	code, stdout = runMain(t, "test", "new", "outro_processo", "--out", out, "--from-request", "700001",
		"--json", "--project", proj, "--server", "homolog")
	env, _ := envelope(t, stdout)
	if code != output.ExitUsage || !strings.Contains(env.Error.Message, "é do processo") {
		t.Errorf("outro processo: exit=%d %+v", code, env.Error)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("nada pode ser gravado")
	}
}

func TestTestNewNaoEncontrado(t *testing.T) {
	stub := &procStub{}
	proj := testProject(t, stub.server(t).URL, "hml", "")
	for _, c := range [][]string{
		{"inexistente"},
		{"compras_entrada_documento", "--version", "99"},
		{"compras_entrada_documento", "--from-request", "700404"},
	} {
		args := append([]string{"test", "new", "--out", filepath.Join(t.TempDir(), "o"), "--json", "--project", proj, "--server", "homolog"}, c...)
		if code, out := runMain(t, args...); code != output.ExitNotFound {
			t.Errorf("%v: exit=%d, quer 4\n%s", c, code, out)
		}
	}
}
