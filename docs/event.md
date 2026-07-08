# fluigcli event — eventos globais

Importa, exporta e exclui eventos globais. Arquivos locais em `events/<id>.js`
(um arquivo por evento; o nome do evento é o basename sem `.js`).

- **import** = servidor → projeto local
- **export** = projeto local → servidor

## `fluigcli event list`

Lista os eventos globais do servidor.

## `fluigcli event import <id>... | --all`

Baixa eventos do servidor para `events/<id>.js` (sobrescreve o arquivo existente
se já houver um com o mesmo nome sob `events/`). `--all` importa todos.

```sh
fluigcli event import displayCustomThemes
fluigcli event import --all
```

## `fluigcli event export <file>...`

Envia eventos locais para o servidor.

> **Importante:** o Fluig salva a **lista completa** de eventos de uma vez. Para
> não apagar nada, a CLI busca a lista atual do servidor e **sobrepõe apenas os
> eventos informados** — exportar um evento não remove os demais.

```sh
fluigcli event export events/meuEvento.js
fluigcli event export events/*.js
```

## `fluigcli event delete <id>...`

Exclui eventos globais no servidor. Pede confirmação (use `--yes` para pular, ou
em modo não-interativo).

```sh
fluigcli event delete eventoAntigo --yes
```

## Lote e exit codes

`import`/`export`/`delete` aceitam vários alvos. Falha parcial em lote → exit
**6**, com `data.results[]` detalhando cada item. Um alvo único que falha
retorna o código real (3 auth, 4 não encontrado, 5 rejeitado pelo servidor).
