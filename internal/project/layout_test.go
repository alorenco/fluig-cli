package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeInfo(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, ApplicationInfoRel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// O application.info real de um layout (formato Properties, chave=valor).
func TestReadApplicationTypeLayout(t *testing.T) {
	dir := t.TempDir()
	writeInfo(t, dir, "# gerado pelo Studio\n\napplication.code=meu_layout\napplication.type = layout\nlayout.file=layout.ftl\n")
	typ, err := ReadApplicationType(dir)
	if err != nil || typ != "layout" {
		t.Fatalf("type=%q err=%v", typ, err)
	}
}

func TestReadApplicationTypeWidgetComBOM(t *testing.T) {
	dir := t.TempDir()
	writeInfo(t, dir, "\uFEFFapplication.type=widget\r\napplication.code=w\r\n")
	typ, err := ReadApplicationType(dir)
	if err != nil || typ != "widget" {
		t.Fatalf("type=%q err=%v", typ, err)
	}
}

// Arquivo presente sem a chave: "" e sem erro — quem chama decide a mensagem.
func TestReadApplicationTypeSemChave(t *testing.T) {
	dir := t.TempDir()
	writeInfo(t, dir, "application.code=x\n")
	typ, err := ReadApplicationType(dir)
	if err != nil || typ != "" {
		t.Fatalf("type=%q err=%v", typ, err)
	}
}

// Arquivo ausente: erro reconhecível por os.ErrNotExist.
func TestReadApplicationTypeAusente(t *testing.T) {
	_, err := ReadApplicationType(t.TempDir())
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err=%v, esperado os.ErrNotExist", err)
	}
}

func TestLayoutDir(t *testing.T) {
	got := LayoutDir("/proj", "meu_layout")
	want := filepath.Join("/proj", "wcm", "layout", "meu_layout")
	if got != want {
		t.Fatalf("LayoutDir=%q, quer %q", got, want)
	}
}
