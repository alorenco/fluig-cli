package testrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unitProj cria uma raiz de projeto com um script e devolve a raiz.
func unitProj(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "forms", "Compras", "events")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "validateForm.js"), []byte("function validateForm(form) {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "inputFields.js"), []byte("function inputFields(form) {}\nfunction calc(a) { return a; }\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "tests", "unit", "fixtures"), 0o755)
	os.WriteFile(filepath.Join(root, "tests", "unit", "fixtures", "card.json"),
		[]byte(`{"abertura": "{{today}}", "solicitante": "{{actors.eu.login}}", "valor": 10}`), 0o644)
	return root
}

func parseUnit(t *testing.T, root, body string) (*UnitFile, error) {
	t.Helper()
	return ParseUnitFile(filepath.Join(root, "tests", "unit", "x.unit.json"), root, []byte(body))
}

func TestUnitParseMesclaDefaults(t *testing.T) {
	root := unitProj(t)
	f, err := parseUnit(t, root, `{
	  "scripts": ["forms/Compras/events/inputFields.js"],
	  "defaults": {
	    "today": "2026-11-03",
	    "wk": {"WKNumState": "4", "WKNextState": "5"},
	    "user": {"login": "jsilva", "code": "jsilva"},
	    "form": {"card": "fixtures/card.json", "children": {"itens": [1, 2]}},
	    "datasets": {"ds_a": null}
	  },
	  "cases": [
	    {"name": "evento", "event": "validateForm", "scripts": ["forms/Compras/events/validateForm.js"],
	     "wk": {"WKNumState": "9"}, "form": {"fields": {"valor": "20"}},
	     "expect": {"throws": "obrigatório"}},
	    {"name": "função", "call": "calc", "args": ["$form", 3, {"$java": "x"}],
	     "today": "2026-12-25T08:30", "datasets": {"ds_b": {"columns": ["A"], "rows": [["1"]]}},
	     "expect": {"return": "ok", "returnType": "js-string", "form": {"x": "1", "y": null, "$only": true}}}
	  ]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	ev, fn := f.Cases[0], f.Cases[1]
	if ev.Call != "validateForm" || len(ev.Args) != 1 || string(ev.Args[0]) != `"$form"` || ev.Scripts[0].Name != "forms/Compras/events/validateForm.js" {
		t.Errorf("evento de formulário: %+v", ev)
	}
	if string(ev.Setup.WK["WKNumState"]) != `"9"` || string(ev.Setup.WK["WKNextState"]) != `"5"` {
		t.Errorf("wk mescla chave a chave: %v", ev.Setup.WK)
	}
	// A fixture resolve {{today}} com o today do caso e {{actors.eu.login}} com o user.
	if ev.Setup.Fields["abertura"] != "03/11/2026" || ev.Setup.Fields["solicitante"] != "jsilva" || ev.Setup.Fields["valor"] != "20" {
		t.Errorf("card: %v", ev.Setup.Fields)
	}
	if fn.Setup.Fields["abertura"] != "25/12/2026" || strings.Join(intsToStr(fn.Setup.Today), ",") != "2026,12,25,8,30,0" {
		t.Errorf("today do caso: %v %v", fn.Setup.Fields["abertura"], fn.Setup.Today)
	}
	if len(fn.Setup.Datasets) != 2 || len(fn.Setup.Children["itens"]) != 2 {
		t.Errorf("datasets e children dos defaults: %v %v", fn.Setup.Datasets, fn.Setup.Children)
	}
	e := fn.Expect
	if !e.HasReturn || e.ReturnType != "js-string" || !e.FormOnly || e.Form["y"] != nil || *e.Form["x"] != "1" {
		t.Errorf("expect: %+v", e)
	}
}

func intsToStr(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		b, _ := json.Marshal(x)
		out[i] = string(b)
	}
	return out
}

func TestUnitParseErros(t *testing.T) {
	root := unitProj(t)
	base := func(caso string) string {
		return `{"scripts": ["forms/Compras/events/inputFields.js"], "cases": [` + caso + `]}`
	}
	casos := []struct{ nome, body, want string }{
		{"chave desconhecida", base(`{"name": "a", "call": "f", "expcet": {}}`), "chave desconhecida \"expcet\""},
		{"sem expect", base(`{"name": "a", "call": "f"}`), "\"expect\" é obrigatório"},
		{"call e event", base(`{"name": "a", "call": "f", "event": "validateForm", "expect": {"throws": true}}`), "\"call\" OU \"event\""},
		{"sem call", base(`{"name": "a", "expect": {"throws": true}}`), "falta \"call\""},
		{"args com event", base(`{"name": "a", "event": "validateForm", "args": [1], "expect": {"throws": true}}`), "\"args\" não combina"},
		{"throws false", base(`{"name": "a", "call": "f", "expect": {"throws": false}}`), "\"throws\": false não existe"},
		{"throws e return", base(`{"name": "a", "call": "f", "expect": {"throws": "x", "return": 1}}`), "não combina com \"return\""},
		{"returnType", base(`{"name": "a", "call": "f", "expect": {"returnType": "texto"}}`), "returnType \"texto\" inválido"},
		{"nome repetido", base(`{"name": "a", "call": "f", "expect": {"throws": true}}, {"name": "a", "call": "f", "expect": {"throws": true}}`), "nome repetido"},
		{"linha do dataset", base(`{"name": "a", "call": "f", "datasets": {"d": {"columns": ["A", "B"], "rows": [["1"]]}}, "expect": {"throws": true}}`), "rows[0] tem 1 valores para 2 colunas"},
		{"today", base(`{"name": "a", "call": "f", "today": "03/11/2026", "expect": {"throws": true}}`), "não é data"},
		{"returns", base(`{"name": "a", "call": "f", "returns": {"getX": 1}, "expect": {"throws": true}}`), "\"hAPI.<método>\""},
		{"nome de função", base(`{"name": "a", "call": "f()", "expect": {"throws": true}}`), "não é nome de função"},
		{"calls sem alvo", base(`{"name": "a", "call": "f", "expect": {"calls": [{"method": "setTaskComments"}]}}`), "\"hAPI.<método>\""},
		{"sem casos", `{"scripts": ["a.js"], "cases": []}`, "\"cases\" está vazio"},
		{"sem scripts", `{"cases": [{"name": "a", "call": "f", "expect": {"throws": true}}]}`, "nenhum script"},
	}
	for _, c := range casos {
		_, err := parseUnit(t, root, c.body)
		if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: erro %v, quer %q", c.nome, err, c.want)
		}
	}
	_, err := parseUnit(t, root, `{"scripts": ["forms/nao/existe.js"], "cases": [{"name": "a", "call": "f", "expect": {"throws": true}}]}`)
	if !errors.Is(err, ErrMissing) {
		t.Errorf("script ausente = ErrMissing: %v", err)
	}
}

// wk aceita texto, número, booleano e null; WK booleana do fluig.d.ts como
// texto gera aviso (no servidor é java.lang.Boolean).
func TestUnitWKTipos(t *testing.T) {
	root := unitProj(t)
	f, err := parseUnit(t, root, `{"scripts": ["forms/Compras/events/inputFields.js"],
	  "defaults": {"wk": {"WKMobile": "false", "WKNumState": 23}},
	  "cases": [
	    {"name": "a", "call": "calc", "wk": {"WKManagerMode": true, "WKUser": null}, "expect": {"throws": true}},
	    {"name": "b", "call": "calc", "expect": {"throws": true}}
	  ]}`)
	if err != nil {
		t.Fatal(err)
	}
	wk := f.Cases[0].Setup.WK
	if string(wk["WKManagerMode"]) != "true" || string(wk["WKNumState"]) != "23" || string(wk["WKUser"]) != "null" {
		t.Errorf("wk: %v", wk)
	}
	if len(f.Warnings) != 2 || !strings.Contains(f.Warnings[0], "wk.WKMobile é texto, mas no servidor é java.lang.Boolean") {
		t.Errorf("avisos (um por caso): %v", f.Warnings)
	}
	_, err = parseUnit(t, root, `{"scripts": ["forms/Compras/events/inputFields.js"],
	  "cases": [{"name": "a", "call": "calc", "wk": {"WKUser": ["x"]}, "expect": {"throws": true}}]}`)
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "wk.WKUser espera texto, número, true/false ou null") {
		t.Errorf("lista no wk: %v", err)
	}
}

func TestUnitFilesInDir(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"b.unit.json", "a/c.unit.json", "suite.json", "fixtures/x.unit.json"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		os.WriteFile(filepath.Join(root, p), []byte("{}"), 0o644)
	}
	got, err := UnitFilesInDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, g := range got {
		r, _ := filepath.Rel(root, g)
		rel = append(rel, filepath.ToSlash(r))
	}
	if strings.Join(rel, ",") != "a/c.unit.json,b.unit.json" {
		t.Errorf("arquivos: %v", rel)
	}
}

func evalCase(t *testing.T, expect string, obs string) UnitCaseResult {
	t.Helper()
	e, err := parseUnitExpect(json.RawMessage(expect), "expect")
	if err != nil {
		t.Fatal(err)
	}
	var o unitObs
	if err := json.Unmarshal([]byte(obs), &o); err != nil {
		t.Fatal(err)
	}
	return evalUnitCase(&UnitCase{Name: "c", Expect: e}, &o)
}

func failed(r UnitCaseResult) []string {
	var out []string
	for _, a := range r.Assertions {
		if !a.Passed {
			out = append(out, FormatAssertion(a))
		}
	}
	return out
}

func TestUnitEvalReturn(t *testing.T) {
	r := evalCase(t, `{"return": {"urgente": true, "diasUteis": "1"}, "returnType": "object"}`,
		`{"return": {"urgente": true, "diasUteis": "1"}, "returnType": "object"}`)
	if r.Status != UnitPassed {
		t.Errorf("igual: %v", failed(r))
	}
	r = evalCase(t, `{"return": {"urgente": false}}`, `{"return": {"urgente": true}, "returnType": "object"}`)
	if r.Status != UnitFailed || !strings.Contains(failed(r)[0], `esperado {"urgente":false}, obtido {"urgente":true}`) {
		t.Errorf("diferente: %v", failed(r))
	}
	// Número e texto não se misturam.
	r = evalCase(t, `{"return": 1}`, `{"return": "1", "returnType": "java-string"}`)
	if r.Status != UnitFailed {
		t.Error("1 e \"1\" são diferentes")
	}
	r = evalCase(t, `{"returnType": "js-string"}`, `{"return": "a", "returnType": "java-string"}`)
	if r.Status != UnitFailed {
		t.Error("o tipo pega o java.lang.String")
	}
}

func TestUnitEvalThrows(t *testing.T) {
	html := `{"thrown": {"kind": "value", "message": "<b style='color:red'>Os campos:<br>- Fornecedor.</b>"}}`
	if r := evalCase(t, `{"throws": ["os campos", "fornecedor"]}`, html); r.Status != UnitPassed {
		t.Errorf("contém, sem HTML e sem caixa: %v", failed(r))
	}
	if r := evalCase(t, `{"throws": "Filial"}`, html); r.Status != UnitFailed || !strings.Contains(failed(r)[0], "não contém") {
		t.Errorf("texto ausente: %v", failed(r))
	}
	if r := evalCase(t, `{"throws": true}`, `{"return": null}`); r.Status != UnitFailed || !strings.Contains(failed(r)[0], "terminou sem erro") {
		t.Errorf("esperava erro: %v", failed(r))
	}
	// Erro inesperado reprova, com a linha e a dica da global sem falso.
	r := evalCase(t, `{"return": 1}`, `{"thrown": {"kind": "error", "name": "ReferenceError", "message": "\"getCardValue\" is not defined.", "file": "a.js", "line": 7}}`)
	if r.Status != UnitFailed {
		t.Fatal("erro inesperado reprova")
	}
	msg := strings.Join(failed(r), "\n")
	if !strings.Contains(msg, "a.js linha 7") || !strings.Contains(msg, "a global getCardValue não existe no teste") {
		t.Errorf("mensagem: %s", msg)
	}
}

func TestUnitEvalCampos(t *testing.T) {
	obs := `{"form": {"pgtoUrgente": "S", "diasUteis": "1"}, "card": {"x": "2"}}`
	if r := evalCase(t, `{"form": {"pgtoUrgente": "S", "outro": null}, "card": {"x": "2"}}`, obs); r.Status != UnitPassed {
		t.Errorf("campos: %v", failed(r))
	}
	r := evalCase(t, `{"form": {"pgtoUrgente": "N", "diasUteis": null, "falta": "1", "$only": true}}`, obs)
	msg := strings.Join(failed(r), "\n")
	for _, want := range []string{`form.pgtoUrgente: esperado "N", obtido "S"`, `form.diasUteis: esperado "(não gravado)", obtido "1"`, "form.falta: esperado \"1\", obtido (ausente); o campo não foi gravado"} {
		if !strings.Contains(msg, want) {
			t.Errorf("sem %q em:\n%s", want, msg)
		}
	}
	r = evalCase(t, `{"form": {"pgtoUrgente": "S", "$only": true}}`, obs)
	if r.Status != UnitFailed || !strings.Contains(strings.Join(failed(r), ""), "diasUteis") {
		t.Errorf("$only aponta o campo extra: %v", failed(r))
	}
}

func TestUnitEvalLogDatasetCalls(t *testing.T) {
	obs := `{"logs": [{"level": "warn", "message": "Sem vencimento válido"}],
	  "datasetCalls": [{"name": "ds_a", "constraints": [{"field": "dataFinal", "initial": "05/11/2026"}]}],
	  "calls": [{"target": "hAPI", "method": "setTaskComments", "args": ["u", 3, "ok"]}]}`
	r := evalCase(t, `{"log": [{"level": "warn", "contains": "sem vencimento"}],
	  "datasetCalls": [{"name": "ds_a", "constraints": {"dataFinal": "05/11/2026"}}],
	  "calls": [{"method": "hAPI.setTaskComments", "args": ["u", 3, "ok"]}, {"method": "hAPI.setTaskComments"}]}`, obs)
	if r.Status != UnitPassed {
		t.Errorf("log/datasetCalls/calls: %v", failed(r))
	}
	r = evalCase(t, `{"log": [{"level": "error", "contains": "x"}],
	  "datasetCalls": [{"name": "ds_a", "constraints": {"dataFinal": "06/11/2026"}}],
	  "calls": [{"method": "hAPI.setDueDate"}]}`, obs)
	msg := strings.Join(failed(r), "\n")
	for _, want := range []string{"nenhuma linha de log neste nível", `datasetCalls.ds_a[0].dataFinal: esperado "06/11/2026", obtido "05/11/2026"`, "o método não foi chamado"} {
		if !strings.Contains(msg, want) {
			t.Errorf("sem %q em:\n%s", want, msg)
		}
	}
	if r := evalCase(t, `{"datasetCalls": []}`, obs); r.Status != UnitFailed {
		t.Error("datasetCalls: [] exige nenhuma chamada")
	}
}

func TestUnitEvalErros(t *testing.T) {
	r := evalCase(t, `{"throws": true}`, `{"loadError": {"phase": "load", "script": "a.js", "message": "TypeError: redeclaration of const y.", "line": 3, "compile": true}}`)
	if r.Status != UnitErrored || !strings.Contains(r.Error, "a.js não compila no Rhino (linha 3): TypeError: redeclaration of const y.") {
		t.Errorf("compilação: %+v", r)
	}
	r = evalCase(t, `{"throws": true}`, `{"missing": ["dataset ds_x"], "thrown": {"message": "fluigcli: o dataset \"ds_x\" não tem falso"}}`)
	if r.Status != UnitErrored || !strings.HasPrefix(r.Error, "falso ausente") {
		t.Errorf("falso ausente não conta como throws: %+v", r)
	}
	r = evalCase(t, `{"throws": true}`, `{"setupError": "a função \"x\" não existe nos scripts carregados"}`)
	if r.Status != UnitErrored {
		t.Errorf("função ausente: %+v", r)
	}
}

// RunUnit com executor falso: o plano chega certo ao motor, o script repetido
// entra uma vez só, e um estouro de tempo preserva os casos já terminados.
func TestRunUnitPlano(t *testing.T) {
	root := unitProj(t)
	f, err := parseUnit(t, root, `{"scripts": ["forms/Compras/events/inputFields.js"],
	  "defaults": {"today": "2026-11-03", "wk": {"WKNumState": "23"}},
	  "cases": [
	    {"name": "a", "call": "calc", "args": [1], "expect": {"return": 1}},
	    {"name": "b", "event": "inputFields", "expect": {"form": {"x": "1"}}},
	    {"name": "c", "call": "calc", "args": [2], "expect": {"return": 2}}
	  ]}`)
	if err != nil {
		t.Fatal(err)
	}
	var plan unitPlan
	exec := func(ctx context.Context, boot, planPath string) error {
		b, _ := os.ReadFile(planPath)
		json.Unmarshal(b, &plan)
		if bb, _ := os.ReadFile(boot); !strings.Contains(string(bb), "__fluigcliRun()") {
			t.Error("o bootstrap não foi gravado")
		}
		out := `{"file":0,"case":0,"return":1,"returnType":"number"}` + "\n" +
			`{"file":0,"case":1,"form":{"x":"2"}}` + "\n"
		os.WriteFile(plan.Out, []byte(out), 0o600)
		return ErrUnitTimeout
	}
	rep, err := RunUnit(context.Background(), []*UnitFile{f}, exec, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Scripts) != 1 || !strings.Contains(plan.Scripts[0].Source, "function calc") || plan.Prelude == "" {
		t.Errorf("scripts do plano: %+v", plan.Scripts)
	}
	s := plan.Cases[1].Setup
	if s.Call != "inputFields" || string(s.WK["WKNumState"]) != `"23"` || len(s.Today) != 6 || len(s.HAPIMembers) < 15 || len(s.FormMembers) < 15 {
		t.Errorf("setup do caso: %+v", s)
	}
	cs := rep.Files[0].Cases
	if cs[0].Status != UnitPassed || cs[1].Status != UnitFailed || cs[2].Status != UnitErrored || !strings.Contains(cs[2].Error, "tempo esgotado") {
		t.Errorf("resultados: %+v", cs)
	}
	if rep.Summary.Passed != 1 || rep.Summary.Failed != 1 || rep.Summary.Errored != 1 {
		t.Errorf("resumo: %+v", rep.Summary)
	}

	// Motor que cai antes do primeiro caso: um erro só.
	_, err = RunUnit(context.Background(), []*UnitFile{f}, func(context.Context, string, string) error {
		return errors.New("ClassNotFoundException")
	}, t.TempDir())
	if !errors.Is(err, ErrUnitEngine) {
		t.Errorf("motor quebrado: %v", err)
	}
}

func TestSelfCheckFile(t *testing.T) {
	f := SelfCheckFile()
	if len(f.Cases) != 21 {
		t.Fatalf("sondas: %d", len(f.Cases))
	}
	for _, c := range f.Cases {
		if c.Call != "__fluigcliProbe" || !c.Expect.HasReturn || len(c.Args) != 1 {
			t.Errorf("sonda %s malformada", c.Name)
		}
	}
}

func TestDecodeScriptLatin1(t *testing.T) {
	if got := decodeScript([]byte{'a', 0xe7, 0xe3, 'o'}); got != "ação" {
		t.Errorf("CP-1252: %q", got)
	}
}
