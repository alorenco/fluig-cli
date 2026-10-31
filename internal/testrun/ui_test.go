package testrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeUI simula a tela da tarefa: um mapa de elementos e os hiddens. O send
// aceita ou recusa conforme a tabela.
type fakeUI struct {
	eng      *fakeEngine
	elems    map[string]UIElem
	css      map[string]map[string]string
	hidden   map[string]string
	calls    []string
	refuse   map[int]string // etapa → mensagem de recusa
	failOpen error
	vp       UIViewport
	closed   bool
}

func (f *fakeUI) NewSession(context.Context) (UISession, error) { return f, nil }
func (f *fakeUI) SetViewport(_ context.Context, vp UIViewport) error {
	f.vp = vp
	return nil
}
func (f *fakeUI) OpenTask(_ context.Context, id, mov int, code string) error {
	f.calls = append(f.calls, "open task")
	if f.failOpen != nil {
		return f.failOpen
	}
	f.calls[len(f.calls)-1] += " " + code
	return nil
}
func (f *fakeUI) OpenView(context.Context, int) error {
	f.calls = append(f.calls, "open view")
	return nil
}
func (f *fakeUI) Fill(_ context.Context, pairs [][2]string) error {
	for _, p := range pairs {
		f.calls = append(f.calls, "fill "+p[0]+"="+p[1])
		if el, ok := f.elems[p[0]]; ok && !el.Editable {
			return errors.New(p[0] + ": campo bloqueado nesta etapa (displayFields)")
		}
		f.hidden[p[0]] = p[1]
	}
	return nil
}
func (f *fakeUI) Click(_ context.Context, k string) error {
	f.calls = append(f.calls, "click "+k)
	return nil
}
func (f *fakeUI) Send(_ context.Context, state int) (*UISendResult, error) {
	f.calls = append(f.calls, "send")
	// O beforeSendValidate muda o hidden a cada envio.
	f.hidden["controleEnvio"] += "x"
	if msg, ok := f.refuse[state]; ok {
		delete(f.refuse, state) // recusa só uma vez
		return &UISendResult{Posted: true, Status: 500, Message: msg, FormData: copyMap(f.hidden)}, nil
	}
	f.eng.current = f.eng.landing[state]
	return &UISendResult{Posted: true, Status: 200, FormData: copyMap(f.hidden)}, nil
}
func (f *fakeUI) Save(context.Context) (*UISendResult, error) {
	return &UISendResult{Posted: true, Status: 200}, nil
}
func (f *fakeUI) Inspect(_ context.Context, names []string, css map[string][]string) (*UIState, error) {
	st := &UIState{Elems: map[string]UIElem{}, CSS: map[string]map[string]string{}}
	for _, n := range names {
		st.Elems[n] = f.elems[n]
	}
	for k := range css {
		st.CSS[k] = f.css[k]
	}
	return st, nil
}
func (f *fakeUI) Hidden(_ context.Context, names []string) (map[string]*string, error) {
	out := map[string]*string{}
	for _, n := range names {
		if v, ok := f.hidden[n]; ok {
			v := v
			out[n] = &v
		}
	}
	return out, nil
}
func (f *fakeUI) Reset(context.Context)                      { f.calls = append(f.calls, "reset") }
func (f *fakeUI) Screenshot(context.Context) ([]byte, error) { return []byte("png"), nil }
func (f *fakeUI) Close()                                     { f.closed = true }

func copyMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sp(s string) *string { return &s }

func newFakeUI(eng *fakeEngine) *fakeUI {
	return &fakeUI{
		eng: eng,
		elems: map[string]UIElem{
			"painelFin":   {Exists: true, Visible: true},
			"painelAnexo": {Exists: true},
			"aprFin":      {Exists: true, Visible: true, Editable: true, Value: sp("")},
			"comentFin":   {Exists: true, Visible: true, Editable: true, Value: sp("")},
			"codColigada": {Exists: true, Visible: true, Value: sp("29")},
			"aprovadoPor": {Exists: true, Visible: true, Text: sp("por João Silva em 31/10/2026")},
		},
		css:    map[string]map[string]string{"painelFin": {"background-color": "rgb(255, 243, 205)"}},
		hidden: map[string]string{"controleEnvio": ""},
		refuse: map[int]string{},
	}
}

const suiteUI = `{"process": "P", "scenarios": [{"name": "c", "steps": [
  {"start": true}, {"assume": true}, {"move": 76}, {"move": 110, "expect": {"state": 112}}, {"assume": true},
  {"ui": {"open": "task"}, "expect": {
     "visible": ["painelFin"], "hidden": ["painelAnexo"], "enabled": ["aprFin", "comentFin"], "disabled": ["codColigada"],
     "value": {"codColigada": "29"}, "text": {"aprovadoPor": {"contains": "por "}},
     "css": {"painelFin": {"background-color": "rgb(255,243,205)"}}}},
  {"ui": {"fill": {"aprFin": "reprovado", "comentFin": ""}, "send": 13},
   "expect": {"message": "obrigatório incluir um COMENTÁRIO", "changed": ["controleEnvio"], "state": 112}},
  {"ui": {"fill": {"comentFin": "x"}, "send": 13}, "expect": {"state": 13, "changed": ["controleEnvio"]}}
]}]}`

