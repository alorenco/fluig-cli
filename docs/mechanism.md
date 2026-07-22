# fluigcli mechanism — mecanismos de atribuição

O grupo `mechanism` importa, exporta e exclui mecanismos de atribuição
**customizados**. Os arquivos locais ficam em `mechanisms/<id>.js`. O nome é o
basename sem `.js`.

- **import** = servidor → projeto local
- **export** = projeto local → servidor

## `fluigcli mechanism new <name>`

Este comando cria `mechanisms/<name>.js` com o esqueleto do mecanismo
customizado. A função devolve a lista de usuários aptos a receber a tarefa. Use
sempre o **userCode/matrícula**. Não use o login. O comando cria **só o arquivo
local**. Publique depois com `mechanism export`.

```sh
fluigcli mechanism new mec_gestor_area
fluigcli mechanism export mechanisms/mec_gestor_area.js --name "Gestor da Área"
```

## `fluigcli mechanism list`

Este comando lista os mecanismos customizados do servidor.

## `fluigcli mechanism import <id>... | --all`

Este comando baixa o código dos mecanismos para `mechanisms/<id>.js`.

```sh
fluigcli mechanism import mec_gestor_area
fluigcli mechanism import --all
```

## `fluigcli mechanism export <file>... [--name "..."] [--description "..."]`

Este comando envia mecanismos locais. Se o mecanismo já existe, o comando o
atualiza. Neste caso, o comando preserva o DTO e troca só o código. Se o
mecanismo não existe, o comando o cria. As opções `--name` e `--description`
definem os metadados. O valor padrão de cada uma é o id. Os campos técnicos são
fixos. Por exemplo, `assignmentType=1` e `controlClass`.

```sh
fluigcli mechanism export mechanisms/mec_gestor_area.js
fluigcli mechanism export mechanisms/mec_novo.js --name "Novo Mecanismo"
```

## `fluigcli mechanism delete <id>...`

Este comando exclui mecanismos no servidor. O comando pede confirmação. Use
`--yes` para pular a confirmação.

## Lote e exit codes

Cada comando aceita vários alvos. A falha parcial em lote retorna exit **6**.
Neste caso, `data.results[]` detalha cada item. Um alvo único que falha retorna
o código real. Os códigos são 3, 4 e 5.
