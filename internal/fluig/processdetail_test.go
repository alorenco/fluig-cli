package fluig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseProcessDetail cobre o parser do export XML nativo com a fixture
// real sanitizada do compras_entrada_documento v25 (Fase 0, 2026-07-21).
func TestParseProcessDetail(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "rest_process_export_full.xml"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	d, err := ParseProcessDetail(data)
	if err != nil {
		t.Fatalf("ParseProcessDetail: %v", err)
	}

	if d.ID != "compras_entrada_documento" {
		t.Errorf("ID = %q", d.ID)
	}
	if d.Description != "Entrada de Documento" {
		t.Errorf("Description = %q", d.Description)
	}
	if d.Version != 25 {
		t.Errorf("Version = %d, quero 25", d.Version)
	}
	if d.Author == "" {
		t.Errorf("Author vazio (esperava o autor sanitizado)")
	}
	if d.FormID != 263801 {
		t.Errorf("FormID = %d, quero 263801", d.FormID)
	}
	if len(d.States) != 21 {
		t.Fatalf("States = %d, quero 21", len(d.States))
	}
	// Ordenado por sequence.
	for i := 1; i < len(d.States); i++ {
		if d.States[i-1].Sequence > d.States[i].Sequence {
			t.Errorf("States fora de ordem em %d", i)
		}
	}
	if strings.TrimSpace(d.DiagramSVG) == "" || !strings.Contains(d.DiagramSVG, "<svg") {
		t.Errorf("DiagramSVG não parece um SVG (len=%d)", len(d.DiagramSVG))
	}

	byName := map[string]ProcessStateDetail{}
	bySeq := map[int]ProcessStateDetail{}
	for _, s := range d.States {
		byName[s.Name] = s
		bySeq[s.Sequence] = s
	}

	// Atribuição por Papel: Faturar Documento (seq 17) → faturista.
	fat := byName["Faturar Documento"]
	if fat.Kind != "task" || fat.Assignment == nil {
		t.Fatalf("Faturar Documento: kind=%q assignment=%v", fat.Kind, fat.Assignment)
	}
	if fat.Assignment.Mechanism != "Pool Papel" || fat.Assignment.Kind != "role" || fat.Assignment.Value != "faturista" {
		t.Errorf("Faturar Documento assignment = %+v", fat.Assignment)
	}

	// Atribuição por Campo de Formulário: Aprovação Diretoria (seq 5).
	dir := byName["Aprovação Diretoria"]
	if dir.Assignment == nil || dir.Assignment.Kind != "formField" || dir.Assignment.Value != "diretorAprovador" {
		t.Errorf("Aprovação Diretoria assignment = %+v", dir.Assignment)
	}

	// Atribuição por Grupo: Corrigir Integração → TI.
	corr := byName["Corrigir Integração"]
	if corr.Assignment == nil || corr.Assignment.Kind != "group" || corr.Assignment.Value != "TI" {
		t.Errorf("Corrigir Integração assignment = %+v", corr.Assignment)
	}

	// Executor de Atividade: Revisar Processo (seq 26) → baseActivity 4.
	rev := byName["Revisar Processo"]
	if rev.Assignment == nil || rev.Assignment.Kind != "baseActivity" || rev.Assignment.Value != "4" {
		t.Errorf("Revisar Processo assignment = %+v", rev.Assignment)
	}

	// Service task: Integrar Totvs RM (seq 19) — kind service, config e transição.
	itg := byName["Integrar Totvs RM"]
	if itg.Kind != "service" || itg.Service == nil {
		t.Fatalf("Integrar Totvs RM: kind=%q service=%v", itg.Kind, itg.Service)
	}
	if itg.Service.Attempts != 2 || itg.Service.Frequency != 5 {
		t.Errorf("Integrar Totvs RM service = %+v", itg.Service)
	}
	// Transição: Faturar Documento (17) → Integrar Totvs RM (19) e Revisar Processo (26).
	dests := map[int]bool{}
	for _, tr := range fat.Transitions {
		dests[tr.To] = true
	}
	if !dests[19] || !dests[26] {
		t.Errorf("transições de Faturar Documento = %+v (quero 19 e 26)", fat.Transitions)
	}

	// Gateway: Aprovar Documento (seq 50) — condições com regras.
	gw := byName["Aprovar Documento"]
	if gw.Kind != "gateway" || len(gw.Conditions) == 0 {
		t.Fatalf("Aprovar Documento: kind=%q conditions=%d", gw.Kind, len(gw.Conditions))
	}
	foundRule := false
	for _, c := range gw.Conditions {
		for _, r := range c.Rules {
			if r.Field == "aprNivel1" {
				foundRule = true
			}
		}
	}
	if !foundRule {
		t.Errorf("gateway Aprovar Documento sem a regra aprNivel1: %+v", gw.Conditions)
	}

	// Eventos: 9, com beforeTaskSave e servicetask19 marcado como service.
	if len(d.Events) != 9 {
		t.Errorf("Events = %d, quero 9", len(d.Events))
	}
	var hasBTS, hasSvc bool
	for _, e := range d.Events {
		if e.Event == "beforeTaskSave" && e.CodeChars > 0 {
			hasBTS = true
		}
		if e.Event == "servicetask19" && e.Service {
			hasSvc = true
		}
	}
	if !hasBTS {
		t.Errorf("evento beforeTaskSave ausente ou vazio")
	}
	if !hasSvc {
		t.Errorf("evento servicetask19 não marcado como service")
	}
}

func TestBpmnKind(t *testing.T) {
	cases := map[int]string{10: "start", 60: "end", 80: "task", 82: "service", 120: "gateway", 43: "event", 999: "unknown"}
	for bpmn, want := range cases {
		if got := bpmnKind(bpmn); got != want {
			t.Errorf("bpmnKind(%d) = %q, quero %q", bpmn, got, want)
		}
	}
}
