package testrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Path é um caminho do início até um fim (ou até uma etapa humana, no
// fallback), como lista de índices de aresta do grafo.
type Path struct {
	Edges []int
	Novel []int // arestas que estavam pendentes quando o caminho foi escolhido
}

// States devolve as etapas do caminho, na ordem.
func (g *Graph) pathStates(p Path) []int {
	if len(p.Edges) == 0 {
		return nil
	}
	out := []int{g.Edges[p.Edges[0]].From}
	for _, i := range p.Edges {
		out = append(out, g.Edges[i].To)
	}
	return out
}

// shortest faz uma BFS 0-1 sobre as arestas main: custo 0 para aresta
// pendente e 1 para aresta já coberta. Devolve o caminho de `from` até a
// primeira etapa em `targets`. Determinística: as arestas estão ordenadas por
// origem e destino, e o empate fica com a ordem de visita.
func (g *Graph) shortest(from int, targets map[int]bool, pending map[int]bool, noService bool) ([]int, bool) {
	if targets[from] {
		return nil, true
	}
	const inf = 1 << 30
	dist := map[int]int{from: 0}
	prev := map[int]int{} // etapa → índice da aresta que chegou nela
	type item struct{ seq, d int }
	deque := []item{{from, 0}}
	for len(deque) > 0 {
		cur := deque[0]
		deque = deque[1:]
		if d, ok := dist[cur.seq]; ok && cur.d > d {
			continue
		}
		for i, e := range g.Edges {
			if e.From != cur.seq || e.Category != CatMain {
				continue
			}
			if noService && g.States[e.To].Kind == "service" {
				continue
			}
			w := 1
			if pending[i] {
				w = 0
			}
			nd := cur.d + w
			old, ok := dist[e.To]
			if !ok {
				old = inf
			}
			if nd < old {
				dist[e.To] = nd
				prev[e.To] = i
				if w == 0 {
					deque = append([]item{{e.To, nd}}, deque...)
				} else {
					deque = append(deque, item{e.To, nd})
				}
			}
		}
	}
	// Alvo mais barato; empate pelo menor número de etapa.
	best, bestD := 0, inf
	var alvos []int
	for t := range targets {
		alvos = append(alvos, t)
	}
	sort.Ints(alvos)
	for _, t := range alvos {
		if d, ok := dist[t]; ok && d < bestD {
			best, bestD = t, d
		}
	}
	if bestD == inf {
		return nil, false
	}
	var path []int
	for cur := best; cur != from; {
		i := prev[cur]
		path = append([]int{i}, path...)
		cur = g.Edges[i].From
	}
	return path, true
}

// EdgeCover escolhe caminhos que percorrem as arestas main (SPEC-TEST-C §5.2).
// Guloso e determinístico: a cada rodada, para cada aresta pendente (u→v),
// monta início→u + (u→v) + v→fim, preferindo arestas pendentes; fica com o
// caminho que cobre mais pendentes (empate: menos arestas; depois a ordem das
// etapas). Cada aresta aparece no máximo 2 vezes num caminho.
//
// Ajuste medido no piloto: quando início→u→v não passa por service task, o
// caminho evita service task e termina na etapa humana alcançada (ou no fim,
// se o fim vier sem service task). Sem isso, todo cenário seguia até o fim
// pela integração, e a suíte rápida ficava vazia. Só as arestas que exigem
// service task geram cenário até o fim por ela.
// Devolve os caminhos e as arestas que ficaram pendentes.
func (g *Graph) EdgeCover(maxPaths int) ([]Path, []int) {
	pending := map[int]bool{}
	for i, e := range g.Edges {
		if e.Category == CatMain {
			pending[i] = true
		}
	}
	ends := map[int]bool{}
	for _, e := range g.End {
		ends[e] = true
	}
	tasks := map[int]bool{}
	for seq, s := range g.States {
		if s.Kind == "task" {
			tasks[seq] = true
		}
	}
	starts := map[int]bool{}
	for _, s := range g.Start {
		starts[s] = true
	}
	var paths []Path
	for len(pending) > 0 && len(paths) < maxPaths {
		var best []int
		bestNovel := -1
		for _, idx := range sortedKeys(pending) {
			e := g.Edges[idx]
			var cand []int
			ok := false
			for _, s := range g.Start {
				if pre, found := g.shortest(s, map[int]bool{e.From: true}, pending, true); found {
					cand, ok = append(append([]int(nil), pre...), idx), true
					break
				}
				if pre, found := g.shortest(s, map[int]bool{e.From: true}, pending, false); found {
					cand, ok = append(append([]int(nil), pre...), idx), true
					break
				}
			}
			if !ok {
				continue
			}
			var suf []int
			found := false
			if !g.touchesService(cand) {
				// Sem service até aqui: termina sem passar por service task.
				if suf, found = g.shortest(e.To, ends, pending, true); !found {
					suf, found = g.shortest(e.To, tasks, pending, true)
				}
			}
			if !found {
				if suf, found = g.shortest(e.To, ends, pending, false); !found {
					// Fallback: para na primeira etapa humana depois da aresta.
					suf, found = g.shortest(e.To, tasks, pending, false)
				}
			}
			if !found {
				continue
			}
			cand = append(cand, suf...)
			if !maxTwice(cand) {
				continue
			}
			novel := countNovel(cand, pending)
			if novel > bestNovel || (novel == bestNovel && lessPath(cand, best)) {
				best, bestNovel = cand, novel
			}
		}
		if best == nil {
			break
		}
		p := Path{Edges: best}
		visto := map[int]bool{}
		for _, i := range best {
			if pending[i] && !visto[i] {
				p.Novel = append(p.Novel, i)
				visto[i] = true
			}
		}
		for _, i := range best {
			delete(pending, i)
		}
		paths = append(paths, p)
	}
	return paths, sortedKeys(pending)
}

// touchesService informa se o caminho entra em alguma service task.
func (g *Graph) touchesService(p []int) bool {
	for _, i := range p {
		if g.States[g.Edges[i].To].Kind == "service" {
			return true
		}
	}
	return false
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func maxTwice(p []int) bool {
	n := map[int]int{}
	for _, i := range p {
		n[i]++
		if n[i] > 2 {
			return false
		}
	}
	return true
}

func countNovel(p []int, pending map[int]bool) int {
	visto := map[int]bool{}
	for _, i := range p {
		if pending[i] {
			visto[i] = true
		}
	}
	return len(visto)
}

func lessPath(a, b []int) bool {
	if b == nil {
		return true
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// CoveredByPaths conta as arestas main percorridas pelos caminhos.
func (g *Graph) CoveredByPaths(paths []Path) (covered, total int) {
	seen := map[int]bool{}
	for _, p := range paths {
		for _, i := range p.Edges {
			seen[i] = true
		}
	}
	for i, e := range g.Edges {
		if e.Category == CatMain {
			total++
			if seen[i] {
				covered++
			}
		}
	}
	return covered, total
}

// --- tradução de caminho em suíte (§5.3) ---

// ScaffoldOptions controla a geração.
type ScaffoldOptions struct {
	MaxScenarios int
	Card         string // valor de defaults.card ("" = sem fixture)
	SplitService bool   // separa os cenários que passam por service task
	ServiceCard  string // defaults.card do arquivo separado
}

// ScaffoldResult é o esqueleto gerado.
type ScaffoldResult struct {
	Main          []byte // suíte principal (JSON)
	Service       []byte // suíte dos cenários com service task (só com SplitService)
	MainCount     int
	ServiceCount  int
	Covered       int
	Total         int
	Todos         int
	Pending       []string // arestas que nenhum caminho cobriu
	ScenarioNames []string
}

type suiteOut struct {
	Process     string        `json:"process"`
	Description string        `json:"description"`
	Defaults    defaultsOut   `json:"defaults"`
	Scenarios   []scenarioOut `json:"scenarios"`
}

type defaultsOut struct {
	Card    string   `json:"card,omitempty"`
	Cleanup []string `json:"cleanup"`
}

type scenarioOut struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Steps       []stepOut `json:"steps"`
}

type stepOut struct {
	Description string     `json:"description,omitempty"`
	Start       *targetOut `json:"start,omitempty"`
	Assume      *bool      `json:"assume,omitempty"`
	Move        *targetOut `json:"move,omitempty"`
	Wait        *waitOut   `json:"wait,omitempty"`
	Expect      *expectOut `json:"expect,omitempty"`
}

type targetOut struct {
	Target int                `json:"target"`
	Fields *map[string]string `json:"fields,omitempty"`
}

type waitOut struct {
	State  int    `json:"state,omitempty"`
	Status string `json:"status,omitempty"`
}

type expectOut struct {
	State  int    `json:"state,omitempty"`
	Status string `json:"status,omitempty"`
}

func (g *Graph) name(seq int) string {
	if n := g.States[seq].Name; n != "" {
		return n
	}
	return strconv.Itoa(seq)
}

func (g *Graph) condText(e Edge) string {
	var alts []string
	for _, c := range e.Conds {
		if c.Expression != "" {
			alts = append(alts, c.Expression)
			continue
		}
		var parts []string
		for _, r := range c.Rules {
			parts = append(parts, strings.TrimSpace(r.Field+" "+r.Operator+" "+r.Value))
		}
		alts = append(alts, strings.Join(parts, " e "))
	}
	if len(alts) == 0 {
		return "(saída sem condição no diagrama)"
	}
	return strings.Join(alts, " ou ")
}

// mechanismNote descreve como a etapa humana é atribuída.
func (g *Graph) mechanismNote(seq int) string {
	s := g.States[seq]
	switch s.AssignKind {
	case "formField":
		return fmt.Sprintf("TODO: a etapa %d é atribuída pelo campo %s: preencha %s com {{actors.eu.code}} na fixture, ou troque este passo por setFields + manager.",
			seq, s.AssignValue, s.AssignValue)
	case "role", "group":
		return ""
	case "":
		if s.Mechanism == "" {
			return ""
		}
	}
	return fmt.Sprintf("TODO: a etapa %d usa o mecanismo %q (%s). Confira se o usuário da sessão é o responsável; senão, troque por setFields + manager.",
		seq, s.Mechanism, s.AssignValue)
}

func isPool(s StateInfo) bool { return strings.HasPrefix(s.Mechanism, "Pool") }

// scenarioFromPath traduz um caminho em cenário.
func (g *Graph) scenarioFromPath(n int, p Path) (scenarioOut, []int, int) {
	states := g.pathStates(p)
	sc := scenarioOut{}
	todos := 0
	var services []int
	for _, s := range states {
		if g.States[s].Kind == "service" && !containsInt(services, s) {
			services = append(services, s)
		}
	}

	// Paradas: etapas humanas do caminho (o início também é parada).
	var stops []int
	for i, s := range states {
		if i == 0 || g.States[s].Kind == "task" {
			stops = append(stops, i)
		}
	}
	last := states[len(states)-1]
	terminaNoFim := g.States[last].Kind == "end"

	for k, pos := range stops {
		next := len(states) - 1
		if k+1 < len(stops) {
			next = stops[k+1]
		}
		if pos == len(states)-1 {
			break // parada final (fallback): só o expect do passo anterior
		}
		seg := states[pos : next+1]
		var notas []string
		cur := states[pos]
		if k > 0 {
			if isPool(g.States[cur]) {
				t := true
				sc.Steps = append(sc.Steps, stepOut{Assume: &t})
			}
			if note := g.mechanismNote(cur); note != "" {
				notas = append(notas, note)
			}
		}
		// Condições dos gateways atravessados neste trecho.
		temService := false
		for j := 0; j+1 < len(seg); j++ {
			e := g.Edges[p.Edges[pos+j]]
			if g.States[e.From].Kind == "gateway" {
				notas = append(notas, fmt.Sprintf("TODO: o gateway %d (%s) vai para %d quando %s. Ajuste os fields para cumprir a condição.",
					e.From, g.name(e.From), e.To, g.condText(e)))
			}
			if g.States[e.To].Kind == "service" {
				temService = true
			}
		}
		alvo := states[pos+1]
		fields := map[string]string{}
		step := stepOut{}
		if k == 0 {
			step.Start = &targetOut{Target: alvo}
		} else {
			step.Move = &targetOut{Target: alvo, Fields: &fields}
			notas = append([]string{fmt.Sprintf("Sai de %d (%s) para %d (%s).", cur, g.name(cur), alvo, g.name(alvo))}, notas...)
		}
		destino := states[next]
		final := next == len(states)-1
		switch {
		case temService && final && terminaNoFim:
			sc.Steps = append(sc.Steps, withNotes(step, notas, &todos))
			sc.Steps = append(sc.Steps, stepOut{
				Description: "As service tasks rodam depois da resposta do move.",
				Wait:        &waitOut{Status: "FINALIZED"},
				Expect:      &expectOut{State: destino},
			})
		case temService:
			sc.Steps = append(sc.Steps, withNotes(step, notas, &todos))
			sc.Steps = append(sc.Steps, stepOut{
				Description: "As service tasks rodam depois da resposta do move.",
				Wait:        &waitOut{State: destino},
			})
		case final && terminaNoFim:
			step.Expect = &expectOut{Status: "FINALIZED"}
			sc.Steps = append(sc.Steps, withNotes(step, notas, &todos))
		default:
			step.Expect = &expectOut{State: destino}
			sc.Steps = append(sc.Steps, withNotes(step, notas, &todos))
		}
	}

	// Nome: etapa final + o gateway cuja saída era nova neste caminho.
	via := ""
	for _, i := range p.Novel {
		e := g.Edges[i]
		if g.States[e.From].Kind == "gateway" && len(g.States[e.From].Transitions) > 1 {
			via = g.name(e.From)
			break
		}
	}
	sc.Name = fmt.Sprintf("%02d: %s", n, g.name(last))
	if via != "" {
		sc.Name += " via " + via
	}
	caminho := make([]string, len(states))
	for i, s := range states {
		caminho[i] = strconv.Itoa(s)
	}
	desc := "Caminho: " + strings.Join(caminho, " → ") + "."
	if !terminaNoFim {
		desc += fmt.Sprintf(" O cenário termina na etapa %d (%s), antes de qualquer service task; a limpeza cancela a solicitação.",
			last, g.name(last))
	}
	if len(services) > 0 {
		ss := make([]string, len(services))
		for i, s := range services {
			ss[i] = strconv.Itoa(s)
		}
		desc = "ATENÇÃO: passa pelas service tasks " + strings.Join(ss, ", ") + "; pode gravar em sistema externo. " + desc
	}
	sc.Description = desc
	return sc, services, todos
}

func withNotes(s stepOut, notas []string, todos *int) stepOut {
	for _, n := range notas {
		if strings.HasPrefix(n, "TODO:") {
			*todos++
		}
	}
	s.Description = strings.Join(notas, " ")
	return s
}

func containsInt(xs []int, n int) bool {
	for _, x := range xs {
		if x == n {
			return true
		}
	}
	return false
}

// Scaffold gera o esqueleto da suíte a partir do grafo.
func Scaffold(g *Graph, o ScaffoldOptions) (*ScaffoldResult, error) {
	if o.MaxScenarios <= 0 {
		o.MaxScenarios = 30
	}
	paths, pend := g.EdgeCover(o.MaxScenarios)
	res := &ScaffoldResult{}
	res.Covered, res.Total = g.CoveredByPaths(paths)
	for _, i := range pend {
		e := g.Edges[i]
		res.Pending = append(res.Pending, fmt.Sprintf("%d→%d", e.From, e.To))
	}
	base := fmt.Sprintf("TODO: esqueleto gerado pelo fluigcli test new a partir da versão %d de %s. "+
		"Complete os fields de cada passo e confira as descriptions com TODO.", g.Version, g.Process)
	if o.Card == "" {
		base += " TODO: crie a fixture com os valores do formulário (fluigcli test new --from-request <n>) e aponte em defaults.card."
		res.Todos++
	}
	main := suiteOut{Process: g.Process, Description: base, Defaults: defaultsOut{Card: o.Card, Cleanup: []string{"cancel"}}}
	svc := suiteOut{Process: g.Process, Description: base + " Estes cenários passam por service task.",
		Defaults: defaultsOut{Card: o.ServiceCard, Cleanup: []string{"cancel"}}}
	if o.Card == "" {
		svc.Defaults.Card = ""
	}
	for n, p := range paths {
		sc, services, todos := g.scenarioFromPath(n+1, p)
		res.Todos += todos
		res.ScenarioNames = append(res.ScenarioNames, sc.Name)
		if o.SplitService && len(services) > 0 {
			svc.Scenarios = append(svc.Scenarios, sc)
		} else {
			main.Scenarios = append(main.Scenarios, sc)
		}
	}
	res.MainCount, res.ServiceCount = len(main.Scenarios), len(svc.Scenarios)
	var err error
	if len(main.Scenarios) > 0 {
		if res.Main, err = marshalSuite(main); err != nil {
			return nil, err
		}
	}
	if len(svc.Scenarios) > 0 {
		if res.Service, err = marshalSuite(svc); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func marshalSuite(s suiteOut) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
