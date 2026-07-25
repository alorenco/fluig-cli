package helperwar

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestWARContemManifesto garante que o WAR embutido é o do fluigcliHelper
// (zip válido com o application.info correto).
func TestWARContemManifesto(t *testing.T) {
	zr, err := zip.NewReader(bytes.NewReader(WAR), int64(len(WAR)))
	if err != nil {
		t.Fatalf("WAR embutido não é um zip válido: %v", err)
	}
	var info string
	for _, f := range zr.File {
		if f.Name == "WEB-INF/classes/application.info" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("abrindo application.info: %v", err)
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("lendo application.info: %v", err)
			}
			info = string(data)
		}
	}
	if info == "" {
		t.Fatal("WAR embutido não contém WEB-INF/classes/application.info")
	}
	if !strings.Contains(info, "application.code=fluigcliHelper") {
		t.Errorf("application.info sem o código esperado:\n%s", info)
	}
}

// TestHelperWARAtualizado acusa drift entre as fontes Java (helper/src +
// pom.xml) e o WAR versionado: o build.sh grava em .srchash o hash das fontes
// que geraram o WAR; se as fontes mudaram sem rebuild, o hash não bate.
func TestHelperWARAtualizado(t *testing.T) {
	want, err := os.ReadFile(".srchash")
	if err != nil {
		t.Fatalf("lendo .srchash (rode helper/build.sh): %v", err)
	}

	h := sha256.New()
	writeFile := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("lendo %s: %v", path, err)
		}
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write(data)
	}

	writeFile("pom.xml")
	var files []string
	err = filepath.WalkDir("src", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("varrendo src: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		writeFile(f)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.TrimSpace(string(want)) {
		t.Errorf("fontes do helper mudaram sem rebuild do WAR versionado — rode helper/build.sh (hash das fontes %s ≠ .srchash %s)", got, strings.TrimSpace(string(want)))
	}
}

// A versão embutida sai do próprio WAR e tem de casar com o <version> do pom —
// a divergência entre os dois foi o que fez o helper 0.8.0 subir anunciando
// 0.7.0 em 2026-07-25 (ROADMAP §2.10-J).
func TestVersionDoWAREmbutido(t *testing.T) {
	got := Version()
	if got == "" {
		t.Fatal("Version() vazia: o WAR embutido não trouxe application.version")
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(got) {
		t.Errorf("Version()=%q, esperado MAJOR.MINOR.PATCH", got)
	}
	pom, err := os.ReadFile("pom.xml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`<version>([^<]+)</version>`).FindSubmatch(pom)
	if m == nil {
		t.Fatal("não achei <version> no pom.xml")
	}
	if quer := string(m[1]); got != quer {
		t.Errorf("WAR embutido anuncia %q, pom.xml diz %q — rode helper/build.sh", got, quer)
	}
}

func TestVersionFromInfo(t *testing.T) {
	casos := map[string]string{
		"application.version=0.8.0\nview.file=view.ftl\n": "0.8.0",
		"# comentário\napplication.version = 1.2.3 \n":    "1.2.3",
		"application.version=":                            "",
		"outra.chave=0.9.9\n":                             "",
		"":                                                "",
	}
	for info, quer := range casos {
		if got := versionFromInfo(info); got != quer {
			t.Errorf("versionFromInfo(%q)=%q, quer %q", info, got, quer)
		}
	}
}
