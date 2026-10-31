// Package testrun é o executor de cenários do `fluigcli test` (ROADMAP §5.15).
//
// Um teste de processo percorre o motor BPM real pela API, sem navegador: abre
// uma solicitação, move, assume, grava campos e confere o estado depois de cada
// passo. O pacote não importa cobra nem faz I/O de terminal (regra 3 do
// CLAUDE.md): a carga e a validação são puras, e a execução fala com o
// servidor pela interface Engine.
package testrun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Actor é o apelido reservado do usuário da sessão. Na fase A é o único ator.
const ActorSelf = "eu"

// Suite é um arquivo de suíte já carregado e validado.
type Suite struct {
	File        string // caminho como o usuário informou (aparece no relatório)
	Dir         string // pasta do arquivo (base dos caminhos de fixture)
	Process     string
	Server      string
	Description string
	Vars        map[string]string
	Defaults    Defaults
	Scenarios   []*Scenario
}

// Defaults são os valores herdados por todos os cenários.
type Defaults struct {
	Card    string
	Cleanup []string
	Actor   string
}

// Scenario é uma sequência ordenada de passos sobre UMA solicitação.
type Scenario struct {
	Name        string
	Description string
	Card        string            // caminho da fixture (já herdado de defaults)
	CardValues  map[string]string // valores da fixture (ainda com expressões)
	Fields      map[string]string // sobrescritas do cenário (ainda com expressões)
	Steps       []*Step
	Cleanup     []string // "cancel" | "none"
	Index       int      // 1-based, posição no arquivo (base do {{unique}})
}

// Ações de passo.
const (
	ActStart     = "start"
	ActMove      = "move"
	ActAssume    = "assume"
	ActManager   = "manager"
	ActSetFields = "setFields"
	ActCheck     = "check"
	ActWait      = "wait"
	ActCancel    = "cancel"
)

// Step é um passo do cenário, já normalizado (formas curtas expandidas).
type Step struct {
	Index int    // 1-based
	Name  string // rótulo do relatório (gerado quando ausente)
	// Description é orientação livre (o test new grava os TODO aqui). Não
	// muda a execução.
	Description string
	ID          string // referência em {{steps.<id>.nextState}}
	As          string
	Action      string

	Target    int               // start, move, manager, wait(state)
	Fields    map[string]string // start, move, setFields
	Comment   string            // start, move, manager, cancel
	Assignee  string            // start, move
	Movement  int               // move
	Thread    int               // assume, manager
	WaitState int               // wait
	WaitStat  string            // wait
	Timeout   string            // wait (duração Go, ex.: "3m")
	Interval  string            // wait

	Expect *Expect
}

// Expect são as asserções de um passo.
type Expect struct {
	State     *int
	StateName *string
	Status    *string
	Error     *ErrorExpect
	Card      []CardExpect // na ordem do arquivo
	Path      *PathExpect
	Assignee  *string
}

// ErrorExpect é a asserção de erro esperado.
type ErrorExpect struct {
	Contains string
	Code     string
	Matches  string
}

// CardExpect é a asserção de um campo do card.
type CardExpect struct {
	Field string
	Op    string // eq | ne | in | empty | notEmpty | contains | matches
	Value string
	List  []string // só no "in"
}

// PathExpect é a asserção sobre as etapas visitadas.
type PathExpect struct {
	Visited    []int `json:"visited"`
	NotVisited []int `json:"notVisited"`
}

// ErrUsage marca erro de arquivo/uso (exit 2). ErrMissing marca arquivo ou
// fixture inexistente (exit 4). A CLI traduz.
var (
	ErrUsage   = errors.New("suíte inválida")
	ErrMissing = errors.New("arquivo não encontrado")
)

func usageErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUsage, fmt.Sprintf(format, args...))
}

// --- formato cru (JSON) ---

