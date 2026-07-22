package output

import (
	"archive/zip"
	"fmt"
	"io"
	"strings"
)

// XLSXSheet é uma planilha (aba) de uma pasta de trabalho: um nome e uma matriz
// de células (texto). A primeira linha costuma ser o cabeçalho.
type XLSXSheet struct {
	Name string
	Rows [][]string
}

// WriteXLSX escreve uma pasta de trabalho .xlsx mínima (Office Open XML) com as
// planilhas dadas, usando só a biblioteca padrão (archive/zip + XML manual) —
// sem dependência externa. Todas as células são texto (inlineStr).
func WriteXLSX(w io.Writer, sheets []XLSXSheet) error {
	if len(sheets) == 0 {
		sheets = []XLSXSheet{{Name: "Planilha1"}}
	}
	zw := zip.NewWriter(w)
	add := func(name, content string) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.WriteString(f, content)
		return err
	}

	// [Content_Types].xml — declara cada planilha.
	var ct strings.Builder
	ct.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	for i := range sheets {
		fmt.Fprintf(&ct, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i+1)
	}
	ct.WriteString(`</Types>`)
	if err := add("[Content_Types].xml", ct.String()); err != nil {
		return err
	}

	if err := add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>`+
		`</Relationships>`); err != nil {
		return err
	}

	// xl/workbook.xml + rels
	var wb, rels strings.Builder
	wb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	rels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, sh := range sheets {
		fmt.Fprintf(&wb, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xlsxEscape(xlsxSheetName(sh.Name)), i+1, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
	}
	wb.WriteString(`</sheets></workbook>`)
	rels.WriteString(`</Relationships>`)
	if err := add("xl/workbook.xml", wb.String()); err != nil {
		return err
	}
	if err := add("xl/_rels/workbook.xml.rels", rels.String()); err != nil {
		return err
	}

	for i, sh := range sheets {
		if err := add(fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), sheetXML(sh)); err != nil {
			return err
		}
	}
	return zw.Close()
}

func sheetXML(sh XLSXSheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range sh.Rows {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, val := range row {
			fmt.Fprintf(&b, `<c r="%s%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
				xlsxColName(c), r+1, xlsxEscape(val))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

// xlsxColName converte um índice de coluna (0-based) em A, B, ..., Z, AA, ...
func xlsxColName(n int) string {
	name := ""
	for n >= 0 {
		name = string(rune('A'+n%26)) + name
		n = n/26 - 1
	}
	return name
}

func xlsxEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// xlsxSheetName sanea o nome da aba (Excel proíbe : \ / ? * [ ] e >31 chars).
func xlsxSheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ':', '\\', '/', '?', '*', '[', ']':
			return '-'
		}
		return r
	}, s)
	if len(s) > 31 {
		s = s[:31]
	}
	if s == "" {
		s = "Planilha"
	}
	return s
}
