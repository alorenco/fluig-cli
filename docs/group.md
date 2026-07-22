# fluigcli group — grupos da plataforma

O grupo `group` consulta e administra grupos e seus membros. Ele usa o módulo
administrativo (`/admin/api/v1`). Estes comandos **precisam de um usuário com
privilégio administrativo**. Sem o privilégio, a API responde 401 (exit 3).

Um grupo tem apenas três campos. O **`code`** é o identificador. A
**`description`** é o rótulo humano. Não há campo "nome". O **`type`** indica a
origem do grupo:

- **`user`** — grupos criados e administrados pelos administradores.
- **`community`** — grupos automáticos das comunidades. Os códigos têm o prefixo
  `MODERATOR_` ou `MEMBER_`. Não crie nem apague estes grupos à mão.

## `fluigcli group list [flags]`

Este comando lista os grupos com código, descrição e tipo. Ele mostra os grupos
`user` em verde.

| Flag | Uso |
|---|---|
| `--type user\|community` | filtra por tipo |
| `--search <texto>` | substring em código **ou** descrição (case-insensitive) |
| `--limit N` | máximo (default 50; 0 = todos) |

> ⚠️ O servidor **ignora** os parâmetros `type` e `pattern` da API. Na
> homologação, toda variação devolve a lista inteira. Por isso o comando aplica
> `--type` e `--search` **no cliente**, sobre as páginas já buscadas.

```sh
fluigcli group list --type user
fluigcli group list --search compras
fluigcli group list --limit 0 --json
```

## `fluigcli group show <code>`

Este comando mostra o grupo (código, descrição, tipo) e a lista de **membros**
(logins).

```sh
fluigcli group show Compras
fluigcli group show Compras --json   # inclui group + users estruturados
```

Código inexistente → exit **4**.

## `fluigcli group create <code> --description <texto> [--type user|community]`

Este comando cria um grupo. A **descrição é obrigatória**. Sem ela o servidor
responde 500. O tipo default é `user`.

```sh
fluigcli group create Compras --description "Setor de Compras"
```

Código já existente → exit **5** (`já existe um grupo com o código …`).

## `fluigcli group update <code> --description <texto>`

Este comando atualiza a descrição. O PUT **mescla**. Por isso o comando preserva
o código e o tipo.

```sh
fluigcli group update Compras --description "Compras e Suprimentos"
```

## `fluigcli group delete <code>`

Este comando exclui o grupo. Código inexistente → exit **4**.

```sh
fluigcli group delete Compras
```

## Membros — `fluigcli group users|add-user|remove-user`

```sh
fluigcli group users Compras                 # lista os membros (tabela)
fluigcli group add-user Compras jsilva       # adiciona um usuário
fluigcli group remove-user Compras jsilva    # remove um usuário
```

> ⚠️ A API de adicionar membro **não valida** o grupo nem o login. Ela responde
> sucesso mesmo para inexistentes e cria uma associação órfã. Por isso o
> `add-user` **valida o grupo e o usuário antes**. Ele devolve exit **4** limpo
> quando algum não existe. O `remove-user` de quem não é membro também dá exit
> **4**.

Em servidor `prod`, as operações de escrita (`create`/`update`/`delete`/
`add-user`/`remove-user`) respeitam a trava de confirmação (`--yes`).

## Notas

- **Não há endpoint para trocar diretamente os grupos DE um usuário.** A
  associação é sempre pelo lado do grupo (`group add-user`/`remove-user`), como
  aqui, ou pelos papéis (ciclo `role`, futuro).
- Os papéis e subgrupos de um grupo (`/groups/{code}/roles|groups`) existem na
  API. Estes comandos ficaram fora por ora. Não há demanda.
