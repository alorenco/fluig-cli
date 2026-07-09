package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func TestSuggestFormLinks(t *testing.T) {
	root := t.TempDir()
	// Bucket de OUTRO servidor: pasta_x já vinculada ao "Form A" lá.
	other, err := project.LoadFormMap(root, "hml:8080/1")
	if err != nil {
		t.Fatal(err)
	}
	other.Upsert(project.FormLink{Folder: "pasta_x", DocumentID: 77, Name: "Form A"})
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	fmap, err := project.LoadFormMap(root, "prod:443/1")
	if err != nil {
		t.Fatal(err)
	}

	forms := []fluig.Form{
		{DocumentID: 1, Description: "Form A"},
		{DocumentID: 2, Description: "frm_b"},
		{DocumentID: 3, Description: "Duplicado"},
		{DocumentID: 4, Description: "Duplicado"},
	}
	folders := []string{"pasta_x", "frm_b", "form a", "duplicado", "sem_par"}
	got := suggestFormLinks(folders, forms, fmap)

	want := map[string]int{ // pasta → documentId sugerido (0 = sem sugestão)
		"pasta_x":   1, // nome vindo do bucket do outro servidor
		"frm_b":     2, // nome exato
		"form a":    0, // case-insensitive casaria com Form A, mas já sugerido para pasta_x
		"duplicado": 0, // dois forms com o mesmo nome → ambíguo
		"sem_par":   0,
	}
	for _, s := range got {
		if s.Form.DocumentID != want[s.Folder] {
			t.Errorf("%s: sugerido documentId %d, quer %d (fonte %q)", s.Folder, s.Form.DocumentID, want[s.Folder], s.Source)
		}
	}
	// A fonte da sugestão cruzada cita o servidor de origem.
	if got[0].Source != "vínculo em hml:8080/1" {
		t.Errorf("fonte da sugestão cruzada: %q", got[0].Source)
	}
}

func TestFormLinkAuto(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	for _, dir := range []string{"Formulario de Teste", "frm_sem_match"} {
		if err := os.MkdirAll(filepath.Join(proj, "forms", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	code, out := runMain(t, "form", "link", "--auto", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d out=%s", code, out)
	}
	var env struct {
		Data struct {
			Linked  []string `json:"linked"`
			Skipped []string `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, out)
	}
	if len(env.Data.Linked) != 1 || env.Data.Linked[0] != "Formulario de Teste" {
		t.Errorf("linked = %v", env.Data.Linked)
	}
	if len(env.Data.Skipped) != 1 || env.Data.Skipped[0] != "frm_sem_match" {
		t.Errorf("skipped = %v", env.Data.Skipped)
	}
	// O vínculo foi persistido no bucket do servidor (schema v2).
	mapData, err := os.ReadFile(filepath.Join(proj, ".fluigcli", "forms.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mapData), `"version": "2.0.0"`) ||
		!strings.Contains(string(mapData), `"documentId": 42`) {
		t.Errorf("forms.json não gravado no schema v2:\n%s", mapData)
	}

	// Rodar de novo: a pasta já vinculada não é reprocessada; só a sem match
	// continua pendente.
	code, out = runMain(t, "form", "link", "--auto", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("segunda rodada: exit=%d out=%s", code, out)
	}
	env.Data.Linked, env.Data.Skipped = nil, nil
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Linked) != 0 || len(env.Data.Skipped) != 1 {
		t.Errorf("segunda rodada: linked=%v skipped=%v", env.Data.Linked, env.Data.Skipped)
	}
}

func TestFormLinkGuards(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	// --json sem --auto é recusado (interativo não tem envelope).
	code, _ := runMain(t, "form", "link", "--json", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("--json interativo: exit=%d, quer %d", code, output.ExitUsage)
	}
	// Sem TTY e sem --auto também.
	code, _ = runMain(t, "form", "link", "--project", proj, "--server", "homolog")
	if code != output.ExitUsage {
		t.Errorf("sem TTY: exit=%d, quer %d", code, output.ExitUsage)
	}
}
