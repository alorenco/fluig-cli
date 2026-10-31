package testrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeEngine simula um processo: start cai em 20 (pool), move segue a tabela
// landing (destino → etapa onde para) e grava os campos no card.
type fakeEngine struct {
	current   int
	movement  int
	status    string
	card      map[string]string
	landing   map[int]int
	rejectMv  map[int]error // destino → erro do servidor (não muda nada)
	advanceMv map[int]bool  // destino → erro, MAS a solicitação avança (evento que grava e faz throw)
	startErr  error
	cancelErr error
	writeErr  error
	tasks     []TaskInfo
	calls     []string
	written   map[string]string
	// waitAfter: quantas leituras até a solicitação finalizar (simula service task).
	waitAfter int
	reads     int
}

func newFake() *fakeEngine {
	return &fakeEngine{
		status:  "OPEN",
		card:    map[string]string{"valor": "10", "venc___1": "05/11/2026"},
		landing: map[int]int{76: 23, 110: 112, 13: 13},
	}
}

func (f *fakeEngine) Process(context.Context, string) (*ProcessInfo, error) { return testProc(), nil }

func (f *fakeEngine) Start(_ context.Context, pid string, in MoveInput) (*MoveOutput, error) {
	f.calls = append(f.calls, "start")
	if f.startErr != nil {
		return nil, f.startErr
	}
	for k, v := range in.Fields {
		f.card[k] = v
	}
	f.current, f.movement = 20, 3
	f.tasks = []TaskInfo{{Sequence: 98, Status: "COMPLETED"}, {Sequence: 20, Status: "NOT_COMPLETED", AssigneeCode: "Pool:Role:revisor"}}
	return &MoveOutput{RequestID: 500, CardID: 900, NextState: 20}, nil
}

func (f *fakeEngine) Move(_ context.Context, id int, in MoveInput) (*MoveOutput, error) {
	f.calls = append(f.calls, fmt.Sprintf("move %d", in.Target))
	if err, ok := f.rejectMv[in.Target]; ok {
		if f.advanceMv[in.Target] {
			f.current++
			f.movement++
		}
		return nil, err
	}
	for k, v := range in.Fields {
		f.card[k] = v
	}
	f.tasks = append(f.tasks, TaskInfo{Sequence: in.Target, Status: "COMPLETED"})
	f.current = f.landing[in.Target]
	f.movement++
	f.tasks = append(f.tasks, TaskInfo{Sequence: f.current, Status: "NOT_COMPLETED", AssigneeCode: "c-jsilva", AssigneeLogin: "jsilva"})
	return &MoveOutput{RequestID: id, NextState: f.current}, nil
}

func (f *fakeEngine) ManagerMove(ctx context.Context, id int, in MoveInput) (*MoveOutput, error) {
	f.calls = append(f.calls, "manager")
	return f.Move(ctx, id, in)
}

func (f *fakeEngine) Assume(context.Context, int, int) error {
	f.calls = append(f.calls, "assume")
	return nil
}

func (f *fakeEngine) Request(context.Context, int) (*RequestInfo, error) {
	f.reads++
	if f.waitAfter > 0 && f.reads >= f.waitAfter {
		f.status, f.current = "FINALIZED", 52
	}
	// Pool assumido: a REST lista a mesma etapa duas vezes no mesmo movimento.
	cur := []StepState{{Sequence: f.current, Name: fmt.Sprintf("Etapa %d", f.current), Movement: f.movement}}
	cur = append(cur, cur[0])
	return &RequestInfo{Status: f.status, Current: cur, FormID: 157, CardID: 900}, nil
}

func (f *fakeEngine) Tasks(context.Context, int) ([]TaskInfo, error) { return f.tasks, nil }

func (f *fakeEngine) ReadCard(context.Context, int, int) (map[string]string, error) {
	return merge(f.card), nil
}

func (f *fakeEngine) WriteCard(_ context.Context, _, _ int, v map[string]string) error {
	f.calls = append(f.calls, "write")
	if f.writeErr != nil {
		return f.writeErr
	}
	f.written = v
	f.card = merge(v)
	return nil
}

func (f *fakeEngine) Cancel(context.Context, int, string) error {
	f.calls = append(f.calls, "cancel")
	if f.cancelErr != nil {
		return f.cancelErr
	}
	f.status = "CANCELED"
	return nil
}

func (f *fakeEngine) SelfMembership(context.Context) ([]string, []string, error) {
	return []string{"revisor"}, []string{"ti"}, nil
}

