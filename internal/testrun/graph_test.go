package testrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// pilotDetail lê o ProcessDetail real (saneado) do processo piloto, v32.
func pilotDetail(t *testing.T) *fluig.ProcessDetail {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test", "process_export_pagamentos_v32.xml"))
	if err != nil {
		t.Fatal(err)
	}
	pd, err := fluig.ParseProcessDetail(b)
	if err != nil {
		t.Fatal(err)
	}
	return pd
}

// pilotHistory lê um histórico real (HISTOR_PROCES) gravado no Gate C.
func pilotHistory(t *testing.T, id int) []fluig.HistoryMove {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test", fmt.Sprintf("history_%d.json", id)))
	if err != nil {
		t.Fatal(err)
	}
	var res fluig.DbResult
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	h, err := fluig.ParseHistoryRows(id, &res)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func edgeKeys(g *Graph, cat string) []string {
	var out []string
	for _, e := range g.Edges {
		if e.Category == cat {
			out = append(out, fmt.Sprintf("%d→%d", e.From, e.To))
		}
	}
	sort.Strings(out)
	return out
}

// C0.2: o diagrama tem as 22 arestas principais da spec e 15 de erro (as 14
// da spec mais a seta do próprio link 106→104).
func TestBuildGraphPiloto(t *testing.T) {
	g := BuildGraph(pilotDetail(t))
	main := edgeKeys(g, CatMain)
	want := []string{"4→98", "98→20", "98→23", "20→76", "20→13", "76→73", "76→23", "73→23", "73→13",
		"23→110", "23→13", "110→112", "110→28", "112→28", "112→13", "13→98",
		"28→34", "34→40", "40→46", "46→83", "83→91", "91→52"}
	sort.Strings(want)
	if strings.Join(main, " ") != strings.Join(want, " ") {
		t.Errorf("arestas main:\n got %v\nwant %v", main, want)
	}
	errs := edgeKeys(g, CatError)
	wantErr := []string{"34→36", "36→34", "40→42", "42→40", "46→48", "48→46", "28→31", "31→28", "31→106",
		"106→104", "104→13", "83→85", "85→83", "91→94", "94→91"}
	sort.Strings(wantErr)
	if strings.Join(errs, " ") != strings.Join(wantErr, " ") {
		t.Errorf("arestas error:\n got %v\nwant %v", errs, wantErr)
	}
	// O evento de erro 55 fica em Via da aresta 28→31 (boundary).
	for _, e := range g.Edges {
		if e.From == 28 && e.To == 31 && (len(e.Via) != 1 || e.Via[0] != 55) {
			t.Errorf("28→31 deveria atravessar o evento 55: %+v", e)
		}
	}
	// C0.3: a expressão do gateway 110 chega na aresta.
	achou := 0
	for _, e := range g.Edges {
		if e.From == 110 && e.To == 112 && len(e.Conds) == 1 && e.Conds[0].Expression == "hAPI.getCardValue('pgtoUrgente') == 'S'" {
			achou++
		}
		if e.From == 110 && e.To == 28 && len(e.Conds) == 1 && e.Conds[0].Expression == "hAPI.getCardValue('pgtoUrgente') != 'S'" {
			achou++
		}
		if e.From == 98 && e.To == 23 && len(e.Conds) != 3 {
			t.Errorf("98→23 tem três alternativas (coligadas 21, 28 e 13): %+v", e.Conds)
		}
	}
	if achou != 2 {
		t.Errorf("expressões do gateway 110 ausentes")
	}
	// C0.4: link de envio 106 é evento (antes saía "unknown").
	if s := g.States[106]; s.Kind != "event" || s.EventType != "linkThrow" {
		t.Errorf("106: %+v", s)
	}
	if s := g.States[55]; s.EventType != "error" || s.Parent != 28 {
		t.Errorf("55: %+v", s)
	}
	if got := fmt.Sprint(g.MainTasks()); got != "[13 20 23 73 112]" {
		t.Errorf("etapas humanas principais: %s", got)
	}
}

// C0.1: os históricos reais casam com arestas do diagrama, inclusive o erro de
// integração (28→31 sem movimento no evento) e o modo gestor (23→13).
func TestTraversedEdgesHistoricosReais(t *testing.T) {
	g := BuildGraph(pilotDetail(t))
	casos := map[int]string{
		230864: "4→98 98→20 20→76 76→23 23→13",
		230867: "4→98 98→20 20→76 76→23 23→110 110→112 112→13",
		230848: "4→98 98→20 20→76 76→23 23→110 110→28 28→34 34→40 40→46 46→83 83→91 91→52",
		226170: "4→98 98→23 23→28 28→31 31→28 28→31 31→28 28→31 31→28 28→31 31→28 28→34 34→40 40→46 46→83 83→91 91→52",
	}
	for id, want := range casos {
		var got []string
		for _, tr := range TraversedEdges(pilotHistory(t, id)) {
			got = append(got, fmt.Sprintf("%d→%d", tr.From, tr.To))
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%d:\n got %s\nwant %s", id, strings.Join(got, " "), want)
		}
	}
	// 226170 rodou na versão 29: o 23→28 não existe na v32 (lá há o gateway 110).
	if m := g.Match(Transition{From: 23, To: 28}); m != nil {
		t.Errorf("23→28 não existe na v32: %v", m)
	}
	if m := g.Match(Transition{From: 28, To: 31}); len(m) != 1 || g.Edges[m[0]].Category != CatError {
		t.Errorf("28→31 deveria casar com a aresta de erro: %v", m)
	}
	// Link 31→106→104→13: caso o histórico pule os eventos de link.
	m := g.Match(Transition{From: 31, To: 13})
	if len(m) != 3 {
		t.Errorf("31→13 deveria atravessar o link (3 arestas): %v", m)
	}
	if v := pilotHistory(t, 230841)[0].Version; v != 31 {
		t.Errorf("a 230841 rodou na v31: %d", v)
	}
}
