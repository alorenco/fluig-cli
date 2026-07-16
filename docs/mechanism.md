# fluigcli mechanism — mecanismos de atribuição

Importa, exporta e exclui mecanismos de atribuição **customizados**. Arquivos
locais em `mechanisms/<id>.js` (o nome é o basename sem `.js`).

- **import** = servidor → projeto local
- **export** = projeto local → servidor

## `fluigcli mechanism new <name>`

Cria `mechanisms/<name>.js` com o esqueleto do mecanismo customizado — a
função que devolve a lista de usuários aptos a receber a tarefa (sempre por
**userCode/matrícula**, nunca por login). **Só local** — publique depois com
`mechanism export`.

```sh
fluigcli mechanism new mec_gestor_area
fluigcli mechanism export mechanisms/mec_gestor_area.js --name "Gestor da Área"
```

## `fluigcli mechanism list`

Lista os mecanismos customizados do servidor.

## `fluigcli mechanism import <id>... | --all`

Baixa o código dos mecanismos para `mechanisms/<id>.js`.

```sh
fluigcli mechanism import mec_gestor_area
fluigcli mechanism import --all
```

## `fluigcli mechanism export <file>... [--name "..."] [--description "..."]`

Envia mecanismos locais. Se o mecanismo já existe, atualiza (preserva o DTO e
troca só o código). Se não existe, cria — `--name`/`--description` definem os
metadados (default: o id). Os campos técnicos (`assignmentType=1`,
`controlClass`) são fixos.

```sh
fluigcli mechanism export mechanisms/mec_gestor_area.js
fluigcli mechanism export mechanisms/mec_novo.js --name "Novo Mecanismo"
```

## `fluigcli mechanism delete <id>...`

Exclui mecanismos no servidor (confirmação; `--yes` para pular).

## Lote e exit codes

Vários alvos por comando; falha parcial em lote → exit **6** com
`data.results[]`. Alvo único que falha retorna o código real (3/4/5).
