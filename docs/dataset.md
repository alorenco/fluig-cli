# fluigcli dataset — datasets

Importa, exporta e consulta datasets. Convenção de vocabulário:

- **import** = servidor → projeto local
- **export** = projeto local → servidor

Arquivos locais ficam em `datasets/<id>.js` (um arquivo por dataset, subpastas
permitidas). O nome do dataset é o basename do arquivo sem `.js`.

Todos os comandos precisam de um servidor alvo (`--server`/`FLUIGCLI_SERVER`) e
autenticam segundo a precedência de senha da [config](server.md).

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

## Lote e exit codes

`import`/`export` aceitam vários alvos. Se algum item falha e outros têm
sucesso, o exit code é **6** (sucesso parcial) e o JSON traz `data.results[]`
com `success`/`error` por item. Um alvo único que falha retorna o código real
(3 auth, 4 não encontrado, 5 rejeitado pelo servidor).
