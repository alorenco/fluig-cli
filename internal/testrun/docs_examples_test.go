package testrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Os exemplos COMPLETOS da documentação (um bloco ```json com "scenarios" ou
// "cases") têm de carregar no parser real. Pessoas e agentes de IA copiam
// esses exemplos; um exemplo quebrado ensina o formato errado.
var jsonBlockRe = regexp.MustCompile("(?s)```json\\n(.*?)```")

func TestExemplosDaDocumentacaoCarregam(t *testing.T) {
	arquivos := map[string]int{ // arquivo → mínimo de exemplos completos
		"../../docs/test-guia.md":                    3,
		"../../docs/test.md":                         1,
		"../../skills/fluigcli/reference/testing.md": 3,
	}
	for arq, minimo := range arquivos {
		data, err := os.ReadFile(arq)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for i, m := range jsonBlockRe.FindAllSubmatch(data, -1) {
			bloco := m[1]
			var top map[string]json.RawMessage
			if json.Unmarshal(bloco, &top) != nil {
				continue // fragmento (passos soltos), não é arquivo completo
			}
			_, suite := top["scenarios"]
			_, unit := top["cases"]
			if !suite && !unit {
				continue
			}
			n++
			dir := t.TempDir()
			if suite {
				p := filepath.Join(dir, "tests", "proc", "s.json")
				criaFixtures(t, filepath.Dir(p), bloco)
				if _, err := ParseSuite(p, bloco, LoadOptions{}); err != nil {
					t.Errorf("%s, bloco %d (suíte): %v", arq, i+1, err)
				}
				continue
			}
			p := filepath.Join(dir, "tests", "unit", "u.unit.json")
			criaFixtures(t, filepath.Dir(p), bloco)
			criaScripts(t, dir, bloco)
			if _, err := ParseUnitFile(p, dir, bloco); err != nil {
				t.Errorf("%s, bloco %d (unidade): %v", arq, i+1, err)
			}
		}
		if n < minimo {
			t.Errorf("%s: %d exemplos completos, esperava ao menos %d (o extrator quebrou?)", arq, n, minimo)
		}
	}
}

var (
	cardRe   = regexp.MustCompile(`"card"\s*:\s*"([^"]+\.json)"`)
	scriptRe = regexp.MustCompile(`"([\w./-]+\.js)"`)
)

// criaFixtures grava um card vazio em cada fixture citada pelo exemplo.
func criaFixtures(t *testing.T, base string, bloco []byte) {
	t.Helper()
	for _, m := range cardRe.FindAllSubmatch(bloco, -1) {
		p := filepath.Join(base, filepath.FromSlash(string(m[1])))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(`{"valor": "1"}`), 0o644)
	}
}

// criaScripts grava um script vazio em cada caminho .js citado.
func criaScripts(t *testing.T, root string, bloco []byte) {
	t.Helper()
	for _, m := range scriptRe.FindAllSubmatch(bloco, -1) {
		p := filepath.Join(root, filepath.FromSlash(string(m[1])))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("function f() {}\n"), 0o644)
	}
}
