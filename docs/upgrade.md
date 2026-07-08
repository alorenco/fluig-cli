# fluigcli upgrade — atualização da própria CLI

Baixa a release do GitHub, confere o `sha256` contra o `checksums.txt`
publicado e substitui o próprio binário no lugar. Nenhuma configuração ou
sessão é tocada.

```sh
fluigcli upgrade                    # instala a última versão publicada
fluigcli upgrade --check            # só consulta, sem instalar
fluigcli upgrade --version 0.1.0    # instala uma versão específica (inclusive mais antiga)
```

- Se o binário estiver em um diretório protegido (ex.: `/usr/local/bin`),
  repita com `sudo`.
- No Windows, o binário em execução não pode ser sobrescrito: o antigo fica ao
  lado como `fluigcli.exe.old` (pode apagar depois).
- Builds de desenvolvimento (`fluigcli version` mostrando `dev`) não têm
  release correspondente — atualize pelo mesmo meio da instalação
  (ex.: `go install github.com/alorenco/fluig-cli/cmd/fluigcli@latest`).

## Aviso automático de versão nova

Ao fim de um comando, a CLI avisa **no stderr** quando existe versão mais nova
— no máximo **uma consulta por dia** (resultado em cache em
`<cache>/fluigcli/update-check.json`), somente em terminal interativo e nunca
misturado à saída de `--json` (que continua recebendo só o envelope no stdout).

Para desativar (ex.: em CI ou por preferência):

```sh
export FLUIGCLI_NO_UPDATE_CHECK=1
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
