package fluig

import (
	"archive/zip"
	"bytes"
	"testing"
)

// BuildWAR usa STORE e preserva conteúdo binário byte a byte.
func TestBuildWARStoreAndBinary(t *testing.T) {
	binary := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0x10, 0x42} // PNG-ish + NUL
	files := []WARFile{
		{Name: "WEB-INF/application.xml", Content: []byte("<application/>")},
		{Name: "resources/img/logo.png", Content: binary},
	}
	war, err := BuildWAR(files)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(war), int64(len(war)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]byte{}
	for _, f := range zr.File {
		if f.Method != zip.Store {
			t.Errorf("%s não está em STORE (method=%d)", f.Name, f.Method)
		}
		rc, _ := f.Open()
		content := new(bytes.Buffer)
		content.ReadFrom(rc)
		rc.Close()
		found[f.Name] = content.Bytes()
	}
	if !bytes.Equal(found["resources/img/logo.png"], binary) {
		t.Errorf("binário corrompido no WAR: %v", found["resources/img/logo.png"])
	}
	if string(found["WEB-INF/application.xml"]) != "<application/>" {
		t.Errorf("xml inesperado: %q", found["WEB-INF/application.xml"])
	}
}
