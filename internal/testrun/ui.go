package testrun

// Passo "ui": a camada de interface do `fluigcli test` (ROADMAP §5.19, spec em
// docs/interno/SPEC-TEST-E.md). A API leva a solicitação até a etapa; o
// navegador abre o render REAL da tarefa (displayFields no servidor, form.js)
// e confere o que a tela mostra.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ActUI é a ação do passo de interface.
const ActUI = "ui"

// UIStep é a ação de um passo ui, já validada.
type UIStep struct {
	Open     string      // "task" | "view"
	Fill     [][2]string // na ordem do arquivo (máscaras e eventos dependem dela)
	Click    string
	Send     int  // etapa de destino (sequência)
	Save     bool // Salvar
	Viewport *UIViewport
}

// UIViewport é o tamanho da janela.
type UIViewport struct {
	Name   string `json:"name,omitempty"` // desktop | mobile | ""
	Width  int    `json:"w,omitempty"`
	Height int    `json:"h,omitempty"`
	Mobile bool   `json:"mobile,omitempty"`
}

// UIExpect são as asserções de tela de um passo ui.
type UIExpect struct {
	Visible   []string
	Hidden    []string
	Enabled   []string
	Disabled  []string
	Value     []CardExpect
	Text      []CardExpect
	CSS       map[string]map[string]string
	Message   *string
	Changed   []string
	Unchanged []string
}

// UIDriver abre janelas de navegador (uma por cenário).
type UIDriver interface {
	NewSession(ctx context.Context) (UISession, error)
}

// UISession é uma janela com a sessão do Fluig.
type UISession interface {
	SetViewport(ctx context.Context, vp UIViewport) error
	OpenTask(ctx context.Context, requestID, movement int, userCode string) error
	OpenView(ctx context.Context, requestID int) error
	Fill(ctx context.Context, pairs [][2]string) error
	Click(ctx context.Context, key string) error
	Send(ctx context.Context, state int) (*UISendResult, error)
	Save(ctx context.Context) (*UISendResult, error)
	Inspect(ctx context.Context, names []string, css map[string][]string) (*UIState, error)
	Hidden(ctx context.Context, names []string) (map[string]*string, error)
	Reset(ctx context.Context)
	Screenshot(ctx context.Context) ([]byte, error)
	Close()
}

// UISendResult é o desfecho de um envio pela tela.
type UISendResult struct {
	Posted   bool
	Status   int
	Message  string
	FormData map[string]string
}

// UIElem é o que a tela mostra de um campo ou elemento.
type UIElem struct {
	Exists   bool
	Visible  bool
	Editable bool
	Value    *string
	Text     *string
}

// UIState é o resultado de uma inspeção.
type UIState struct {
	Elems map[string]UIElem
	CSS   map[string]map[string]string
}

type rawUI struct {
	Open     string          `json:"open"`
	Fill     json.RawMessage `json:"fill"`
	Click    string          `json:"click"`
	Send     int             `json:"send"`
	Save     bool            `json:"save"`
	Viewport json.RawMessage `json:"viewport"`
}

func parseUIStep(raw json.RawMessage, path string) (*UIStep, error) {
	var r rawUI
	if err := strict(raw, &r, path); err != nil {
		return nil, err
	}
	u := &UIStep{Open: r.Open, Click: r.Click, Send: r.Send, Save: r.Save}
	if r.Open != "" && r.Open != "task" && r.Open != "view" {
		return nil, usageErr("%s.open: %q inválido; use \"task\" (a tarefa em edição) ou \"view\" (os detalhes)", path, r.Open)
	}
	if !isNull(r.Fill) {
		f, err := orderedFields(r.Fill, path+".fill")
		if err != nil {
			return nil, err
		}
		u.Fill = f
	}
	if !isNull(r.Viewport) {
		vp, err := parseViewport(r.Viewport, path+".viewport")
		if err != nil {
			return nil, err
		}
		u.Viewport = vp
	}
	if r.Send < 0 {
		return nil, usageErr("%s.send: informe a sequência da etapa de destino", path)
	}
	acoes := 0
	for _, on := range []bool{u.Open != "", len(u.Fill) > 0, u.Click != "", u.Send > 0, u.Save} {
		if on {
			acoes++
		}
	}
	// fill + send/save é a única combinação (o caso comum: preencher e enviar).
	combo := len(u.Fill) > 0 && (u.Send > 0 || u.Save) && u.Open == "" && u.Click == ""
	switch {
	case acoes == 0 && u.Viewport == nil:
		return nil, usageErr("%s: o passo ui precisa de uma ação: open, fill, click, send ou save", path)
	case acoes > 1 && !combo:
		return nil, usageErr("%s: um passo ui faz uma ação (open, fill, click, send ou save); só \"fill\" combina com \"send\" ou \"save\"", path)
	case u.Send > 0 && u.Save:
		return nil, usageErr("%s: use \"send\" OU \"save\"", path)
	}
	return u, nil
}

