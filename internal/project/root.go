// Package project descobre a raiz do projeto Fluig pela convenção de pastas
package project

import (
	"os"
	"path/filepath"
)

// strongMarker é o marcador definitivo da raiz: a pasta que a própria CLI cria
// (`server add` grava o servers.json dentro dela).
const strongMarker = ".fluigcli"

// conventionalDirs são as pastas que caracterizam um projeto Fluig quando o
// marcador forte ainda não existe. É o caso do projeto recém-clonado, antes do
// primeiro `server add`.
var conventionalDirs = []string{
	"datasets",
	"events",
	"mechanisms",
	"forms",
	"workflow",
	"wcm",
}

// FindRoot procura a raiz do projeto a partir de startDir, subindo pelos
// ancestrais. Retorna "" se nenhum for encontrado.
//
// A busca tem duas passadas. A primeira procura só o marcador forte
// (`.fluigcli/`) e vence sempre. A segunda usa as pastas convencionais.
// Assim uma pasta convencional solta no meio do caminho não sequestra a
// descoberta de um projeto já cadastrado.
func FindRoot(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	if root := climb(dir, hasStrongMarker); root != "" {
		return root
	}
	return climb(dir, looksLikeProjectRoot)
}

// climb sobe de dir até a raiz do sistema de arquivos e devolve o primeiro
// diretório aceito por match.
func climb(dir string, match func(string) bool) string {
	for {
		if match(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func hasStrongMarker(dir string) bool {
	return isDir(filepath.Join(dir, strongMarker))
}

// looksLikeProjectRoot aplica a heurística das pastas convencionais.
func looksLikeProjectRoot(dir string) bool {
	for _, name := range conventionalDirs {
		if !isDir(filepath.Join(dir, name)) {
			continue
		}
		// ⚠️ `events/` também é subpasta de CADA formulário (ver forms.go).
		// Sem esta guarda, rodar de dentro de `forms/<form>/` fazia a
		// descoberta parar na pasta do formulário e a CLI procurava o
		// servers.json lá — o erro saía como "nenhum servidor cadastrado",
		// que mandava quem depurava para o lado errado (corrigido em
		// 2026-08-06).
		if name == "events" && filepath.Base(filepath.Dir(dir)) == "forms" {
			continue
		}
		return true
	}
	return false
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
