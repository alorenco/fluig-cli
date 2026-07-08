package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWorkflowScriptName(t *testing.T) {
	cases := []struct {
		in        string
		wantProc  string
		wantEvent string
		wantOK    bool
	}{
		{"Compras.beforeTaskSave.js", "Compras", "beforeTaskSave", true},
		{"meu_processo.afterTaskCreate.js", "meu_processo", "afterTaskCreate", true},
		{"a.b.onLoad.js", "a.b", "onLoad", true},
		{"semponto.js", "", "", false},
		{"Compras..js", "", "", false}, // evento vazio → inválido
	}
	for _, tc := range cases {
		pid, ev, ok := ParseWorkflowScriptName(tc.in)
		if ok != tc.wantOK || pid != tc.wantProc || ev != tc.wantEvent {
			t.Errorf("ParseWorkflowScriptName(%q) = (%q,%q,%v), quer (%q,%q,%v)",
				tc.in, pid, ev, ok, tc.wantProc, tc.wantEvent, tc.wantOK)
		}
	}
}

func TestFindProcessScripts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, WorkflowScriptsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Compras.beforeTaskSave.js", "Compras.afterTaskComplete.js", "Vendas.onLoad.js", "leiame.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FindProcessScripts(root, "Compras")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 scripts de Compras, veio %d: %+v", len(got), got)
	}
	for _, s := range got {
		if s.ProcessID != "Compras" {
			t.Errorf("processId inesperado: %+v", s)
		}
	}
}
