# fluigcli event — eventos globais

O grupo `event` importa, exporta e exclui eventos globais. Os arquivos locais
ficam em `events/<id>.js`. Há um arquivo por evento. O nome do evento é o
basename sem `.js`.

- **import** = servidor → projeto local
- **export** = projeto local → servidor

## `fluigcli event new <name>`

Este comando cria `events/<name>.js` com a função do evento. O nome do arquivo
é o **id do evento global** que a plataforma dispara. Por exemplo,
`displayCustomThemes` ou `beforeConvertViewToPDF`. Os parâmetros variam por
evento. Ajuste a assinatura gerada. O comando cria **só o arquivo local**.
Publique depois com `event export`.

```sh
fluigcli event new displayCustomThemes
fluigcli event export events/displayCustomThemes.js
```

## `fluigcli event list`

Este comando lista os eventos globais do servidor.

## `fluigcli event import <id>... | --all`

Este comando baixa eventos do servidor para `events/<id>.js`. O comando
sobrescreve o arquivo existente com o mesmo nome sob `events/`. A opção `--all`
importa todos os eventos.

```sh
fluigcli event import displayCustomThemes
fluigcli event import --all
```

## `fluigcli event export <file>...`

Este comando envia eventos locais para o servidor.

> **Importante:** o Fluig salva a **lista completa** de eventos de uma vez.
> Por isso, a CLI busca a lista atual do servidor e **sobrepõe apenas os
> eventos informados**. Assim, o comando não apaga os demais eventos.

```sh
fluigcli event export events/meuEvento.js
fluigcli event export events/*.js
```

## `fluigcli event delete <id>...`

Este comando exclui eventos globais no servidor. O comando pede confirmação.
Use `--yes` para pular a confirmação. O modo não-interativo também pula a
confirmação.

```sh
fluigcli event delete eventoAntigo --yes
```

## Lote e exit codes

Os comandos `import`, `export` e `delete` aceitam vários alvos. A falha parcial
em lote retorna exit **6**. Neste caso, `data.results[]` detalha cada item. Um
alvo único que falha retorna o código real. O código 3 indica erro de
autenticação. O código 4 indica alvo não encontrado. O código 5 indica que o
servidor rejeitou a operação.
