package testrun

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func scaffoldPiloto(t *testing.T, split bool) *ScaffoldResult {
	t.Helper()
	g := BuildGraph(pilotDetail(t))
	res, err := Scaffold(g, ScaffoldOptions{Card: "fixtures/card_base.json", SplitService: split, ServiceCard: "../fixtures/card_base.json"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// §7.2: os caminhos gerados cobrem as 22 arestas main do piloto.
func TestScaffoldPilotoCobreTudo(t *testing.T) {
	res := scaffoldPiloto(t, true)
	if res.Covered != 22 || res.Total != 22 || len(res.Pending) != 0 {
		t.Fatalf("cobertura dos caminhos: %d/%d pendentes %v", res.Covered, res.Total, res.Pending)
	}
	if res.MainCount == 0 || res.ServiceCount == 0 {
		t.Fatalf("o split precisa dos dois arquivos: %d + %d", res.MainCount, res.ServiceCount)
	}
	main, svc := string(res.Main), string(res.Service)
	if strings.Contains(main, "ATENÇÃO: passa pelas service tasks") || strings.Contains(main, `"wait"`) {
		t.Error("a suíte rápida não pode ter cenário que passa por service task")
	}
	if !strings.Contains(svc, "ATENÇÃO: passa pelas service tasks 28, 34, 40, 46, 83, 91") ||
		!strings.Contains(svc, `"status": "FINALIZED"`) || !strings.Contains(svc, `"card": "../fixtures/card_base.json"`) {
		t.Errorf("suíte de integração incompleta:\n%s", svc)
	}
	todo := main + svc
	for _, want := range []string{
		"vai para 112 quando hAPI.getCardValue('pgtoUrgente') == 'S'",
		"atribuída pelo campo diretorAprovador",
		"vai para 23 quando codColigada == 21 ou codColigada == 28 ou codColigada == 13",
		`"fields": {}`,
	} {
		if !strings.Contains(todo, want) {
			t.Errorf("esqueleto sem %q", want)
		}
	}
}

// Passos que saem de etapa de pool (20 e 112) têm assume antes; o JSON gerado
// é uma suíte válida para o próprio carregador.
func TestScaffoldAssumeEValidade(t *testing.T) {
	res, err := Scaffold(BuildGraph(pilotDetail(t)), ScaffoldOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := ParseSuite(filepath.Join("x", "s.json"), res.Main, LoadOptions{})
	if err != nil {
		t.Fatalf("o esqueleto não carrega: %v", err)
	}
	g := BuildGraph(pilotDetail(t))
	for _, sc := range s.Scenarios {
		atual := 0
		for i, st := range sc.Steps {
			if st.Action == ActMove && isPool(g.States[atual]) {
				if i == 0 || sc.Steps[i-1].Action != ActAssume {
					t.Errorf("%s passo %d: sai do pool %d sem assume", sc.Name, st.Index, atual)
				}
			}
			if st.Expect != nil && st.Expect.State != nil {
				atual = *st.Expect.State
			}
			if st.Action == ActWait && st.WaitState > 0 {
				atual = st.WaitState
			}
		}
		if sc.Steps[0].Action != ActStart || strings.Join(sc.Cleanup, ",") != "cancel" {
			t.Errorf("%s: começa com start e limpa com cancel", sc.Name)
		}
	}
	if len(s.Scenarios) != res.MainCount {
		t.Errorf("sem split, tudo vai para o arquivo principal: %d", len(s.Scenarios))
	}
}

func TestScaffoldDeterministico(t *testing.T) {
	a, b := scaffoldPiloto(t, true), scaffoldPiloto(t, true)
	if !bytes.Equal(a.Main, b.Main) || !bytes.Equal(a.Service, b.Service) {
		t.Error("a geração não é determinística")
	}
}

func TestScaffoldLimiteDeCenarios(t *testing.T) {
	g := BuildGraph(pilotDetail(t))
	res, err := Scaffold(g, ScaffoldOptions{MaxScenarios: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.MainCount+res.ServiceCount != 2 || len(res.Pending) == 0 {
		t.Errorf("limite: %d cenários, pendentes %v", res.MainCount+res.ServiceCount, res.Pending)
	}
	// Sem fixture: o defaults.card fica de fora e a suíte pede para criar.
	var su struct {
		Description string         `json:"description"`
		Defaults    map[string]any `json:"defaults"`
	}
	json.Unmarshal(res.Main, &su)
	if _, ok := su.Defaults["card"]; ok || !strings.Contains(su.Description, "--from-request") {
		t.Errorf("sem fixture: %+v", su)
	}
}
