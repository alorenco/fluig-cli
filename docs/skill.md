# fluigcli skill — Skill para agentes de IA

Instala e exibe a Skill que ensina agentes de IA (Claude Code, Codex e afins) a
usar o fluigcli corretamente: o contrato de saída `--json`, os exit codes e o
mapa de comandos.

O conteúdo canônico fica versionado em [`skills/fluigcli/`](../skills/fluigcli/)
e é **embutido no binário** — a instalação não acessa a rede. Assim há uma única
fonte da verdade: o mesmo material que você lê no repositório é o que é instalado.

```
skills/fluigcli/
├── SKILL.md              # Skill do Claude Code (frontmatter + guia)
├── reference/
│   ├── contract.md       # envelope --json + tabela de exit codes
│   └── commands.md       # mapa de comandos e receitas
└── codex/AGENTS.md       # mesmo guia, condensado, para o AGENTS.md do Codex
```

## `fluigcli skill install`

Escreve os arquivos no lugar esperado por cada ferramenta.

```sh
fluigcli skill install                      # padrão: --target claude, no projeto
fluigcli skill install --target all         # Claude Code + Codex
fluigcli skill install --target codex       # só o bloco do Codex
fluigcli skill install --target claude --global   # no diretório do usuário
```

| flag | efeito |
|---|---|
| `--target claude\|codex\|all` | ferramenta(s) alvo (padrão `claude`) |
| `--global` | instala no diretório do usuário em vez do projeto |
| `--force` | sobrescreve arquivos modificados localmente |

Destinos:

| alvo | projeto | `--global` |
|---|---|---|
| `claude` | `<projeto>/.claude/skills/fluigcli/` | `~/.claude/skills/fluigcli/` |
| `codex` | `<projeto>/AGENTS.md` | `~/.codex/AGENTS.md` |

Comportamento:

- **Claude Code**: copia `SKILL.md` e `reference/`. Se um arquivo já existir e
  diferir do gerado, ele é **preservado** (status `skipped`) — use `--force`
  para sobrescrever. Idênticos viram `unchanged`.
- **Codex**: injeta um **bloco gerenciado** delimitado por marcadores
  (`<!-- fluigcli:start … -->` / `<!-- fluigcli:end -->`) no `AGENTS.md`.
  Reinstalar atualiza só esse bloco, sem tocar no resto do arquivo e sem
  duplicá-lo — é seguro rodar quantas vezes quiser.

Com `--json`, `data` traz `{target, files:[{path, status}]}`, com `status` em
`written` · `updated` · `unchanged` · `skipped`.

## `fluigcli skill show`

Imprime o guia no stdout — útil para inspecionar ou canalizar para outra
ferramenta.

```sh
fluigcli skill show --target claude   # SKILL.md + reference/ concatenados
fluigcli skill show --target codex    # o guia do Codex
```

Com `--json`, o conteúdo vem em `data.content`.
