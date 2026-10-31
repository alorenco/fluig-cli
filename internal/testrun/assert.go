package testrun

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Snapshot é o estado da solicitação lido UMA vez depois do passo. Só o que
// as asserções do passo pedem é preenchido (Need diz o quê).
type Snapshot struct {
	Status   string      // OPEN | FINALIZED | CANCELED
	Current  []StepState // etapas correntes, sem repetição (etapa, movimento)
	Open     []string    // responsáveis das tarefas abertas (código e login)
	Visited  []int       // etapas por onde a solicitação passou (G0.9)
	Card     map[string]string
	HasCard  bool
	HasTasks bool
}

// StepState é uma etapa corrente.
type StepState struct {
	Sequence int    `json:"sequence"`
	Name     string `json:"name"`
	Movement int    `json:"movement"`
}

// Need diz o que a avaliação precisa ler do servidor.
type Need struct {
	Request bool // etapa, nome, status
	Tasks   bool // caminho e responsável
	Card    bool
}

// NeedOf calcula as leituras que um expect exige.
func NeedOf(e *Expect) Need {
	if e == nil {
		return Need{}
	}
	return Need{
		Request: e.State != nil || e.StateName != nil || e.Status != nil || e.Error != nil,
		Tasks:   e.Path != nil || e.Assignee != nil,
		Card:    len(e.Card) > 0,
	}
}

