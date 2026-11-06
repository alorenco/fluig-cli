package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func writeSpec(t *testing.T, v any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.json")
	b, _ := json.Marshal(v)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServiceSpecValidate(t *testing.T) {
	cases := map[string]serviceSpec{
		"sem código":        {URL: "https://x", Auth: "none"},
		"url ruim":          {Code: "a", URL: "x", Auth: "none"},
		"oauth":             {Code: "a", URL: "https://x", Auth: "oauth2"},
		"basic sem usuário": {Code: "a", URL: "https://x", Auth: "basic", SecretEnv: "X"},
		"custom sem env":    {Code: "a", URL: "https://x", Auth: "custom"},
		"env em none":       {Code: "a", URL: "https://x", Auth: "none", SecretEnv: "X"},
		"env com nome ruim": {Code: "a", URL: "https://x", Auth: "custom", SecretEnv: "A-B"},
		"usuário sem basic": {Code: "a", URL: "https://x", Auth: "none", Username: "u"},
	}
	for name, sp := range cases {
		if err := sp.validate(); err == nil {
			t.Errorf("%s: esperava erro", name)
		}
	}
	ok := serviceSpec{Code: "a", URL: "https://x", Auth: "custom", SecretEnv: "FLUIGCLI_SECRET_A"}
	if err := ok.validate(); err != nil {
		t.Errorf("spec válida recusada: %v", err)
	}
	if secretEnvName("sabia-agentos.v2") != "FLUIGCLI_SECRET_SABIA_AGENTOS_V2" {
		t.Errorf("nome de variável inesperado: %s", secretEnvName("sabia-agentos.v2"))
	}
}

func TestServiceSpecCampoDesconhecido(t *testing.T) {
	path := writeSpec(t, map[string]any{"code": "a", "url": "https://x", "auth": "none", "password": "x"})
	if _, err := readServiceSpec(path); err == nil || !strings.Contains(err.Error(), "password") {
		t.Errorf("campo desconhecido aceito: %v", err)
	}
}

func TestServiceApply(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	// Igual ao fixture: nada muda.
	same := writeSpec(t, serviceSpec{Code: "jusbrasil", URL: "https://op.digesto.com.br", Auth: "none"})
	code, out := runMain(t, "service", "apply", same, "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK || !strings.Contains(out, "já está igual") || stub.updated != nil {
		t.Fatalf("exit=%d updated=%v\n%s", code, stub.updated, out)
	}
	// URL diferente: update só da URL.
	diff := writeSpec(t, serviceSpec{Code: "jusbrasil", URL: "https://novo.example.com", Auth: "none"})
	if code, out := runMain(t, "service", "apply", diff, "--project", proj, "--server", "homolog", "--json"); code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if o := stub.updated["oAuthVO"].(map[string]any); o["domain"] != "https://novo.example.com" {
		t.Errorf("update inesperado: %+v", o)
	}
	// Serviço novo custom sem a variável: recusa antes de gravar.
	novo := writeSpec(t, serviceSpec{Code: "novo", URL: "https://x", Auth: "custom", SecretEnv: "ZZ_TEST_HDR"})
	if code, _ := runMain(t, "service", "apply", novo, "--project", proj, "--server", "homolog", "--json"); code != output.ExitUsage || stub.created != nil {
		t.Errorf("sem segredo: exit=%d created=%v", code, stub.created)
	}
}

func TestServiceApplySegredoCustom(t *testing.T) {
	stub := &svcStub{}
	proj := mechProject(t, stub.server(t).URL)
	spec := writeSpec(t, serviceSpec{Code: "APIInfoBipWhatsApp", URL: "https://rpxjkp.api.infobip.com", Auth: "custom", SecretEnv: "ZZ_TEST_HDR"})
	// Mesmo valor guardado (a fixture tem "Authorization: REDACTED"): nada muda.
	t.Setenv("ZZ_TEST_HDR", "Authorization: REDACTED\n")
	if code, out := runMain(t, "service", "apply", spec, "--project", proj, "--server", "homolog", "--json"); code != output.ExitOK || stub.updated != nil {
		t.Fatalf("exit=%d updated=%v\n%s", code, stub.updated, out)
	}
	t.Setenv("ZZ_TEST_HDR", "Authorization: Bearer novo")
	code, out := runMain(t, "service", "apply", spec, "--project", proj, "--server", "homolog", "--json")
	if code != output.ExitOK || strings.Contains(out, "Bearer novo") {
		t.Fatalf("exit=%d (ou vazou o segredo)\n%s", code, out)
	}
	if o := stub.updated["oAuthVO"].(map[string]any); o["custom"] != "Authorization: Bearer novo" {
		t.Errorf("segredo não gravado: %+v", o)
	}
}

func TestPageSpecValidate(t *testing.T) {
	cases := map[string]pageSpec{
		"sem título":       {Code: "a", Layout: "l"},
		"sem layout":       {Code: "a", Title: "T"},
		"pai ela mesma":    {Code: "a", Title: "T", Layout: "l", Parent: "a"},
		"autorização ruim": {Code: "a", Title: "T", Layout: "l", Authorization: "x"},
		"url com barra":    {Code: "a", Title: "T", Layout: "l", FriendlyURL: "a/b"},
		"widget sem slot":  {Code: "a", Title: "T", Layout: "l", Widgets: []pageSpecWidget{{Widget: "w"}}},
	}
	for name, sp := range cases {
		if err := sp.validate(); err == nil {
			t.Errorf("%s: esperava erro", name)
		}
	}
}

func financeiroSpec() pageSpec {
	return pageSpec{
		Code: "financeiro", Title: "Painel Financeiro", Description: "Painel Financeiro", Layout: "portais",
		EnabledChildren: true,
		Widgets:         []pageSpecWidget{{Widget: "financeiro_painel", Slot: "SlotA", Preferences: map[string]string{"titulo": "Painel"}}},
	}
}

func TestPageApplyIgual(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	path := writeSpec(t, financeiroSpec())
	code, out := runMain(t, pageArgs(proj, "page", "apply", path, "--json")...)
	if code != output.ExitOK || !strings.Contains(out, "já está igual") {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if stub.put != nil || stub.publish != 0 || stub.prefs != nil {
		t.Errorf("apply sem diferença gravou: put=%v publish=%d prefs=%v", stub.put, stub.publish, stub.prefs)
	}
}

func TestPageApplyComMudancas(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	spec := financeiroSpec()
	spec.Title = "Novo título"
	spec.Widgets[0].Preferences = map[string]string{"titulo": "Outro"}
	code, out := runMain(t, pageArgs(proj, "page", "apply", writeSpec(t, spec), "--json")...)
	if code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, out)
	}
	if stub.put["title"] != "Novo título" || stub.publish != 1 || stub.prefs["titulo"] != "Outro" {
		t.Errorf("put=%v publish=%d prefs=%v", stub.put, stub.publish, stub.prefs)
	}
}