type rawSuite struct {
	Process     string            `json:"process"`
	Server      string            `json:"server"`
	Description string            `json:"description"`
	Actors      map[string]string `json:"actors"`
	Vars        map[string]string `json:"vars"`
	Holidays    json.RawMessage   `json:"holidays"`
	Defaults    *rawDefaults      `json:"defaults"`
	Scenarios   []json.RawMessage `json:"scenarios"`
}

type rawDefaults struct {
	Card    string            `json:"card"`
	Cleanup []json.RawMessage `json:"cleanup"`
	Actor   string            `json:"actor"`
}

type rawScenario struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Card        string            `json:"card"`
	Fields      map[string]any    `json:"fields"`
	Steps       []json.RawMessage `json:"steps"`
	Cleanup     []json.RawMessage `json:"cleanup"`
}

type rawStep struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ID          string `json:"id"`
	As          string `json:"as"`

	Start     json.RawMessage `json:"start"`
	Move      json.RawMessage `json:"move"`
	Assume    json.RawMessage `json:"assume"`
	Manager   json.RawMessage `json:"manager"`
	SetFields json.RawMessage `json:"setFields"`
	Check     json.RawMessage `json:"check"`
	Wait      json.RawMessage `json:"wait"`
	Cancel    json.RawMessage `json:"cancel"`

	Expect json.RawMessage `json:"expect"`
}

type rawMove struct {
	Target   int            `json:"target"`
	Fields   map[string]any `json:"fields"`
	Comment  string         `json:"comment"`
	Assignee string         `json:"assignee"`
	Movement int            `json:"movement"`
}

type rawManager struct {
	Target  int             `json:"target"`
	Comment string          `json:"comment"`
	Thread  int             `json:"thread"`
	Fields  json.RawMessage `json:"fields"` // proibido: aceito só para a mensagem certa
}

type rawAssume struct {
	Thread int `json:"thread"`
}

type rawWait struct {
	State    int    `json:"state"`
	Status   string `json:"status"`
	Timeout  string `json:"timeout"`
	Interval string `json:"interval"`
}

type rawCancel struct {
	Comment string `json:"comment"`
}

type rawExpect struct {
	State       *int            `json:"state"`
	StateName   *string         `json:"stateName"`
	Status      *string         `json:"status"`
	Error       json.RawMessage `json:"error"`
	Card        json.RawMessage `json:"card"`
	Path        json.RawMessage `json:"path"`
	Assignee    *string         `json:"assignee"`
	Observation json.RawMessage `json:"observation"`
	DB          json.RawMessage `json:"db"`
	Dataset     json.RawMessage `json:"dataset"`
}

// strict decodifica recusando chave desconhecida. O caminho entra na mensagem:
// um erro de digitação ("expcet") não pode virar teste que passa sem conferir.
func strict(raw []byte, v any, path string) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "json: unknown field ") {
			campo := strings.TrimPrefix(msg, "json: unknown field ")
			return usageErr("%s: chave desconhecida %s", path, campo)
		}
		return usageErr("%s: %s", path, strings.TrimPrefix(msg, "json: "))
	}
	return nil
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null"
}

// LoadOptions são as entradas externas da carga.
type LoadOptions struct {
	// Vars vem do --var e vence o "vars" do arquivo.
	Vars map[string]string
}

// LoadSuite lê, desserializa e valida um arquivo de suíte. A validação é
// local: nenhuma chamada ao servidor.
func LoadSuite(path string, opts LoadOptions) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: suíte %q", ErrMissing, path)
		}
		return nil, usageErr("não consegui ler %s: %v", path, err)
	}
	return ParseSuite(path, data, opts)
}

