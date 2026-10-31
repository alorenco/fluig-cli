package testrun

// Teste de unidade dos scripts Fluig no Rhino local (ROADMAP §5.18, spec em
// docs/interno/SPEC-TEST-D.md). Este arquivo tem o formato *.unit.json: a
// leitura estrita, a mescla dos defaults e as asserções esperadas. A execução
// mora em unit_run.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// UnitExt é a extensão dos arquivos de teste de unidade.
const UnitExt = ".unit.json"

// UnitFile é um arquivo de teste de unidade validado.
type UnitFile struct {
	Path        string
	Description string
	Cases       []*UnitCase
	// Warnings são avisos de fidelidade da carga (ex.: WK booleana como texto).
	Warnings []string
}

// wkBoolean são as variáveis do getValue que o fluig.d.ts declara como
// boolean (getValuePropertiesBoolean). No servidor elas chegam como
// java.lang.Boolean: o texto "false" NÃO é == false no Rhino. Lacuna achada
// pelo projeto Fluig em 2026-10-31 (beforeTaskSave com WKMobile).
var wkBoolean = map[string]bool{
	"WKIsTransfer": true, "WKMobile": true, "WKIsService": true, "WKManagerMode": true, "WKPrivateDocument": true,
}

// ScriptRef é um script carregado no escopo do caso. Name aparece nas
// mensagens de erro (caminho relativo à raiz do projeto). Source, quando
// preenchido, dispensa a leitura do disco (usado pelo --self-check).
type ScriptRef struct {
	Name   string
	Path   string
	Source string
}

// UnitCase é um caso pronto para execução.
type UnitCase struct {
	Name    string
	Scripts []ScriptRef
	Call    string
	Args    []json.RawMessage
	Setup   UnitSetup
	Expect  *UnitExpect
}

// UnitSetup é o estado dos falsos de um caso (contrato com o prelúdio JS).
type UnitSetup struct {
	Today     []int                      `json:"today,omitempty"` // ano, mês, dia, hora, min, seg
	WK        map[string]json.RawMessage `json:"wk,omitempty"`
	User      *UnitUser                  `json:"user,omitempty"`
	Fields    map[string]string          `json:"fields,omitempty"`
	Children  map[string][]int           `json:"children,omitempty"`
	FormMode  string                     `json:"formMode,omitempty"`
	Mobile    bool                       `json:"mobile,omitempty"`
	CompanyID *int                       `json:"companyId,omitempty"`
	Datasets  map[string]json.RawMessage `json:"datasets,omitempty"`
	Returns   map[string]json.RawMessage `json:"returns,omitempty"`
}

// UnitUser é o usuário corrente do fluigAPI.
type UnitUser struct {
	Login    string `json:"login,omitempty"`
	Code     string `json:"code,omitempty"`
	FullName string `json:"fullName,omitempty"`
	Email    string `json:"email,omitempty"`
}

// UnitExpect são as asserções de um caso.
type UnitExpect struct {
	Return       json.RawMessage
	HasReturn    bool
	ReturnType   string
	Throws       []string // textos que a mensagem contém
	ThrowsAny    bool     // "throws": true
	Form         map[string]*string
	FormOnly     bool
	Card         map[string]*string
	CardOnly     bool
	Log          []UnitLogExpect
	DatasetCalls []UnitDatasetCallExpect
	HasDSCalls   bool
	Calls        []UnitCallExpect
	HTML         []string
}

// UnitLogExpect confere uma linha de log.
type UnitLogExpect struct {
	Level    string `json:"level"`
	Contains string `json:"contains"`
}

// UnitDatasetCallExpect confere uma chamada ao DatasetFactory.getDataset.
type UnitDatasetCallExpect struct {
	Name        string            `json:"name"`
	Constraints map[string]string `json:"constraints"`
}

// UnitCallExpect confere uma chamada a um método do hAPI ou do form.
type UnitCallExpect struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

// --- formato cru ---

type rawUnitFile struct {
	Description string            `json:"description"`
	Scripts     []string          `json:"scripts"`
	Defaults    json.RawMessage   `json:"defaults"`
	Cases       []json.RawMessage `json:"cases"`
}

