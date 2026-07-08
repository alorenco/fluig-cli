package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// WorkflowScriptsDir é a pasta convencional dos scripts de processo:
// workflow/scripts/<Processo>.<evento>.js.
var WorkflowScriptsDir = filepath.Join("workflow", "scripts")

// ParseWorkflowScriptName extrai (processId, evento) do nome de um script.
// O separador é o ÚLTIMO ponto: "Compras.beforeTaskSave" → ("Compras",
// "beforeTaskSave"); "a.b.onLoad" → ("a.b", "onLoad"). Falha se não houver ponto.
func ParseWorkflowScriptName(path string) (processID, event string, ok bool) {
	base := strings.TrimSuffix(filepath.Base(path), ".js")
	i := strings.LastIndex(base, ".")
	if i <= 0 || i == len(base)-1 {
		return "", "", false
	}
	return base[:i], base[i+1:], true
}

// ProcessScript é um script de evento de processo local.
type ProcessScript struct {
	ProcessID string
	Event     string
	Path      string
}

// FindProcessScripts lista todos os scripts de um processo sob
// workflow/scripts/<processId>.*.js (busca recursiva).
func FindProcessScripts(root, processID string) ([]ProcessScript, error) {
	base := filepath.Join(root, WorkflowScriptsDir)
	var out []ProcessScript
	walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".js") {
			return nil
		}
		pid, ev, ok := ParseWorkflowScriptName(d.Name())
		if ok && pid == processID {
			out = append(out, ProcessScript{ProcessID: pid, Event: ev, Path: p})
		}
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return nil, walkErr
	}
	return out, nil
}
