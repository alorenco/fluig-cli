// Package helperwar embute o WAR do fluigcliHelper, o componente auxiliar que
// o fluigcli publica no servidor (ver helper/README.md). O artefato é
// versionado no Git para que o build da CLI não exija toolchain Java; o
// build.sh reconstrói o WAR e o hash de fontes que o teste anti-drift confere.
package helperwar

import _ "embed"

//go:embed fluigcliHelper.war
var WAR []byte

// Name é o nome de arquivo com que o WAR é publicado no servidor.
const Name = "fluigcliHelper.war"
