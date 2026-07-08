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