// ParseSuite valida o conteúdo de uma suíte. path é o nome usado nas mensagens
// e a base dos caminhos de fixture.
func ParseSuite(path string, data []byte, opts LoadOptions) (*Suite, error) {
	var rs rawSuite
	if err := strict(data, &rs, path); err != nil {
		return nil, err
	}
	s := &Suite{
		File:        path,
		Dir:         filepath.Dir(path),
		Process:     strings.TrimSpace(rs.Process),
		Server:      rs.Server,
		Description: rs.Description,
		Vars:        map[string]string{},
	}
	if s.Process == "" {
		return nil, usageErr("%s: a chave \"process\" é obrigatória (processId no servidor)", path)
	}
	for k, v := range rs.Vars {
		s.Vars[k] = v
	}
	for k, v := range opts.Vars {
		s.Vars[k] = v
	}
	// Fase A: só o ator "eu" (decisão da spec, §5.3).
	if len(rs.Actors) > 0 {
		return nil, usageErr("%s: \"actors\" ainda não é suportado. Nesta versão só existe o ator %q "+
			"(o usuário da sessão); atores entram na fase B. Use os passos assume e manager para agir "+
			"nas etapas de outros usuários", path, ActorSelf)
	}
	if !isNull(rs.Holidays) {
		return nil, usageErr("%s: \"holidays\" ainda não é suportado (entra com {{workday}} na fase B)", path)
	}
	if rs.Defaults != nil {
		s.Defaults.Card = rs.Defaults.Card
		s.Defaults.Actor = rs.Defaults.Actor
		if err := checkActor(rs.Defaults.Actor, path+".defaults.actor"); err != nil {
			return nil, err
		}
		cl, err := parseCleanup(rs.Defaults.Cleanup, path+".defaults.cleanup")
		if err != nil {
			return nil, err
		}
		s.Defaults.Cleanup = cl
	}
	if len(rs.Scenarios) == 0 {
		return nil, usageErr("%s: a suíte precisa de pelo menos um cenário em \"scenarios\"", path)
	}
	nomes := map[string]bool{}
	fixtures := map[string]map[string]string{}
	for i, raw := range rs.Scenarios {
		sp := fmt.Sprintf("%s: scenarios[%d]", path, i)
		sc, err := parseScenario(raw, sp, s, fixtures)
		if err != nil {
			return nil, err
		}
		sc.Index = i + 1
		if nomes[sc.Name] {
			return nil, usageErr("%s: nome de cenário repetido %q (o nome é o alvo do --scenario)", sp, sc.Name)
		}
		nomes[sc.Name] = true
		s.Scenarios = append(s.Scenarios, sc)
	}
	if err := validateExpressions(s); err != nil {
		return nil, err
	}
	return s, nil
}

func checkActor(as, path string) error {
	if as == "" || as == ActorSelf {
		return nil
	}
	return usageErr("%s: ator %q desconhecido. Nesta versão só existe o ator %q; atores entram na fase B",
		path, as, ActorSelf)
}

