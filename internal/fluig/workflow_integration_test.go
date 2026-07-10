//go:build integration

package fluig

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"testing"
)

func integrationProcessID() string {
	if p := os.Getenv("FLUIGCLI_TEST_PROCESS"); p != "" {
		return p
	}
	return "meu_processo" // placeholder; defina FLUIGCLI_TEST_PROCESS
}

// TestIntegrationListProcesses confirma a listagem de processos (REST v2,
// read-only): envelope {items, hasNext} paginado.
func TestIntegrationListProcesses(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	procs, err := c.ListProcesses(context.Background())
	if err != nil {
		t.Fatalf("ListProcesses: %v", err)
	}
	t.Logf("%d processo(s) no servidor", len(procs))
	if len(procs) == 0 {
		t.Error("nenhum processo listado — esperado ao menos 1 na homologação")
	}
	for _, p := range procs {
		if p.ID == "" {
			t.Errorf("processo sem ID: %+v", p)
		}
	}
}

// TestIntegrationProcessPublishCycle valida o ciclo de escrita do publish
// nativo: cria um processo de teste (import novo), aplica um script alterado
// (import = versão nova) e confere pelo export; remove tudo ao final.
// Opt-in: FLUIGCLI_TEST_PROCESS_WRITE=1 (cria e apaga zz_fluigcli_test_publish).
func TestIntegrationProcessPublishCycle(t *testing.T) {
	if os.Getenv("FLUIGCLI_TEST_PROCESS_WRITE") != "1" {
		t.Skip("defina FLUIGCLI_TEST_PROCESS_WRITE=1 para rodar o ciclo de escrita de processo")
	}
	const pid = "zz_fluigcli_test_publish"
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// A fixture é o export real de um processo mínimo com o evento beforeTaskSave.
	seed, err := os.ReadFile("../../testdata/rest_process_export.xml")
	if err != nil {
		t.Fatal(err)
	}
	seed = bytes.ReplaceAll(seed, []byte(">zz_fluigcli_test_pub<"), []byte(">"+pid+"<"))
	if err := c.ImportNewProcessXML(ctx, pid, seed); err != nil {
		t.Fatalf("ImportNewProcessXML: %v", err)
	}
	t.Cleanup(func() {
		for {
			vs, err := c.ProcessVersions(ctx, pid)
			if err != nil || len(vs) <= 1 {
				break
			}
			if err := c.DeleteLatestProcessVersion(ctx, pid); err != nil {
				t.Logf("limpeza: DeleteLatestProcessVersion: %v", err)
				break
			}
		}
		if err := c.DeleteProcess(ctx, pid); err != nil {
			t.Errorf("limpeza: DeleteProcess: %v (remova %s à mão)", err, pid)
		}
	})

	xmlData, err := c.ExportProcessXML(ctx, pid)
	if err != nil {
		t.Fatalf("ExportProcessXML: %v", err)
	}
	const novo = "function beforeTaskSave(colleagueId,nextSequenceId,userList){ /* publish integração */ }"
	newXML, updated, missing := ApplyProcessEventScripts(xmlData, map[string]string{"beforeTaskSave": novo})
	if len(missing) != 0 || len(updated) != 1 {
		t.Fatalf("apply: updated=%v missing=%v", updated, missing)
	}
	if err := c.ImportProcessXML(ctx, pid, newXML); err != nil {
		t.Fatalf("ImportProcessXML: %v", err)
	}

	vs, err := c.ProcessVersions(ctx, pid)
	if err != nil {
		t.Fatalf("ProcessVersions: %v", err)
	}
	if got := LatestProcessVersion(vs); got != 2 {
		t.Errorf("última versão = %d, quer 2 (import cria versão nova)", got)
	}
	events, err := c.ProcessEventScripts(ctx, pid)
	if err != nil {
		t.Fatalf("ProcessEventScripts: %v", err)
	}
	if events["beforeTaskSave"] != novo {
		t.Errorf("script não foi aplicado na versão nova:\n%q", events["beforeTaskSave"])
	}
}

// TestIntegrationWorkflowVersion confirma o workflow version nativo (read-only).
func TestIntegrationWorkflowVersion(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.WorkflowVersion(context.Background(), integrationProcessID())
	if err != nil {
		t.Fatalf("WorkflowVersion: %v", err)
	}
	t.Logf("processo %q → versão %d", integrationProcessID(), v)
	if v <= 0 {
		t.Errorf("versão inesperada (%d) — processo existe?", v)
	}
}

// TestIntegrationWorkflowExportZip valida o export nativo do processo (zip com a
// ProcessDefinition), base do futuro workflow publish.
func TestIntegrationWorkflowExportZip(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.ExportProcessZip(context.Background(), integrationProcessID())
	if err != nil {
		t.Fatalf("ExportProcessZip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip inválido (%d bytes): %v", len(data), err)
	}
	if len(zr.File) == 0 {
		t.Fatal("zip do processo vazio")
	}
	t.Logf("export ok: %d bytes, %d entrada(s)", len(data), len(zr.File))
}

// TestIntegrationProcessStates valida os endpoints da simulação de contexto do
// `fluigcli dev` (tudo read-only): versões com formId, etapas (states) da
// última versão e a busca reversa de processos por formId (expand=versions).
// ⚠️ Primeiro gate de validação viva destes endpoints — os schemas vieram do
// swagger real, mas a homologação estava fora do ar em 2026-07-10.
func TestIntegrationProcessStates(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pid := integrationProcessID()

	vs, err := c.ProcessVersions(ctx, pid)
	if err != nil {
		t.Fatalf("ProcessVersions: %v", err)
	}
	latest := LatestProcessVersion(vs)
	if latest == 0 {
		t.Fatalf("processo %q sem versões", pid)
	}
	states, err := c.ProcessStates(ctx, pid, latest)
	if err != nil {
		t.Fatalf("ProcessStates(%q, %d): %v", pid, latest, err)
	}
	if len(states) < 2 {
		t.Errorf("esperava ao menos início e fim, veio %d estado(s): %+v", len(states), states)
	}
	for _, st := range states {
		t.Logf("etapa %d — %q (stateType=%q bpmnType=%q)", st.Sequence, st.Name, st.StateType, st.BpmnType)
	}

	// Busca reversa pelo formId da última versão (quando o processo tem form).
	formID := 0
	for _, v := range vs {
		if v.Version == latest {
			formID = v.FormID
		}
	}
	if formID <= 0 {
		t.Logf("processo %q sem formulário na versão %d — busca reversa não testada", pid, latest)
		return
	}
	links, err := c.FindProcessesByFormID(ctx, formID)
	if err != nil {
		t.Fatalf("FindProcessesByFormID(%d): %v", formID, err)
	}
	found := false
	for _, l := range links {
		t.Logf("formId %d → processo %q (%s) v%d", formID, l.ProcessID, l.Description, l.Version)
		if l.ProcessID == pid {
			found = true
		}
	}
	if !found {
		t.Errorf("busca reversa não devolveu o próprio processo %q: %+v", pid, links)
	}
}
