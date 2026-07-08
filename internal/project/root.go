// Package project descobre a raiz do projeto Fluig pela convenção de pastas
package project

import (
	"os"
	"path/filepath"
)

// conventionalDirs são as pastas que caracterizam um projeto Fluig.
var conventionalDirs = []string{
	".fluigcli",
	"datasets",
	"events",
	"mechanisms",
	"forms",
	"workflow",
	"wcm",
}

// FindRoot procura a raiz do projeto a partir de startDir, subindo pelos
// ancestrais até encontrar um diretório com `.fluigcli/` ou pelo menos uma
// pasta convencional. Retorna "" se nenhum for encontrado.
func FindRoot(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		if isProjectRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isProjectRoot(dir string) bool {
	for _, name := range conventionalDirs {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
