package testrun

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alorenco/fluig-cli/internal/audit"
)

//go:embed unit_prelude.js
var unitPrelude string

//go:embed unit_boot.js
var unitBoot string

// UnitExecutor roda o bootstrap no motor: boot é o caminho do unit_boot.js e
// plan o do plano. O bootstrap grava os resultados no arquivo "out" do plano.
// A CLI usa o Rhino real (internal/rhino); os testes usam um falso.
type UnitExecutor func(ctx context.Context, boot, plan string) error

// ErrUnitTimeout marca o estouro do tempo da execução. ErrUnitEngine marca o
// motor que não chegou a rodar nenhum caso.
var (
	ErrUnitTimeout = errors.New("tempo esgotado")
	ErrUnitEngine  = errors.New("o motor não rodou")
)

// Resultados de caso.
const (
	UnitPassed  = "passed"
	UnitFailed  = "failed"
	UnitErrored = "errored"
)

// UnitEngine descreve o motor usado (contrato --json).
type UnitEngine struct {
	Rhino           string `json:"rhino"`
	Source          string `json:"source"` // maven | cache | flag
	Jar             string `json:"jar"`
	Java            string `json:"java"`
	LanguageVersion int    `json:"languageVersion"`
}

// UnitReport é o relatório do test unit (contrato --json).
type UnitReport struct {
	Engine  UnitEngine       `json:"engine"`
	Files   []UnitFileResult `json:"files"`
	Summary UnitSummary      `json:"summary"`
}

// UnitFileResult é o resultado de um arquivo.
type UnitFileResult struct {
	Path        string           `json:"path"`
	Description string           `json:"description,omitempty"`
	Cases       []UnitCaseResult `json:"cases"`
}

// UnitCaseResult é o resultado de um caso.
type UnitCaseResult struct {
	Name       string      `json:"name"`
	Status     string      `json:"status"`
	DurationMs float64     `json:"durationMs"`
	Error      string      `json:"error,omitempty"`
	Assertions []Assertion `json:"assertions"`
	Logs       []UnitLog   `json:"logs"`
}

// UnitLog é uma linha de log do script.
type UnitLog struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// UnitSummary resume a execução.
type UnitSummary struct {
	Cases      int   `json:"cases"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Errored    int   `json:"errored"`
	DurationMs int64 `json:"durationMs"`
}

// --- plano (contrato com o unit_boot.js) ---

type unitPlan struct {
	Out     string           `json:"out"`
	Prelude string           `json:"prelude"`
	Scripts []unitPlanScript `json:"scripts"`
	Cases   []unitPlanCase   `json:"cases"`
}

type unitPlanScript struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type unitPlanCase struct {
	File    int           `json:"file"`
	Case    int           `json:"case"`
	Scripts []int         `json:"scripts"`
	Setup   unitPlanSetup `json:"setup"`
}

type unitPlanSetup struct {
	UnitSetup
	Call        string            `json:"call"`
	Args        []json.RawMessage `json:"args"`
	FormMembers []string          `json:"formMembers"`
	HAPIMembers []string          `json:"hapiMembers"`
}

// --- observações (saída do bootstrap) ---

type unitObs struct {
	File         int                `json:"file"`
	Case         int                `json:"case"`
	LoadError    *unitLoadError     `json:"loadError"`
	SetupError   string             `json:"setupError"`
	Return       json.RawMessage    `json:"return"`
	ReturnType   string             `json:"returnType"`
	Thrown       *unitThrown        `json:"thrown"`
	DurationMs   float64            `json:"durationMs"`
	Form         map[string]*string `json:"form"`
	Card         map[string]*string `json:"card"`
	Logs         []UnitLog          `json:"logs"`
	DatasetCalls []unitObsDSCall    `json:"datasetCalls"`
	Calls        []unitObsCall      `json:"calls"`
	HTML         []string           `json:"html"`
	Missing      []string           `json:"missing"`
}

type unitLoadError struct {
	Phase   string `json:"phase"`
	Script  string `json:"script"`
	Message string `json:"message"`
	Line    int    `json:"line"`
	Compile bool   `json:"compile"`
}

type unitThrown struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Message   string `json:"message"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	JavaClass string `json:"javaClass"`
}

type unitObsDSCall struct {
	Name        string `json:"name"`
	Constraints []struct {
		Field   string  `json:"field"`
		Initial *string `json:"initial"`
	} `json:"constraints"`
}

