# fluigcli diff — conferir antes de publicar

Compara artefatos locais com o conteúdo atual do servidor, **sem alterar
nada** — o complemento natural da trava de produção: ela pergunta "quer mesmo
publicar?", o `diff` mostra *o que* seria publicado.

```sh
fluigcli diff                                # varre datasets/, events/, mechanisms/, forms/ e workflow/scripts/
fluigcli diff datasets/ds_clientes.js        # compara só um arquivo
fluigcli diff forms/MinhaPasta               # compara um formulário inteiro (anexos + eventos)
fluigcli diff forms/MinhaPasta/events/x.js   # compara um único arquivo do formulário
fluigcli diff workflow/scripts/Compras.beforeTaskSave.js   # um script de processo
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
  existem no servidor (datasets **customizados**, eventos, mecanismos,
  formulários, processos sem nenhum script local e — nos processos com script
  local — eventos de processo).
- Diferenças só de quebra de linha (CRLF/LF) e de quebra final **não contam** —
  é a mesma normalização que o ciclo import/export já faz.
- Cobertura: datasets, eventos globais, mecanismos, formulários (`forms/<pasta>`,
  arquivo a arquivo, incluindo `events/`) e scripts de processo
  (`workflow/scripts/<Processo>.<evento>.js`).

### Formulários

- O diff compara **cada arquivo** da pasta com o anexo/evento correspondente no
  servidor. Um arquivo `only-server` seria **removido** por um `form export` da
  pasta (o export envia a lista completa de anexos) — importe o formulário se
  quiser preservá-lo.
- Anexos binários (imagens etc.) são comparados byte a byte; quando diferem, o
  status é `modified` sem diff textual.
- O formulário-alvo é resolvido como no `form export`: mapeamento
  `.fluigcli/forms.json` > nome da pasta.

### Scripts de processo

- A comparação usa o **export nativo** do processo (zip com o XML de
  definição) — funciona sem a fluiggersWidget e considera a versão mais
  recente do processo.
- Na varredura, processos do servidor **sem nenhum script local** aparecem como
  `only-server` (enumerados pela mesma API nativa do `workflow list`). O diff
  não baixa os scripts desses processos — a comparação evento a evento
  acontece só nos processos com script local.

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
