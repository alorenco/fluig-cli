package testrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envFixo() *Env {
	return &Env{
		Today:     time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		Vars:      map[string]string{"venc": "13/11/2026"},
		SelfLogin: "jsilva", SelfCode: "c-jsilva",
		RequestID: 1234, Steps: map[string]int{"dir": 112}, Unique: "zztabc",
	}
}

func TestResolve(t *testing.T) {
	env := envFixo()
	casos := map[string]string{
		"{{today}}":                       "31/10/2026",
		"{{today(+5)}}":                   "05/11/2026",
		"{{ today(-1) }}":                 "30/10/2026",
		"doc {{unique}}":                  "doc zztabc",
		"{{var.venc}}":                    "13/11/2026",
		"{{actors.eu.login}}":             "jsilva",
		"{{actors.eu.code}}":              "c-jsilva",
		"nº {{requestId}}":                "nº 1234",
		"{{steps.dir.nextState}}":         "112",
		"literal {{{{today}} e {{today}}": "literal {{today}} e 31/10/2026",
		"sem expressão":                   "sem expressão",
	}
	for in, want := range casos {
		got, err := Resolve(in, env)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; quer %q", in, got, err, want)
		}
	}
	env.RequestID = 0
	if _, err := Resolve("{{requestId}}", env); err == nil {
		t.Error("{{requestId}} antes do start deveria falhar")
	}
	if _, err := Resolve("{{steps.x.nextState}}", env); err == nil {
		t.Error("passo que não rodou deveria falhar")
	}
}

func TestUniqueValue(t *testing.T) {
	u := UniqueValue(time.Unix(1_800_000_000, 0), 3)
	if !strings.HasPrefix(u, "zzt") || !strings.HasSuffix(u, "3") || len(u) != 12 {
		t.Errorf("formato inesperado: %q", u)
	}
	if UniqueValue(time.Unix(1_800_000_000, 0), 4) == u {
		t.Error("cenários diferentes precisam de valores diferentes")
	}
}