type unitObsCall struct {
	Target string            `json:"target"`
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

// RunUnit monta o plano, executa o motor e avalia as asserções. dir é uma
// pasta temporária vazia (o chamador apaga).
func RunUnit(ctx context.Context, files []*UnitFile, exec UnitExecutor, dir string) (*UnitReport, error) {
	plan := unitPlan{Out: filepath.Join(dir, "resultados.ndjson"), Prelude: unitPrelude}
	scriptIdx := map[string]int{}
	formMembers := audit.CatalogMembers("form")
	hapiMembers := audit.CatalogMembers("hAPI")
	for fi, f := range files {
		for ci, c := range f.Cases {
			pc := unitPlanCase{File: fi, Case: ci, Setup: unitPlanSetup{
				UnitSetup: c.Setup, Call: c.Call, Args: c.Args,
				FormMembers: formMembers, HAPIMembers: hapiMembers,
			}}
			if pc.Setup.Args == nil {
				pc.Setup.Args = []json.RawMessage{}
			}
			for _, s := range c.Scripts {
				key := s.Path + "\x00" + s.Name
				if s.Source != "" {
					key = "inline\x00" + s.Name + "\x00" + s.Source
				}
				ix, ok := scriptIdx[key]
				if !ok {
					src := s.Source
					if src == "" {
						b, err := os.ReadFile(s.Path)
						if err != nil {
							return nil, fmt.Errorf("%w: script %s: %v", ErrMissing, s.Name, err)
						}
						src = decodeScript(b)
					}
					ix = len(plan.Scripts)
					plan.Scripts = append(plan.Scripts, unitPlanScript{Name: s.Name, Source: src})
					scriptIdx[key] = ix
				}
				pc.Scripts = append(pc.Scripts, ix)
			}
			plan.Cases = append(plan.Cases, pc)
		}
	}
	planPath := filepath.Join(dir, "plano.json")
	bootPath := filepath.Join(dir, "unit_boot.js")
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(planPath, data, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(bootPath, []byte(unitBoot), 0o600); err != nil {
		return nil, err
	}

	execErr := exec(ctx, bootPath, planPath)
	obs, err := readUnitObs(plan.Out)
	if err != nil && execErr == nil {
		return nil, err
	}
	// A JVM caiu antes do primeiro caso: o problema é o motor (jar sem o shell
	// do Rhino, Java quebrado), não os casos. Um erro só, em vez de um por caso.
	if execErr != nil && len(obs) == 0 && !errors.Is(execErr, ErrUnitTimeout) {
		return nil, fmt.Errorf("%w: %v", ErrUnitEngine, execErr)
	}

	rep := &UnitReport{}
	for fi, f := range files {
		fr := UnitFileResult{Path: f.Path, Description: f.Description}
		for ci, c := range f.Cases {
			o, ok := obs[[2]int{fi, ci}]
			var cr UnitCaseResult
			switch {
			case ok:
				cr = evalUnitCase(c, o)
			case execErr != nil:
				cr = UnitCaseResult{Name: c.Name, Status: UnitErrored, Assertions: []Assertion{}, Logs: []UnitLog{}}
				if errors.Is(execErr, ErrUnitTimeout) {
					cr.Error = "tempo esgotado: o motor foi interrompido antes deste caso terminar (laço infinito?)"
				} else {
					cr.Error = "o motor terminou com erro antes deste caso: " + execErr.Error()
				}
			default:
				cr = UnitCaseResult{Name: c.Name, Status: UnitErrored, Error: "o motor não devolveu resultado para este caso",
					Assertions: []Assertion{}, Logs: []UnitLog{}}
			}
			rep.Summary.Cases++
			switch cr.Status {
			case UnitPassed:
				rep.Summary.Passed++
			case UnitFailed:
				rep.Summary.Failed++
			default:
				rep.Summary.Errored++
			}
			fr.Cases = append(fr.Cases, cr)
		}
		rep.Files = append(rep.Files, fr)
	}
	return rep, nil
}

// decodeScript lê o fonte como UTF-8. Um arquivo em CP-1252/Latin-1 (comum
// em projeto Fluig antigo) é convertido byte a byte.
func decodeScript(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func readUnitObs(path string) (map[[2]int]*unitObs, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("o motor não gravou resultados: %w", err)
	}
	defer f.Close()
	out := map[[2]int]*unitObs{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 64<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var o unitObs
		if err := json.Unmarshal(line, &o); err != nil {
			return nil, fmt.Errorf("resultado ilegível do motor: %v", err)
		}
		out[[2]int{o.File, o.Case}] = &o
	}
	return out, sc.Err()
}

var notDefinedRe = regexp.MustCompile(`^"?([\w$]+)"? is not defined`)

// evalUnitCase avalia todas as asserções de um caso.
func evalUnitCase(c *UnitCase, o *unitObs) UnitCaseResult {
	cr := UnitCaseResult{Name: c.Name, DurationMs: o.DurationMs, Logs: o.Logs, Assertions: []Assertion{}}
	if cr.Logs == nil {
		cr.Logs = []UnitLog{}
	}
	if o.LoadError != nil {
		le := o.LoadError
		cr.Status = UnitErrored
		switch {
		case le.Phase == "harness":
			cr.Error = "falha do fluigcli ao preparar o caso: " + le.Message
		case le.Compile:
			cr.Error = fmt.Sprintf("o script %s não compila no Rhino (linha %d): %s", le.Script, le.Line, le.Message)
		default:
			cr.Error = fmt.Sprintf("erro ao carregar %s (linha %d): %s", le.Script, le.Line, le.Message)
		}
		return cr
	}
	if o.SetupError != "" {
		cr.Status, cr.Error = UnitErrored, o.SetupError
		return cr
	}
	if len(o.Missing) > 0 && o.Thrown != nil {
		cr.Status = UnitErrored
		cr.Error = "falso ausente: " + o.Thrown.Message
		return cr
	}
	e := c.Expect
	add := func(a Assertion) { cr.Assertions = append(cr.Assertions, a) }

	// Erro lançado.
	thrownMsg := ""
	if o.Thrown != nil {
		thrownMsg = NormalizeServerText(o.Thrown.Message)
	}
	switch {
	case e.ThrowsAny || len(e.Throws) > 0:
		a := Assertion{Type: "throws", Actual: nilIfEmpty(o.Thrown != nil, thrownMsg)}
		if e.ThrowsAny {
			a.Expected = "(qualquer erro)"
			a.Passed = o.Thrown != nil
		} else {
			a.Expected = e.Throws
			a.Passed = o.Thrown != nil
			for _, t := range e.Throws {
				if !strings.Contains(strings.ToLower(thrownMsg), strings.ToLower(NormalizeServerText(t))) {
					a.Passed = false
					if o.Thrown != nil {
						a.Message = "a mensagem não contém " + fmt.Sprintf("%q", t)
					}
					break
				}
			}
		}
		if o.Thrown == nil {
			a.Message = "a função terminou sem erro"
		}
		add(a)
	case o.Thrown != nil:
		a := Assertion{Type: "throws", Expected: "(sem erro)", Actual: thrownMsg, Message: thrownWhere(o.Thrown)}
		if m := notDefinedRe.FindStringSubmatch(o.Thrown.Message); m != nil {
			a.Message = strings.TrimPrefix(a.Message+"; ", "; ") + fmt.Sprintf("a global %s não existe no teste: é um falso que falta ou um script que não foi carregado", m[1])
		}
		add(a)
	}

	if e.HasReturn {
		var exp, got any
		_ = json.Unmarshal(e.Return, &exp)
		if o.Return != nil {
			_ = json.Unmarshal(o.Return, &got)
		}
		add(Assertion{Type: "return", Expected: exp, Actual: got, Passed: o.Thrown == nil && jsonEqual(exp, got)})
	}
	if e.ReturnType != "" {
		add(Assertion{Type: "returnType", Expected: e.ReturnType, Actual: o.ReturnType, Passed: e.ReturnType == o.ReturnType})
	}
	evalFields := func(typ string, exp map[string]*string, only bool, got map[string]*string) {
		for _, k := range strKeys(exp) {
			want := exp[k]
			v, set := got[k]
			a := Assertion{Type: typ, Field: k}
			if want == nil {
				a.Expected, a.Op = "(não gravado)", "notSet"
				a.Passed = !set
				if set {
					a.Actual = derefOr(v, "null")
				}
			} else {
				a.Expected, a.Op = *want, "eq"
				if set {
					a.Actual = derefOr(v, "null")
				}
				a.Passed = set && v != nil && *v == *want
				if !set {
					a.Message = "o campo não foi gravado"
				}
			}
			add(a)
		}
		if only {
			var extra []string
			for k := range got {
				if _, ok := exp[k]; !ok {
					extra = append(extra, k)
				}
			}
			if len(extra) > 0 {
				sortStrings(extra)
				add(Assertion{Type: typ, Op: "$only", Expected: "(só os campos listados)", Actual: extra,
					Message: "campos gravados fora da lista"})
			} else {
				add(Assertion{Type: typ, Op: "$only", Expected: "(só os campos listados)", Actual: "(só os campos listados)", Passed: true})
			}
		}
	}
	evalFields("form", e.Form, e.FormOnly, o.Form)
	evalFields("card", e.Card, e.CardOnly, o.Card)

	for _, l := range e.Log {
		a := Assertion{Type: "log", Field: l.Level, Expected: "contém " + fmt.Sprintf("%q", l.Contains)}
		var vistos []string
		for _, got := range o.Logs {
			if l.Level != "" && got.Level != l.Level {
				continue
			}
			vistos = append(vistos, got.Message)
			if strings.Contains(strings.ToLower(got.Message), strings.ToLower(l.Contains)) {
				a.Passed = true
				a.Actual = got.Message
			}
		}
		if !a.Passed {
			a.Actual = vistos
			if len(vistos) == 0 {
				a.Message = "nenhuma linha de log neste nível"
			}
		}
		add(a)
	}

	if e.HasDSCalls {
		gotNames := make([]string, len(o.DatasetCalls))
		for i, d := range o.DatasetCalls {
			gotNames[i] = d.Name
		}
		expNames := make([]string, len(e.DatasetCalls))
		for i, d := range e.DatasetCalls {
			expNames[i] = d.Name
		}
		a := Assertion{Type: "datasetCalls", Expected: expNames, Actual: gotNames, Passed: reflect.DeepEqual(expNames, gotNames) || (len(expNames) == 0 && len(gotNames) == 0)}
		if !a.Passed {
			a.Message = "datasets chamados diferem (nome e ordem)"
		}
		add(a)
		if a.Passed {
			for i, d := range e.DatasetCalls {
				for _, f := range strKeys(d.Constraints) {
					want := d.Constraints[f]
					var got any
					for _, c := range o.DatasetCalls[i].Constraints {
						if c.Field == f {
							got = derefOr(c.Initial, "null")
						}
					}
					add(Assertion{Type: "datasetCalls", Field: fmt.Sprintf("%s[%d].%s", d.Name, i, f), Expected: want, Actual: got, Passed: got == want})
				}
			}
		}
	}

	for _, c := range e.Calls {
		a := Assertion{Type: "calls", Field: c.Method}
		var expArgs []any
		for _, r := range c.Args {
			var v any
			_ = json.Unmarshal(r, &v)
			expArgs = append(expArgs, v)
		}
		if c.Args != nil {
			a.Expected = expArgs
		} else {
			a.Expected = "(chamado)"
		}
		var vistos []any
		for _, got := range o.Calls {
			if got.Target+"."+got.Method != c.Method {
				continue
			}
			var gotArgs []any
			for _, r := range got.Args {
				var v any
				_ = json.Unmarshal(r, &v)
				gotArgs = append(gotArgs, v)
			}
			vistos = append(vistos, gotArgs)
			if c.Args == nil || jsonEqual(expArgs, gotArgs) {
				a.Passed = true
				a.Actual = gotArgs
				if c.Args == nil {
					a.Actual = "(chamado)"
				}
			}
		}
		if !a.Passed {
			if len(vistos) == 0 {
				a.Message = "o método não foi chamado"
			} else {
				a.Actual = vistos
				a.Message = "chamado com outros argumentos"
			}
		}
		add(a)
	}

	for _, h := range e.HTML {
		all := strings.Join(o.HTML, "")
		add(Assertion{Type: "html", Expected: "contém " + fmt.Sprintf("%q", h), Actual: nilIfEmpty(all != "", all),
			Passed: strings.Contains(all, h)})
	}

	cr.Status = UnitPassed
	for _, a := range cr.Assertions {
		if !a.Passed {
			cr.Status = UnitFailed
		}
	}
	return cr
}

func thrownWhere(t *unitThrown) string {
	if t.File != "" && t.Line > 0 {
		return fmt.Sprintf("%s linha %d", t.File, t.Line)
	}
	if t.JavaClass != "" {
		return "exceção Java " + t.JavaClass
	}
	return ""
}

func nilIfEmpty(ok bool, s string) any {
	if !ok {
		return nil
	}
	return s
}

func derefOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

func strKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) { sort.Strings(s) }

// jsonEqual compara dois valores decodificados de JSON. Número e texto não se
// misturam: o teste precisa dizer o tipo certo.
func jsonEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}
