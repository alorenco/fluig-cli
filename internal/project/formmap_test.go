package project

import (
	"os"
	"strings"
	"testing"
)

const (
	scopeHml  = "10.0.0.1:8080/1"
	scopeProd = "fluig.empresa.com:443/1"
)

// O mesmo formulário tem documentId diferente em cada servidor — os buckets
// não podem vazar um no outro.
func TestFormMapEscopoPorServidor(t *testing.T) {
	root := t.TempDir()

	hml, err := LoadFormMap(root, scopeHml)
	if err != nil {
		t.Fatal(err)
	}
	hml.Upsert(FormLink{Folder: "frm_x", DocumentID: 42, Name: "Form X"})
	if err := hml.Save(); err != nil {
		t.Fatal(err)
	}

	// Na produção, a mesma pasta não tem vínculo (id 42 é da homologação).
	prod, err := LoadFormMap(root, scopeProd)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prod.ByFolder("frm_x"); ok {
		t.Fatal("vínculo da homologação não pode aparecer na produção")
	}
	prod.Upsert(FormLink{Folder: "frm_x", DocumentID: 999, Name: "Form X"})
	if err := prod.Save(); err != nil {
		t.Fatal(err)
	}

	// O Save da produção preserva o bucket da homologação.
	hml2, err := LoadFormMap(root, scopeHml)
	if err != nil {
		t.Fatal(err)
	}
	l, ok := hml2.ByFolder("frm_x")
	if !ok || l.DocumentID != 42 {
		t.Fatalf("bucket da homologação perdido: ok=%v link=%+v", ok, l)
	}
	prod2, _ := LoadFormMap(root, scopeProd)
	if l, _ := prod2.ByFolder("frm_x"); l.DocumentID != 999 {
		t.Fatalf("bucket da produção errado: %+v", l)
	}
}

func TestFormMapUpsertEConsultas(t *testing.T) {
	root := t.TempDir()
	m, _ := LoadFormMap(root, scopeHml)
	m.Upsert(FormLink{Folder: "a", DocumentID: 1, Name: "Form A"})
	m.Upsert(FormLink{Folder: "a", DocumentID: 2, Name: "Form A2"}) // atualiza pela pasta
	m.Upsert(FormLink{Folder: "b", DocumentID: 3, Name: "Form B"})

	if l, ok := m.ByFolder("a"); !ok || l.DocumentID != 2 {
		t.Errorf("ByFolder: %+v ok=%v", l, ok)
	}
	if l, ok := m.ByDocumentID(3); !ok || l.Folder != "b" {
		t.Errorf("ByDocumentID: %+v ok=%v", l, ok)
	}
	if l, ok := m.ByName("Form A2"); !ok || l.Folder != "a" {
		t.Errorf("ByName: %+v ok=%v", l, ok)
	}
	if _, ok := m.ByFolder("z"); ok {
		t.Error("pasta sem vínculo não pode resolver")
	}
}

// Schema v1 (abandonado sem retrocompat, decisão do mantenedor 2026-07-09):
// tratado como vazio, sem erro.
func TestFormMapV1ViraVazio(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/.fluigcli", 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := `{"version":"1.0.0","forms":[{"folder":"x","documentId":7,"name":"X"}]}`
	if err := os.WriteFile(FormMapPath(root), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadFormMap(root, scopeHml)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.ByFolder("x"); ok {
		t.Fatal("v1 deveria ser ignorado")
	}
	// O primeiro Save reescreve como v2.
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(FormMapPath(root))
	if !strings.Contains(string(data), `"version": "2.0.0"`) {
		t.Errorf("arquivo não migrou para v2:\n%s", data)
	}
}
