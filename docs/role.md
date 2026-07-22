# fluigcli role — papéis da plataforma

O grupo `role` consulta e administra papéis e seus usuários. Ele usa o módulo
administrativo (`/admin/api/v1`). Estes comandos **precisam de um usuário com
privilégio administrativo**. Sem o privilégio, a API responde 401 (exit 3).

Um papel tem apenas dois campos. O **`code`** é o identificador. A
**`description`** é o rótulo humano. Este grupo é a via para dar ou tirar um papel
de um usuário. Não há endpoint para alterar os papéis pelo lado do usuário.

## `fluigcli role list [flags]`

Este comando lista os papéis com código e descrição.

| Flag | Uso |
|---|---|
| `--search <texto>` | substring em código **ou** descrição (case-insensitive) |
| `--limit N` | máximo (default 50; 0 = todos) |

> ⚠️ O servidor **ignora** os filtros na query, como acontece nos grupos. Por
> isso o comando aplica `--search` **no cliente**, sobre as páginas já buscadas.

```sh
fluigcli role list
fluigcli role list --search aprova
fluigcli role list --limit 0 --json
```

## `fluigcli role show <code>`

Este comando mostra o papel (código, descrição) e a lista de **usuários**
vinculados (logins).

```sh
fluigcli role show aprovadores
fluigcli role show aprovadores --json   # inclui role + users estruturados
```

Código inexistente → exit **4**.

## `fluigcli role create <code> [--description <texto>]`

Este comando cria um papel. A **descrição é opcional**. Quando você a omite, o
comando usa o próprio código como descrição. Assim o papel não fica com a
descrição em branco.

```sh
fluigcli role create aprovadores --description "Aprovadores de requisição"
fluigcli role create aprovadores           # descrição = "aprovadores"
```

Código já existente → exit **5** (`já existe um papel com o código …`).

## `fluigcli role update <code> --description <texto>`

Este comando atualiza a descrição. O PUT **mescla**. Por isso o comando preserva
o código.

```sh
fluigcli role update aprovadores --description "Aprovadores (nível 1)"
```

## `fluigcli role delete <code>`

Este comando exclui o papel. Código inexistente → exit **4**.

## Usuários — `fluigcli role users|add-user|remove-user`

```sh
fluigcli role users aprovadores               # lista os usuários com o papel
fluigcli role add-user aprovadores jsilva      # dá o papel a um usuário
fluigcli role remove-user aprovadores jsilva   # tira o papel de um usuário
```

A API de vínculo de papel **valida** o papel e o login, ao contrário dos grupos.
Um valor inexistente gera erro. Mas a API usa a **mesma exceção genérica** para
os dois casos. Por isso a CLI **pré-valida** para apontar exatamente o que falta.
Ela devolve exit **4** com a mensagem certa (`papel …` ou `usuário …`). Remover o
papel de quem não o tem também dá exit **4**.

Em servidor `prod`, as operações de escrita (`create`/`update`/`delete`/
`add-user`/`remove-user`) respeitam a trava de confirmação (`--yes`).

## Relação com `user`

O `user show <login>` lista os papéis do usuário. O `user list --role <papel>`
filtra por papel. Ambos consomem o mesmo cadastro exposto aqui.