func readTD(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Os dois formatos reais do Gate 0 (G0.4) normalizam para texto limpo.
func TestNormalizeServerTextFormatosReais(t *testing.T) {
	// validateForm: HTTP 500, texto entre chaves, com HTML.
	vf := NormalizeServerText(readTD(t, "start_error_validateform.txt"))
	if !strings.HasPrefix(vf, "Erro ao salvar dados de formulário:") || strings.Contains(vf, "<") ||
		!strings.Contains(vf, "Os seguintes campos não foram preenchidos: - Tipo de Pagamento / Documento.") {
		t.Errorf("validateForm mal normalizado: %q", vf)
	}
	// beforeTaskSave: HTTP 400 JSON; o texto vem no message, com HTML.
	var e struct{ Code, Message string }
	json.Unmarshal([]byte(readTD(t, "move_error_beforetasksave.json")), &e)
	if e.Code != "BPMBeforeTaskSaveException" {
		t.Fatalf("fixture inesperada: %+v", e)
	}
	bt := NormalizeServerText(e.Message)
	if bt != "Erro ao executar evento do processo, java.lang.Exception: Decisão da aprovação é incompatível com a próxima etapa selecionada!" {
		t.Errorf("beforeTaskSave mal normalizado: %q", bt)
	}
	if NormalizeServerText("a &amp; b  <br/> c") != "a & b c" {
		t.Error("entidade HTML não decodificada")
	}
}

func intp(i int) *int       { return &i }
func strp(s string) *string { return &s }

func TestEvaluate(t *testing.T) {
	env := envFixo()
	snap := &Snapshot{
		Status:  "OPEN",
		Current: []StepState{{Sequence: 112, Name: "Aprovação Financeira", Movement: 7}},
		Open:    []string{"Pool:Role:financeiro"},
		Visited: []int{4, 20, 23, 98, 110, 112},
		Card:    map[string]string{"pgtoUrgente": "S", "dias": "3", "obs": "", "nome": "João Silva", "data": "31/10/2026 12:32", "venc___1": "05/11/2026"},
	}
	e := &Expect{
		State: intp(112), StateName: strp("Aprovação Financeira"), Status: strp("OPEN"),
		Assignee: strp("pool:role:financeiro"),
		Path:     &PathExpect{Visited: []int{23, 112}, NotVisited: []int{28}},
		Card: []CardExpect{
			{Field: "pgtoUrgente", Op: "eq", Value: "S"},
			{Field: "pgtoUrgente", Op: "ne", Value: "N"},
			{Field: "dias", Op: "in", List: []string{"0", "1", "2", "3"}},
			{Field: "obs", Op: "empty"},
			{Field: "inexistente", Op: "empty"},
			{Field: "nome", Op: "notEmpty"},
			{Field: "nome", Op: "contains", Value: "Silva"},
			{Field: "data", Op: "matches", Value: `^\d{2}/\d{2}/\d{4} \d{2}:\d{2}$`},
			{Field: "venc___1", Op: "eq", Value: "{{today(+5)}}"},
		},
	}
	as, err := Evaluate(e, snap, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range as {
		if !a.Passed {
			t.Errorf("asserção deveria passar: %+v", a)
		}
	}
	if len(as) != 16 {
		t.Errorf("esperava 16 asserções, veio %d", len(as))
	}

	// Falhas: todas aparecem, com esperado × obtido.
	ruim := &Expect{
		State:  intp(28),
		Status: strp("FINALIZED"),
		Path:   &PathExpect{Visited: []int{28}, NotVisited: []int{112}},
		Card: []CardExpect{
			{Field: "pgtoUrgente", Op: "eq", Value: "N"},
			{Field: "naoExiste", Op: "eq", Value: "x"},
			{Field: "obs", Op: "notEmpty"},
		},
	}
	as, _ = Evaluate(ruim, snap, nil, env)
	falhas := 0
	for _, a := range as {
		if !a.Passed {
			falhas++
		}
	}
	if falhas != len(as) || falhas != 7 {
		t.Errorf("esperava 7 falhas, veio %d de %d: %+v", falhas, len(as), as)
	}
	linhas := []string{}
	for _, a := range as {
		linhas = append(linhas, FormatAssertion(a))
	}
	texto := strings.Join(linhas, "\n")
	for _, want := range []string{
		`state: esperado 28, obtido 112; etapa atual: 112 ("Aprovação Financeira")`,
		`card.pgtoUrgente: esperado "N", obtido "S"`,
		"card.naoExiste: campo naoExiste não existe no card",
		`status: esperado "FINALIZED", obtido "OPEN"`,
	} {
		if !strings.Contains(texto, want) {
			t.Errorf("relatório sem %q:\n%s", want, texto)
		}
	}
}

func TestEvaluateErro(t *testing.T) {
	env := envFixo()
	snap := &Snapshot{Current: []StepState{{Sequence: 23}}}
	msg := "Erro ao executar evento do processo, java.lang.Exception: Decisão da aprovação é incompatível com a próxima etapa selecionada!"
	casos := []struct {
		nome string
		exp  ErrorExpect
		err  *StepError
		ok   bool
	}{
		{"contains sem caixa", ErrorExpect{Contains: "decisão DA aprovação"}, &StepError{Code: "SERVER_ERROR", Message: msg}, true},
		{"contains com HTML no esperado", ErrorExpect{Contains: "<b>incompatível</b>"}, &StepError{Message: msg}, true},
		{"contains ausente", ErrorExpect{Contains: "COMENTÁRIO"}, &StepError{Message: msg}, false},
		{"code certo", ErrorExpect{Code: "server_error"}, &StepError{Code: "SERVER_ERROR", Message: msg}, true},
		{"code errado", ErrorExpect{Code: "POOL_TASK_NOT_ASSIGNED"}, &StepError{Code: "SERVER_ERROR", Message: msg}, false},
		{"matches", ErrorExpect{Matches: `incompat[ií]vel`}, &StepError{Message: msg}, true},
		{"passo aceito", ErrorExpect{Contains: "x"}, nil, false},
	}
	for _, c := range casos {
		as, err := Evaluate(&Expect{Error: &c.exp}, snap, c.err, env)
		if err != nil {
			t.Fatal(err)
		}
		if as[0].Passed != c.ok {
			t.Errorf("%s: passed=%v, quer %v (%+v)", c.nome, as[0].Passed, c.ok, as[0])
		}
	}
}

func TestNeedOf(t *testing.T) {
	n := NeedOf(&Expect{Card: []CardExpect{{Field: "a", Op: "eq"}}})
	if n.Request || n.Tasks || !n.Card {
		t.Errorf("card só lê o card: %+v", n)
	}
	n = NeedOf(&Expect{Path: &PathExpect{Visited: []int{1}}})
	if n.Request || !n.Tasks || n.Card {
		t.Errorf("path só lê as tarefas: %+v", n)
	}
	if (NeedOf(nil) != Need{}) {
		t.Error("sem expect não lê nada")
	}
}
