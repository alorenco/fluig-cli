# fluigcli metalist — listas customizadas

O grupo `metalist` consulta as **listas customizadas** do Painel de Controle
(menu Listas) e mantém os registros delas. Uma lista guarda dados que widgets,
formulários e datasets usam. Por exemplo: uma tabela de cartórios ou de ramais.
O Fluig cria uma tabela no banco para cada lista.

O comando identifica uma lista pelo **id** ou pelo **nome**. Um número é
sempre o id. Um nome é procurado sem diferenciar maiúsculas. Use
`metalist list` para ver os dois. Lista inexistente termina com exit **4**.

A API é a legada do portal (`/ecm/api/rest/ecm/metaList/...`). Ela fica fora
dos swaggers.

## Tipos de coluna

| Tipo | Valor aceito na escrita |
|---|---|
| `texto` | texto |
| `area-texto` | texto |
| `inteiro` | só dígitos. Por exemplo: `42` |
| `data` | `dd/mm/aaaa` ou `aaaa-mm-dd`. A CLI grava `dd/mm/aaaa` |
| `numerico` | `12.5` ou `12,5`. A CLI grava `12.5` |
| `metalista` | o id de um registro da lista referenciada |

## Validação antes da escrita

O servidor aceita dado inválido sem avisar. Por exemplo:

- Uma coluna inexistente cria um registro vazio.
- Uma data em outro formato fica vazia.
- Uma referência a um registro inexistente fica vazia na leitura.

Por isso, a CLI valida cada valor antes de gravar. Um valor inválido termina
com exit **2**, e nada é gravado.

## `fluigcli metalist list`

Este comando lista as listas da empresa, com id e nome, em ordem de nome.

```sh
fluigcli metalist list
fluigcli metalist list --json
```

## `fluigcli metalist show <id|nome>`

Este comando mostra a lista e as colunas dela.

A coluna **Coluna** é o nome que os registros usam. Use esse nome em `--field`
e em `--where`. O título também é aceito nessas flags. Uma coluna desativada
não tem nome e não aceita valores.

```sh
fluigcli metalist show 87
fluigcli metalist show Cartórios --json
```

## `fluigcli metalist records list <id|nome> [flags]`

Este comando lista os registros da lista. O servidor devolve todas as linhas
de uma vez.

Uma coluna do tipo `metalista` mostra o **rótulo** do registro referenciado.
Para ver o id da referência, use `metalist records show`.

| Flag | Uso |
|---|---|
| `--where COLUNA=VALOR` | filtra pelo valor inteiro, sem diferenciar maiúsculas. Repita a flag para combinar filtros |
| `--columns a,b` | escolhe as colunas da tabela. O `--json` traz todas |

```sh
fluigcli metalist records list Cartórios
fluigcli metalist records list 87 --where CIDADE=DOURADOS/MS --columns NOME_FANTASIA
fluigcli metalist records list 87 --json
```

## `fluigcli metalist records show <id|nome> <recordId>`

Este comando mostra um registro. Uma coluna do tipo `metalista` mostra o **id**
do registro referenciado. Use esse id em `--field`. Registro inexistente
termina com exit **4**.

```sh
fluigcli metalist records show 87 12
```

## `fluigcli metalist records create <id|nome> [flags]`

Este comando inclui um registro na lista. O resultado mostra o id do registro
novo.

| Flag | Uso |
|---|---|
| `--field COLUNA=VALOR` | valor de uma coluna. Repita a flag para cada coluna |
| `--fields-file <arquivo>` | valores em JSON plano `{"COLUNA": "valor"}`. Use `-` para ler do stdin. O `--field` sobrepõe o arquivo |

Uma coluna fora dos valores fica vazia. Um valor vazio também deixa a coluna
vazia.

```sh
fluigcli metalist records create Cartórios \
  --field CIDADE=DOURADOS/MS \
  --field "NOME_FANTASIA=CARTÓRIO EXEMPLO" \
  --field "RAZAO_SOCIAL=CARTÓRIO EXEMPLO LTDA"

echo '{"RAMAL": "5600", "NOME": "João Silva", "SETOR": "Compras"}' \
  | fluigcli metalist records create Ramais --fields-file - --json
```

## `fluigcli metalist records update <id|nome> <recordId> [flags]`

Este comando altera as colunas informadas de um registro. As outras colunas
ficam como estão. As flags são as mesmas do `create`. Registro inexistente
termina com exit **4**.

```sh
fluigcli metalist records update Ramais 12 --field RAMAL=5601
```

## `fluigcli metalist records delete <id|nome> <recordId>...`

Este comando exclui registros da lista. O comando pede confirmação. Use `--yes`
para não perguntar.

A CLI confere cada id antes e depois da exclusão. Um id inexistente entra no
resultado como falha. Neste caso, com mais de um id, o exit é **6**.

```sh
fluigcli metalist records delete Ramais 12 13 --yes
```

## Produção

Os comandos de escrita (`create`, `update` e `delete`) num servidor de
produção pedem confirmação. Use `--yes` no modo não-interativo.