func parseViewport(raw json.RawMessage, path string) (*UIViewport, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "desktop":
			return &UIViewport{Name: s, Width: 1366, Height: 900}, nil
		case "mobile":
			return &UIViewport{Name: s, Width: 390, Height: 844, Mobile: true}, nil
		}
		return nil, usageErr("%s: %q inválido; use \"desktop\", \"mobile\" ou {\"w\": 800, \"h\": 600}", path, s)
	}
	var vp UIViewport
	if err := strict(raw, &vp, path); err != nil {
		return nil, err
	}
	if vp.Name != "" || vp.Width < 200 || vp.Height < 200 || vp.Width > 4000 || vp.Height > 4000 {
		return nil, usageErr("%s: largura e altura entre 200 e 4000 (\"w\" e \"h\")", path)
	}
	return &vp, nil
}

// orderedFields lê um objeto JSON plano preservando a ordem das chaves.
func orderedFields(raw json.RawMessage, path string) ([][2]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, usageErr("%s: espera um objeto {\"campo\": \"valor\"}", path)
	}
	var out [][2]string
	seen := map[string]bool{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, usageErr("%s: %v", path, err)
		}
		k := kt.(string)
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil, usageErr("%s.%s: %v", path, k, err)
		}
		var s string
		switch t := v.(type) {
		case string:
			s = t
		case json.Number:
			s = t.String()
		case bool:
			s = fmt.Sprint(t)
		default:
			return nil, usageErr("%s.%s: espera texto, número ou booleano", path, k)
		}
		if seen[k] {
			return nil, usageErr("%s: campo %q repetido", path, k)
		}
		seen[k] = true
		out = append(out, [2]string{k, s})
	}
	if len(out) == 0 {
		return nil, usageErr("%s: fill sem campos", path)
	}
	return out, nil
}

type rawUIExpect struct {
	Visible   []string                     `json:"visible"`
	Hidden    []string                     `json:"hidden"`
	Enabled   []string                     `json:"enabled"`
	Disabled  []string                     `json:"disabled"`
	Value     json.RawMessage              `json:"value"`
	Text      json.RawMessage              `json:"text"`
	CSS       map[string]map[string]string `json:"css"`
	Message   *string                      `json:"message"`
	Changed   []string                     `json:"changed"`
	Unchanged []string                     `json:"unchanged"`
}

func (r *rawUIExpect) empty() bool {
	return len(r.Visible)+len(r.Hidden)+len(r.Enabled)+len(r.Disabled)+len(r.CSS)+len(r.Changed)+len(r.Unchanged) == 0 &&
		isNull(r.Value) && isNull(r.Text) && r.Message == nil
}

func parseUIExpect(r *rawUIExpect, path string) (*UIExpect, error) {
	e := &UIExpect{Visible: r.Visible, Hidden: r.Hidden, Enabled: r.Enabled, Disabled: r.Disabled,
		CSS: r.CSS, Message: r.Message, Changed: r.Changed, Unchanged: r.Unchanged}
	if !isNull(r.Value) {
		v, err := parseCardExpect(r.Value, path+".value")
		if err != nil {
			return nil, err
		}
		e.Value = v
	}
	if !isNull(r.Text) {
		v, err := parseCardExpect(r.Text, path+".text")
		if err != nil {
			return nil, err
		}
		e.Text = v
	}
	if r.Message != nil && strings.TrimSpace(*r.Message) == "" {
		return nil, usageErr("%s.message: informe um trecho da mensagem", path)
	}
	for sel, props := range r.CSS {
		if len(props) == 0 {
			return nil, usageErr("%s.css.%s: informe ao menos uma propriedade", path, sel)
		}
	}
	return e, nil
}

