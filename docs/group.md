# fluigcli group — grupos da plataforma

Consulta, gestão e membros de grupos via módulo administrativo
(`/admin/api/v1`). **Requer usuário com privilégio administrativo** — sem ele a
API responde 401 (exit 3).

Um grupo tem apenas **`code`** (identificador), **`description`** (rótulo humano
— não há campo "nome") e **`type`**:

- **`user`** — grupos criados/administrados pelos administradores.
- **`community`** — grupos automáticos das comunidades (códigos com prefixo
  `MODERATOR_`/`MEMBER_`); não crie/apague à mão.

## `fluigcli group list [flags]`

Lista os grupos com código, descrição e tipo (grupos `user` em verde).

| Flag | Uso |
|---|---|
| `--type user\|community` | filtra por tipo |
| `--search <texto>` | substring em código **ou** descrição (case-insensitive) |
| `--limit N` | máximo (default 50; 0 = todos) |

> ⚠️ Os parâmetros `type`/`pattern` da API são **ignorados pelo servidor** (na
> homologação toda variação devolve a lista inteira), então `--type` e
> `--search` são aplicados **no cliente**, sobre as páginas já buscadas.

```sh
fluigcli group list --type user
fluigcli group list --search compras
fluigcli group list --limit 0 --json
```

## `fluigcli group show <code>`

Mostra o grupo (código, descrição, tipo) e a lista de **membros** (logins).

```sh
fluigcli group show Compras
fluigcli group show Compras --json   # inclui group + users estruturados
```

Código inexistente → exit **4**.

## `fluigcli group create <code> --description <texto> [--type user|community]`

Cria um grupo. A **descrição é obrigatória** (sem ela o servidor responde 500).
O tipo default é `user`.

```sh
fluigcli group create Compras --description "Setor de Compras"
```

Código já existente → exit **5** (`já existe um grupo com o código …`).

## `fluigcli group update <code> --description <texto>`

Atualiza a descrição (o PUT **mescla** — código e tipo são preservados).

```sh
fluigcli group update Compras --description "Compras e Suprimentos"
```

## `fluigcli group delete <code>`

Exclui o grupo. Código inexistente → exit **4**.

```sh
fluigcli group delete Compras
```

## Membros — `fluigcli group users|add-user|remove-user`

```sh
fluigcli group users Compras                 # lista os membros (tabela)
fluigcli group add-user Compras jsilva       # adiciona um usuário
fluigcli group remove-user Compras jsilva    # remove um usuário
```

> ⚠️ A API de **adicionar membro não valida** o grupo nem o login (responde
> sucesso mesmo para inexistentes, criando associação órfã). Por isso o
> `add-user` **valida grupo e usuário antes** e devolve exit **4** limpo se
> algum não existir. `remove-user` de quem não é membro também dá exit **4**.

Em servidor `prod`, as operações de escrita (`create`/`update`/`delete`/
`add-user`/`remove-user`) respeitam a trava de confirmação (`--yes`).

## Notas

- **Não há endpoint para trocar diretamente os grupos DE um usuário** — a
  associação é sempre pelo lado do grupo (`group add-user`/`remove-user`), como
  aqui, ou pelos papéis (ciclo `role`, futuro).
- Papéis e subgrupos de um grupo (`/groups/{code}/roles|groups`) existem na API
  mas ficaram fora dos comandos por ora (sem demanda).
