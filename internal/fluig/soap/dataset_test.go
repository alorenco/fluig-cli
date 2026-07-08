package soap

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBuildFindAllDatasets(t *testing.T) {
	body, err := BuildFindAllDatasets(1, "admin", "s3nh@<>&")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// Namespace e operação presentes.
	if !strings.Contains(s, "findAllFormulariesDatasets") || !strings.Contains(s, NSDataset) {
		t.Errorf("envelope sem operação/namespace:\n%s", s)
	}
	// A senha com caracteres especiais deve ser escapada (não pode quebrar o XML).
	if strings.Contains(s, "s3nh@<>&") {
		t.Errorf("caracteres especiais da senha não foram escapados:\n%s", s)
	}
	// O envelope precisa ser XML válido.
	if err := xml.Unmarshal(body, new(any)); err != nil {
		t.Errorf("envelope não é XML válido: %v", err)
	}
}

func TestParseFindAllDatasets(t *testing.T) {
	items, err := ParseFindAllDatasets(fixture(t, "soap_findAllDatasets.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("esperava 2 itens, veio %d", len(items))
	}
	if items[0].DatasetID != "ds_exemplo" || items[0].Type != "CUSTOM" || items[0].Version != 3 {
		t.Errorf("item[0] inesperado: %+v", items[0])
	}
	if items[1].Type != "DEFAULT" {
		t.Errorf("item[1].Type = %q", items[1].Type)
	}
}

func TestParseGetDataset(t *testing.T) {
	res, err := ParseGetDataset(fixture(t, "soap_getDataset.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Columns, ",") != "codigo,nome,ativo" {
		t.Errorf("colunas inesperadas: %v", res.Columns)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("esperava 2 linhas, veio %d", len(res.Rows))
	}
	if res.Rows[0][1] == nil || *res.Rows[0][1] != "Alpha" {
		t.Errorf("linha 0 col 1 = %v, quer Alpha", res.Rows[0][1])
	}
	// O valor xsi:nil deve virar nil.
	if res.Rows[1][1] != nil {
		t.Errorf("linha 1 col 1 deveria ser nil, veio %v", *res.Rows[1][1])
	}
	if res.Rows[1][2] == nil || *res.Rows[1][2] != "false" {
		t.Errorf("linha 1 col 2 = %v, quer false", res.Rows[1][2])
	}
}

func TestParseSOAPFault(t *testing.T) {
	_, err := ParseFindAllDatasets(fixture(t, "soap_fault.xml"))
	var fault *Fault
	if err == nil || !strings.Contains(err.Error(), "sem permissão") {
		t.Fatalf("esperava Fault com a mensagem, veio %v", err)
	}
	if !asFault(err, &fault) {
		t.Errorf("erro deveria ser *Fault, veio %T", err)
	}
}

func asFault(err error, target **Fault) bool {
	f, ok := err.(*Fault)
	if ok {
		*target = f
	}
	return ok
}

func TestBuildGetDatasetWithConstraints(t *testing.T) {
	body, err := BuildGetDataset(1, "u", "p", "ds_x",
		[]string{"a", "b"},
		[]Constraint{{FieldName: "codigo", InitialValue: "10", FinalValue: "10"}},
		[]string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{"<name>ds_x</name>", "<fieldName>codigo</fieldName>", "<item>a</item>"} {
		if !strings.Contains(s, want) {
			t.Errorf("envelope sem %q:\n%s", want, s)
		}
	}
}