func TestUIPassoPassa(t *testing.T) {
	f := newFake()
	ui := newFakeUI(f)
	ui.refuse[13] = "<b>Em caso de reprovação é obrigatório incluir um COMENTÁRIO!</b>"
	o := opts()
	o.UI = ui
	r := run(t, f, suiteUI, o)
	if r.Result != ResPassed {
		t.Fatalf("resultado %s: %+v", r.Result, r.Steps)
	}
	if !ui.closed {
		t.Error("a janela fecha no fim do cenário")
	}
	if ui.vp.Width != 1366 {
		t.Errorf("viewport padrão desktop: %+v", ui.vp)
	}
	joined := strings.Join(ui.calls, "|")
	if !strings.Contains(joined, "open task c-jsilva") || !strings.Contains(joined, "fill aprFin=reprovado|fill comentFin=|send|reset") {
		t.Errorf("chamadas: %s", joined)
	}
}

func TestUIRecusaSemMessageFalhaComTela(t *testing.T) {
	f := newFake()
	ui := newFakeUI(f)
	ui.refuse[13] = "Falta o comentário"
	o := opts()
	o.UI = ui
	o.UIArtifacts = t.TempDir()
	r := run(t, f, `{"process": "P", "scenarios": [{"name": "c", "steps": [
	  {"start": true}, {"ui": {"open": "task"}}, {"ui": {"fill": {"aprFin": "x"}, "send": 13}, "expect": {"state": 13}}]}]}`, o)
	st := r.Steps[2]
	if r.Result != ResFailed || st.Error != "o Fluig recusou o envio: Falta o comentário" {
		t.Fatalf("recusa inesperada: %+v", st)
	}
	if st.Screenshot == "" || filepath.Dir(st.Screenshot) != o.UIArtifacts {
		t.Fatalf("screenshot: %q", st.Screenshot)
	}
	if b, _ := os.ReadFile(st.Screenshot); string(b) != "png" {
		t.Error("o arquivo do screenshot não foi gravado")
	}
}

func TestUIAssercoesFalham(t *testing.T) {
	f := newFake()
	ui := newFakeUI(f)
	o := opts()
	o.UI = ui
	r := run(t, f, `{"process": "P", "scenarios": [{"name": "c", "steps": [
	  {"start": true},
	  {"ui": {"open": "view", "viewport": "mobile"}, "expect": {
	     "enabled": ["codColigada"], "disabled": ["aprFin", "naoExiste"], "hidden": ["painelFin"],
	     "css": {"painelFin": {"background-color": "red"}}, "text": {"aprovadoPor": "outro"}}}]}]}`, o)
	if r.Result != ResFailed {
		t.Fatal("as asserções erradas reprovam")
	}
	var msgs []string
	for _, a := range r.Steps[1].Assertions {
		if !a.Passed {
			msgs = append(msgs, FormatAssertion(a))
		}
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{
		`enabled.codColigada: esperado "editável", obtido "bloqueado"`,
		`disabled.aprFin: esperado "bloqueado", obtido "editável"`,
		`disabled.naoExiste: esperado "bloqueado", obtido "(ausente)"; o campo não existe na página`,
		"hidden.painelFin: esperado true, obtido false",
		`css.painelFin background-color: esperado "red", obtido "rgb(255, 243, 205)"`,
		`text.aprovadoPor: esperado "outro"`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("sem %q em:\n%s", want, all)
		}
	}
	if !ui.vp.Mobile || ui.vp.Width != 390 {
		t.Errorf("viewport mobile: %+v", ui.vp)
	}
}

func TestUIMessageMasAceito(t *testing.T) {
	f := newFake()
	ui := newFakeUI(f)
	o := opts()
	o.UI = ui
	r := run(t, f, `{"process": "P", "scenarios": [{"name": "c", "steps": [
	  {"start": true}, {"ui": {"open": "task"}},
	  {"ui": {"fill": {"aprFin": "x"}, "send": 13}, "expect": {"message": "COMENTÁRIO", "unchanged": ["controleEnvio"]}}]}]}`, o)
	var msgs []string
	for _, a := range r.Steps[2].Assertions {
		if !a.Passed {
			msgs = append(msgs, FormatAssertion(a))
		}
	}
	all := strings.Join(msgs, "\n")
	if !strings.Contains(all, "nenhuma mensagem apareceu (o envio foi aceito)") || !strings.Contains(all, "unchanged.controleEnvio") {
		t.Errorf("asserções:\n%s", all)
	}
}

