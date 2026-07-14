# fluigcli user — usuários da plataforma

Consulta de usuários via módulo administrativo (`/admin/api/v1`). **Requer
usuário com privilégio administrativo** — sem ele a API responde 401
(exit 3).

## `fluigcli user list [flags]`

Lista os usuários com login, nome, e-mail e estado (ativos em verde).

| Flag | Uso |
|---|---|
| `--search <texto>` | busca por **prefixo de nome** (case-insensitive; substring não funciona — peculiaridade da API) |
| `--role <papel>` | usuários com o papel |
| `--inactive` | inclui usuários desativados |
| `--limit N` | máximo (default 50; 0 = todos) |

```sh
fluigcli user list --search Alessandro
fluigcli user list --role admin
fluigcli user list --inactive --limit 0 --json
```

## `fluigcli user show <login>`

Mostra um usuário com e-mail, código (userCode), última atualização e as
listas de **papéis** e **grupos**.

```sh
fluigcli user show alorenco
fluigcli user show alorenco --json    # inclui roles/groups estruturados
```

Login inexistente → exit **4**.
