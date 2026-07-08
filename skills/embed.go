// Package skillassets embute o conteúdo canônico da Skill de agentes de IA do
// fluigcli, para que `fluigcli skill install` seja autossuficiente (sem rede).
// Os mesmos arquivos ficam versionados em skills/fluigcli/ para leitura direta
// no repositório e cópia manual — é a única fonte da verdade (sem duplicação).
package skillassets

import "embed"

// FS contém a árvore skills/fluigcli/ (SKILL.md, reference/, codex/).
// O prefixo all: garante que arquivos iniciados por "." ou "_" também entrem.
//
//go:embed all:fluigcli
var FS embed.FS
