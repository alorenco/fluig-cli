package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/config"
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

// Sugestão pelo nome do DATASET do formulário (ROADMAP §5.3): a pasta usa o
// nome técnico, que não parece com o nome visível no servidor.
func TestSuggestFormLinksPorDatasetName(t *testing.T) {
	fmap, err := project.LoadFormMap(t.TempDir(), "prod:443/1")
	if err != nil {
		t.Fatal(err)
	}
	forms := []fluig.Form{
		{DocumentID: 9, Description: "Adiantamento ao Fornecedor", DatasetName: "frm_fin_adiantamento_pagar"},
	}
	got := suggestFormLinks([]string{"frm_fin_adiantamento_pagar"}, forms, fmap)
	if len(got) != 1 || got[0].Form.DocumentID != 9 {
		t.Fatalf("sugestão = %+v", got)
	}
	if got[0].Source != "nome do dataset" {
		t.Errorf("fonte = %q, quer %q", got[0].Source, "nome do dataset")
	}
}

// form link <pasta> --document-id: vincula sem prompt, aceita --json e é
// idempotente na segunda execução (ROADMAP §5.3).
func TestFormLinkAlvoExplicito(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	if err := os.MkdirAll(filepath.Join(proj, "forms", "frm_fin_adiantamento_pagar"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, out := runMain(t, "form", "link", "frm_fin_adiantamento_pagar", "--document-id", "42",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("exit=%d out=%s", code, out)
	}
	var env struct {
		Data struct {
			Linked     []string `json:"linked"`
			Action     string   `json:"action"`
			DocumentID int      `json:"documentId"`
			Name       string   `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, out)
	}
	if env.Data.Action != "linked" || env.Data.DocumentID != 42 || env.Data.Name != "Formulario de Teste" {
		t.Errorf("data = %+v", env.Data)
	}
	if len(env.Data.Linked) != 1 || env.Data.Linked[0] != "frm_fin_adiantamento_pagar" {
		t.Errorf("linked = %v", env.Data.Linked)
	}
	mapData, err := os.ReadFile(filepath.Join(proj, ".fluigcli", "forms.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mapData), `"folder": "frm_fin_adiantamento_pagar"`) ||
		!strings.Contains(string(mapData), `"documentId": 42`) {
		t.Errorf("vínculo não gravado:\n%s", mapData)
	}

	// Repetir o mesmo comando não é erro nem grava de novo.
	code, out = runMain(t, "form", "link", "frm_fin_adiantamento_pagar", "--document-id", "42",
		"--json", "--project", proj, "--server", "homolog")
	if code != output.ExitOK {
		t.Fatalf("2ª rodada: exit=%d out=%s", code, out)
	}
	env.Data.Action = ""
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Action != "unchanged" {
		t.Errorf("2ª rodada: action=%q, quer unchanged", env.Data.Action)
	}
}

// O alvo é conferido na listagem do servidor e o mapa não fica ambíguo.
func TestFormLinkAlvoExplicitoGuards(t *testing.T) {
	stub := &formStub{}
	proj := formProject(t, stub.server(t).URL)
	for _, dir := range []string{"frm_a", "frm_b"} {
		if err := os.MkdirAll(filepath.Join(proj, "forms", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	base := []string{"--project", proj, "--server", "homolog", "--json"}
	run := func(args ...string) (int, string) {
		return runMain(t, append(append([]string{"form", "link"}, args...), base...)...)
	}

	// --document-id sem a pasta → uso incorreto.
	if code, _ := run("--document-id", "42"); code != output.ExitUsage {
		t.Errorf("sem pasta: exit=%d, quer %d", code, output.ExitUsage)
	}
	// --document-id junto com --name → uso incorreto (ambiguidade silenciosa).
	if code, _ := run("frm_a", "--document-id", "42", "--name", "Formulario de Teste"); code != output.ExitUsage {
		t.Errorf("duas flags de alvo: exit=%d, quer %d", code, output.ExitUsage)
	}
	// Pasta local inexistente → não encontrado.
	if code, _ := run("frm_zzz", "--document-id", "42"); code != output.ExitNotFound {
		t.Errorf("pasta inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	// documentId que não existe no servidor → não encontrado (nada é gravado).
	if code, _ := run("frm_a", "--document-id", "999"); code != output.ExitNotFound {
		t.Errorf("documentId inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	if _, err := os.Stat(filepath.Join(proj, ".fluigcli", "forms.json")); !os.IsNotExist(err) {
		t.Errorf("forms.json foi criado por um alvo inválido")
	}
	// --name inexistente → não encontrado com sugestão do nome próximo.
	code, out := run("frm_a", "--name", "Formulario de Test")
	if code != output.ExitNotFound {
		t.Errorf("--name inexistente: exit=%d, quer %d", code, output.ExitNotFound)
	}
	if !strings.Contains(out, "Formulario de Teste") {
		t.Errorf("faltou a sugestão do nome próximo: %s", out)
	}

	// Vincula frm_a e tenta apontar o MESMO formulário para frm_b: recusa.
	if code, out := run("frm_a", "--document-id", "42"); code != output.ExitOK {
		t.Fatalf("link inicial: exit=%d out=%s", code, out)
	}
	code, out = run("frm_b", "--document-id", "42")
	if code != output.ExitUsage {
		t.Errorf("formulário já vinculado: exit=%d, quer %d", code, output.ExitUsage)
	}
	if !strings.Contains(out, "frm_a") || !strings.Contains(out, "--force") {
		t.Errorf("a recusa deve citar a pasta atual e o --force: %s", out)
	}
	// Com --force o vínculo MOVE: frm_b fica com o formulário e frm_a sem.
	if code, out := run("frm_b", "--document-id", "42", "--force"); code != output.ExitOK {
		t.Fatalf("--force: exit=%d out=%s", code, out)
	}
	fmap, err := project.LoadFormMap(proj, formScopeDe(t, proj))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fmap.ByFolder("frm_a"); ok {
		t.Errorf("frm_a continuou vinculada após o --force")
	}
	if l, ok := fmap.ByFolder("frm_b"); !ok || l.DocumentID != 42 {
		t.Errorf("frm_b não recebeu o vínculo: %+v", l)
	}
}

// formScopeDe devolve a chave do bucket do forms.json (host:porta/companyId)
// do servidor "homolog" cadastrado no projeto de teste.
func formScopeDe(t *testing.T, proj string) string {
	t.Helper()
	s, err := config.NewStore(proj).Get("homolog")
	if err != nil {
		t.Fatal(err)
	}
	return s.FormScopeKey()
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
