package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// FL005 — método do hAPI chamado como função global em script de processo
// (ROADMAP3 §4.11-A). O caso real: getCardValue("x") solto passou no audit e
// derrubou a service task em runtime.
func TestProcessGlobalFindings(t *testing.T) {
	rel := "workflow/scripts/proc.servicetask7.js"

	t.Run("o caso do relato: getCardValue solto é erro", func(t *testing.T) {
		src := []byte(`function servicetask7(a, b, c) {
	var codColigada = getCardValue("codColigada");
}`)
		fs := processGlobalFindings(t.TempDir(), rel, src)
		if len(fs) != 1 || fs[0].Rule != RuleBareHAPICall || fs[0].Severity != SeverityError {
			t.Fatalf("esperava 1 FL005 erro: %+v", fs)
		}
		if fs[0].Line != 2 || fs[0].Suggestion != "use hAPI.getCardValue(...)" {
			t.Errorf("linha/sugestão: %+v", fs[0])
		}
	})

	t.Run("com o hAPI. na frente não há achado", func(t *testing.T) {
		src := []byte(`var x = hAPI.getCardValue("codColigada");`)
		if fs := processGlobalFindings(t.TempDir(), rel, src); len(fs) != 0 {
			t.Errorf("chamada correta não pode acusar: %+v", fs)
		}
	})

	t.Run("getValue global é legítimo (não é membro do hAPI)", func(t *testing.T) {
		src := []byte(`var estado = getValue("WKNumState");`)
		if fs := processGlobalFindings(t.TempDir(), rel, src); len(fs) != 0 {
			t.Errorf("getValue é A global de script de processo: %+v", fs)
		}
	})

	t.Run("helper do usuário declarado no MESMO arquivo é descontado", func(t *testing.T) {
		src := []byte(`function getCardValue(campo) { return hAPI.getCardValue(campo); }
var x = getCardValue("y");`)
		if fs := processGlobalFindings(t.TempDir(), rel, src); len(fs) != 0 {
			t.Errorf("função declarada no arquivo não pode acusar: %+v", fs)
		}
	})

	t.Run("helper declarado em OUTRO script do mesmo processo é descontado", func(t *testing.T) {
		// Os eventos de um processo compartilham o escopo do Rhino.
		root := t.TempDir()
		dir := filepath.Join(root, "workflow", "scripts")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		helper := []byte(`function getCardValue(campo) { return hAPI.getCardValue(campo); }`)
		if err := os.WriteFile(filepath.Join(dir, "proc.beforeTaskSave.js"), helper, 0o644); err != nil {
			t.Fatal(err)
		}
		src := []byte(`var x = getCardValue("y");`)
		if fs := processGlobalFindings(root, rel, src); len(fs) != 0 {
			t.Errorf("helper em script irmão do processo não pode acusar: %+v", fs)
		}
		// Script de OUTRO processo não conta.
		src2 := []byte(`var x = getCardValue("y");`)
		if fs := processGlobalFindings(root, "workflow/scripts/outro.servicetask1.js", src2); len(fs) != 1 {
			t.Errorf("helper de processo diferente não desconta: %+v", fs)
		}
	})

	t.Run("string e comentário são mascarados", func(t *testing.T) {
		src := []byte(`// getCardValue("comentado")
var msg = 'texto com getCardValue("x") dentro';`)
		if fs := processGlobalFindings(t.TempDir(), rel, src); len(fs) != 0 {
			t.Errorf("string/comentário não pode acusar: %+v", fs)
		}
	})

	t.Run("repetição vira UM achado por nome", func(t *testing.T) {
		src := []byte(`var a = getCardValue("a");
var b = getCardValue("b");
setCardValue("c", "1");`)
		fs := processGlobalFindings(t.TempDir(), rel, src)
		if len(fs) != 2 {
			t.Errorf("esperava getCardValue ×1 + setCardValue ×1: %+v", fs)
		}
	})
}

// A FL005 só roda em script de processo — nos outros server-side o conjunto de
// globais não é fechado (e o hAPI nem existe lá).
func TestProcessGlobalSoNoScriptDeProcesso(t *testing.T) {
	src := []byte(`var x = getCardValue("y");`)
	fs := scanJS("", "datasets/ds_x.js", src)
	for _, f := range fs {
		if f.Rule == RuleBareHAPICall {
			t.Errorf("FL005 fora de workflow/scripts/: %+v", f)
		}
	}
	fs = scanJS("", "workflow/scripts/p.beforeTaskSave.js", src)
	found := false
	for _, f := range fs {
		found = found || f.Rule == RuleBareHAPICall
	}
	if !found {
		t.Error("FL005 não rodou no script de processo")
	}
}