func testProc() *ProcessInfo {
	return &ProcessInfo{Version: 7, Start: []int{4}, States: map[int]StateInfo{
		4:   {Name: "Início", Kind: "start", Transitions: []int{98}},
		98:  {Name: "Gateway", Kind: "gateway", Transitions: []int{20, 23}},
		20:  {Name: "Revisar", Kind: "task", Transitions: []int{13, 76}, Mechanism: "Pool Papel", AssignKind: "role", AssignValue: "revisor"},
		76:  {Name: "Aprovar", Kind: "gateway", Transitions: []int{23}},
		23:  {Name: "Diretoria", Kind: "task", Transitions: []int{13, 110}, Mechanism: "Campo Formulário", AssignKind: "formField", AssignValue: "dir"},
		110: {Name: "Urgente?", Kind: "gateway", Transitions: []int{28, 112}},
		112: {Name: "Financeiro", Kind: "task", Transitions: []int{13, 28}, Mechanism: "Pool Papel", AssignKind: "role", AssignValue: "financeiro"},
		13:  {Name: "Revisar Solicitação", Kind: "task", Transitions: []int{98}},
		28:  {Name: "Integração", Kind: "service", Transitions: []int{52}},
		52:  {Name: "Fim", Kind: "end"},
	}}
}

func parse(t *testing.T, js string) *Suite {
	t.Helper()
	s, err := ParseSuite("suite.json", []byte(js), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func opts() Options {
	return Options{
		Today: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC), SelfLogin: "jsilva", SelfCode: "c-jsilva",
		Sleep: func(context.Context, time.Duration) error { return nil },
	}
}

func run(t *testing.T, f *fakeEngine, js string, o Options) ScenarioResult {
	t.Helper()
	sr := RunSuite(context.Background(), f, parse(t, js), testProc(), o)
	if len(sr.Scenarios) != 1 {
		t.Fatalf("esperava 1 cenário, veio %d", len(sr.Scenarios))
	}
	return sr.Scenarios[0]
}

func TestRunCenarioPassa(t *testing.T) {
	f := newFake()
	r := run(t, f, `{"process":"P","scenarios":[{"name":"ok","fields":{"doc":"{{unique}}","dt":"{{today(+1)}}"},"steps":[
		{"start":true,"expect":{"state":20,"status":"OPEN","assignee":"Pool:Role:revisor"}},
		{"assume":true},
		{"move":{"target":76,"fields":{"apr":"aprovado","ref":"{{requestId}}"}},"id":"m","expect":{"state":23,"card":{"apr":"aprovado","ref":"500"}}},
		{"move":110,"expect":{"path":{"visited":[4,98,20,76,23,110,112],"notVisited":[28]},"stateName":"Etapa 112"}}
	]}]}`, opts())
	if r.Result != ResPassed {
		t.Fatalf("cenário deveria passar: %+v", r)
	}
	if r.RequestID != 500 || r.Cleanup == nil || r.Cleanup.Status != "ok" || f.status != "CANCELED" {
		t.Errorf("limpeza padrão deveria cancelar: %+v / status %s", r.Cleanup, f.status)
	}
	if !strings.HasPrefix(f.card["doc"], "zzt") || f.card["dt"] != "01/11/2026" {
		t.Errorf("expressões do card não resolvidas: %+v", f.card)
	}
	if r.Steps[2].NextState != 23 {
		t.Errorf("nextState não registrado: %+v", r.Steps[2])
	}
}

func TestRunCenarioFalhaParaEPula(t *testing.T) {
	f := newFake()
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},
		{"move":76,"expect":{"state":28,"card":{"valor":"99","nada":"x"}}},
		{"move":110}
	]}]}`, opts())
	if r.Result != ResFailed || r.Steps[1].Result != ResFailed || r.Steps[2].Result != ResSkipped {
		t.Fatalf("falha deveria parar o cenário: %+v", r)
	}
	if len(r.Steps[1].Assertions) != 3 {
		t.Errorf("todas as asserções são avaliadas: %+v", r.Steps[1].Assertions)
	}
	if strings.Join(f.calls, ",") != "start,move 76,cancel" {
		t.Errorf("chamadas inesperadas: %v", f.calls)
	}
}

func TestRunErroEsperado(t *testing.T) {
	f := newFake()
	f.rejectMv = map[int]error{110: &EngineError{Code: "SERVER_ERROR", Message: "Erro ao executar evento do processo, java.lang.Exception: <b>Decisão incompatível</b>"}}
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},{"move":76},
		{"move":110,"expect":{"error":"decisão incompatível","state":23}}
	]}]}`, opts())
	if r.Result != ResPassed {
		t.Fatalf("erro esperado deveria passar: %+v", r.Steps[2])
	}
	tipos := []string{}
	for _, a := range r.Steps[2].Assertions {
		tipos = append(tipos, a.Type)
	}
	if strings.Join(tipos, ",") != "error,state,unchanged" {
		t.Errorf("asserções: %v", tipos)
	}
}

