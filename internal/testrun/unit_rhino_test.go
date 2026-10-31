//go:build rhino

// Teste com o motor REAL: exige Java 11+ e o jar do Rhino (cache, download ou
// FLUIGCLI_RHINO_JAR). Rode com:
//
//	go test -tags=rhino ./internal/testrun/ -run Rhino
package testrun

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/rhino"
)

func realExecutor(t *testing.T) UnitExecutor {
	t.Helper()
	ctx := context.Background()
	java, err := rhino.FindJava(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	jar, err := rhino.EnsureJar(ctx, os.Getenv("FLUIGCLI_RHINO_JAR"))
	if err != nil {
		t.Fatal(err)
	}
	return func(ctx context.Context, boot, plan string) error {
		_, stderr, err := rhino.Run(ctx, java, jar, boot, plan)
		if err != nil {
			t.Logf("stderr: %s", stderr)
		}
		return err
	}
}

func TestRhinoSelfCheck(t *testing.T) {
	rep, err := RunUnit(context.Background(), []*UnitFile{SelfCheckFile()}, realExecutor(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rep.Files[0].Cases {
		if c.Status != UnitPassed {
			t.Errorf("sonda %q: %s %v", c.Name, c.Error, failed(UnitCaseResult{Assertions: c.Assertions}))
		}
	}
}

const rhinoScript = `
function loopConst() { var o = ''; for (var i = 0; i < 3; i++) { const x = i * 10; o += x + ';'; } return o; }
function vazio() { var s = getValue('WKVazio'); return s ? 'truthy' : 'falsy'; }
function linhaPorNome() { var d = DatasetFactory.getDataset('ds', null, null, null); return d.values[0].B; }
function linhaPorIndice() { var d = DatasetFactory.getDataset('ds', null, null, null); return [d.values.length, d.values[0][1], d.getValue(0, 'B'), d.getValue(0, 'ZZ')]; }
function hoje() { var d = new Date(); return [d.getFullYear(), d.getMonth() + 1, d.getDate(), new Date(2020, 0, 2).getFullYear(), d instanceof Date]; }
function semFalso() { return getCardValue('x'); }
function comentario() { hAPI.setTaskComments('jsilva', 7, 0, 'ok'); hAPI.setCardValue('status', 'A'); return hAPI.getCardValue('status'); }
function validar(form) { if (!form.getValue('fornecedor')) { throw "<b>Campo obrigatório:</b><br>- Fornecedor"; } }
function mobile() { var m = getValue('WKMobile'); var r = []; if (m != null && m == true) { r.push('app'); } if (m == null || m == false) { r.push('web'); } return r.concat([typeof getValue('WKNumState'), getValue('WKNumState') == 23]); }
function usuario() { return fluigAPI.getUserService().getCurrent().getLogin(); }
function mostrar(form, customHTML) { customHTML.append('<script>var x = 1;</script>'); form.setVisibleById('div1', false); }
`

func TestRhinoFalsosEDefeitos(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "s.js"), []byte(rhinoScript), 0o644)
	os.WriteFile(filepath.Join(root, "ruim.js"), []byte("function f() { if (true) { const y = 1; } else { const y = 2; } }\n"), 0o644)
	f, err := ParseUnitFile(filepath.Join(root, "x.unit.json"), root, []byte(`{
	  "scripts": ["s.js"],
	  "defaults": {"today": "2026-11-03T10:00", "user": {"login": "jsilva"},
	    "datasets": {"ds": {"columns": ["A", "B"], "rows": [["1", "2"]]}}},
	  "cases": [
	    {"name": "const em laço congela", "call": "loopConst", "expect": {"return": "0;0;0;"}},
	    {"name": "String Java vazia truthy", "call": "vazio", "wk": {"WKVazio": ""}, "expect": {"return": "truthy"}},
	    {"name": "linha por nome quebra", "call": "linhaPorNome", "expect": {"throws": "has no public instance field or method named \"B\""}},
	    {"name": "linha por índice", "call": "linhaPorIndice", "expect": {"return": [1, "2", "2", null], "returnType": "array"}},
	    {"name": "relógio fixo", "call": "hoje", "expect": {"return": [2026, 11, 3, 2020, true]}},
	    {"name": "global sem falso", "call": "semFalso", "expect": {"return": null}},
	    {"name": "hAPI grava", "call": "comentario", "expect": {"return": "A", "returnType": "java-string", "card": {"status": "A"},
	      "calls": [{"method": "hAPI.setTaskComments", "args": ["jsilva", 7, 0, "ok"]}]}},
	    {"name": "validateForm lança HTML", "call": "validar", "args": ["$form"], "expect": {"throws": "Campo obrigatório: - Fornecedor"}},
	    {"name": "usuário", "call": "usuario", "expect": {"return": "jsilva"}},
	    {"name": "WKMobile true", "call": "mobile", "wk": {"WKMobile": true, "WKNumState": 23}, "expect": {"return": ["app", "object", true]}},
	    {"name": "WKMobile false", "call": "mobile", "wk": {"WKMobile": false, "WKNumState": 23}, "expect": {"return": ["web", "object", true]}},
	    {"name": "WKMobile ausente", "call": "mobile", "wk": {"WKNumState": 23}, "expect": {"return": ["web", "object", true]}},
	    {"name": "displayFields", "event": "displayFields", "scripts": ["s.js"], "expect": {"html": ["var x = 1"],
	      "calls": [{"method": "form.setVisibleById", "args": ["div1", false]}]}},
	    {"name": "const irmão não compila", "call": "f", "scripts": ["ruim.js"], "expect": {"throws": true}}
	  ]}`))
	if err != nil {
		t.Fatal(err)
	}
	// displayFields: o caso chama a função "displayFields"; o script tem "mostrar".
	for _, c := range f.Cases {
		if c.Name == "displayFields" {
			c.Call = "mostrar"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rep, err := RunUnit(ctx, []*UnitFile{f}, realExecutor(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"global sem falso": UnitFailed, "const irmão não compila": UnitErrored}
	for _, c := range rep.Files[0].Cases {
		st := want[c.Name]
		if st == "" {
			st = UnitPassed
		}
		if c.Status != st {
			t.Errorf("%q: %s (quer %s) %s %v", c.Name, c.Status, st, c.Error, failed(UnitCaseResult{Assertions: c.Assertions}))
		}
		switch c.Name {
		case "global sem falso":
			if msg := strings.Join(failed(UnitCaseResult{Assertions: c.Assertions}), ""); !strings.Contains(msg, "a global getCardValue não existe no teste") {
				t.Errorf("dica da global: %s", msg)
			}
		case "const irmão não compila":
			if !strings.Contains(c.Error, "redeclaration of const y") || !strings.Contains(c.Error, "ruim.js") {
				t.Errorf("erro de compilação: %s", c.Error)
			}
		}
	}
}
