// Package helperwar embute o WAR do fluigcliHelper, o componente auxiliar que
// o fluigcli publica no servidor (ver helper/README.md). O artefato é
// versionado no Git para que o build da CLI não exija toolchain Java; o
// build.sh reconstrói o WAR e o hash de fontes que o teste anti-drift confere.
package helperwar

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io"
	"strings"
	"sync"
)

//go:embed fluigcliHelper.war
var WAR []byte

// Name é o nome de arquivo com que o WAR é publicado no servidor.
const Name = "fluigcliHelper.war"

// infoPath é o manifesto da widget dentro do WAR. É a MESMA fonte que o
// endpoint /api/version do helper lê para se anunciar, e o Maven resolve o
// número a partir do <version> do pom.
const infoPath = "WEB-INF/classes/application.info"

// Version devolve a versão do WAR embutido neste binário, lida do manifesto
// dentro do próprio artefato.
//
// Ler do WAR, e não de uma constante em Go, é deliberado: a versão que o
// servidor vai anunciar tem de ser a mesma que a CLI compara. Uma constante
// duplicada divergiria — foi essa divergência que fez o helper 0.8.0 subir
// anunciando 0.7.0 em 2026-07-25. Devolve "" quando não consegue ler (WAR
// inesperado): quem chama trata "" como desconhecida, sem quebrar.
var Version = sync.OnceValue(func() string {
	zr, err := zip.NewReader(bytes.NewReader(WAR), int64(len(WAR)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		if f.Name != infoPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return ""
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, 64<<10))
		if err != nil {
			return ""
		}
		return versionFromInfo(string(data))
	}
	return ""
})

// versionFromInfo extrai application.version do manifesto (formato Properties:
// chave=valor por linha, com # de comentário).
func versionFromInfo(info string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "application.version" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
