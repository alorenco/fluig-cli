package testrun

import (
	"fmt"
	"math"
	"sort"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// RequestRef é uma solicitação da fonte, com o cenário de origem quando há.
type RequestRef struct {
	RequestID int    `json:"requestId"`
	Scenario  string `json:"scenario,omitempty"`
	Suite     string `json:"suite,omitempty"`
}

// CoverageSource descreve de onde vieram as solicitações.
type CoverageSource struct {
	Kind     string   `json:"kind"` // run | result | requests
	Suites   []string `json:"suites,omitempty"`
	Requests []int    `json:"requests"`
}

// Ratio é uma métrica covered/total.
type Ratio struct {
	Covered int     `json:"covered"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// CoverageSummary são as métricas da §3.4.
type CoverageSummary struct {
	Main          Ratio `json:"main"`
	Error         Ratio `json:"error"`
	States        Ratio `json:"states"`
	IncludeErrors bool  `json:"includeErrors"`
	// Headline é o percentual que o --min confere: main, ou main+error com
	// --include-errors.
	Headline Ratio `json:"headline"`
}

// EdgeCoverage é uma aresta do diagrama com a cobertura.
type EdgeCoverage struct {
	From       int             `json:"from"`
	FromName   string          `json:"fromName"`
	FromKind   string          `json:"fromKind"`
	To         int             `json:"to"`
	ToName     string          `json:"toName"`
	Via        []int           `json:"via,omitempty"`
	Category   string          `json:"category"`
	Conditions []EdgeCondition `json:"conditions,omitempty"`
	Covered    bool            `json:"covered"`
	Hits       []RequestRef    `json:"hits"`
}

// GatewayCoverage resume as saídas de um gateway.
type GatewayCoverage struct {
	Sequence int              `json:"sequence"`
	Name     string           `json:"name"`
	Outputs  []GatewayOutcome `json:"outputs"`
}

// GatewayOutcome é uma saída de gateway.
type GatewayOutcome struct {
	To         int             `json:"to"`
	ToName     string          `json:"toName"`
	Covered    bool            `json:"covered"`
	Conditions []EdgeCondition `json:"conditions,omitempty"`
}

// StateRef é uma etapa (sequence + nome).
type StateRef struct {
	Sequence int    `json:"sequence"`
	Name     string `json:"name"`
}

// UnknownEdge é um par percorrido que não existe no diagrama de referência.
type UnknownEdge struct {
	From     int   `json:"from"`
	To       int   `json:"to"`
	Requests []int `json:"requests"`
}

// CoverageReport é o data do `test coverage` (SPEC-TEST-C §3.6).
type CoverageReport struct {
	Process         string            `json:"process"`
	Version         int               `json:"version"`
	Server          string            `json:"server"`
	Source          CoverageSource    `json:"source"`
	Summary         CoverageSummary   `json:"summary"`
	Edges           []EdgeCoverage    `json:"edges"`
	Gateways        []GatewayCoverage `json:"gateways"`
	UncoveredStates []StateRef        `json:"uncoveredStates"`
	Unknown         []UnknownEdge     `json:"unknown"`
	Warnings        []string          `json:"warnings"`
	Run             *Report           `json:"run,omitempty"`
}

// percent arredonda para uma casa decimal.
func percent(c, t int) float64 {
	if t == 0 {
		return 0
	}
	return math.Round(float64(c)*1000/float64(t)) / 10
}

// ComputeCoverage cruza os históricos com o grafo. histories traz só as
// solicitações que existem; refs inclui também as do start recusado (sem
// solicitação), que entram com zero arestas.
func ComputeCoverage(g *Graph, refs []RequestRef, histories map[int][]fluig.HistoryMove, includeErrors bool) CoverageReport {
	rep := CoverageReport{Process: g.Process, Version: g.Version, Unknown: []UnknownEdge{}, Warnings: []string{}}
	hits := make([][]RequestRef, len(g.Edges))
	visitadas := map[int]bool{}
	unknown := map[Transition]map[int]bool{}
	avisou := map[int]bool{}

	for _, ref := range refs {
		hist, ok := histories[ref.RequestID]
		if !ok {
			continue
		}
		for _, m := range hist {
			visitadas[m.State] = true
			if m.Version != g.Version && !avisou[ref.RequestID] {
				avisou[ref.RequestID] = true
				rep.Warnings = append(rep.Warnings, fmt.Sprintf(
					"a %d rodou na versão %d; a referência é a %d", ref.RequestID, m.Version, g.Version))
			}
		}
		for _, tr := range TraversedEdges(hist) {
			idx := g.Match(tr)
			if idx == nil {
				if unknown[tr] == nil {
					unknown[tr] = map[int]bool{}
				}
				unknown[tr][ref.RequestID] = true
				continue
			}
			for _, i := range idx {
				if !hasRef(hits[i], ref.RequestID) {
					hits[i] = append(hits[i], ref)
				}
			}
		}
	}

	var main, errs Ratio
	for i, e := range g.Edges {
		ec := EdgeCoverage{
			From: e.From, FromName: g.States[e.From].Name, FromKind: g.States[e.From].Kind,
			To: e.To, ToName: g.States[e.To].Name, Via: e.Via, Category: e.Category,
			Covered: len(hits[i]) > 0, Hits: hits[i],
		}
		if ec.Hits == nil {
			ec.Hits = []RequestRef{}
		}
		if g.States[e.From].Kind == "gateway" {
			ec.Conditions = e.Conds
		}
		r := &main
		if e.Category == CatError {
			r = &errs
		}
		r.Total++
		if ec.Covered {
			r.Covered++
		}
		rep.Edges = append(rep.Edges, ec)
	}
	main.Percent, errs.Percent = percent(main.Covered, main.Total), percent(errs.Covered, errs.Total)
	rep.Summary.Main, rep.Summary.Error = main, errs
	rep.Summary.IncludeErrors = includeErrors
	rep.Summary.Headline = main
	if includeErrors {
		c, t := main.Covered+errs.Covered, main.Total+errs.Total
		rep.Summary.Headline = Ratio{Covered: c, Total: t, Percent: percent(c, t)}
	}

	tasks := g.MainTasks()
	rep.UncoveredStates = []StateRef{}
	for _, s := range tasks {
		if visitadas[s] {
			rep.Summary.States.Covered++
		} else {
			rep.UncoveredStates = append(rep.UncoveredStates, StateRef{Sequence: s, Name: g.States[s].Name})
		}
	}
	rep.Summary.States.Total = len(tasks)
	rep.Summary.States.Percent = percent(rep.Summary.States.Covered, rep.Summary.States.Total)

	// Gateways, na ordem do número da etapa.
	var gws []int
	for seq, s := range g.States {
		if s.Kind == "gateway" {
			gws = append(gws, seq)
		}
	}
	sort.Ints(gws)
	for _, seq := range gws {
		gc := GatewayCoverage{Sequence: seq, Name: g.States[seq].Name}
		for _, ec := range rep.Edges {
			if ec.From == seq {
				gc.Outputs = append(gc.Outputs, GatewayOutcome{To: ec.To, ToName: ec.ToName, Covered: ec.Covered, Conditions: ec.Conditions})
			}
		}
		rep.Gateways = append(rep.Gateways, gc)
	}

	for tr, reqs := range unknown {
		u := UnknownEdge{From: tr.From, To: tr.To}
		for r := range reqs {
			u.Requests = append(u.Requests, r)
		}
		sort.Ints(u.Requests)
		rep.Unknown = append(rep.Unknown, u)
	}
	sort.Slice(rep.Unknown, func(i, j int) bool {
		if rep.Unknown[i].From != rep.Unknown[j].From {
			return rep.Unknown[i].From < rep.Unknown[j].From
		}
		return rep.Unknown[i].To < rep.Unknown[j].To
	})

	// Ordem das linhas: não cobertas primeiro; depois origem e destino.
	sort.SliceStable(rep.Edges, func(i, j int) bool {
		a, b := rep.Edges[i], rep.Edges[j]
		if a.Covered != b.Covered {
			return !a.Covered
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	return rep
}

func hasRef(rs []RequestRef, id int) bool {
	for _, r := range rs {
		if r.RequestID == id {
			return true
		}
	}
	return false
}

// RefsFromReport extrai as solicitações de um relatório do `test run`.
func RefsFromReport(r *Report) []RequestRef {
	var out []RequestRef
	for _, su := range r.Suites {
		for _, sc := range su.Scenarios {
			out = append(out, RequestRef{RequestID: sc.RequestID, Scenario: sc.Name, Suite: su.File})
		}
	}
	return out
}