// O evento grava e depois faz throw: o servidor recusa, mas a solicitação anda.
func TestRunErroEsperadoMasAvancou(t *testing.T) {
	f := newFake()
	f.rejectMv = map[int]error{110: &EngineError{Code: "SERVER_ERROR", Message: "Decisão incompatível"}}
	f.advanceMv = map[int]bool{110: true}
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},{"move":76},{"move":110,"expect":{"error":"incompatível"}}
	]}]}`, opts())
	ult := r.Steps[2].Assertions[len(r.Steps[2].Assertions)-1]
	if r.Result != ResFailed || ult.Type != "unchanged" || ult.Passed || !strings.Contains(ult.Message, "avançou") {
		t.Fatalf("deveria acusar que a solicitação avançou: %+v", r.Steps[2])
	}
}

func TestRunErroEsperadoMasAceito(t *testing.T) {
	f := newFake()
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},{"move":{"target":76},"expect":{"error":"obrigatório"}}
	]}]}`, opts())
	a := r.Steps[1].Assertions[0]
	if r.Result != ResFailed || a.Passed || !strings.Contains(a.Message, "esperava erro, mas o passo foi aceito") {
		t.Fatalf("passo aceito com expect.error deveria falhar: %+v", a)
	}
}

func TestRunErroInesperado(t *testing.T) {
	f := newFake()
	f.rejectMv = map[int]error{76: errors.New("{Erro ao salvar dados de formulário: <b>Campo X</b>}")}
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true},{"move":76},{"move":110}]}]}`, opts())
	if r.Result != ResFailed || r.Steps[1].Error != "Erro ao salvar dados de formulário: Campo X" || r.Steps[2].Result != ResSkipped {
		t.Fatalf("erro inesperado: %+v", r.Steps)
	}
}

func TestRunErroDeInfraestrutura(t *testing.T) {
	f := newFake()
	f.rejectMv = map[int]error{76: &EngineError{Code: "AUTH_FAILED", Message: "sessão expirada", Infra: true}}
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true},{"move":76,"expect":{"error":"x"}}]}]}`, opts())
	if r.Result != ResErrored || r.Steps[1].Result != ResErrored || r.Steps[1].ErrorCode != "AUTH_FAILED" {
		t.Fatalf("infraestrutura deveria virar errored, mesmo com expect.error: %+v", r.Steps[1])
	}
}

func TestRunStartComErroEsperadoNaoLimpa(t *testing.T) {
	f := newFake()
	f.startErr = &EngineError{Code: "SERVER_ERROR", Message: "{Erro ao salvar dados de formulário: <b>Coligada.</b>}"}
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true,"expect":{"error":"coligada"}}]}]}`, opts())
	if r.Result != ResPassed || r.Cleanup != nil || r.RequestID != 0 {
		t.Fatalf("start recusado: nada a limpar: %+v", r)
	}
}

func TestRunLimpezaFalhaNaoMudaResultado(t *testing.T) {
	f := newFake()
	f.cancelErr = errors.New("não é o requisitante")
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true}]}]}`, opts())
	if r.Result != ResPassed || r.Cleanup.Status != "failed" || !strings.Contains(r.Cleanup.Message, "500 ficou aberta") {
		t.Fatalf("limpeza falha vira aviso: %+v", r.Cleanup)
	}
}

func TestRunKeepENone(t *testing.T) {
	f := newFake()
	o := opts()
	o.Keep = true
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true}]}]}`, o)
	if r.Cleanup.Action != "keep" || f.status != "OPEN" {
		t.Errorf("--keep não deveria cancelar: %+v", r.Cleanup)
	}
	f = newFake()
	r = run(t, f, `{"process":"P","scenarios":[{"name":"x","cleanup":["none"],"steps":[{"start":true}]}]}`, opts())
	if f.status != "OPEN" || strings.Contains(strings.Join(f.calls, ","), "cancel") {
		t.Errorf("cleanup none não deveria cancelar: %v", f.calls)
	}
	// Solicitação já encerrada: nada a cancelar.
	f = newFake()
	r = run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true},{"cancel":true}]}]}`, opts())
	if r.Cleanup.Status != "ok" || !strings.Contains(r.Cleanup.Message, "nada a cancelar") {
		t.Errorf("cancelada no cenário: %+v", r.Cleanup)
	}
}

