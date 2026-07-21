package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// O catálogo extraído do fluig.d.ts embutido tem os objetos e membros
// conhecidos — se o parse regredir (ou o fork for corrompido), falha aqui.
func TestAPICatalogExtraido(t *testing.T) {
	cat := apiCatalog()
	checks := map[string][]string{
		"hAPI":           {"getCardValue", "setCardValue", "getChildrenIndexes", "setAutomaticDecision"},
		"form":           {"getCompanyId", "getValue", "setVisibleById", "getChildrenIndexes"},
		"FLUIGC":         {"autocomplete", "toast", "loading", "modal", "message", "calendar"},
		"FLUIGC.message": {"alert", "confirm"},
		"DatasetFactory": {"getAvailableDatasets", "createConstraint", "getDataset"},
		"DatasetBuilder": {"newDataset"},
		"docAPI":         {"copyDocumentToUploadArea"},
		"WCMAPI":         {"getServerURL", "Create", "logoff"},
		"customHTML":     {"append"},
	}
	for obj, members := range checks {
		if !cat.KnownObject(obj) {
			t.Errorf("objeto %s ausente do catálogo", obj)
			continue
		}
		for _, m := range members {
			if !cat.HasMember(obj, m) {
				t.Errorf("%s.%s ausente do catálogo", obj, m)
			}
		}
	}
	if len(cat.members["hAPI"]) < 15 || len(cat.members["form"]) < 20 {
		t.Errorf("catálogo magro: hAPI=%d form=%d", len(cat.members["hAPI"]), len(cat.members["form"]))
	}
	for _, wk := range []string{"WKUser", "WKNumState", "WKCompany", "WKDocument", "WKDef"} {
		if !cat.HasWKVar(wk) {
			t.Errorf("variável %s ausente do catálogo", wk)
		}
	}
	if cat.HasWKVar("WKInventada") {
		t.Error("WKInventada não deveria existir")
	}
	if got := cat.NearestMember("hAPI", "setCardValu"); got != "setCardValue" {
		t.Errorf("vizinho de setCardValu: %q", got)
	}
	if got := cat.NearestWKVar("WKNumStat"); got != "WKNumState" {
		t.Errorf("vizinho de WKNumStat: %q", got)
	}
}

// projeto de teste com violações (e não-violações) das regras FL*.
func flProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("workflow/scripts/proc.beforeTaskSave.js", `function beforeTaskSave(colleagueId, nextSequenceId, userList) {
    hAPI.setCardValue("campo", "ok");
    hAPI.setCardValu("campo", "typo");
    var st = getValue("WKNumState");
    var typo = getValue("WKNumStat");
    var campo = getValue("meu_campo_do_form");
}`)
	write("forms/MeuForm/events/displayFields.js", `function displayFields(form, customHTML) {
    form.setVisibleById("secao", true);
    form.setVisibleByID("secao", false);
    alert("nao sou regra de navegador aqui");
    customHTML.append("<b>x</b>");
}`)
	write("forms/MeuForm/MeuForm.html", `<html><body><form name="f">
<script>
DatasetFactory.getDataset("colleague", null, null, null);
DatasetFactory.getDatased("colleague", null, null, null);
FLUIGC.message.alert({message: "oi"});
FLUIGC.mesage.alert({message: "typo"});
FLUIGC.toast({message: "ok"});
</script>
</form></body></html>`)
	write("datasets/ds_meu.js", `function createDataset(fields, constraints, sortFields) {
    var ds = DatasetBuilder.newDataset();
    var errado = DatasetBuilder.newDatasets();
    return ds;
}`)
	write("wcm/widget/minha/src/main/webapp/resources/js/minha.js", `var MinhaWidget = SuperWidget.extend({
    init: function() {
        var url = WCMAPI.getServerURL();
        var nome = WCMAPI.user.split(" ")[0];
        var form = document.querySelector("form");
        form.submit();
    }
});`)
	return root
}

func TestRunRegrasFL(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(flProject(t), nil, cat, Config{})
	if err != nil {
		t.Fatal(err)
	}
	byRule := map[string][]Finding{}
	for _, f := range res.Findings {
		byRule[f.Rule] = append(byRule[f.Rule], f)
	}

	// FL001: só o setCardValu; com a sugestão do vizinho.
	if n := len(byRule[RuleUnknownHAPI]); n != 1 {
		t.Fatalf("FL001: %d achado(s), quero 1\n%v", n, res.Findings)
	}
	if f := byRule[RuleUnknownHAPI][0]; !strings.Contains(f.Suggestion, "setCardValue") || f.Severity != SeverityWarning {
		t.Errorf("FL001 sem sugestão/severidade: %+v", f)
	}

	// FL002: só o WKNumStat (WKNumState ok; campo sem prefixo WK fora).
	if n := len(byRule[RuleUnknownWKVar]); n != 1 {
		t.Fatalf("FL002: %d achado(s), quero 1\n%v", n, res.Findings)
	}
	if f := byRule[RuleUnknownWKVar][0]; !strings.Contains(f.Suggestion, "WKNumState") {
		t.Errorf("FL002 sem sugestão: %+v", f)
	}

	// FL003: só o setVisibleByID do evento; o form.submit() da widget NÃO conta.
	if n := len(byRule[RuleUnknownFormAPI]); n != 1 {
		t.Fatalf("FL003: %d achado(s), quero 1\n%v", n, res.Findings)
	}
	if f := byRule[RuleUnknownFormAPI][0]; !strings.Contains(f.Suggestion, "setVisibleById") ||
		!strings.HasPrefix(f.File, "forms/") {
		t.Errorf("FL003 fora do lugar: %+v", f)
	}

	// FL004: getDatased + FLUIGC.mesage + newDatasets = 3.
	if n := len(byRule[RuleUnknownAPI]); n != 3 {
		t.Fatalf("FL004: %d achado(s), quero 3\n%v", n, res.Findings)
	}
	for _, f := range byRule[RuleUnknownAPI] {
		if strings.Contains(f.Message, "getDatased") && !strings.Contains(f.Suggestion, "getDataset") {
			t.Errorf("FL004 typo de dataset sem vizinho: %+v", f)
		}
	}

	// SG007 não roda em JS server-side: o alert() do displayFields não conta;
	// nenhum achado SG007 no projeto (a widget não tem diálogo nativo).
	for _, f := range byRule[RuleNativeDialog] {
		t.Errorf("SG007 em código server-side: %+v", f)
	}
}

// As pastas server-side entram nos alvos default e a severidade FL respeita o
// override do audit.json (off desliga).
func TestRunFLSeverityOff(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	root := flProject(t)
	cfg := Config{Severity: map[string]string{
		RuleUnknownHAPI: "off", RuleUnknownWKVar: "off",
		RuleUnknownFormAPI: "off", RuleUnknownAPI: "error",
	}}
	res, err := Run(root, nil, cat, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		switch f.Rule {
		case RuleUnknownHAPI, RuleUnknownWKVar, RuleUnknownFormAPI:
			t.Errorf("regra desligada apareceu: %+v", f)
		case RuleUnknownAPI:
			if f.Severity != SeverityError {
				t.Errorf("FL004 deveria ter virado erro: %+v", f)
			}
		}
	}
}
