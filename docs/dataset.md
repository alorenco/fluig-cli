# fluigcli dataset — datasets

O grupo `dataset` importa, exporta, consulta e administra datasets. A
administração cobre ativação, histórico de versões e restauração. Use este
vocabulário:

- **import** = servidor → projeto local
- **export** = projeto local → servidor

Os arquivos locais ficam em `datasets/<id>.js`. Há um arquivo por dataset.
Subpastas são permitidas. O nome do dataset é o basename do arquivo sem `.js`.

Todos os comandos precisam de um servidor alvo (`--server`/`FLUIGCLI_SERVER`).
Eles autenticam segundo a precedência de senha da [config](server.md). O
`dataset new` é a exceção, porque ele é local.

## `fluigcli dataset new <name>`

Este comando cria `datasets/<name>.js` com o esqueleto de um dataset
customizado. O esqueleto traz `defineStructure`, `createDataset` e a
sincronização `onSync`/`onMobileSync` comentada. O comando é **só local**. Ele
não envia nada ao servidor. Publique depois com `dataset export`. O comando
falha (exit 2) quando já existe um `<name>.js` sob `datasets/`. Isso vale
também para subpasta. Esta regra evita a ambiguidade na hora do export.

```sh
fluigcli dataset new ds_clientes
# edite datasets/ds_clientes.js e publique:
fluigcli dataset export datasets/ds_clientes.js --new
```

## `fluigcli dataset list [--custom-only] [--search <texto>]`

Este comando lista os datasets do servidor pela API REST v2. A lista mostra o
id, o tipo (CUSTOM/BUILTIN/GENERATED), a descrição e se o dataset está ativo. A
opção `--custom-only` mostra apenas os datasets customizados. Estes são os que a
CLI consegue exportar e importar. A opção `--search` filtra por texto no id ou
na descrição.

```sh
fluigcli dataset list --custom-only
fluigcli dataset list --search pagamento
```

Em servidores antigos sem a REST v2 de datasets, a listagem cai automaticamente
para o SOAP. Neste caso, faltam as colunas de descrição e ativo.

> Desde 2026-07-09 a listagem não mostra mais a coluna **Versão**. O campo
> `version` também saiu do `--json`. A API nova não expõe essa informação.

## `fluigcli dataset import <id>... | --all`

Este comando baixa datasets do servidor para arquivos locais. Quando já existe
um arquivo `<id>.js` sob `datasets/`, o comando o sobrescreve no lugar. A busca
é recursiva. Senão, o comando cria o arquivo em `datasets/<id>.js`. A opção
`--all` importa todos os datasets customizados.

```sh
fluigcli dataset import ds_clientes ds_produtos
fluigcli dataset import --all
```

## `fluigcli dataset export <file>... [--description "..."] [--new]`

Este comando envia datasets locais para o servidor. Quando o dataset já existe,
o comando o atualiza. Ele mantém a estrutura e troca só o código. Quando o
dataset não existe, o comando o cria. Para **criar**, você precisa da opção
`--new` em modo não-interativo. Esta proteção evita a criação de dataset por
erro de digitação no nome. A opção `--description` define a descrição na
criação. O valor padrão é o nome.

```sh
fluigcli dataset export datasets/ds_clientes.js
fluigcli dataset export datasets/ds_novo.js --new --description "Cadastro novo"
```

## `fluigcli dataset query <id> [flags]`

Este comando consulta os dados de um dataset pela API REST v2
`dataset-handle/search`. O servidor aplica o `--limit`. Sem limite, a CLI pagina
até o fim.

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

Dataset inexistente → exit **4**. Consulta com campo ou ordenação inválidos →
exit **4**.

## `fluigcli dataset disable <id>...` / `enable <id>...`

O `disable` desativa um dataset **sem apagá-lo**, porque o Fluig não tem API de
exclusão de dataset. O `enable` reativa datasets desativados. Os dois comandos
são nativos (REST v2). Um dataset inativo some das consultas. Mas ele continua
listado (coluna Ativo = "não") e mantém o histórico.

```sh
fluigcli dataset disable ds_legado
fluigcli dataset enable ds_legado ds_outro
```

Dataset inexistente → exit **4**. Em produção, vale a trava de confirmação.

## `fluigcli dataset history <id> [--version N]`

Este comando mostra o histórico de versões de um dataset customizado. A saída
traz a versão, o status (PUBLISHED/DRAFT), o autor, a data e o tamanho do
código. O comando mostra a versão corrente em verde. Com `--version N`, o
comando imprime o **código JS** daquela versão. Use isso para comparar ou salvar
em arquivo.

```sh
fluigcli dataset history ds_clientes
fluigcli dataset history ds_clientes --version 3 > ds_clientes_v3.js
fluigcli dataset history ds_clientes --json     # versões sem o código (lines por versão)
```

Apenas datasets **customizados** têm histórico. Para os demais, a CLI informa e
retorna lista vazia. Dataset inexistente → exit **4**.

## `fluigcli dataset restore <id> <version>`

Este comando restaura o código de um dataset para uma versão anterior do
histórico. O restore **cria uma versão nova** e publicada com o código da versão
alvo. O comando nunca reescreve o histórico. A CLI valida a versão contra o
histórico antes. A CLI avisa quando há rascunho não publicado, porque o restore
o descarta. A CLI pede confirmação. A opção `--yes` pula a confirmação.

```sh
fluigcli dataset history ds_clientes            # descubra a versão boa
fluigcli dataset restore ds_clientes 3 --yes
```

Versão fora do histórico → exit **4**. Neste caso, o comando não toca no
servidor.

## Lote e exit codes

O `import` e o `export` aceitam vários alvos. Quando um item falha e outros têm
sucesso, o exit code é **6** (sucesso parcial). Neste caso, o JSON traz
`data.results[]` com `success`/`error` por item. Um alvo único que falha retorna
o código real (3 auth, 4 não encontrado, 5 rejeitado pelo servidor).