func TestUISemNavegadorEFalhaDeAbertura(t *testing.T) {
	f := newFake()
	r := run(t, f, `{"process": "P", "scenarios": [{"name": "c", "steps": [{"start": true}, {"ui": {"open": "task"}}]}]}`, opts())
	if r.Result != ResErrored || !strings.Contains(r.Steps[1].Error, "nenhum foi configurado") {
		t.Errorf("sem driver: %+v", r.Steps[1])
	}
	ui := newFakeUI(newFake())
	ui.failOpen = errors.New("o Fluig recusou abrir a tarefa: sem permissão")
	o := opts()
	o.UI = ui
	r = run(t, ui.eng, `{"process": "P", "scenarios": [{"name": "c", "steps": [{"start": true}, {"ui": {"open": "task"}}]}]}`, o)
	if r.Result != ResFailed || !strings.Contains(r.Steps[1].Error, "sem permissão") {
		t.Errorf("abertura recusada: %+v", r.Steps[1])
	}
	// Campo bloqueado no fill reprova com o nome do campo.
	ui = newFakeUI(newFake())
	o.UI = ui
	r = run(t, ui.eng, `{"process": "P", "scenarios": [{"name": "c", "steps": [{"start": true}, {"ui": {"open": "task"}},
	  {"ui": {"fill": {"codColigada": "21"}}}]}]}`, o)
	if r.Result != ResFailed || !strings.Contains(r.Steps[2].Error, "codColigada: campo bloqueado") {
		t.Errorf("fill em campo bloqueado: %+v", r.Steps[2])
	}
}

func TestUIParse(t *testing.T) {
	s := parse(t, `{"process": "P", "defaults": {"ui": {"viewport": {"w": 800, "h": 600}}}, "scenarios": [{"name": "c", "steps": [
	  {"start": true},
	  {"ui": {"fill": {"z": "1", "a": 2, "m": true}, "send": 13}, "expect": {"message": "x"}},
	  {"ui": {"open": "task"}}, {"ui": {"save": true}}, {"ui": {"click": "css:#btn"}}]}]}`)
	st := s.Scenarios[0].Steps[1]
	if st.Name != "ui send → 13" || len(st.UI.Fill) != 3 || st.UI.Fill[0][0] != "z" || st.UI.Fill[1] != [2]string{"a", "2"} || st.UI.Fill[2][1] != "true" {
		t.Errorf("fill na ordem do arquivo: %+v", st.UI.Fill)
	}
	if s.Defaults.UIViewport == nil || s.Defaults.UIViewport.Width != 800 || !s.HasUI() {
		t.Errorf("defaults.ui: %+v", s.Defaults.UIViewport)
	}
	casos := []struct{ step, want string }{
		{`{"ui": {"open": "task", "send": 13}}`, "um passo ui faz uma ação"},
		{`{"ui": {}}`, "precisa de uma ação"},
		{`{"ui": {"open": "edit"}}`, `"edit" inválido`},
		{`{"ui": {"fill": {"a": "1"}, "send": 1, "save": true}}`, `use "send" OU "save"`},
		{`{"ui": {"open": "task", "viewport": "tablet"}}`, `"tablet" inválido`},
		{`{"ui": {"open": "task", "viewport": {"w": 10, "h": 10}}}`, "entre 200 e 4000"},
		{`{"ui": {"open": "task"}, "expect": {"message": "x"}}`, "só existe depois de"},
		{`{"ui": {"fill": {"a": "1"}, "send": 13}, "expect": {"error": "x"}}`, "use \"message\""},
		{`{"move": 76, "expect": {"visible": ["x"]}}`, "só valem num passo \"ui\""},
		{`{"ui": {"fill": {"a": ["1"]}}}`, "espera texto, número ou booleano"},
		{`{"ui": {"open": "task"}, "expect": {"css": {"x": {}}}}`, "ao menos uma propriedade"},
	}
	for _, c := range casos {
		_, err := ParseSuite("s.json", []byte(`{"process": "P", "scenarios": [{"name": "c", "steps": [{"start": true}, `+c.step+`]}]}`), LoadOptions{})
		if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: erro %v, quer %q", c.step, err, c.want)
		}
	}
}

// O dry-run confere a etapa do send e trata a recusa esperada como "fica onde
// estava".
func TestUIDryRun(t *testing.T) {
	s := parse(t, `{"process": "P", "scenarios": [{"name": "c", "steps": [
	  {"start": true, "expect": {"state": 20}}, {"assume": true}, {"move": 76, "expect": {"state": 23}},
	  {"ui": {"fill": {"a": "1"}, "send": 110}, "expect": {"message": "x"}},
	  {"ui": {"fill": {"a": "1"}, "send": 52}}]}]}`)
	f := newFake()
	_, probs := DryRunSuite(context.Background(), f, s, testProc(), opts())
	if len(probs) != 1 || !strings.Contains(probs[0], "não há transição da etapa 23 (Diretoria) para a 52") {
		t.Errorf("problemas: %v", probs)
	}
}
