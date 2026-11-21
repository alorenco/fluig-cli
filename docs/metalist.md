# fluigcli metalist — listas customizadas

O grupo `metalist` cria, altera e remove as **listas customizadas** do Painel
de Controle (menu Listas) e mantém os registros delas. Uma lista guarda dados que widgets,
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

## `fluigcli metalist create <nome> --column TÍTULO:TIPO... [flags]`

Este comando cria uma lista com as colunas informadas. O servidor cria uma
tabela no banco para a lista. O resultado mostra o id da lista nova.

| Flag | Uso |
|---|---|
| `--column TÍTULO:TIPO` | uma coluna. Repita a flag para cada coluna |
| `--column TÍTULO:metalista:LISTA` | uma coluna que referencia outra lista (id ou nome) |
| `--label TÍTULO` | marca a coluna como rótulo. Repita a flag para mais de uma |
| `--description <texto>` | descrição da lista (até 254 caracteres) |

O **rótulo** é o texto que aparece quando outra lista referencia esta. Uma
coluna do tipo `metalista` não pode ser rótulo.

```sh
fluigcli metalist create "Tipos de Contrato" \
  --column "Código:inteiro" \
  --column "Descrição:texto" \
  --column "Início:data" \
  --label Descrição

fluigcli metalist create "Contratos por Cartório" \
  --column "Contrato:texto" \
  --column "Cartório:metalista:Cartórios"
```

### Nome da coluna

O servidor gera o nome da coluna a partir do título. A regra é esta:

- As letras ficam em maiúsculas e sem acento.
- Cada espaço vira `_`.
- Os dígitos do início saem. Por exemplo: `1abc` vira `ABC`.
- Os caracteres `-`, `º` e `ª` saem. Por exemplo: `Nº Pedido-X` vira `N_PEDIDOX`.

O nome tem no máximo 30 caracteres. O título aceita só letras, dígitos,
espaço, `_`, `-`, `º` e `ª`. Outro símbolo termina com exit **2**. Por
exemplo: o servidor manteria a `/` no nome, e o banco recusaria a coluna com a
lista já criada.

O servidor recusa palavras reservadas, como `ID`, `DATE`, `USER`, `VALUE`,
`ORDER` e `SELECT`. Neste caso, o comando termina com exit **5**, e nada é
gravado.

### Antes de gravar

A CLI confere estes pontos e termina com exit **2** sem gravar:

- Outra lista já usa o nome. O servidor aceitaria o nome repetido.
- Duas colunas geram o mesmo nome. Por exemplo: `Ação` e `acao`.
- A lista referenciada não existe. Neste caso, o exit é **4**.

## `fluigcli metalist update <id|nome> [flags]`

Este comando altera a lista. Informe ao menos uma flag.

| Flag | Uso |
|---|---|
| `--name <nome>` | nome novo |
| `--description <texto>` | descrição nova. Um valor vazio limpa a descrição |
| `--add-column TÍTULO:TIPO` | coluna nova, no formato do `create`. Repita a flag para cada coluna |
| `--label COLUNA` | marca a coluna (nova ou existente) como rótulo |
| `--no-label COLUNA` | tira o rótulo da coluna |
| `--disable-column COLUNA` | desativa a coluna |
| `--enable-column COLUNA` | reativa a coluna desativada |

Identifique uma coluna existente pelo nome ou pelo título. Uma coluna
desativada não tem nome. Neste caso, use o título.

Uma coluna desativada some dos registros. Os dados dela ficam no banco e
voltam com `--enable-column`.

O comando **não** muda o título nem o tipo de uma coluna existente. A API
aceita a mudança, mas ela corrompe a lista. Para trocar uma coluna, desative a
antiga e inclua uma nova.

Uma coluna nova não pode gerar o nome de outra coluna, ativa ou desativada. O
servidor ignoraria a coluna nova sem avisar. Por isso, a CLI recusa a coluna
com exit **2**.

```sh
fluigcli metalist update "Tipos de Contrato" --add-column "Valor:numerico"
fluigcli metalist update 140 --disable-column Início --name "Tipos de Contrato (2026)"
fluigcli metalist update 140 --enable-column Início --no-label Descrição --label Código
```

## `fluigcli metalist delete <id|nome>...`

Este comando remove as listas. O comando pede confirmação. Use `--yes` para não
perguntar.

O Fluig não apaga os dados. A lista some da interface e da API.

Uma lista que outra lista referencia (coluna `metalista`) não sai. O comando
termina com exit **5**. O bloqueio continua mesmo depois de remover a lista que
a referencia. Por isso, crie a referência só quando ela for definitiva.

```sh
fluigcli metalist delete "Tipos de Contrato" --yes
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

Os comandos de escrita (`create`, `update`, `delete` e os de `records`) num
servidor de produção pedem confirmação. Use `--yes` no modo não-interativo.