// setFields lê o card INTEIRO, mescla e grava tudo (G0.6).
func TestRunSetFieldsGravaCardInteiro(t *testing.T) {
	f := newFake()
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},{"setFields":{"apr":"reprovado","obs":"req {{requestId}}"}},
		{"manager":{"target":13},"expect":{"state":13,"card":{"apr":"reprovado"}}}
	]}]}`, opts())
	if r.Result != ResPassed {
		t.Fatalf("%+v", r.Steps)
	}
	if f.written["valor"] != "10" || f.written["venc___1"] != "05/11/2026" || f.written["obs"] != "req 500" {
		t.Errorf("o PUT deveria levar o card inteiro: %+v", f.written)
	}
	if !strings.Contains(strings.Join(f.calls, ","), "write,manager") {
		t.Errorf("ordem: %v", f.calls)
	}
}

func TestRunWait(t *testing.T) {
	f := newFake()
	f.waitAfter = 4 // a cadeia de service tasks termina na 4ª leitura
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},{"wait":{"status":"FINALIZED","interval":"1s"},"expect":{"state":52}}
	]}]}`, opts())
	if r.Result != ResPassed {
		t.Fatalf("wait deveria esperar a finalização: %+v", r.Steps[1])
	}

	// Timeout: o relógio falso avança a cada sleep.
	f = newFake()
	agora := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	o := opts()
	o.Now = func() time.Time { return agora }
	o.Sleep = func(context.Context, time.Duration) error { agora = agora.Add(10 * time.Second); return nil }
	r = run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[
		{"start":true},{"wait":{"state":52,"timeout":"30s","interval":"10s"}}
	]}]}`, o)
	if r.Result != ResFailed || !strings.Contains(r.Steps[1].Error, "não chegou ao estado esperado em 30s") ||
		!strings.Contains(r.Steps[1].Error, "etapa atual: 20") {
		t.Fatalf("timeout do wait: %+v", r.Steps[1])
	}
}

func TestRunPoolSemDonoSugereAssume(t *testing.T) {
	f := newFake()
	f.rejectMv = map[int]error{76: &EngineError{Code: "POOL_TASK_NOT_ASSIGNED", Message: "a tarefa está no pool Revisor."}}
	r := run(t, f, `{"process":"P","scenarios":[{"name":"x","steps":[{"start":true},{"move":76}]}]}`, opts())
	if !strings.Contains(r.Steps[1].Error, `{"assume": true}`) {
		t.Errorf("deveria sugerir o assume: %q", r.Steps[1].Error)
	}
}

func TestRunFiltroDeCenario(t *testing.T) {
	f := newFake()
	o := opts()
	o.Scenarios = []string{"b"}
	sr := RunSuite(context.Background(), f, parse(t, `{"process":"P","scenarios":[
		{"name":"a","steps":[{"start":true}]},{"name":"b","steps":[{"start":true}]}]}`), testProc(), o)
	if len(sr.Scenarios) != 1 || sr.Scenarios[0].Name != "b" || sr.ProcessVersion != 7 {
		t.Fatalf("filtro: %+v", sr)
	}
	sm := Summarize([]SuiteResult{sr})
	if sm.Passed != 1 {
		t.Errorf("resumo: %+v", sm)
	}
}

func TestDryRun(t *testing.T) {
	f := newFake()
	s := parse(t, `{"process":"P","scenarios":[
		{"name":"ok","steps":[{"start":true,"expect":{"state":20}},{"assume":true},{"move":76,"expect":{"state":23}},
			{"move":110,"expect":{"error":"x"}},{"move":13}]},
		{"name":"ruim","steps":[{"start":{"target":23},"expect":{"state":23}},{"assume":true},{"move":28},
			{"move":999,"expect":{"state":888}}]},
		{"name":"pool alheio","steps":[{"start":true,"expect":{"state":112}},{"assume":true}]},
		{"name":"sem estado","steps":[{"start":true},{"move":76}]}
	]}`)
	sr, probs := DryRunSuite(context.Background(), f, s, testProc(), opts())
	if len(f.calls) != 0 {
		t.Fatalf("o dry-run não pode escrever: %v", f.calls)
	}
	if sr.Scenarios[0].Result != ResValidated {
		t.Errorf("cenário válido: %+v", sr.Scenarios[0])
	}
	texto := strings.Join(probs, "\n")
	for _, want := range []string{
		"não há transição do início para a etapa 23",
		"a etapa 23 (Diretoria) não é de pool",
		"não há transição da etapa 23 (Diretoria) para a 28; destinos possíveis: 13, 110",
		"a etapa de destino 999 não existe na versão 7",
		"a etapa de expect.state 888 não existe",
		`não está no papel "financeiro"`,
	} {
		if !strings.Contains(texto, want) {
			t.Errorf("problemas sem %q:\n%s", want, texto)
		}
	}
	if len(sr.Warnings) != 2 || !strings.Contains(sr.Warnings[1], `"sem estado", passo 2`) {
		t.Errorf("aviso de transição não conferida: %v", sr.Warnings)
	}
}
