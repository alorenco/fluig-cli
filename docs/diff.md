# fluigcli diff — conferir antes de publicar

O comando compara os artefatos locais com o conteúdo atual do servidor. Ele
**não altera nada**. Ele completa a trava de produção. A trava pergunta se você
quer mesmo publicar. O `diff` mostra *o que* seria publicado.

```sh
fluigcli diff                                # varre datasets/, events/, mechanisms/, forms/ e workflow/scripts/
fluigcli diff datasets/ds_clientes.js        # compara só um arquivo
fluigcli diff datasets/                      # compara a pasta inteira (recursivo)
fluigcli diff forms/MinhaPasta               # compara um formulário inteiro (anexos + eventos)
fluigcli diff forms/MinhaPasta/events/x.js   # compara um único arquivo do formulário
fluigcli diff workflow/scripts/Compras.beforeTaskSave.js   # um script de processo
fluigcli diff --server producao              # contra um servidor específico
```

## O que ele reporta

| status | significado |
|---|---|
| `equal` | O local e o servidor são idênticos. |
| `modified` | O conteúdo difere. O comando mostra o diff unificado. |
| `only-local` | O artefato existe no local, mas não no servidor. O export criaria o artefato. |
| `only-server` | O artefato existe no servidor, mas não no local. Importe o artefato com `<tipo> import <id>`. |
| `error` | O comando não conseguiu comparar o artefato. O campo `error` traz o motivo. |

- Sem argumentos, o comando compara os arquivos locais. Ele também aponta os
  artefatos que só existem no servidor. Esses artefatos são datasets
  **customizados**, eventos, mecanismos, formulários e processos sem nenhum
  script local. Nos processos com script local, ele também aponta os eventos de
  processo.
- O comando ignora as diferenças de quebra de linha (CRLF/LF) e de quebra
  final. Ele usa a mesma normalização do ciclo import/export.
- Cobertura: datasets, eventos globais, mecanismos, formulários e scripts de
  processo. Para formulários, o comando compara a pasta `forms/<pasta>` arquivo
  a arquivo, incluindo a pasta `events/`. Para scripts de processo, ele usa
  `workflow/scripts/<Processo>.<evento>.js`.

### Caminhos aceitos

O caminho pode ser um arquivo, uma pasta de formulário ou uma **pasta**. A pasta
é varrida recursivamente.

| caminho | efeito |
|---|---|
| `datasets/ds_x.js` | um arquivo |
| `datasets/` | todos os `.js` da pasta, recursivo, **mais** os datasets que só existem no servidor |
| `datasets/sub/` | só os `.js` da subpasta. O comando não aponta `only-server` aqui |
| `forms/` | todos os formulários locais, mais os que só existem no servidor |
| `forms/MinhaPasta` | um formulário (anexos + eventos) |
| `workflow/` ou `workflow/scripts/` | todos os scripts de processo, mais os processos sem script local |
| `.` (a raiz do projeto) | igual a rodar sem argumentos |

A pasta de uma convenção inteira liga o `only-server` **daquele tipo**. Numa
subpasta o comando não faz isso. Apontar o servidor inteiro a partir de uma
subpasta seria ruído.

Uma pasta sem nenhum artefato comparável termina em erro de uso (exit 2).

### Formulários

- O diff compara **cada arquivo** da pasta com o anexo ou evento correspondente
  no servidor. Um `form export` da pasta **removeria** um arquivo `only-server`.
  Isso ocorre porque o export envia a lista completa de anexos. Importe o
  formulário se quiser preservar esse arquivo.
- O comando compara os anexos binários byte a byte. Um exemplo são as imagens.
  Quando eles diferem, o status é `modified` sem diff textual.
- O comando resolve o formulário-alvo como o `form export`. Ele usa primeiro o
  mapeamento `.fluigcli/forms.json`. Depois ele usa o nome da pasta.

### Scripts de processo

- A comparação usa o **export nativo** do processo. O export é um zip com o XML
  de definição. Ele funciona sem o componente auxiliar. Ele considera a versão
  mais recente do processo.
- Na varredura, os processos do servidor **sem nenhum script local** aparecem
  como `only-server`. A mesma API nativa do `workflow list` enumera esses
  processos. O diff não baixa os scripts desses processos. A comparação evento a
  evento acontece só nos processos com script local.

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

## Artefato que o servidor não entrega

Uma instância real tem artefato inconsistente. Um exemplo comum é o formulário
cujo anexo o servidor não acha na versão publicada. O servidor responde assim:

```
O documento frmServiceAuth.html na versão 2000 e com código (Id) 1144279 não foi
encontrado.
```

O diff **não para** por causa disso. O artefato sai com status `error` e o motivo,
e a varredura continua nos demais. O comando termina em **exit 6**
(`PARTIAL_FAILURE`), com todos os artefatos em `data.artifacts`.

Quando **nenhum** artefato pôde ser comparado, o comando devolve o erro do
servidor, com o exit code dele. Neste caso não existe resultado parcial, e um
exit 6 esconderia a causa.

As listagens do servidor continuam sendo falha fatal. Sem a lista de datasets,
formulários ou processos não existe comparação nenhuma.

## Exit code

O exit code é `0` sempre que o comando conclui a comparação. Uma diferença é um
dado, não um erro. O exit `6` indica que parte dos artefatos não pôde ser
comparada. Veja um fluxo típico para agentes e CI:

```sh
fluigcli diff --json | jq '.data.counts'     # há algo a publicar?
fluigcli dataset export datasets/ds_clientes.js --yes
```


## Caracteres fora do CP-1252 (o `?` do servidor)

O banco do Fluig guarda scripts server-side em colunas **CP-1252**. Na
gravação, um caractere fora dessa página (`→`, `✓`, emoji) vira `?` — de forma
permanente. A acentuação e a pontuação tipográfica (`—`, `…`, aspas curvas)
sobrevivem.

O `diff` compara o lado local **projetado para CP-1252** (a mesma perda que o
servidor aplica). Assim, um script local com `→` não fica `modified` para
sempre contra o `?` do servidor. Quem avisa da perda é a regra `FL007` do
`audit`, no export. Cada ferramenta com o seu papel: o diff responde "está
sincronizado?"; o audit responde "algo vai se perder?".
