package testrun

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// Engine é o servidor visto pelo executor. A implementação real fica em
// engine_fluig.go (sobre *fluig.Client); os testes usam uma falsa.
type Engine interface {
	Process(ctx context.Context, processID string) (*ProcessInfo, error)
	Start(ctx context.Context, processID string, in MoveInput) (*MoveOutput, error)
	Move(ctx context.Context, requestID int, in MoveInput) (*MoveOutput, error)
	ManagerMove(ctx context.Context, requestID int, in MoveInput) (*MoveOutput, error)
	Assume(ctx context.Context, requestID, thread int) error
	Request(ctx context.Context, requestID int) (*RequestInfo, error)
	Tasks(ctx context.Context, requestID int) ([]TaskInfo, error)
	ReadCard(ctx context.Context, formID, cardID int) (map[string]string, error)
	WriteCard(ctx context.Context, formID, cardID int, values map[string]string) error
	Cancel(ctx context.Context, requestID int, comment string) error
	// SelfMembership devolve os papéis e grupos do ator "eu" (só no --dry-run).
	SelfMembership(ctx context.Context) (roles, groups []string, err error)
	// Diagram lê o diagrama de uma versão (0 = a versão ATIVA).
	Diagram(ctx context.Context, processID string, version int) (*fluig.ProcessDetail, error)
	// History lê os movimentos da solicitação (HISTOR_PROCES, via helper).
	History(ctx context.Context, requestID int) ([]fluig.HistoryMove, error)
}

// MoveInput são os dados de start/move/manager.
type MoveInput struct {
	Target   int
	Assignee string
	Comment  string
	Movement int
	Thread   int
	Fields   map[string]string
}

// MoveOutput é a resposta de start/move/manager.
type MoveOutput struct {
	RequestID         int
	CardID            int
	NextState         int
	NeedsAssignee     bool
	PossibleAssignees []string
}

// RequestInfo é o estado lido da solicitação.
type RequestInfo struct {
	Status  string
	Current []StepState
	FormID  int
	CardID  int
}

// TaskInfo é uma tarefa do histórico. Sequence segue a REST: numa tarefa
// concluída é a etapa ESCOLHIDA (G0.9), numa aberta é a própria etapa.
type TaskInfo struct {
	Sequence      int
	Status        string
	AssigneeCode  string
	AssigneeLogin string
}

// ProcessInfo é o diagrama da versão ativa, no recorte que o executor usa.
type ProcessInfo struct {
	Version int
	Start   []int
	States  map[int]StateInfo
}

// StateInfo é uma etapa do diagrama.
type StateInfo struct {
	Name        string
	Kind        string // start|task|service|gateway|event|end|unknown
	Transitions []int
	Mechanism   string // ex.: "Pool Papel"
	AssignKind  string // role|group|formField|...
	AssignValue string
	EventType   string // só evento: error|linkThrow|linkCatch|other
	Parent      int    // evento de erro: a service task onde ele está preso
}

// EngineError é o erro que a Engine devolve já classificado.
type EngineError struct {
	Code    string // código da CLI (SERVER_ERROR, AUTH_FAILED, TIMEOUT…)
	Message string
	Infra   bool // autenticação, rede ou tempo limite: o passo fica "errored"
	Err     error
}

func (e *EngineError) Error() string { return e.Message }
func (e *EngineError) Unwrap() error { return e.Err }

// Resultados de cenário e de passo (contrato --json).
const (
	ResPassed    = "passed"
	ResFailed    = "failed"
	ResErrored   = "errored"
	ResSkipped   = "skipped"
	ResValidated = "validated"
)

// Report é o relatório da execução.
type Report struct {
	Server  string        `json:"server"`
	DryRun  bool          `json:"dryRun,omitempty"`
	Summary Summary       `json:"summary"`
	Suites  []SuiteResult `json:"suites"`
}

// Summary conta os cenários.
type Summary struct {
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Errored    int   `json:"errored"`
	Skipped    int   `json:"skipped"`
	Validated  int   `json:"validated,omitempty"`
	DurationMs int64 `json:"durationMs"`
}

