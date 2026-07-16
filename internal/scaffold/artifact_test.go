package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestValidateArtifactName(t *testing.T) {
	valid := []string{"ds_clientes", "displayCustomThemes", "MecGestor", "a", "x1_Y2"}
	for _, n := range valid {
		if err := ValidateArtifactName(n); err != nil {
			t.Errorf("nome %q deveria ser válido: %v", n, err)
		}
	}
	invalid := []string{"", "1abc", "com-hifen", "com espaço", "acentuadá", "_priv", strings.Repeat("a", 101)}
	for _, n := range invalid {
		if err := ValidateArtifactName(n); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("nome %q deveria ser inválido, err=%v", n, err)
		}
	}
}

// O catálogo fica em ordem alfabética (é a ordem do help) e a busca é
// case-insensitive, devolvendo a forma canônica.
func TestProcessEventCatalog(t *testing.T) {
	names := ProcessEventNames()
	if !sort.StringsAreSorted(names) {
		t.Errorf("catálogo fora de ordem alfabética: %v", names)
	}
	ev, ok := FindProcessEvent("beforetasksave")
	if !ok || ev.Name != "beforeTaskSave" {
		t.Fatalf("busca case-insensitive falhou: %+v ok=%v", ev, ok)
	}
	// Assinatura confirmada em export real da homologação.
	if ev.Params != "colleagueId, nextSequenceId, userList" {
		t.Errorf("assinatura do beforeTaskSave: %q", ev.Params)
	}
	if _, ok := FindProcessEvent("naoExiste"); ok {
		t.Error("evento inexistente não deveria ser encontrado")
	}
}

func TestCreateDatasetFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "datasets", "ds_teste.js")
	if err := CreateDatasetFile(path, "ds_teste"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{
		"Dataset customizado: ds_teste",
		"function defineStructure()",
		"function createDataset(fields, constraints, sortFields)",
		"fluigcli dataset export datasets/ds_teste.js --new",
	} {
		if !strings.Contains(string(got), frag) {
			t.Errorf("esqueleto sem o fragmento %q", frag)
		}
	}

	// Repetir é ErrFileExists (nunca sobrescreve).
	if err := CreateDatasetFile(path, "ds_teste"); !errors.Is(err, ErrFileExists) {
		t.Errorf("arquivo existente: err=%v", err)
	}
	// Nome inválido falha antes de tocar o disco.
	if err := CreateDatasetFile(filepath.Join(t.TempDir(), "x.js"), "não válido"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("nome inválido: err=%v", err)
	}
}

// O nome do evento global vira o nome da função.
func TestCreateGlobalEventFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "displayCustomThemes.js")
	if err := CreateGlobalEventFile(path, "displayCustomThemes"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "function displayCustomThemes()") {
		t.Errorf("evento sem a função nomeada: %s", got)
	}
}

func TestCreateMechanismFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mec_teste.js")
	if err := CreateMechanismFile(path, "mec_teste"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	for _, frag := range []string{
		"function resolve(process, colleague)",
		"java.util.ArrayList",
		"fluigcli mechanism export mechanisms/mec_teste.js",
	} {
		if !strings.Contains(string(got), frag) {
			t.Errorf("mecanismo sem o fragmento %q", frag)
		}
	}
}

func TestCreateProcessScriptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compras.beforeTaskSave.js")
	ev, err := CreateProcessScriptFile(path, "compras", "beforeTaskSave")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Name != "beforeTaskSave" {
		t.Errorf("evento canônico: %q", ev.Name)
	}
	got, _ := os.ReadFile(path)
	for _, frag := range []string{
		"function beforeTaskSave(colleagueId, nextSequenceId, userList)",
		"Processo: compras — evento beforeTaskSave",
		"hAPI",
	} {
		if !strings.Contains(string(got), frag) {
			t.Errorf("script sem o fragmento %q", frag)
		}
	}

	// Evento fora do catálogo e id de processo inválido são erros claros.
	if _, err := CreateProcessScriptFile(filepath.Join(dir, "x.js"), "compras", "naoExiste"); !errors.Is(err, ErrUnknownEvent) {
		t.Errorf("evento desconhecido: err=%v", err)
	}
	if _, err := CreateProcessScriptFile(filepath.Join(dir, "y.js"), "termina.", "beforeTaskSave"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("processo com ponto final: err=%v", err)
	}
	// Ponto INTERMEDIÁRIO é válido (o separador do nome é o último ponto).
	if _, err := CreateProcessScriptFile(filepath.Join(dir, "a.b.onNotify.js"), "a.b", "onNotify"); err != nil {
		t.Errorf("ponto intermediário deveria valer: %v", err)
	}
}

func TestCreateFormDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "frm_pedido")
	files, err := CreateFormDir(dir, "frm_pedido", "Pedido de Compra")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[filepath.ToSlash(f)] = true
	}
	for _, w := range []string{"frm_pedido.html", "events/displayFields.js", "events/validateForm.js"} {
		if !got[w] {
			t.Errorf("arquivo esperado não gerado: %s (gerados: %v)", w, files)
		}
	}
	if len(files) != 3 {
		t.Errorf("gerados %d arquivos, esperava 3: %v", len(files), files)
	}

	html, err := os.ReadFile(filepath.Join(dir, "frm_pedido.html"))
	if err != nil {
		t.Fatal(err)
	}
	// A tag <form> é exigência do form export --new; o título entra no HTML.
	for _, frag := range []string{`<form name="frm_pedido"`, "Pedido de Compra"} {
		if !strings.Contains(string(html), frag) {
			t.Errorf("HTML sem o fragmento %q", frag)
		}
	}
	ev, _ := os.ReadFile(filepath.Join(dir, "events", "displayFields.js"))
	if !strings.Contains(string(ev), "function displayFields(form, customHTML)") {
		t.Errorf("displayFields sem a assinatura: %s", ev)
	}

	// Pasta existente = ErrDirExists.
	if _, err := CreateFormDir(dir, "frm_pedido", ""); !errors.Is(err, ErrDirExists) {
		t.Errorf("pasta existente: err=%v", err)
	}
}