func parseCleanup(raws []json.RawMessage, path string) ([]string, error) {
	if raws == nil {
		return nil, nil
	}
	out := make([]string, 0, len(raws))
	for i, raw := range raws {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, usageErr("%s[%d]: limpeza por dataset ainda não é suportada (fase B); use \"cancel\" ou \"none\"", path, i)
		}
		if s != "cancel" && s != "none" {
			return nil, usageErr("%s[%d]: ação de limpeza %q desconhecida; use \"cancel\" ou \"none\"", path, i, s)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseScenario(raw json.RawMessage, path string, s *Suite, fixtures map[string]map[string]string) (*Scenario, error) {
	var rsc rawScenario
	if err := strict(raw, &rsc, path); err != nil {
		return nil, err
	}
	sc := &Scenario{Name: strings.TrimSpace(rsc.Name), Description: rsc.Description}
	if sc.Name == "" {
		return nil, usageErr("%s: a chave \"name\" é obrigatória", path)
	}
	path = fmt.Sprintf("%s (%q)", path, sc.Name)

	sc.Card = rsc.Card
	if sc.Card == "" {
		sc.Card = s.Defaults.Card
	}
	if sc.Card != "" {
		vals, ok := fixtures[sc.Card]
		if !ok {
			var err error
			vals, err = loadFixture(filepath.Join(s.Dir, sc.Card))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			fixtures[sc.Card] = vals
		}
		sc.CardValues = vals
	}
	f, err := scalarFields(rsc.Fields, path+".fields")
	if err != nil {
		return nil, err
	}
	sc.Fields = f

	cl, err := parseCleanup(rsc.Cleanup, path+".cleanup")
	if err != nil {
		return nil, err
	}
	switch {
	case cl != nil:
		sc.Cleanup = cl
	case s.Defaults.Cleanup != nil:
		sc.Cleanup = s.Defaults.Cleanup
	default:
		sc.Cleanup = []string{"cancel"}
	}

	if len(rsc.Steps) == 0 {
		return nil, usageErr("%s: o cenário precisa de pelo menos um passo em \"steps\"", path)
	}
	ids := map[string]bool{}
	for i, rawStepJSON := range rsc.Steps {
		stp := fmt.Sprintf("%s.steps[%d]", path, i)
		st, err := parseStep(rawStepJSON, stp)
		if err != nil {
			return nil, err
		}
		st.Index = i + 1
		if st.As == "" {
			st.As = s.Defaults.Actor
		}
		if st.As == "" {
			st.As = ActorSelf
		}
		if i == 0 && st.Action != ActStart {
			return nil, usageErr("%s: o primeiro passo do cenário deve ser \"start\" (encontrado %q)", stp, st.Action)
		}
		if i > 0 && st.Action == ActStart {
			return nil, usageErr("%s: \"start\" só pode ser o primeiro passo (cada cenário tem uma solicitação)", stp)
		}
		if st.ID != "" {
			if ids[st.ID] {
				return nil, usageErr("%s: id de passo repetido %q", stp, st.ID)
			}
			ids[st.ID] = true
		}
		sc.Steps = append(sc.Steps, st)
	}
	return sc, nil
}

// loadFixture lê um arquivo plano {"campo": "valor"}.
func loadFixture(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: fixture %q", ErrMissing, path)
		}
		return nil, usageErr("não consegui ler a fixture %s: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, usageErr("fixture %s: JSON inválido (%v); esperado um objeto plano {\"campo\": \"valor\"}", path, err)
	}
	if _, ok := raw["$extends"]; ok {
		return nil, usageErr("fixture %s: \"$extends\" ainda não é suportado (fase B)", path)
	}
	return scalarFields(raw, "fixture "+path)
}

// scalarFields converte valores escalares em string (mesma regra do
// --fields-file). Objeto ou lista = erro: o card é plano.
func scalarFields(raw map[string]any, path string) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch t := v.(type) {
		case string:
			out[k] = t
		case json.Number:
			out[k] = t.String()
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			out[k] = strconv.FormatBool(t)
		case nil:
			out[k] = ""
		default:
			return nil, usageErr("%s: o campo %q precisa de valor simples (texto, número ou booleano). "+
				"Linha de tabela filha usa a chave campo___N", path, k)
		}
	}
	return out, nil
}

// trueOrObject interpreta as formas curtas: true (forma curta), objeto (forma
// longa). Devolve isObject.
func trueOrObject(raw json.RawMessage, path, action string) (bool, error) {
	t := bytes.TrimSpace(raw)
	if string(t) == "true" {
		return false, nil
	}
	if len(t) > 0 && t[0] == '{' {
		return true, nil
	}
	return false, usageErr("%s: \"%s\" aceita true ou um objeto", path, action)
}

