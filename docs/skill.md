# fluigcli skill — Skill para agentes de IA

O grupo `skill` instala e exibe a Skill. A Skill ensina agentes de IA (Claude
Code, Codex e afins) a usar o fluigcli corretamente. Ela cobre o contrato de
saída `--json`, os exit codes e o mapa de comandos.

O conteúdo canônico fica versionado em [`skills/fluigcli/`](https://github.com/alorenco/fluig-cli/tree/main/skills/fluigcli).
A CLI **embute esse conteúdo no binário**. Por isso, a instalação não acessa a
rede. Assim há uma única fonte da verdade. O material que você lê no repositório
é o mesmo que a CLI instala.

```
skills/fluigcli/
├── SKILL.md              # Skill do Claude Code (frontmatter + guia)
├── reference/
│   ├── contract.md       # envelope --json + tabela de exit codes
│   ├── commands.md       # mapa de comandos e receitas
│   ├── styleguide.md     # Style Guide 2.0: variáveis do tema e substituições
│   └── fluig.d.ts        # assinaturas das APIs de script (hAPI, form, FLUIGC…)
└── codex/AGENTS.md       # mesmo guia, condensado, para o AGENTS.md do Codex
```

## `fluigcli skill install`

Este comando escreve os arquivos no lugar que cada ferramenta espera.

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

- **Claude Code**: o comando copia `SKILL.md` e `reference/`. Quando um arquivo
  já existe e difere do gerado, o comando **preserva** esse arquivo (status
  `skipped`). Use `--force` para sobrescrever. Arquivos idênticos viram
  `unchanged`.
- **Codex**: o comando injeta um **bloco gerenciado** no `AGENTS.md`. Os
  marcadores `<!-- fluigcli:start … -->` e `<!-- fluigcli:end -->` delimitam o
  bloco. Reinstalar atualiza só esse bloco. O comando não toca no resto do
  arquivo e não duplica o bloco. Por isso, você pode rodar o comando quantas
  vezes quiser.

Com `--json`, o campo `data` traz `{target, files:[{path, status}]}`. O `status`
é `written`, `updated`, `unchanged` ou `skipped`.

### Aviso de versão desatualizada

Ao instalar o alvo `claude`, a CLI grava a versão que gerou a skill em
`.claude/skills/fluigcli/.fluigcli-version`. A CLI compara essa versão com a
versão do binário. Quando a skill do projeto é de uma versão anterior, a CLI
**sugere no stderr** rodar `fluigcli skill install --force`. Isso acontece depois
de um `fluigcli upgrade` ou em qualquer comando rodado nesse projeto. A CLI
limita a sugestão a uma vez por dia por versão. A sugestão sai só em terminal
interativo. A sugestão nunca sai no `--json`. É só uma sugestão. Desative a
sugestão com `FLUIGCLI_NO_SKILL_CHECK=1`. Ver também [upgrade](upgrade.md).

## `fluigcli skill show`

Este comando imprime o guia no stdout. Use este comando para inspecionar o guia
ou para canalizar o guia para outra ferramenta.

```sh
fluigcli skill show --target claude   # SKILL.md + reference/ concatenados
fluigcli skill show --target codex    # o guia do Codex
```

Com `--json`, o conteúdo vem no campo `data.content`.
