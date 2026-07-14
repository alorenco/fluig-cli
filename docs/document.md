# fluigcli document — GED

Navega, baixa e publica documentos do GED direto do terminal. Nativo (REST v2
`content-management`; as pastas raiz vêm do SOAP `ECMFolderService`, que é a
única rota que as lista).

## `fluigcli document list [<folderId>]`

Sem argumento, lista as **pastas raiz**; com um id, o conteúdo da pasta —
subpastas (em verde), arquivos e artigos, com versão, tamanho, autor e data.
Navegue descendo pelos ids.

```sh
fluigcli document list                # raízes
fluigcli document list 2864           # conteúdo da pasta 2864
fluigcli document list 2864 --json    # para agentes/CI
```

## `fluigcli document download <id>... [--dir <pasta>]`

Baixa documentos pelo id (o nome do arquivo vem dos metadados; round-trip
byte a byte com o upload). Documento cujo arquivo físico sumiu do volume do
servidor gera erro claro (exit 5); id inexistente → exit **4**.

```sh
fluigcli document download 926468 --dir ./downloads
```

## `fluigcli document upload <file>... --folder <id>`

Publica arquivos numa pasta do GED (upload + publish em uma etapa).

```sh
fluigcli document upload relatorio.pdf --folder 1111279
fluigcli document upload *.pdf --folder 1111279
```

## `fluigcli document mkdir <parentId> <nome>`

Cria uma pasta dentro de outra (descubra o pai com `document list`).

```sh
fluigcli document mkdir 2864 "Relatórios 2026"
```

## `fluigcli document delete <id>...`

Envia documentos ou pastas para a **lixeira** do GED (não é exclusão
definitiva). Pede confirmação; `--yes` pula.

```sh
fluigcli document delete 1111280 --yes
```