func parseStep(raw json.RawMessage, path string) (*Step, error) {
	var rs rawStep
	if err := strict(raw, &rs, path); err != nil {
		return nil, err
	}
	st := &Step{Name: rs.Name, Description: rs.Description, ID: rs.ID, As: rs.As}
	if err := checkActor(rs.As, path+".as"); err != nil {
		return nil, err
	}
	acoes := []struct {
		nome string
		raw  json.RawMessage
	}{
		{ActStart, rs.Start}, {ActMove, rs.Move}, {ActAssume, rs.Assume}, {ActManager, rs.Manager},
		{ActSetFields, rs.SetFields}, {ActCheck, rs.Check}, {ActWait, rs.Wait}, {ActCancel, rs.Cancel},
	}
	var achada json.RawMessage
	for _, a := range acoes {
		if isNull(a.raw) {
			continue
		}
		if st.Action != "" {
			return nil, usageErr("%s: o passo tem mais de uma ação (%s e %s); use um passo para cada", path, st.Action, a.nome)
		}
		st.Action, achada = a.nome, a.raw
	}
	if st.Action == "" {
		return nil, usageErr("%s: passo sem ação; informe uma das chaves start, move, assume, manager, setFields, check, wait ou cancel", path)
	}
	ap := path + "." + st.Action
	switch st.Action {
	case ActStart:
		obj, err := trueOrObject(achada, ap, st.Action)
		if err != nil {
			return nil, err
		}
		if obj {
			var m rawMove
			if err := strict(achada, &m, ap); err != nil {
				return nil, err
			}
			if m.Movement != 0 {
				return nil, usageErr("%s: \"movement\" não se aplica ao start", ap)
			}
			if st.Fields, err = scalarFields(m.Fields, ap+".fields"); err != nil {
				return nil, err
			}
			st.Target, st.Comment, st.Assignee = m.Target, m.Comment, m.Assignee
		}
	case ActMove:
		var n int
		if json.Unmarshal(achada, &n) == nil {
			st.Target = n
		} else {
			var m rawMove
			if err := strict(achada, &m, ap); err != nil {
				return nil, err
			}
			f, err := scalarFields(m.Fields, ap+".fields")
			if err != nil {
				return nil, err
			}
			st.Target, st.Fields, st.Comment, st.Assignee, st.Movement = m.Target, f, m.Comment, m.Assignee, m.Movement
		}
		if st.Target <= 0 {
			return nil, usageErr("%s: informe a etapa de destino (\"move\": 110 ou {\"target\": 110})", ap)
		}
	case ActManager:
		var n int
		if json.Unmarshal(achada, &n) == nil {
			st.Target = n
		} else {
			var m rawManager
			if err := strict(achada, &m, ap); err != nil {
				return nil, err
			}
			if !isNull(m.Fields) {
				return nil, usageErr("%s: o modo gestor não aceita \"fields\" (limite do SOAP saveAndSendTask). "+
					"Grave os campos com um passo setFields antes do manager", ap)
			}
			st.Target, st.Comment, st.Thread = m.Target, m.Comment, m.Thread
		}
		if st.Target <= 0 {
			return nil, usageErr("%s: informe a etapa de destino (\"manager\": 110 ou {\"target\": 110})", ap)
		}
	case ActAssume:
		obj, err := trueOrObject(achada, ap, st.Action)
		if err != nil {
			return nil, err
		}
		if obj {
			var m rawAssume
			if err := strict(achada, &m, ap); err != nil {
				return nil, err
			}
			st.Thread = m.Thread
		}
	case ActSetFields:
		dec := json.NewDecoder(bytes.NewReader(achada))
		dec.UseNumber()
		var m map[string]any
		if err := dec.Decode(&m); err != nil || m == nil {
			return nil, usageErr("%s: setFields espera um objeto plano {\"campo\": \"valor\"}", ap)
		}
		if len(m) == 0 {
			return nil, usageErr("%s: setFields sem campos", ap)
		}
		f, err := scalarFields(m, ap)
		if err != nil {
			return nil, err
		}
		st.Fields = f
	case ActCheck:
		if string(bytes.TrimSpace(achada)) != "true" {
			return nil, usageErr("%s: use \"check\": true", ap)
		}
		if isNull(rs.Expect) {
			return nil, usageErr("%s: um passo check sem \"expect\" não confere nada", path)
		}
	case ActWait:
		var m rawWait
		if err := strict(achada, &m, ap); err != nil {
			return nil, err
		}
		if m.State == 0 && m.Status == "" {
			return nil, usageErr("%s: informe \"state\" e/ou \"status\" para esperar", ap)
		}
		if m.Status != "" && !validStatus(m.Status) {
			return nil, usageErr("%s: status %q inválido; use OPEN, FINALIZED ou CANCELED", ap, m.Status)
		}
		for _, d := range []string{m.Timeout, m.Interval} {
			if d != "" {
				if _, err := parseDuration(d); err != nil {
					return nil, usageErr("%s: duração %q inválida (ex.: \"3m\", \"5s\")", ap, d)
				}
			}
		}
		st.WaitState, st.WaitStat, st.Timeout, st.Interval = m.State, strings.ToUpper(m.Status), m.Timeout, m.Interval
	case ActCancel:
		obj, err := trueOrObject(achada, ap, st.Action)
		if err != nil {
			return nil, err
		}
		if obj {
			var m rawCancel
			if err := strict(achada, &m, ap); err != nil {
				return nil, err
			}
			st.Comment = m.Comment
		}
	}
	if !isNull(rs.Expect) {
		e, err := parseExpect(rs.Expect, path+".expect")
		if err != nil {
			return nil, err
		}
		st.Expect = e
	}
	if st.Name == "" {
		st.Name = defaultStepName(st)
	}
	return st, nil
}

