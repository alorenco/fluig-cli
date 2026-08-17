# fluigcli document — GED

Este grupo trabalha o GED direto do terminal: navega (inclusive a árvore
inteira, com `--recursive`), **procura por nome**, mostra onde um documento
está, **move**, baixa, publica, cria pastas e exclui. Ele usa a REST v2
`content-management`; as pastas raiz e o move vêm do SOAP (as únicas rotas).

## `fluigcli document list [<folderId>]`

Sem argumento, este comando lista as **pastas raiz**. Com um id, ele lista o
conteúdo da pasta. O conteúdo inclui subpastas (em verde), arquivos e artigos,
com versão, tamanho, autor e data. Navegue descendo pelos ids.

```sh
fluigcli document list                # raízes
fluigcli document list 2864           # conteúdo da pasta 2864
fluigcli document list 2864 --json    # para agentes/CI
```

## `fluigcli document list <folderId> --recursive [flags]`

Este modo desce a árvore inteira a partir da pasta. Ele mostra o **caminho** de
cada item. Use-o na verificação pós-deploy: "o processo criou o documento na
pasta certa?" vira uma chamada.

```sh
fluigcli document list 605650 --recursive --depth 3
```

| Flag | Uso |
|---|---|
| `--depth N` | profundidade máxima (default 10; `1` = só o conteúdo direto) |

A varredura faz uma requisição por pasta visitada, com teto de **300 pastas**.
Ao atingir o teto, a CLI **avisa** e marca `"truncated": true` no envelope — o
resultado está incompleto. Reduza com `--depth` ou parta de uma pasta mais
específica. Subpasta sem permissão vira um buraco na árvore, sem erro.

## `fluigcli document find --name <padrão> --under <folderId> [flags]`

Este comando procura itens por **nome** na árvore de uma pasta. O padrão é um
glob (`*` e `?`), sem diferenciar maiúsculas. O resultado traz o caminho
completo de cada item.

```sh
fluigcli document find --name "Notificação nº*" --under 605650
fluigcli document find --name "*.pdf" --under 25255 --depth 2
```

O `--under` é obrigatório: ele delimita a busca (descubra as raízes com
`document list` sem argumento). Os limites do `--recursive` valem aqui também
(`--depth`, teto de 300 pastas com aviso).

## `fluigcli document show <id>`

Este comando mostra os metadados de um item do GED: id, nome, tipo, versão e a
**pasta pai** (nome e id). Ele responde "onde este documento está?" sem o
dataset `document`.

```sh
fluigcli document show 1247112
# zz_move_teste.txt (id 1247112, FileDocument, versão 1000)
# pasta pai: zz_fluigcli_test_move_B (id 1247111)
```

No `--json`, `data.id` e `data.documentId` são sinônimos (consistência entre
comandos).

## `fluigcli document move <id>... --folder <destino>`

Este comando move documentos ou pastas para outra pasta do GED. Ele usa o SOAP
`moveDocument` — não há rota REST (o `PATCH` de propriedades recusa
`parentId`).

```sh
fluigcli document move 1247112 --folder 1247110
```

Em lote, cada id tem o próprio resultado em `data.results[]`. Falha parcial
vira exit **6**. A CLI confirma o efeito: ela relê o item e valida o
`parentId`. Destino inexistente ou sem permissão volta como exit 5, com a
mensagem do servidor.
## `fluigcli document download <id>... [--dir <pasta>] [--name-template <t>]`

Este comando baixa documentos pelo id. O round-trip com o upload é byte a byte.
Um documento pode ter o arquivo físico removido do volume do servidor. Neste
caso, o comando gera erro claro (exit 5). Um id inexistente gera exit **4**.

```sh
fluigcli document download 926468 --dir ./downloads
```

### Nome do arquivo gravado

A CLI escolhe o nome nesta ordem:

1. o nome do arquivo físico informado pelo servidor;
2. a descrição do documento;
3. `documento_<id>`, quando os dois estão vazios.

O nome passa por duas correções. Primeiro, o caractere que o sistema de arquivos
não aceita vira `_`. Por exemplo: a descrição `Contrato - Cancelado em Nov/24.`
grava `Contrato - Cancelado em Nov_24`. Segundo, o nome sem extensão recebe a
extensão do tipo do conteúdo. Por exemplo: um PDF cuja descrição não termina em
`.pdf` é gravado com `.pdf`.

Dois documentos do mesmo lote podem ter o mesmo nome. Neste caso, o segundo
recebe o sufixo ` (2)`. Nenhum documento apaga o outro. Um arquivo de execução
anterior continua sendo sobrescrito.

### `--name-template`

Esta flag manda no nome do arquivo. Os marcadores são:

| marcador | conteúdo |
|---|---|
| `{id}` | o id do documento |
| `{name}` | o nome escolhido, sem a extensão |
| `{ext}` | a extensão, com o ponto |
| `{fileName}` | o nome escolhido, com a extensão |

O template aceita `/` e cria a subpasta.

```sh
fluigcli document download 926468 77995 --dir ./box --name-template "{id}_{fileName}"
fluigcli document download 926468 77995 --dir ./box --name-template "{id}/{fileName}"
```

Um marcador desconhecido gera exit **2**.

### Saída em lote

Cada item de `data.results[]` traz o id pedido e o arquivo gravado:

```json
{
  "id": "77995",
  "documentId": 77995,
  "action": "downloaded",
  "success": true,
  "fileName": "Contrato - Cancelado em Nov_24.pdf",
  "path": "/home/joao/box/Contrato - Cancelado em Nov_24.pdf"
}
```

Por isso um lote de ids dispensa uma chamada por documento. Use o `path` para
casar o id com o arquivo.

A falha de gravação em disco tem código próprio: `LOCAL_IO_ERROR`, com exit
**1**. Neste caso, o servidor respondeu bem e o problema está na máquina local.
Não repita a chamada. Conserte o destino.

## `fluigcli document upload <file>... --folder <id>`

Este comando publica arquivos numa pasta do GED. Ele faz upload e publish em uma
etapa.

```sh
fluigcli document upload relatorio.pdf --folder 1111279
fluigcli document upload *.pdf --folder 1111279
```

## `fluigcli document mkdir <parentId> <nome>`

Este comando cria uma pasta dentro de outra. Descubra o pai com `document list`.

```sh
fluigcli document mkdir 2864 "Relatórios 2026"
```

## `fluigcli document delete <id>...`

Este comando envia documentos ou pastas para a **lixeira** do GED. Ele não faz
exclusão definitiva. Ele pede confirmação. Informe `--yes` para pular.

```sh
fluigcli document delete 1111280 --yes
```