type rawUnitCommon struct {
	Scripts   []string                   `json:"scripts"`
	Today     *string                    `json:"today"`
	WK        map[string]json.RawMessage `json:"wk"`
	User      *UnitUser                  `json:"user"`
	Form      *rawUnitForm               `json:"form"`
	Datasets  map[string]json.RawMessage `json:"datasets"`
	Returns   map[string]json.RawMessage `json:"returns"`
	CompanyID *int                       `json:"companyId"`
}

type rawUnitForm struct {
	Card     string            `json:"card"`
	Fields   map[string]string `json:"fields"`
	Children map[string][]int  `json:"children"`
	Mode     string            `json:"mode"`
	Mobile   *bool             `json:"mobile"`
}

type rawUnitCase struct {
	rawUnitCommon
	Name      string            `json:"name"`
	Call      string            `json:"call"`
	Event     string            `json:"event"`
	Args      []json.RawMessage `json:"args"`
	EventArgs []json.RawMessage `json:"eventArgs"`
	Expect    json.RawMessage   `json:"expect"`
}

type rawUnitExpect struct {
	Return       json.RawMessage         `json:"return"`
	ReturnType   string                  `json:"returnType"`
	Throws       json.RawMessage         `json:"throws"`
	Form         map[string]any          `json:"form"`
	Card         map[string]any          `json:"card"`
	Log          []UnitLogExpect         `json:"log"`
	DatasetCalls []UnitDatasetCallExpect `json:"datasetCalls"`
	Calls        []UnitCallExpect        `json:"calls"`
	HTML         []string                `json:"html"`
}

// formEvents recebem (form); displayFields recebe também o customHTML.
var formEvents = map[string][]string{
	"validateForm":  {"$form"},
	"inputFields":   {"$form"},
	"enableFields":  {"$form"},
	"displayFields": {"$form", "$customHTML"},
}

var validReturnTypes = map[string]bool{
	"js-string": true, "java-string": true, "number": true, "boolean": true, "null": true,
	"undefined": true, "array": true, "java-array": true, "object": true, "java-object": true,
	"date": true, "function": true,
}

var todayRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2}))?)?$`)

// LoadUnitFile lê e valida um arquivo *.unit.json. root é a raiz do projeto:
// a base dos caminhos de "scripts". As fixtures de "form.card" são relativas à
// pasta do arquivo de teste, como nas suítes de processo.
func LoadUnitFile(path, root string) (*UnitFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: arquivo de teste %q", ErrMissing, path)
		}
		return nil, usageErr("não consegui ler %s: %v", path, err)
	}
	return ParseUnitFile(path, root, data)
}

// ParseUnitFile valida o conteúdo de um arquivo de teste de unidade.
func ParseUnitFile(path, root string, data []byte) (*UnitFile, error) {
	var raw rawUnitFile
	if err := strict(data, &raw, path); err != nil {
		return nil, err
	}
	if len(raw.Cases) == 0 {
		return nil, usageErr("%s: \"cases\" está vazio", path)
	}
	var defs rawUnitCommon
	if !isNull(raw.Defaults) {
		if err := strict(raw.Defaults, &defs, path+": defaults"); err != nil {
			return nil, err
		}
	}
	if len(defs.Scripts) > 0 {
		return nil, usageErr("%s: defaults.scripts não existe; use \"scripts\" na raiz do arquivo", path)
	}
	uf := &UnitFile{Path: path, Description: raw.Description}
	fixtures := map[string]map[string]string{}
	names := map[string]bool{}
	for i, rc := range raw.Cases {
		where := fmt.Sprintf("%s: cases[%d]", path, i)
		var c rawUnitCase
		if err := strict(rc, &c, where); err != nil {
			return nil, err
		}
		if c.Name == "" {
			return nil, usageErr("%s: \"name\" é obrigatório", where)
		}
		where = fmt.Sprintf("%s (%s)", where, c.Name)
		if names[c.Name] {
			return nil, usageErr("%s: nome repetido", where)
		}
		names[c.Name] = true
		uc, err := buildUnitCase(&c, &defs, raw.Scripts, where, path, root, fixtures, &uf.Warnings)
		if err != nil {
			return nil, err
		}
		uf.Cases = append(uf.Cases, uc)
	}
	return uf, nil
}

func buildUnitCase(c *rawUnitCase, d *rawUnitCommon, fileScripts []string, where, path, root string,
	fixtures map[string]map[string]string, warns *[]string) (*UnitCase, error) {
	uc := &UnitCase{Name: c.Name}

	// Chamada: call+args OU event.
	switch {
	case c.Call != "" && c.Event != "":
		return nil, usageErr("%s: use \"call\" OU \"event\", não os dois", where)
	case c.Call == "" && c.Event == "":
		return nil, usageErr("%s: falta \"call\" (a função) ou \"event\" (o evento)", where)
	case c.Event != "":
		if len(c.Args) > 0 {
			return nil, usageErr("%s: \"args\" não combina com \"event\"; os argumentos do evento são fixos (use \"eventArgs\" nos eventos de processo)", where)
		}
		uc.Call = c.Event
		if fa, ok := formEvents[c.Event]; ok {
			if len(c.EventArgs) > 0 {
				return nil, usageErr("%s: o evento %s recebe só o form; \"eventArgs\" não se aplica", where, c.Event)
			}
			for _, a := range fa {
				uc.Args = append(uc.Args, json.RawMessage(fmt.Sprintf("%q", a)))
			}
		} else {
			uc.Args = c.EventArgs
		}
	default:
		if len(c.EventArgs) > 0 {
			return nil, usageErr("%s: \"eventArgs\" só vale com \"event\"; com \"call\", use \"args\"", where)
		}
		uc.Call = c.Call
		uc.Args = c.Args
	}
	if !jsIdent(uc.Call) {
		return nil, usageErr("%s: %q não é nome de função JS", where, uc.Call)
	}

	// Scripts: o do caso substitui o do arquivo.
	scripts := fileScripts
	if len(c.Scripts) > 0 {
		scripts = c.Scripts
	}
	if len(scripts) == 0 {
		return nil, usageErr("%s: nenhum script; informe \"scripts\" no arquivo ou no caso", where)
	}
	for _, s := range scripts {
		p := s
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, filepath.FromSlash(s))
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%w: %s: script %q (procurado em %s)", ErrMissing, where, s, p)
		}
		uc.Scripts = append(uc.Scripts, ScriptRef{Name: filepath.ToSlash(s), Path: p})
	}

	// Falsos: o caso sobrescreve os defaults campo a campo.
	st := &uc.Setup
	today := d.Today
	if c.Today != nil {
		today = c.Today
	}
	if today != nil && *today != "" {
		t, err := parseToday(*today)
		if err != nil {
			return nil, usageErr("%s: today: %v", where, err)
		}
		st.Today = t
	}
	st.WK = mergeRawMap(d.WK, c.WK)
	for k, v := range st.WK {
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return nil, usageErr("%s: wk.%s: %v", where, k, err)
		}
		switch val.(type) {
		case nil, string, bool, float64:
		default:
			return nil, usageErr("%s: wk.%s espera texto, número, true/false ou null", where, k)
		}
		if _, ok := val.(string); ok && wkBoolean[k] {
			*warns = appendOnce(*warns, fmt.Sprintf("%s: wk.%s é texto, mas no servidor é java.lang.Boolean (fluig.d.ts); use true/false, porque \"false\" == false é falso no Rhino", where, k))
		}
	}
	st.User = d.User
	if c.User != nil {
		st.User = c.User
	}
	st.CompanyID = d.CompanyID
	if c.CompanyID != nil {
		st.CompanyID = c.CompanyID
	}
	st.Datasets = mergeRawMap(d.Datasets, c.Datasets)
	for name, spec := range st.Datasets {
		if err := checkDatasetSpec(spec, fmt.Sprintf("%s: datasets.%s", where, name)); err != nil {
			return nil, err
		}
	}
	st.Returns = mergeRawMap(d.Returns, c.Returns)
	for k := range st.Returns {
		if !strings.HasPrefix(k, "hAPI.") && !strings.HasPrefix(k, "form.") && !strings.HasPrefix(k, "DatasetFactory.") {
			return nil, usageErr("%s: returns.%s: a chave é \"hAPI.<método>\" ou \"form.<método>\"", where, k)
		}
	}
	for _, f := range []*rawUnitForm{d.Form, c.Form} {
		if f == nil {
			continue
		}
		if f.Card != "" {
			fx, err := unitFixture(path, f.Card, fixtures)
			if err != nil {
				return nil, fmt.Errorf("%s: form.card: %w", where, err)
			}
			st.Fields = map[string]string{}
			for k, v := range fx {
				st.Fields[k] = v
			}
		}
		for k, v := range f.Fields {
			if st.Fields == nil {
				st.Fields = map[string]string{}
			}
			st.Fields[k] = v
		}
		for k, v := range f.Children {
			if st.Children == nil {
				st.Children = map[string][]int{}
			}
			st.Children[k] = v
		}
		if f.Mode != "" {
			st.FormMode = f.Mode
		}
		if f.Mobile != nil {
			st.Mobile = *f.Mobile
		}
	}

	// A fixture do test new --from-request traz {{today}} e {{actors.eu.*}}.
	// Aqui o "hoje" é o today do caso (ou a data local) e o "eu" é o user.
	if st.Fields != nil {
		env := &Env{Today: time.Now(), Unique: "zzt00000000"}
		if st.Today != nil {
			env.Today = time.Date(st.Today[0], time.Month(st.Today[1]), st.Today[2], 0, 0, 0, 0, time.Local)
		}
		if st.User != nil {
			env.SelfLogin, env.SelfCode = st.User.Login, st.User.Code
		}
		f, err := ResolveMap(st.Fields, env)
		if err != nil {
			return nil, usageErr("%s: form: %v", where, err)
		}
		st.Fields = f
	}

	if isNull(c.Expect) {
		return nil, usageErr("%s: \"expect\" é obrigatório (um caso sem asserção sempre passa)", where)
	}
	e, err := parseUnitExpect(c.Expect, where+": expect")
	if err != nil {
		return nil, err
	}
	uc.Expect = e
	return uc, nil
}

var jsIdentRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func jsIdent(s string) bool { return jsIdentRe.MatchString(s) }

func parseToday(s string) ([]int, error) {
	m := todayRe.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("%q não é data; use AAAA-MM-DD ou AAAA-MM-DDTHH:MM[:SS]", s)
	}
	out := make([]int, 6)
	for i := 1; i <= 6; i++ {
		if m[i] != "" {
			out[i-1], _ = strconv.Atoi(m[i])
		}
	}
	if out[1] < 1 || out[1] > 12 || out[2] < 1 || out[2] > 31 || out[3] > 23 || out[4] > 59 || out[5] > 59 {
		return nil, fmt.Errorf("%q fora do intervalo", s)
	}
	return out, nil
}

func appendOnce(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func mergeRawMap(a, b map[string]json.RawMessage) map[string]json.RawMessage {
	if a == nil && b == nil {
		return nil
	}
	out := map[string]json.RawMessage{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

type rawDatasetRule struct {
	When    map[string]string `json:"when"`
	Columns []string          `json:"columns"`
	Rows    [][]*string       `json:"rows"`
	Null    bool              `json:"null"`
	Error   *string           `json:"error"`
}

// checkDatasetSpec valida o falso de um dataset: null, uma regra ou uma lista
// de regras.
func checkDatasetSpec(spec json.RawMessage, where string) error {
	if isNull(spec) {
		return nil
	}
	var rules []json.RawMessage
	if t := bytes.TrimSpace(spec); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(spec, &rules); err != nil {
			return usageErr("%s: %v", where, err)
		}
	} else {
		rules = []json.RawMessage{spec}
	}
	if len(rules) == 0 {
		return usageErr("%s: lista de regras vazia; use null para o getDataset devolver null", where)
	}
	for i, r := range rules {
		w := fmt.Sprintf("%s[%d]", where, i)
		var rule rawDatasetRule
		if err := strict(r, &rule, w); err != nil {
			return err
		}
		if (rule.Null || rule.Error != nil) && (len(rule.Columns) > 0 || len(rule.Rows) > 0) {
			return usageErr("%s: \"null\"/\"error\" não combinam com columns/rows", w)
		}
		for j, row := range rule.Rows {
			if len(row) != len(rule.Columns) {
				return usageErr("%s: rows[%d] tem %d valores para %d colunas", w, j, len(row), len(rule.Columns))
			}
		}
	}
	return nil
}

func unitFixture(suitePath, rel string, cache map[string]map[string]string) (map[string]string, error) {
	p := rel
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(suitePath), filepath.FromSlash(rel))
	}
	if fx, ok := cache[p]; ok {
		return fx, nil
	}
	fx, err := loadFixture(p)
	if err != nil {
		return nil, err
	}
	cache[p] = fx
	return fx, nil
}

func parseUnitExpect(raw json.RawMessage, where string) (*UnitExpect, error) {
	var r rawUnitExpect
	if err := strict(raw, &r, where); err != nil {
		return nil, err
	}
	e := &UnitExpect{
		ReturnType: r.ReturnType, Log: r.Log, Calls: r.Calls, HTML: r.HTML,
		DatasetCalls: r.DatasetCalls, HasDSCalls: r.DatasetCalls != nil,
	}
	if r.Return != nil {
		e.Return, e.HasReturn = r.Return, true
	}
	if e.ReturnType != "" && !validReturnTypes[e.ReturnType] {
		tipos := make([]string, 0, len(validReturnTypes))
		for k := range validReturnTypes {
			tipos = append(tipos, k)
		}
		sort.Strings(tipos)
		return nil, usageErr("%s: returnType %q inválido; use um de: %s", where, e.ReturnType, strings.Join(tipos, ", "))
	}
	if !isNull(r.Throws) {
		var b bool
		var s string
		var ss []string
		switch {
		case json.Unmarshal(r.Throws, &b) == nil:
			if !b {
				return nil, usageErr("%s: \"throws\": false não existe; sem \"throws\", o caso já exige que a função NÃO lance erro", where)
			}
			e.ThrowsAny = true
		case json.Unmarshal(r.Throws, &s) == nil:
			e.Throws = []string{s}
		case json.Unmarshal(r.Throws, &ss) == nil && len(ss) > 0:
			e.Throws = ss
		default:
			return nil, usageErr("%s: \"throws\" espera true, um texto ou uma lista de textos", where)
		}
		if e.HasReturn || e.ReturnType != "" {
			return nil, usageErr("%s: \"throws\" não combina com \"return\"/\"returnType\" (a função lança, não devolve)", where)
		}
	}
	var err error
	if e.Form, e.FormOnly, err = fieldExpect(r.Form, where+": form"); err != nil {
		return nil, err
	}
	if e.Card, e.CardOnly, err = fieldExpect(r.Card, where+": card"); err != nil {
		return nil, err
	}
	for i, l := range e.Log {
		switch l.Level {
		case "", "info", "warn", "error", "debug", "fatal":
		default:
			return nil, usageErr("%s: log[%d].level %q inválido (info, warn, error, debug, fatal)", where, i, l.Level)
		}
	}
	for i, c := range e.Calls {
		if !strings.Contains(c.Method, ".") {
			return nil, usageErr("%s: calls[%d].method %q: use \"hAPI.<método>\" ou \"form.<método>\"", where, i, c.Method)
		}
	}
	return e, nil
}

// fieldExpect lê {"campo": "valor" | null, "$only": true}.
func fieldExpect(m map[string]any, where string) (map[string]*string, bool, error) {
	if m == nil {
		return nil, false, nil
	}
	out := map[string]*string{}
	only := false
	for k, v := range m {
		if k == "$only" {
			b, ok := v.(bool)
			if !ok {
				return nil, false, usageErr("%s: \"$only\" espera true ou false", where)
			}
			only = b
			continue
		}
		switch t := v.(type) {
		case nil:
			out[k] = nil
		case string:
			s := t
			out[k] = &s
		case float64, bool:
			s := fmt.Sprint(t)
			out[k] = &s
		default:
			return nil, false, usageErr("%s.%s: espera texto ou null", where, k)
		}
	}
	return out, only, nil
}

// UnitFilesInDir lista os *.unit.json de uma pasta e das subpastas, em ordem
// alfabética. A subpasta fixtures/ fica de fora.
func UnitFilesInDir(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && d.Name() == "fixtures" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), UnitExt) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
