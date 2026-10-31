package testrun

import (
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

func TestComputeCoveragePiloto(t *testing.T) {
	g := BuildGraph(pilotDetail(t))
	hist := map[int][]fluig.HistoryMove{}
	refs := []RequestRef{}
	for _, id := range []int{230841, 230848, 230864, 226170} {
		hist[id] = pilotHistory(t, id)
		refs = append(refs, RequestRef{RequestID: id})
	}
	// Start recusado: entra sem solicitação e sem aviso.
	refs = append(refs, RequestRef{Scenario: "start recusado"})
	c := ComputeCoverage(g, refs, hist, false)

	if c.Summary.Main.Total != 22 || c.Summary.Error.Total != 15 {
		t.Fatalf("totais: %+v", c.Summary)
	}
	// 230848/230841: caminho não urgente até o fim (12 arestas) + 230864: 23→13
	// + 226170 (v29): 98→23.
	if c.Summary.Main.Covered != 14 || c.Summary.Main.Percent != 63.6 {
		t.Errorf("main: %+v", c.Summary.Main)
	}
	if c.Summary.Error.Covered != 2 { // 28→31 e 31→28 (226170)
		t.Errorf("error: %+v", c.Summary.Error)
	}
	if c.Summary.Headline != c.Summary.Main {
		t.Errorf("sem --include-errors a manchete é o main: %+v", c.Summary.Headline)
	}
	avisos := strings.Join(c.Warnings, "\n")
	for _, want := range []string{"a 230841 rodou na versão 31; a referência é a 32", "a 226170 rodou na versão 29"} {
		if !strings.Contains(avisos, want) {
			t.Errorf("avisos sem %q:\n%s", want, avisos)
		}
	}
	if strings.Count(avisos, "230841") != 1 {
		t.Error("um aviso por solicitação")
	}
	// 23→28 só existe na v29: vai para unknown, fora do percentual.
	if len(c.Unknown) != 1 || c.Unknown[0].From != 23 || c.Unknown[0].To != 28 || c.Unknown[0].Requests[0] != 226170 {
		t.Errorf("unknown: %+v", c.Unknown)
	}
	// Não cobertas primeiro.
	if c.Edges[0].Covered || !c.Edges[len(c.Edges)-1].Covered {
		t.Error("ordem: não cobertas primeiro")
	}
	// Etapas humanas: 20, 23 e 13 visitadas; 73 e 112 não.
	if c.Summary.States.Covered != 3 || c.Summary.States.Total != 5 || len(c.UncoveredStates) != 2 ||
		c.UncoveredStates[0].Sequence != 73 || c.UncoveredStates[1].Sequence != 112 {
		t.Errorf("etapas: %+v %+v", c.Summary.States, c.UncoveredStates)
	}
	for _, gw := range c.Gateways {
		if gw.Sequence == 110 {
			for _, o := range gw.Outputs {
				if (o.To == 28) != o.Covered {
					t.Errorf("110: %+v", gw)
				}
				if len(o.Conditions) != 1 || o.Conditions[0].Expression == "" {
					t.Errorf("110 sem expressão: %+v", o)
				}
			}
		}
	}

	c = ComputeCoverage(g, refs, hist, true)
	if c.Summary.Headline.Total != 37 || c.Summary.Headline.Covered != 16 || !c.Summary.IncludeErrors {
		t.Errorf("--include-errors: %+v", c.Summary.Headline)
	}
}

func TestRefsFromReport(t *testing.T) {
	r := &Report{Suites: []SuiteResult{{File: "a.json", Scenarios: []ScenarioResult{{Name: "x", RequestID: 5}, {Name: "y"}}}}}
	refs := RefsFromReport(r)
	if len(refs) != 2 || refs[0].RequestID != 5 || refs[0].Scenario != "x" || refs[0].Suite != "a.json" {
		t.Errorf("%+v", refs)
	}
}
