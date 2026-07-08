# fluigcli diff — conferir antes de publicar

Compara artefatos locais com o conteúdo atual do servidor, **sem alterar
nada** — o complemento natural da trava de produção: ela pergunta "quer mesmo
publicar?", o `diff` mostra *o que* seria publicado.

```sh
fluigcli diff                                # varre datasets/, events/ e mechanisms/
fluigcli diff datasets/ds_clientes.js        # compara só um arquivo
fluigcli diff --server producao              # contra um servidor específico
```

## O que ele reporta

| status | significado |
|---|---|
| `equal` | local e servidor idênticos |
| `modified` | conteúdo difere — mostra o diff unificado |
| `only-local` | existe localmente, não no servidor (o export criaria) |
| `only-server` | existe no servidor, não localmente (importe com `<tipo> import <id>`) |

- Sem argumentos, além de comparar os arquivos locais, aponta artefatos que só
  existem no servidor (datasets **customizados**, eventos e mecanismos).
- Diferenças só de quebra de linha (CRLF/LF) e de quebra final **não contam** —
  é a mesma normalização que o ciclo import/export já faz.
- Cobertura atual: datasets, eventos globais e mecanismos (artefatos de arquivo
  único). Formulários e scripts de workflow ficam para uma próxima versão.

## Saída `--json`

```json
{
  "ok": true,
  "command": "diff",
  "server": "homolog",
  "data": {
    "artifacts": [
      {"type": "dataset", "id": "ds_clientes", "path": "datasets/ds_clientes.js", "status": "equal"},
      {"type": "event", "id": "displayCentralTasks", "path": "events/displayCentralTasks.js",
       "status": "modified", "diff": "--- servidor:displayCentralTasks\n+++ local:events/...\n@@ ... @@\n..."}
    ],
    "counts": {"equal": 1, "modified": 1}
  },
  "error": null
}
```

O exit code é `0` sempre que a comparação foi concluída — diferença é dado, não
erro. Fluxo típico para agentes/CI:

```sh
fluigcli diff --json | jq '.data.counts'     # há algo a publicar?
fluigcli dataset export datasets/ds_clientes.js --yes
```
