package output

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestWriteXLSX(t *testing.T) {
	var buf bytes.Buffer
	sheets := []XLSXSheet{
		{Name: "Resumo", Rows: [][]string{{"Chave", "Valor"}, {"Nome", "A & B <ok>"}}},
		{Name: "Aba/Inválida:2", Rows: [][]string{{"x"}}},
	}
	if err := WriteXLSX(&buf, sheets); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("saída não é um zip válido: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
	}
	for _, want := range []string{
		"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml",
		"xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml",
	} {
		if _, ok := parts[want]; !ok {
			t.Errorf("faltou a parte %q", want)
		}
	}
	// Escape de XML aplicado.
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], "A &amp; B &lt;ok&gt;") {
		t.Errorf("célula não escapada:\n%s", parts["xl/worksheets/sheet1.xml"])
	}
	// Nome de aba saneado (proibidos viram '-').
	if !strings.Contains(parts["xl/workbook.xml"], "Aba-Inválida-2") {
		t.Errorf("nome de aba não saneado:\n%s", parts["xl/workbook.xml"])
	}
}

func TestXLSXColName(t *testing.T) {
	cases := map[int]string{0: "A", 1: "B", 25: "Z", 26: "AA", 27: "AB", 51: "AZ", 52: "BA"}
	for n, want := range cases {
		if got := xlsxColName(n); got != want {
			t.Errorf("xlsxColName(%d)=%q, quer %q", n, got, want)
		}
	}
}