// SuiteResult é o resultado de uma suíte.
type SuiteResult struct {
	File           string           `json:"file"`
	Process        string           `json:"process"`
	ProcessVersion int              `json:"processVersion,omitempty"`
	Description    string           `json:"description,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
	Scenarios      []ScenarioResult `json:"scenarios"`
}

// ScenarioResult é o resultado de um cenário.
type ScenarioResult struct {
	Name       string         `json:"name"`
	Result     string         `json:"result"`
	RequestID  int            `json:"requestId,omitempty"`
	DurationMs int64          `json:"durationMs"`
	Cleanup    *CleanupResult `json:"cleanup,omitempty"`
	Steps      []StepResult   `json:"steps"`
}

// StepResult é o resultado de um passo.
type StepResult struct {
	Index      int         `json:"index"`
	Name       string      `json:"name"`
	Action     string      `json:"action"`
	Result     string      `json:"result"`
	DurationMs int64       `json:"durationMs,omitempty"`
	NextState  int         `json:"nextState,omitempty"`
	Error      string      `json:"error,omitempty"`
	ErrorCode  string      `json:"errorCode,omitempty"`
	Assertions []Assertion `json:"assertions,omitempty"`
}

// CleanupResult é o resultado da limpeza do cenário.
type CleanupResult struct {
	Action  string `json:"action"` // cancel | none | keep
	Status  string `json:"status"` // ok | skipped | failed
	Message string `json:"message,omitempty"`
}

// Options controla a execução.
type Options struct {
	Today       time.Time // data local do servidor
	SelfLogin   string
	SelfCode    string
	Keep        bool          // --keep: não roda a limpeza
	StepTimeout time.Duration // tempo máximo de cada passo
	Scenarios   []string      // --scenario: filtro por nome exato
	// Now e Sleep entram por injeção para os testes. Nil = relógio real.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
	// Progress, se não-nil, recebe cada cenário concluído (saída ao vivo).
	Progress func(suite *Suite, r ScenarioResult)
}

const (
	defaultWaitTimeout  = 3 * time.Minute
	defaultWaitInterval = 3 * time.Second
	defaultStepTimeout  = 5 * time.Minute
)

func (o *Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *Options) sleep(ctx context.Context, d time.Duration) error {
	if o.Sleep != nil {
		return o.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Selected informa se o cenário passa pelo filtro --scenario.
func (o *Options) Selected(name string) bool {
	if len(o.Scenarios) == 0 {
		return true
	}
	for _, s := range o.Scenarios {
		if s == name {
			return true
		}
	}
	return false
}

// RunSuite executa os cenários da suíte em série. proc é o diagrama da versão
// ativa, lido antes pelo chamador (processo inexistente é erro de carga).
func RunSuite(ctx context.Context, eng Engine, s *Suite, proc *ProcessInfo, opts Options) SuiteResult {
	sr := SuiteResult{File: s.File, Process: s.Process, ProcessVersion: proc.Version, Description: s.Description}
	for _, sc := range s.Scenarios {
		if !opts.Selected(sc.Name) {
			continue
		}
		r := runScenario(ctx, eng, s, sc, proc, &opts)
		sr.Scenarios = append(sr.Scenarios, r)
		if opts.Progress != nil {
			opts.Progress(s, r)
		}
	}
	return sr
}

// scenarioRun guarda o estado vivo de um cenário.
type scenarioRun struct {
	eng       Engine
	suite     *Suite
	sc        *Scenario
	proc      *ProcessInfo
	opts      *Options
	env       *Env
	base      map[string]string // fixture + fields do cenário (com expressões)
	requestID int
	formID    int
	cardID    int
}

func runScenario(ctx context.Context, eng Engine, s *Suite, sc *Scenario, proc *ProcessInfo, opts *Options) ScenarioResult {
	inicio := opts.now()
	run := &scenarioRun{
		eng: eng, suite: s, sc: sc, proc: proc, opts: opts,
		env: &Env{
			Today: opts.Today, Vars: s.Vars, SelfLogin: opts.SelfLogin, SelfCode: opts.SelfCode,
			Steps: map[string]int{}, Unique: UniqueValue(inicio, sc.Index),
		},
		base: merge(sc.CardValues, sc.Fields),
	}
	res := ScenarioResult{Name: sc.Name, Result: ResPassed}
	parou := false
	for _, st := range sc.Steps {
		if parou {
			res.Steps = append(res.Steps, StepResult{Index: st.Index, Name: st.Name, Action: st.Action, Result: ResSkipped})
			continue
		}
		sr := run.step(ctx, st)
		res.Steps = append(res.Steps, sr)
		if sr.Result != ResPassed {
			res.Result = sr.Result
			parou = true
		}
	}
	res.RequestID = run.requestID
	res.Cleanup = run.cleanup(ctx)
	res.DurationMs = opts.now().Sub(inicio).Milliseconds()
	return res
}

func merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// classify extrai código, mensagem e tipo do erro da Engine.
func classify(err error) (code, msg string, infra bool) {
	var ee *EngineError
	if errors.As(err, &ee) {
		return ee.Code, NormalizeServerText(ee.Message), ee.Infra
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT", "o passo excedeu o tempo limite (--step-timeout)", true
	}
	return "SERVER_ERROR", NormalizeServerText(err.Error()), false
}

func (r *scenarioRun) stepTimeout(st *Step) time.Duration {
	d := r.opts.StepTimeout
	if d <= 0 {
		d = defaultStepTimeout
	}
	if st.Action == ActWait {
		w := defaultWaitTimeout
		if st.Timeout != "" {
			w, _ = parseDuration(st.Timeout)
		}
		if w+30*time.Second > d {
			d = w + 30*time.Second
		}
	}
	return d
}

// step executa um passo e avalia o expect.
func (r *scenarioRun) step(parent context.Context, st *Step) StepResult {
	t0 := r.opts.now()
	sr := StepResult{Index: st.Index, Name: st.Name, Action: st.Action}
	ctx, cancel := context.WithTimeout(parent, r.stepTimeout(st))
	defer cancel()
	defer func() { sr.DurationMs = r.opts.now().Sub(t0).Milliseconds() }()

	if st.Action != ActStart && r.requestID == 0 {
		sr.Result = ResFailed
		sr.Error = "não existe solicitação neste cenário: o start não criou nenhuma"
		return sr
	}

	esperaErro := st.Expect != nil && st.Expect.Error != nil
	var antes []StepState
	if esperaErro && r.requestID > 0 {
		if info, err := r.eng.Request(ctx, r.requestID); err == nil {
			antes = dedupSteps(info.Current)
		}
	}

	next, err := r.act(ctx, st)
	if next > 0 {
		sr.NextState = next
		if st.ID != "" {
			r.env.Steps[st.ID] = next
		}
	}
	var stepErr *StepError
	if err != nil {
		code, msg, infra := classify(err)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && parent.Err() == nil {
			code, msg, infra = "TIMEOUT", fmt.Sprintf("o passo excedeu o tempo limite de %s (--step-timeout): %s", r.stepTimeout(st), msg), true
		}
		sr.Error, sr.ErrorCode = msg, code
		if infra {
			sr.Result = ResErrored
			return sr
		}
		if !esperaErro {
			sr.Result = ResFailed
			return sr
		}
		stepErr = &StepError{Code: code, Message: msg}
	}

	if st.Expect == nil {
		sr.Result = ResPassed
		return sr
	}
	snap, err := r.snapshot(ctx, NeedOf(st.Expect))
	if err != nil {
		_, msg, _ := classify(err)
		sr.Result, sr.Error = ResErrored, "falha ao reler a solicitação: "+msg
		return sr
	}
	as, err := Evaluate(st.Expect, snap, stepErr, r.env)
	if err != nil {
		sr.Result, sr.Error = ResFailed, err.Error()
		return sr
	}
	// Erro esperado: a solicitação NÃO pode ter mudado de etapa. Pega o evento
	// que grava e depois faz throw.
	if stepErr != nil && antes != nil {
		a := Assertion{Type: "unchanged", Expected: seqs(antes), Actual: seqs(snap.Current), Passed: sameSteps(antes, snap.Current)}
		if !a.Passed {
			a.Message = "o servidor rejeitou, mas a solicitação avançou"
		}
		as = append(as, a)
	}
	if stepErr == nil && st.Expect.Error != nil {
		for i := range as {
			if as[i].Type == "error" && len(snap.Current) > 0 {
				as[i].Message = fmt.Sprintf("esperava erro, mas o passo foi aceito (%s)", describeCurrent(snap))
			}
		}
	}
	sr.Assertions = as
	sr.Result = ResPassed
	for _, a := range as {
		if !a.Passed {
			sr.Result = ResFailed
		}
	}
	return sr
}

func sameSteps(a, b []StepState) bool {
	if len(a) != len(b) {
		return false
	}
	ka, kb := stepKeys(a), stepKeys(b)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

func stepKeys(cs []StepState) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, fmt.Sprintf("%d/%d", c.Sequence, c.Movement))
	}
	sort.Strings(out)
	return out
}

// act executa a ação do passo. Devolve o nextState quando a ação o informa.
func (r *scenarioRun) act(ctx context.Context, st *Step) (int, error) {
	switch st.Action {
	case ActStart:
		fields, err := ResolveMap(merge(r.base, st.Fields), r.env)
		if err != nil {
			return 0, err
		}
		in, err := r.moveInput(st, fields)
		if err != nil {
			return 0, err
		}
		out, err := r.eng.Start(ctx, r.suite.Process, in)
		if err != nil {
			return 0, err
		}
		if out.NeedsAssignee {
			return 0, needsAssignee(out)
		}
		r.requestID, r.cardID = out.RequestID, out.CardID
		r.env.RequestID = out.RequestID
		info, err := r.eng.Request(ctx, out.RequestID)
		if err != nil {
			return out.NextState, err
		}
		r.formID = info.FormID
		if info.CardID > 0 {
			r.cardID = info.CardID
		}
		return out.NextState, nil
	case ActMove, ActManager:
		fields, err := ResolveMap(st.Fields, r.env)
		if err != nil {
			return 0, err
		}
		in, err := r.moveInput(st, fields)
		if err != nil {
			return 0, err
		}
		var out *MoveOutput
		if st.Action == ActMove {
			out, err = r.eng.Move(ctx, r.requestID, in)
		} else {
			out, err = r.eng.ManagerMove(ctx, r.requestID, in)
		}
		if err != nil {
			if code, _, _ := classify(err); code == "POOL_TASK_NOT_ASSIGNED" {
				return 0, &EngineError{Code: code, Message: err.Error() + " Acrescente um passo {\"assume\": true} antes deste.", Err: err}
			}
			return 0, err
		}
		if out.NeedsAssignee {
			return 0, needsAssignee(out)
		}
		return out.NextState, nil
	case ActAssume:
		return 0, r.eng.Assume(ctx, r.requestID, st.Thread)
	case ActSetFields:
		fields, err := ResolveMap(st.Fields, r.env)
		if err != nil {
			return 0, err
		}
		if r.formID == 0 || r.cardID == 0 {
			return 0, fmt.Errorf("não sei o card desta solicitação (formulário %d, card %d)", r.formID, r.cardID)
		}
		// O PUT de cards roda o validateForm (G0.1/G0.6): com payload parcial
		// ele vê o card sem as tabelas filhas e recusa. Por isso o passo lê o
		// card inteiro, mescla e grava tudo.
		card, err := r.eng.ReadCard(ctx, r.formID, r.cardID)
		if err != nil {
			return 0, err
		}
		return 0, r.eng.WriteCard(ctx, r.formID, r.cardID, merge(card, fields))
	case ActCheck:
		return 0, nil
	case ActWait:
		return r.wait(ctx, st)
	case ActCancel:
		comment, err := Resolve(st.Comment, r.env)
		if err != nil {
			return 0, err
		}
		if comment == "" {
			comment = r.cleanupComment()
		}
		return 0, r.eng.Cancel(ctx, r.requestID, comment)
	}
	return 0, fmt.Errorf("ação %q desconhecida", st.Action)
}

func (r *scenarioRun) moveInput(st *Step, fields map[string]string) (MoveInput, error) {
	in := MoveInput{Target: st.Target, Movement: st.Movement, Thread: st.Thread, Fields: fields}
	var err error
	if in.Comment, err = Resolve(st.Comment, r.env); err != nil {
		return in, err
	}
	if in.Assignee, err = Resolve(st.Assignee, r.env); err != nil {
		return in, err
	}
	return in, nil
}

func needsAssignee(out *MoveOutput) error {
	return &EngineError{Code: "USAGE_ERROR", Message: fmt.Sprintf(
		"o servidor pede a escolha do responsável; informe \"assignee\" no passo. Opções: %s",
		strings.Join(out.PossibleAssignees, ", "))}
}

// wait relê a solicitação até cumprir TODAS as condições do passo.
func (r *scenarioRun) wait(ctx context.Context, st *Step) (int, error) {
	timeout, interval := defaultWaitTimeout, defaultWaitInterval
	if st.Timeout != "" {
		timeout, _ = parseDuration(st.Timeout)
	}
	if st.Interval != "" {
		interval, _ = parseDuration(st.Interval)
	}
	limite := r.opts.now().Add(timeout)
	var ultimo *RequestInfo
	for {
		info, err := r.eng.Request(ctx, r.requestID)
		if err != nil {
			return 0, err
		}
		ultimo = info
		okState := st.WaitState == 0
		for _, c := range info.Current {
			if c.Sequence == st.WaitState {
				okState = true
			}
		}
		okStatus := st.WaitStat == "" || info.Status == st.WaitStat
		if okState && okStatus {
			return 0, nil
		}
		if !r.opts.now().Before(limite) {
			break
		}
		if err := r.opts.sleep(ctx, interval); err != nil {
			return 0, err
		}
	}
	return 0, &EngineError{Code: "TEST_WAIT_TIMEOUT", Message: fmt.Sprintf(
		"a solicitação não chegou ao estado esperado em %s; último estado: %s, %s",
		timeout, ultimo.Status, describeCurrent(&Snapshot{Current: ultimo.Current}))}
}

// snapshot lê só o que o expect pede.
func (r *scenarioRun) snapshot(ctx context.Context, need Need) (*Snapshot, error) {
	snap := &Snapshot{}
	if r.requestID == 0 {
		// O start falhou (erro esperado): não há solicitação para reler.
		return snap, nil
	}
	if need.Request {
		info, err := r.eng.Request(ctx, r.requestID)
		if err != nil {
			return nil, err
		}
		snap.Status = info.Status
		snap.Current = dedupSteps(info.Current)
	}
	if need.Tasks {
		tasks, err := r.eng.Tasks(ctx, r.requestID)
		if err != nil {
			return nil, err
		}
		snap.HasTasks = true
		seen := map[int]bool{}
		for _, s := range r.proc.Start {
			seen[s] = true
		}
		for _, t := range tasks {
			seen[t.Sequence] = true
			if t.Status == "NOT_COMPLETED" {
				if t.AssigneeCode != "" {
					snap.Open = append(snap.Open, t.AssigneeCode)
				}
				if t.AssigneeLogin != "" && t.AssigneeLogin != t.AssigneeCode {
					snap.Open = append(snap.Open, t.AssigneeLogin)
				}
			}
		}
		for s := range seen {
			snap.Visited = append(snap.Visited, s)
		}
		sort.Ints(snap.Visited)
	}
	if need.Card {
		card, err := r.eng.ReadCard(ctx, r.formID, r.cardID)
		if err != nil {
			return nil, err
		}
		snap.Card, snap.HasCard = card, true
	}
	return snap, nil
}

// dedupSteps remove a repetição (etapa, movimento): com o pool assumido, a
// REST lista a mesma etapa duas vezes no mesmo movimento (Gate 0).
func dedupSteps(cs []StepState) []StepState {
	seen := map[string]bool{}
	var out []StepState
	for _, c := range cs {
		k := fmt.Sprintf("%d/%d", c.Sequence, c.Movement)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, c)
	}
	return out
}

func (r *scenarioRun) cleanupComment() string {
	return fmt.Sprintf("fluigcli test: %s / %s", r.suite.File, r.sc.Name)
}

// cleanup roda as ações de limpeza. Falha aqui NÃO muda o resultado do
// cenário: vira aviso com o número da solicitação que ficou aberta.
func (r *scenarioRun) cleanup(parent context.Context) *CleanupResult {
	if r.requestID == 0 {
		return nil
	}
	if r.opts.Keep {
		return &CleanupResult{Action: "keep", Status: "skipped", Message: "--keep: a solicitação ficou como está"}
	}
	res := &CleanupResult{Action: strings.Join(r.sc.Cleanup, ","), Status: "ok"}
	for _, a := range r.sc.Cleanup {
		if a != "cancel" {
			continue
		}
		ctx, cancel := context.WithTimeout(parent, r.stepTimeout(&Step{}))
		info, err := r.eng.Request(ctx, r.requestID)
		if err == nil && info.Status != "OPEN" {
			cancel()
			res.Message = "solicitação " + strings.ToLower(info.Status) + ": nada a cancelar"
			continue
		}
		if err == nil {
			err = r.eng.Cancel(ctx, r.requestID, r.cleanupComment())
		}
		cancel()
		if err != nil {
			_, msg, _ := classify(err)
			res.Status = "failed"
			res.Message = fmt.Sprintf("a solicitação %d ficou aberta: %s", r.requestID, msg)
		}
	}
	return res
}

// Summarize conta os cenários do relatório.
func Summarize(suites []SuiteResult) Summary {
	var s Summary
	for _, su := range suites {
		for _, sc := range su.Scenarios {
			switch sc.Result {
			case ResPassed:
				s.Passed++
			case ResFailed:
				s.Failed++
			case ResErrored:
				s.Errored++
			case ResSkipped:
				s.Skipped++
			case ResValidated:
				s.Validated++
			}
		}
	}
	return s
}