// Assertion é o resultado de uma asserção (contrato --json).
type Assertion struct {
	Type     string `json:"type"`
	Field    string `json:"field,omitempty"`
	Op       string `json:"op,omitempty"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Passed   bool   `json:"passed"`
	Message  string `json:"message,omitempty"`
}

// StepError é o erro de um passo como as asserções o veem.
type StepError struct {
	Code    string // código da CLI (SERVER_ERROR, POOL_TASK_NOT_ASSIGNED…)
	Message string // texto do servidor já normalizado
}

// NormalizeServerText remove HTML, entidades e espaço redundante. Os eventos
// do Fluig fazem throw com HTML de destaque, e o texto chega em dois formatos
// (G0.4): 400 JSON do beforeTaskSave e 500 texto entre chaves do validateForm.
func NormalizeServerText(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && !json.Valid([]byte(s)) {
		s = s[1 : len(s)-1]
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}

// Evaluate avalia TODAS as asserções do expect, mesmo depois da primeira
// falha: o relatório mostra todas as divergências de uma vez. stepErr é nil
// quando o passo foi aceito.
func Evaluate(e *Expect, snap *Snapshot, stepErr *StepError, env *Env) ([]Assertion, error) {
	if e == nil {
		return nil, nil
	}
	var out []Assertion
	res := func(s string) (string, error) { return Resolve(s, env) }

	if e.Error != nil {
		a := Assertion{Type: "error"}
		var exp []string
		if e.Error.Contains != "" {
			c, err := res(e.Error.Contains)
			if err != nil {
				return nil, err
			}
			exp = append(exp, "contém \""+c+"\"")
			if stepErr != nil && !strings.Contains(strings.ToLower(stepErr.Message), strings.ToLower(NormalizeServerText(c))) {
				a.Message = "a mensagem não contém o texto esperado"
			}
		}
		if e.Error.Code != "" {
			exp = append(exp, "código "+e.Error.Code)
			if stepErr != nil && !strings.EqualFold(stepErr.Code, e.Error.Code) {
				a.Message = "código de erro diferente: " + stepErr.Code
			}
		}
		if e.Error.Matches != "" {
			exp = append(exp, "casa /"+e.Error.Matches+"/")
			re, err := regexp.Compile(e.Error.Matches)
			if err != nil {
				return nil, err
			}
			if stepErr != nil && !re.MatchString(stepErr.Message) {
				a.Message = "a mensagem não casa com a expressão regular"
			}
		}
		a.Expected = strings.Join(exp, " e ")
		if stepErr == nil {
			a.Actual = "passo aceito"
			a.Message = "esperava erro, mas o passo foi aceito"
		} else {
			a.Actual = stepErr.Message
			a.Passed = a.Message == ""
		}
		out = append(out, a)
	}

	if e.State != nil {
		a := Assertion{Type: "state", Expected: *e.State, Actual: seqs(snap.Current)}
		for _, c := range snap.Current {
			if c.Sequence == *e.State {
				a.Passed = true
			}
		}
		if len(snap.Current) == 1 {
			a.Actual = snap.Current[0].Sequence
		}
		if !a.Passed {
			a.Message = describeCurrent(snap)
		}
		out = append(out, a)
	}
	if e.StateName != nil {
		exp, err := res(*e.StateName)
		if err != nil {
			return nil, err
		}
		a := Assertion{Type: "stateName", Expected: exp, Actual: names(snap.Current)}
		for _, c := range snap.Current {
			if c.Name == exp {
				a.Passed = true
			}
		}
		if len(snap.Current) == 1 {
			a.Actual = snap.Current[0].Name
		}
		out = append(out, a)
	}
	if e.Status != nil {
		out = append(out, Assertion{Type: "status", Expected: *e.Status, Actual: snap.Status, Passed: snap.Status == *e.Status})
	}
	if e.Assignee != nil {
		exp, err := res(*e.Assignee)
		if err != nil {
			return nil, err
		}
		a := Assertion{Type: "assignee", Expected: exp, Actual: snap.Open}
		for _, o := range snap.Open {
			if strings.EqualFold(o, exp) {
				a.Passed = true
			}
		}
		out = append(out, a)
	}
	if e.Path != nil {
		visit := map[int]bool{}
		for _, v := range snap.Visited {
			visit[v] = true
		}
		for _, v := range e.Path.Visited {
			out = append(out, Assertion{Type: "path", Op: "visited", Expected: v, Actual: snap.Visited, Passed: visit[v]})
		}
		for _, v := range e.Path.NotVisited {
			out = append(out, Assertion{Type: "path", Op: "notVisited", Expected: v, Actual: snap.Visited, Passed: !visit[v]})
		}
	}
	for _, c := range e.Card {
		a, err := evalCard(c, snap.Card, env)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func evalCard(c CardExpect, card map[string]string, env *Env) (Assertion, error) {
	a := Assertion{Type: "card", Field: c.Field, Op: c.Op}
	val, existe := card[c.Field]
	a.Actual = val
	if !existe {
		a.Actual = nil
	}
	exp, err := Resolve(c.Value, env)
	if err != nil {
		return a, err
	}
	switch c.Op {
	case "empty":
		a.Expected = ""
		a.Passed = strings.TrimSpace(val) == ""
		return a, nil
	case "notEmpty":
		a.Expected = "(preenchido)"
	case "in":
		var lista []string
		for _, x := range c.List {
			r, err := Resolve(x, env)
			if err != nil {
				return a, err
			}
			lista = append(lista, r)
		}
		a.Expected = lista
	default:
		a.Expected = exp
	}
	if !existe {
		a.Message = fmt.Sprintf("campo %s não existe no card", c.Field)
		return a, nil
	}
	switch c.Op {
	case "eq":
		a.Passed = val == exp
	case "ne":
		a.Passed = val != exp
	case "contains":
		a.Passed = strings.Contains(val, exp)
	case "matches":
		re, err := regexp.Compile(exp)
		if err != nil {
			return a, err
		}
		a.Passed = re.MatchString(val)
	case "notEmpty":
		a.Passed = strings.TrimSpace(val) != ""
	case "in":
		for _, x := range a.Expected.([]string) {
			if val == x {
				a.Passed = true
			}
		}
	}
	return a, nil
}

func seqs(cs []StepState) []int {
	out := make([]int, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Sequence)
	}
	return out
}

func names(cs []StepState) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func describeCurrent(s *Snapshot) string {
	if len(s.Current) == 0 {
		return "a solicitação não tem etapa corrente"
	}
	partes := make([]string, 0, len(s.Current))
	for _, c := range s.Current {
		partes = append(partes, strconv.Itoa(c.Sequence)+" (\""+c.Name+"\")")
	}
	return "etapa atual: " + strings.Join(partes, ", ")
}

// FormatAssertion devolve a linha humana "esperado × obtido" de uma asserção.
func FormatAssertion(a Assertion) string {
	nome := a.Type
	if a.Field != "" {
		nome += "." + a.Field
	}
	if a.Type == "path" {
		nome += "." + a.Op
	}
	if a.Message != "" && a.Type == "card" && a.Actual == nil {
		return nome + ": " + a.Message
	}
	linha := fmt.Sprintf("%s: esperado %s, obtido %s", nome, show(a.Expected), show(a.Actual))
	if a.Type == "card" && a.Op != "eq" {
		linha = fmt.Sprintf("%s (%s): esperado %s, obtido %s", nome, a.Op, show(a.Expected), show(a.Actual))
	}
	if a.Message != "" {
		linha += "; " + a.Message
	}
	return linha
}

func show(v any) string {
	switch t := v.(type) {
	case nil:
		return "(ausente)"
	case string:
		return strconv.Quote(t)
	case int:
		return strconv.Itoa(t)
	case []int:
		p := make([]string, len(t))
		for i, x := range t {
			p[i] = strconv.Itoa(x)
		}
		return "[" + strings.Join(p, ", ") + "]"
	case []string:
		p := make([]string, len(t))
		for i, x := range t {
			p[i] = strconv.Quote(x)
		}
		return "[" + strings.Join(p, ", ") + "]"
	}
	return fmt.Sprint(v)
}
