package testrun

import (
	"sort"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// Categorias de aresta (SPEC-TEST-C §3.2).
const (
	CatMain  = "main"
	CatError = "error"
)

// Edge é uma transição do diagrama.
type Edge struct {
	From     int             `json:"from"`
	To       int             `json:"to"`
	Via      []int           `json:"via,omitempty"` // evento de erro atravessado (boundary)
	Category string          `json:"category"`
	Label    string          `json:"label,omitempty"`
	Conds    []EdgeCondition `json:"conditions,omitempty"` // alternativas (OU) do gateway
}

// EdgeCondition é uma alternativa de condição de um gateway que leva ao
// destino da aresta. Um destino pode ter várias alternativas (OU): o gateway
// 98 do piloto vai à 23 com a coligada 21, OU 28, OU 13.
type EdgeCondition struct {
	Expression string              `json:"expression,omitempty"`
	Rules      []fluig.GatewayRule `json:"rules,omitempty"`
}

// Graph é o diagrama de uma versão do processo, pronto para cobertura e para
// geração de caminhos.
type Graph struct {
	Process string
	Version int
	States  map[int]StateInfo
	Edges   []Edge
	Start   []int
	End     []int
	// reach marca as etapas alcançáveis do início sem passar por evento de erro.
	reach map[int]bool
}

// BuildGraph monta o grafo a partir do ProcessDetail e classifica as arestas.
//
// O evento de erro (bpmnType 43) fica preso à service task pelo
// ParentSequence e não tem seta vinda dela. Medido no Gate C: o erro NÃO cria
// movimento no evento; a solicitação vai direto da service task para a tarefa
// de correção. Por isso a seta "evento → correção" vira a aresta
// "service task → correção", com o evento em Via.
func BuildGraph(pd *fluig.ProcessDetail) *Graph {
	g := &Graph{Process: pd.ID, Version: pd.Version, States: map[int]StateInfo{}}
	for _, s := range pd.States {
		si := stateInfoFrom(s)
		g.States[s.Sequence] = si
		switch s.Kind {
		case "start":
			g.Start = append(g.Start, s.Sequence)
		case "end":
			g.End = append(g.End, s.Sequence)
		}
	}
	for _, s := range pd.States {
		conds := map[int][]EdgeCondition{}
		if s.Kind == "gateway" {
			for _, c := range s.Conditions {
				if c.Expression == "" && len(c.Rules) == 0 {
					continue
				}
				conds[c.To] = append(conds[c.To], EdgeCondition{Expression: c.Expression, Rules: c.Rules})
			}
		}
		for _, t := range s.Transitions {
			e := Edge{From: s.Sequence, To: t.To, Label: t.Label, Conds: conds[t.To]}
			if s.EventType == "error" && s.ParentSequence > 0 {
				e.From, e.Via = s.ParentSequence, []int{s.Sequence}
			}
			g.Edges = append(g.Edges, e)
		}
	}
	sort.SliceStable(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		return g.Edges[i].To < g.Edges[j].To
	})
	g.Classify()
	return g
}

func stateInfoFrom(s fluig.ProcessStateDetail) StateInfo {
	si := StateInfo{Name: s.Name, Kind: s.Kind, EventType: s.EventType, Parent: s.ParentSequence}
	for _, t := range s.Transitions {
		si.Transitions = append(si.Transitions, t.To)
	}
	if s.Assignment != nil {
		si.Mechanism, si.AssignKind, si.AssignValue = s.Assignment.Mechanism, s.Assignment.Kind, s.Assignment.Value
	}
	return si
}

// ProcessInfoFrom monta o ProcessInfo da fase A a partir do detalhe.
func ProcessInfoFrom(pd *fluig.ProcessDetail) *ProcessInfo {
	p := &ProcessInfo{Version: pd.Version, States: map[int]StateInfo{}}
	for _, s := range pd.States {
		p.States[s.Sequence] = stateInfoFrom(s)
		if s.Kind == "start" {
			p.Start = append(p.Start, s.Sequence)
		}
	}
	return p
}

// Classify marca cada aresta como main ou error: BFS a partir do início SEM
// passar por evento de erro. Uma etapa que a busca não alcança é de
// tratamento de erro, e toda aresta que sai dela também.
func (g *Graph) Classify() {
	g.reach = map[int]bool{}
	fila := append([]int(nil), g.Start...)
	for _, s := range fila {
		g.reach[s] = true
	}
	for len(fila) > 0 {
		cur := fila[0]
		fila = fila[1:]
		for _, e := range g.Edges {
			if e.From != cur || len(e.Via) > 0 || g.reach[e.To] {
				continue
			}
			g.reach[e.To] = true
			fila = append(fila, e.To)
		}
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		if len(e.Via) > 0 || !g.reach[e.From] {
			e.Category = CatError
		} else {
			e.Category = CatMain
		}
	}
}

// Reachable informa se a etapa é do fluxo principal.
func (g *Graph) Reachable(seq int) bool { return g.reach[seq] }

// MainTasks lista as etapas humanas do fluxo principal, em ordem.
func (g *Graph) MainTasks() []int {
	var out []int
	for seq, s := range g.States {
		if s.Kind == "task" && g.reach[seq] {
			out = append(out, seq)
		}
	}
	sort.Ints(out)
	return out
}

// Transition é um par (origem, destino) percorrido por uma solicitação.
type Transition struct {
	From, To int
}

// TraversedEdges converte o histórico em pares percorridos: (etapa do
// movimento anterior, etapa do movimento). O movimento com anterior 0 é a
// entrada pelo início e não gera par.
func TraversedEdges(hist []fluig.HistoryMove) []Transition {
	estado := map[int]int{}
	for _, m := range hist {
		estado[m.Movement] = m.State
	}
	var out []Transition
	for _, m := range hist {
		if m.PreviousMovement <= 0 {
			continue
		}
		pai, ok := estado[m.PreviousMovement]
		if !ok {
			continue
		}
		out = append(out, Transition{From: pai, To: m.State})
	}
	return out
}

// Match devolve os índices das arestas do diagrama que explicam o par
// percorrido. Primeiro a aresta direta. Depois um caminho que só atravessa
// eventos intermediários sem movimento próprio (link de envio e recebimento).
// Vazio = par desconhecido no diagrama.
func (g *Graph) Match(t Transition) []int {
	for i, e := range g.Edges {
		if e.From == t.From && e.To == t.To {
			return []int{i}
		}
	}
	// BFS por eventos intermediários (não de erro) entre From e To.
	type no struct {
		seq  int
		path []int
	}
	visto := map[int]bool{t.From: true}
	fila := []no{{seq: t.From}}
	for len(fila) > 0 {
		cur := fila[0]
		fila = fila[1:]
		for i, e := range g.Edges {
			if e.From != cur.seq || len(e.Via) > 0 {
				continue
			}
			path := append(append([]int(nil), cur.path...), i)
			if e.To == t.To {
				return path
			}
			st := g.States[e.To]
			if st.Kind == "event" && st.EventType != "error" && !visto[e.To] {
				visto[e.To] = true
				fila = append(fila, no{seq: e.To, path: path})
			}
		}
	}
	return nil
}