func TestPageApplyRecusa(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	bad := financeiroSpec()
	bad.Widgets[0].Widget = "nao_existe"
	if code, _ := runMain(t, pageArgs(proj, "page", "apply", writeSpec(t, bad), "--json")...); code != output.ExitNotFound {
		t.Errorf("widget inexistente: exit=%d", code)
	}
	bad = financeiroSpec()
	bad.Widgets[0].Slot = "SlotZZ"
	if code, _ := runMain(t, pageArgs(proj, "page", "apply", writeSpec(t, bad), "--json")...); code != output.ExitUsage {
		t.Errorf("slot inválido: exit=%d", code)
	}
	stub.status = "DRAFT"
	if code, _ := runMain(t, pageArgs(proj, "page", "apply", writeSpec(t, financeiroSpec()), "--json")...); code != output.ExitUsage {
		t.Errorf("rascunho aberto: exit=%d", code)
	}
	if stub.put != nil || stub.publish != 0 {
		t.Errorf("recusa gravou: put=%v publish=%d", stub.put, stub.publish)
	}
}

func TestPageExport(t *testing.T) {
	stub := &cliPageStub{}
	proj := mechProject(t, stub.server(t).URL)
	out := filepath.Join(t.TempDir(), "p.json")
	if code, o := runMain(t, pageArgs(proj, "page", "export", "financeiro", "-o", out, "--json")...); code != output.ExitOK {
		t.Fatalf("exit=%d\n%s", code, o)
	}
	spec, err := readPageSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	// Só a widget de conteúdo (SlotA); as de sistema (SlotGlobalAlert…) ficam fora.
	if len(spec.Widgets) != 1 || spec.Widgets[0].Widget != "financeiro_painel" || spec.Widgets[0].Preferences["titulo"] != "Painel" {
		t.Errorf("export inesperado: %+v", spec.Widgets)
	}
	if spec.Layout != "portais" || len(spec.Permissions) == 0 {
		t.Errorf("export sem layout/permissões: %+v", spec)
	}
}

func TestResolveSpecPath(t *testing.T) {
	root := t.TempDir()
	app := &App{Project: root}
	casos := map[string]string{
		"sabia_agentos":         filepath.Join(root, "services", "sabia_agentos.json"),
		"services/x.json":       "services/x.json",
		"outro.JSON":            "outro.JSON",
		filepath.Join("a", "b"): filepath.Join("a", "b"),
	}
	for arg, want := range casos {
		got, err := app.resolveSpecPath(arg, project.ServicesDirName)
		if err != nil || got != want {
			t.Errorf("resolveSpecPath(%q) = %q, %v; esperava %q", arg, got, err, want)
		}
	}
	got, _ := app.resolveSpecPath("painel", project.PagesDir)
	if want := filepath.Join(root, "wcm", "page", "painel.json"); got != want {
		t.Errorf("página: %q; esperava %q", got, want)
	}
}