// uiNames lista as chaves que a inspeção precisa ler.
func (e *UIExpect) uiNames() []string {
	set := map[string]bool{}
	for _, l := range [][]string{e.Visible, e.Hidden, e.Enabled, e.Disabled} {
		for _, n := range l {
			set[n] = true
		}
	}
	for _, c := range e.Value {
		set[c.Field] = true
	}
	for _, c := range e.Text {
		set[c.Field] = true
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (e *UIExpect) cssQuery() map[string][]string {
	if len(e.CSS) == 0 {
		return nil
	}
	out := map[string][]string{}
	for sel, props := range e.CSS {
		for p := range props {
			out[sel] = append(out[sel], p)
		}
		sort.Strings(out[sel])
	}
	return out
}

func (e *UIExpect) needsInspect() bool {
	return len(e.uiNames()) > 0 || len(e.CSS) > 0
}

// evalUI avalia as asserções de tela. before/after são os valores dos campos
// de changed/unchanged no início e no fim (no envio, os valores ENVIADOS).
func evalUI(e *UIExpect, st *UIState, send *UISendResult, before, after map[string]*string, env *Env) ([]Assertion, error) {
	var out []Assertion
	elem := func(n string) UIElem {
		if st == nil {
			return UIElem{}
		}
		return st.Elems[n]
	}
	for _, n := range e.Visible {
		el := elem(n)
		a := Assertion{Type: "visible", Field: n, Expected: true, Actual: el.Visible, Passed: el.Visible}
		if !el.Exists {
			a.Message = "o elemento não existe na página"
		}
		out = append(out, a)
	}
	for _, n := range e.Hidden {
		el := elem(n)
		out = append(out, Assertion{Type: "hidden", Field: n, Expected: true, Actual: !el.Visible, Passed: !el.Visible})
	}
	for _, n := range e.Enabled {
		el := elem(n)
		a := Assertion{Type: "enabled", Field: n, Expected: "editável", Actual: describeElem(el), Passed: el.Editable}
		out = append(out, a)
	}
	for _, n := range e.Disabled {
		el := elem(n)
		// Bloqueado = existe e não é editável (disabled, readonly, trocado
		// pelo "_campo" do setEnabled ou só hidden). Ausente reprova: um nome
		// errado não pode passar como "bloqueado".
		a := Assertion{Type: "disabled", Field: n, Expected: "bloqueado", Actual: describeElem(el), Passed: el.Exists && !el.Editable}
		if !el.Exists {
			a.Message = "o campo não existe na página (nome errado?)"
		}
		out = append(out, a)
	}
	for _, c := range e.Value {
		a, err := evalUIText("value", c, elem(c.Field).Value, env)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	for _, c := range e.Text {
		a, err := evalUIText("text", c, elem(c.Field).Text, env)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	for _, sel := range sortedCSSKeys(e.CSS) {
		var got map[string]string
		if st != nil {
			got = st.CSS[sel]
		}
		props := e.CSS[sel]
		names := make([]string, 0, len(props))
		for p := range props {
			names = append(names, p)
		}
		sort.Strings(names)
		for _, p := range names {
			a := Assertion{Type: "css", Field: sel + " " + p, Expected: props[p]}
			if got == nil {
				a.Message = "o elemento não existe na página"
			} else {
				a.Actual = got[p]
				a.Passed = normCSS(got[p]) == normCSS(props[p])
			}
			out = append(out, a)
		}
	}
	if e.Message != nil {
		want, err := Resolve(*e.Message, env)
		if err != nil {
			return nil, err
		}
		a := Assertion{Type: "message", Expected: "contém \"" + want + "\""}
		if send != nil && send.Message != "" {
			a.Actual = NormalizeServerText(send.Message)
			a.Passed = strings.Contains(strings.ToLower(a.Actual.(string)), strings.ToLower(NormalizeServerText(want)))
		} else {
			a.Message = "nenhuma mensagem apareceu (o envio foi aceito)"
		}
		out = append(out, a)
	}
	for _, n := range e.Changed {
		b, f := derefOr(before[n], "(ausente)"), derefOr(after[n], "(ausente)")
		a := Assertion{Type: "changed", Field: n, Expected: "diferente de " + fmt.Sprintf("%q", b), Actual: f, Passed: after[n] != nil && b != f}
		out = append(out, a)
	}
	for _, n := range e.Unchanged {
		b, f := derefOr(before[n], "(ausente)"), derefOr(after[n], "(ausente)")
		out = append(out, Assertion{Type: "unchanged", Field: n, Expected: b, Actual: f, Passed: b == f})
	}
	return out, nil
}

func describeElem(el UIElem) string {
	switch {
	case !el.Exists:
		return "(ausente)"
	case el.Editable:
		return "editável"
	case !el.Visible:
		return "oculto"
	}
	return "bloqueado"
}

func evalUIText(typ string, c CardExpect, got *string, env *Env) (Assertion, error) {
	card := map[string]string{}
	if got != nil {
		card[c.Field] = *got
	}
	a, err := evalCard(c, card, env)
	if err != nil {
		return a, err
	}
	a.Type = typ
	if got == nil {
		a.Passed = false
		a.Message = "o elemento não existe na página"
	}
	return a, nil
}

func sortedCSSKeys(m map[string]map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// normCSS compara valores de CSS sem espaço e sem caixa ("rgb(1, 2, 3)" =
// "rgb(1,2,3)").
func normCSS(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
}
