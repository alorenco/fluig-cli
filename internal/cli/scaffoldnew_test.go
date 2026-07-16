package cli

// Testes dos subcomandos `new` (scaffolds locais — nenhum toca o servidor).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

func TestDatasetNew(t *testing.T) {
	proj := t.TempDir()
	code, stdout := runMain(t, "dataset", "new", "ds_clientes", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é JSON: %v\n%s", err, stdout)
	}
	data, _ := env.Data.(map[string]any)
	if data["dataset"] != "ds_clientes" || data["file"] != "datasets/ds_clientes.js" {
		t.Errorf("data inesperado: %+v", data)
	}
	got, err := os.ReadFile(filepath.Join(proj, "datasets", "ds_clientes.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "function createDataset(fields, constraints, sortFields)") {
		t.Errorf("esqueleto sem createDataset: %s", got)
	}

	// Repetir é erro de uso (o arquivo já existe), sem sobrescrever.
	if code, _ := runMain(t, "dataset", "new", "ds_clientes", "--project", proj, "--json"); code != output.ExitUsage {
		t.Errorf("repetição: exit=%d, quer %d", code, output.ExitUsage)
	}
}

// Homônimo em SUBPASTA também bloqueia (o export/import procuram recursivo).
func TestDatasetNewDuplicataEmSubpasta(t *testing.T) {
	proj := t.TempDir()
	sub := filepath.Join(proj, "datasets", "financeiro")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "ds_x.js"), []byte("// já existe"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _ := runMain(t, "dataset", "new", "ds_x", "--project", proj, "--json")
	if code != output.ExitUsage {
		t.Errorf("exit=%d, quer %d", code, output.ExitUsage)
	}
}

func TestEventNew(t *testing.T) {
	proj := t.TempDir()
	code, _ := runMain(t, "event", "new", "displayCustomThemes", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	got, err := os.ReadFile(filepath.Join(proj, "events", "displayCustomThemes.js"))
	if err != nil {
		t.Fatal(err)
	}
	// O nome do evento vira o nome da função (camelCase é aceito).
	if !strings.Contains(string(got), "function displayCustomThemes()") {
		t.Errorf("evento sem a função nomeada: %s", got)
	}
}

func TestMechanismNew(t *testing.T) {
	proj := t.TempDir()
	code, _ := runMain(t, "mechanism", "new", "mec_gestor", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d", code)
	}
	got, err := os.ReadFile(filepath.Join(proj, "mechanisms", "mec_gestor.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "function resolve(process, colleague)") {
		t.Errorf("mecanismo sem o resolve: %s", got)
	}
}

func TestFormNew(t *testing.T) {
	proj := t.TempDir()
	code, stdout := runMain(t, "form", "new", "frm_pedido", "--title", "Pedido de Compra", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["form"] != "frm_pedido" || data["dir"] != "forms/frm_pedido" {
		t.Errorf("data inesperado: %+v", data)
	}
	for _, rel := range []string{
		"forms/frm_pedido/frm_pedido.html",
		"forms/frm_pedido/events/displayFields.js",
		"forms/frm_pedido/events/validateForm.js",
	} {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(rel))); err != nil {
			t.Errorf("arquivo esperado ausente: %s (%v)", rel, err)
		}
	}
	html, _ := os.ReadFile(filepath.Join(proj, "forms", "frm_pedido", "frm_pedido.html"))
	if !strings.Contains(string(html), "Pedido de Compra") {
		t.Errorf("HTML sem o título: %s", html)
	}

	// Pasta existente é erro de uso.
	if code, _ := runMain(t, "form", "new", "frm_pedido", "--project", proj, "--json"); code != output.ExitUsage {
		t.Errorf("repetição: exit=%d, quer %d", code, output.ExitUsage)
	}
}

func TestWorkflowNewScript(t *testing.T) {
	proj := t.TempDir()
	// Evento em caixa diferente resolve para a forma canônica do catálogo.
	code, stdout := runMain(t, "workflow", "new-script", "compras_requisicao", "beforetasksave", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["event"] != "beforeTaskSave" || data["file"] != "workflow/scripts/compras_requisicao.beforeTaskSave.js" {
		t.Errorf("data inesperado: %+v", data)
	}
	got, err := os.ReadFile(filepath.Join(proj, "workflow", "scripts", "compras_requisicao.beforeTaskSave.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "function beforeTaskSave(colleagueId, nextSequenceId, userList)") {
		t.Errorf("script sem a assinatura: %s", got)
	}

	// Evento fora do catálogo = erro de uso listando os válidos.
	code, _ = runMain(t, "workflow", "new-script", "compras_requisicao", "naoExiste", "--project", proj, "--json")
	if code != output.ExitUsage {
		t.Errorf("evento desconhecido: exit=%d, quer %d", code, output.ExitUsage)
	}
	// Script já existente = erro de uso, sem sobrescrever.
	code, _ = runMain(t, "workflow", "new-script", "compras_requisicao", "beforeTaskSave", "--project", proj, "--json")
	if code != output.ExitUsage {
		t.Errorf("repetição: exit=%d, quer %d", code, output.ExitUsage)
	}
}
