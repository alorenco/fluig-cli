# fluigcli upgrade — atualização da própria CLI

O comando `upgrade` atualiza o próprio binário. Ele baixa a release do GitHub.
Ele confere o `sha256` contra o `checksums.txt` publicado. Ele substitui o
binário no lugar. O comando não toca em nenhuma configuração nem sessão.

```sh
fluigcli upgrade                    # instala a última versão publicada
fluigcli upgrade --check            # só consulta, sem instalar
fluigcli upgrade --version 0.1.0    # instala uma versão específica (inclusive mais antiga)
```

- O binário pode ficar em um diretório protegido. Um exemplo é `/usr/local/bin`.
  Neste caso, repita o comando com `sudo`.
- No Windows, o comando não sobrescreve o binário em execução. O antigo fica ao
  lado como `fluigcli.exe.old`. Você apaga este arquivo depois.
- Um build de desenvolvimento não tem release correspondente. O comando
  `fluigcli version` mostra `dev` neste caso. Atualize pelo mesmo meio da
  instalação. Por exemplo: `go install github.com/alorenco/fluig-cli/cmd/fluigcli@latest`.

## Aviso automático de versão nova

A CLI avisa quando existe uma versão mais nova. Ela mostra o aviso **no stderr**
ao fim de um comando. Ela faz **uma consulta por dia** no máximo. Ela guarda o
resultado em cache em `<cache>/fluigcli/update-check.json`. Ela mostra o aviso
somente em terminal interativo. Ela nunca mistura o aviso à saída de `--json`.
O `--json` continua a receber só o envelope no stdout.

Você desativa o aviso em CI ou por preferência. Defina a variável:

```sh
export FLUIGCLI_NO_UPDATE_CHECK=1
```

## Aviso de skill desatualizada

A skill de agente evolui junto com a CLI. Você a instala com o comando
`fluigcli skill install`. Na instalação, a CLI **carimba a versão** que gerou a
skill. Ela grava a versão em `.claude/skills/fluigcli/.fluigcli-version`.

- A CLI **sugere** atualizar a skill logo após um `upgrade` bem-sucedido. Ela
  faz isso quando a skill está instalada no projeto atual. A sugestão é
  `fluigcli skill install --force`.
- A CLI avisa **no stderr** quando a skill do projeto é de uma versão anterior à
  do binário. Ela mostra o aviso em qualquer comando. Ela faz isso **1×/dia por
  versão** no máximo, só em terminal interativo, nunca no `--json`. O aviso é só
  uma sugestão. A CLI não reescreve nada. Você precisa rodar o `skill install`.

Você desativa esse aviso. Defina a variável:

```sh
export FLUIGCLI_NO_SKILL_CHECK=1
```

## Saída `--json`

```json
{"ok":true,"command":"upgrade","server":"","data":{"current":"0.1.0","latest":"0.2.0","updated":true,"path":"/usr/local/bin/fluigcli"},"error":null}
```

| campo | significado |
|---|---|
| `current` | versão em execução antes da atualização |
| `latest` | versão alvo (última release ou a pedida em `--version`) |
| `updated` | `true` se o binário foi substituído |
| `path` | caminho do binário atualizado (ausente quando nada foi feito) |
