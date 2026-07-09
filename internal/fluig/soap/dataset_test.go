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

