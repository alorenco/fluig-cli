# fluigcli dataset — datasets

Importa, exporta, consulta e administra datasets (ativação, histórico de
versões e restauração). Convenção de vocabulário:

- **import** = servidor → projeto local
- **export** = projeto local → servidor

Arquivos locais ficam em `datasets/<id>.js` (um arquivo por dataset, subpastas
permitidas). O nome do dataset é o basename do arquivo sem `.js`.

Todos os comandos precisam de um servidor alvo (`--server`/`FLUIGCLI_SERVER`) e
autenticam segundo a precedência de senha da [config](server.md) — exceto o
`dataset new`, que é local.

## `fluigcli dataset new <name>`

Cria `datasets/<name>.js` com o esqueleto de um dataset customizado
(`defineStructure`, `createDataset` e a sincronização `onSync`/`onMobileSync`
comentada). **Só local** — nada é enviado ao servidor; publique depois com
`dataset export`. Falha (exit 2) se já existir um `<name>.js` sob `datasets/`
(inclusive em subpasta — evita a ambiguidade na hora do export).

```sh
fluigcli dataset new ds_clientes
# edite datasets/ds_clientes.js e publique:
fluigcli dataset export datasets/ds_clientes.js --new
```

## `fluigcli dataset list [--custom-only] [--search <texto>]`

Lista os datasets do servidor (API REST v2) com id, tipo
(CUSTOM/BUILTIN/GENERATED), descrição e se está ativo. `--custom-only` mostra
apenas os customizados (os que a CLI consegue exportar/importar); `--search`
filtra por texto no id ou na descrição.

```sh
fluigcli dataset list --custom-only
fluigcli dataset list --search pagamento
```

Em servidores antigos sem a REST v2 de datasets, a listagem cai automaticamente
para o SOAP (sem as colunas de descrição/ativo).

> Desde 2026-07-09 a listagem não mostra mais a coluna **Versão** (e o campo
> `version` saiu do `--json`): a API nova não expõe essa informação.

## `fluigcli dataset import <id>... | --all`

Baixa datasets do servidor para arquivos locais. Se já existir um arquivo
`<id>.js` sob `datasets/` (busca recursiva), ele é sobrescrito no lugar; senão é
criado em `datasets/<id>.js`. `--all` importa todos os customizados.

```sh
fluigcli dataset import ds_clientes ds_produtos
fluigcli dataset import --all
```

## `fluigcli dataset export <file>... [--description "..."] [--new]`

Envia datasets locais para o servidor. Se o dataset já existe, atualiza (mantém
a estrutura e troca só o código). Se não existe, cria — e **criar exige `--new`**
em modo não-interativo (proteção contra criar dataset por erro de digitação no
nome). `--description` define a descrição na criação (default: o nome).

```sh
fluigcli dataset export datasets/ds_clientes.js
fluigcli dataset export datasets/ds_novo.js --new --description "Cadastro novo"
```

## `fluigcli dataset query <id> [flags]`

Consulta os dados de um dataset (API REST v2 `dataset-handle/search` — o
`--limit` é aplicado no servidor; sem limite a CLI pagina até o fim).

| Flag | Descrição |
|---|---|
| `--fields a,b` | campos a retornar (sem a flag, todos) |
| `--constraint campo=valor` | filtro de igualdade (pode repetir) |
| `--order campo` | ordenação por **um** campo (sufixo `_DESC` inverte) |
| `--limit N` | máximo de linhas (0 = sem limite) |

```sh
fluigcli dataset query ds_clientes --fields codigo,nome --constraint ativo=true --limit 50
fluigcli dataset query colleague --fields login --order colleagueName_DESC --json
```

Dataset inexistente (ou consulta com campo/ordenação inválidos) → exit **4**.

## `fluigcli dataset disable <id>...` / `enable <id>...`

Desativa um dataset **sem apagá-lo** (não há API de exclusão de dataset no
Fluig) e reativa datasets desativados. Nativo (REST v2). Um dataset inativo
some das consultas, mas continua listado (coluna Ativo = "não") e mantém o
histórico.

```sh
fluigcli dataset disable ds_legado
fluigcli dataset enable ds_legado ds_outro
```

Dataset inexistente → exit **4**. Em produção, vale a trava de confirmação.

## `fluigcli dataset history <id> [--version N]`

Mostra o histórico de versões de um dataset customizado — versão, status
(PUBLISHED/DRAFT), autor, data e tamanho do código; a versão corrente aparece
em verde. Com `--version N`, imprime o **código JS** daquela versão (bom para
comparar ou salvar em arquivo).

```sh
fluigcli dataset history ds_clientes
fluigcli dataset history ds_clientes --version 3 > ds_clientes_v3.js
fluigcli dataset history ds_clientes --json     # versões sem o código (lines por versão)
```

Apenas datasets **customizados** têm histórico: para os demais a CLI informa e
retorna lista vazia. Dataset inexistente → exit **4**.

## `fluigcli dataset restore <id> <version>`

Restaura o código de um dataset para uma versão anterior do histórico. O
restore **cria uma versão nova** (publicada) com o código da versão alvo — o
histórico nunca é reescrito. A CLI valida a versão contra o histórico antes,
avisa se houver rascunho não publicado (o restore o descarta) e pede
confirmação (`--yes` pula).

```sh
fluigcli dataset history ds_clientes            # descubra a versão boa
fluigcli dataset restore ds_clientes 3 --yes
```

Versão fora do histórico → exit **4** (sem tocar no servidor).

## Lote e exit codes

`import`/`export` aceitam vários alvos. Se algum item falha e outros têm
sucesso, o exit code é **6** (sucesso parcial) e o JSON traz `data.results[]`
com `success`/`error` por item. Um alvo único que falha retorna o código real
(3 auth, 4 não encontrado, 5 rejeitado pelo servidor).