func validStatus(s string) bool {
	switch strings.ToUpper(s) {
	case "OPEN", "FINALIZED", "CANCELED":
		return true
	}
	return false
}

func defaultStepName(st *Step) string {
	switch st.Action {
	case ActMove, ActManager:
		return fmt.Sprintf("%s → %d", st.Action, st.Target)
	case ActStart:
		if st.Target > 0 {
			return fmt.Sprintf("start → %d", st.Target)
		}
	case ActWait:
		var partes []string
		if st.WaitState > 0 {
			partes = append(partes, strconv.Itoa(st.WaitState))
		}
		if st.WaitStat != "" {
			partes = append(partes, st.WaitStat)
		}
		return "wait " + strings.Join(partes, " ")
	}
	return st.Action
}

func parseExpect(raw json.RawMessage, path string) (*Expect, error) {
	var re rawExpect
	if err := strict(raw, &re, path); err != nil {
		return nil, err
	}
	for nome, v := range map[string]json.RawMessage{"observation": re.Observation, "db": re.DB, "dataset": re.Dataset} {
		if !isNull(v) {
			return nil, usageErr("%s: a asserção %q ainda não é suportada (fase B)", path, nome)
		}
	}
	e := &Expect{State: re.State, StateName: re.StateName, Assignee: re.Assignee}
	if re.Status != nil {
		up := strings.ToUpper(*re.Status)
		if !validStatus(up) {
			return nil, usageErr("%s.status: %q inválido; use OPEN, FINALIZED ou CANCELED", path, *re.Status)
		}
		e.Status = &up
	}
	if !isNull(re.Error) {
		ee, err := parseErrorExpect(re.Error, path+".error")
		if err != nil {
			return nil, err
		}
		e.Error = ee
	}
	if !isNull(re.Card) {
		ce, err := parseCardExpect(re.Card, path+".card")
		if err != nil {
			return nil, err
		}
		e.Card = ce
	}
	if !isNull(re.Path) {
		var pe PathExpect
		if err := strict(re.Path, &pe, path+".path"); err != nil {
			return nil, err
		}
		if len(pe.Visited) == 0 && len(pe.NotVisited) == 0 {
			return nil, usageErr("%s.path: informe \"visited\" e/ou \"notVisited\"", path)
		}
		e.Path = &pe
	}
	return e, nil
}

