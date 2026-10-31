package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TestsDir é a pasta convencional das suítes do `fluigcli test`:
// tests/<processo>/<suíte>.json, com as fixtures em tests/<processo>/fixtures/.
//
// A pasta NÃO entra na heurística de descoberta da raiz (conventionalDirs):
// "tests" é um nome comum demais e faria qualquer pasta de testes parecer a
// raiz de um projeto Fluig.
const TestsDir = "tests"

// TestFixturesDir é a subpasta de fixtures. Ela nunca é lida como suíte.
const TestFixturesDir = "fixtures"

// UnitTestsDir é a pasta dos testes de unidade (tests/unit/*.unit.json). O
// test run a ignora: ela não é pasta de processo.
const UnitTestsDir = "unit"

// unitSuffix é a extensão dos testes de unidade; o test run não os lê.
const unitSuffix = ".unit.json"

// SuitesInDir lista os arquivos *.json de uma pasta, em ordem alfabética, sem
// recursão. A subpasta fixtures/ fica de fora naturalmente.
func SuitesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") ||
			strings.HasSuffix(strings.ToLower(e.Name()), unitSuffix) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// DefaultSuites lista as suítes de <root>/tests/*/ (uma subpasta por
// processo), em ordem alfabética de pasta e de arquivo.
func DefaultSuites(root string) ([]string, error) {
	base := filepath.Join(root, TestsDir)
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != TestFixturesDir && e.Name() != UnitTestsDir {
			dirs = append(dirs, filepath.Join(base, e.Name()))
		}
	}
	sort.Strings(dirs)
	var out []string
	for _, d := range dirs {
		files, err := SuitesInDir(d)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
	}
	return out, nil
}