func parseErrorExpect(raw json.RawMessage, path string) (*ErrorExpect, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if strings.TrimSpace(s) == "" {
			return nil, usageErr("%s: texto esperado vazio", path)
		}
		return &ErrorExpect{Contains: s}, nil
	}
	var ee struct {
		Contains string `json:"contains"`
		Code     string `json:"code"`
		Matches  string `json:"matches"`
	}
	if err := strict(raw, &ee, path); err != nil {
		return nil, err
	}
	if ee.Contains == "" && ee.Code == "" && ee.Matches == "" {
		return nil, usageErr("%s: informe \"contains\", \"code\" ou \"matches\"", path)
	}
	if ee.Matches != "" {
		if _, err := compileRegex(ee.Matches); err != nil {
			return nil, usageErr("%s.matches: expressão regular inválida: %v", path, err)
		}
	}
	return &ErrorExpect{Contains: ee.Contains, Code: ee.Code, Matches: ee.Matches}, nil
}

var cardOps = []string{"eq", "ne", "in", "empty", "notEmpty", "contains", "matches"}

func parseCardExpect(raw json.RawMessage, path string) ([]CardExpect, error) {
	// Ordem do arquivo: o relatório lista as asserções na ordem escrita.
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, usageErr("%s: espera um objeto {\"campo\": valor}", path)
	}
	var out []CardExpect
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, usageErr("%s: %v", path, err)
		}
		field := kt.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, usageErr("%s.%s: %v", path, field, err)
		}
		ce, err := parseCardOp(field, v, path+"."+field)
		if err != nil {
			return nil, err
		}
		out = append(out, ce)
	}
	if len(out) == 0 {
		return nil, usageErr("%s: nenhum campo para conferir", path)
	}
	return out, nil
}

func parseCardOp(field string, v json.RawMessage, path string) (CardExpect, error) {
	ce := CardExpect{Field: field}
	t := bytes.TrimSpace(v)
	if len(t) == 0 || t[0] != '{' {
		var s any
		if err := json.Unmarshal(t, &s); err != nil {
			return ce, usageErr("%s: valor inválido", path)
		}
		val, err := scalarFields(map[string]any{field: s}, path)
		if err != nil {
			return ce, err
		}
		ce.Op, ce.Value = "eq", val[field]
		return ce, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(t, &m); err != nil {
		return ce, usageErr("%s: %v", path, err)
	}
	if len(m) != 1 {
		return ce, usageErr("%s: o comparador precisa de exatamente um operador (%s)", path, strings.Join(cardOps, ", "))
	}
	for op, arg := range m {
		ce.Op = op
		switch op {
		case "eq", "ne", "contains", "matches":
			var s any
			if err := json.Unmarshal(arg, &s); err != nil {
				return ce, usageErr("%s.%s: valor inválido", path, op)
			}
			val, err := scalarFields(map[string]any{field: s}, path+"."+op)
			if err != nil {
				return ce, err
			}
			ce.Value = val[field]
			if op == "matches" {
				if _, err := compileRegex(ce.Value); err != nil {
					return ce, usageErr("%s.matches: expressão regular inválida: %v", path, err)
				}
			}
		case "in":
			var lista []any
			if err := json.Unmarshal(arg, &lista); err != nil || len(lista) == 0 {
				return ce, usageErr("%s.in: espera uma lista não vazia", path)
			}
			for i, x := range lista {
				val, err := scalarFields(map[string]any{field: x}, fmt.Sprintf("%s.in[%d]", path, i))
				if err != nil {
					return ce, err
				}
				ce.List = append(ce.List, val[field])
			}
		case "empty", "notEmpty":
			var b bool
			if err := json.Unmarshal(arg, &b); err != nil || !b {
				return ce, usageErr("%s.%s: use %q: true", path, op, op)
			}
		default:
			return ce, usageErr("%s: operador %q desconhecido; use %s", path, op, strings.Join(cardOps, ", "))
		}
	}
	return ce, nil
}
