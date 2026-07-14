# fluigcli user — usuários da plataforma

Consulta e gestão de usuários via módulo administrativo (`/admin/api/v1`).
**Requer usuário com privilégio administrativo** — sem ele a API responde 401
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

## `fluigcli user create <login> --email … --first-name … --last-name …`

Cria um usuário. A **senha nunca vai por argumento** (regra de segurança): vem
da variável `FLUIGCLI_NEW_USER_PASSWORD` ou, em modo interativo, de um prompt
oculto com confirmação. O código (userCode) usa o login quando `--code` não é
informado.

```sh
# interativo (pede a senha, oculta)
fluigcli user create jsilva --email joao@empresa.com --first-name João --last-name Silva

# automação (senha por env)
FLUIGCLI_NEW_USER_PASSWORD='Seg@redo123' fluigcli user create jsilva \
  --email joao@empresa.com --first-name João --last-name Silva --json
```

Login já existente → exit **5** (`já existe um usuário com o login …`).

## `fluigcli user update <login> [flags]`

Atualiza só os campos informados (os demais são **preservados** — o PUT
mescla). `--set-password` redefine a senha lendo de `FLUIGCLI_NEW_USER_PASSWORD`
ou do prompt (nunca por argumento).

| Flag | Uso |
|---|---|
| `--email`, `--first-name`, `--last-name`, `--full-name` | novos valores (só os informados mudam) |
| `--set-password` | redefine a senha (env ou prompt) |

```sh
fluigcli user update jsilva --email joao.silva@empresa.com
fluigcli user update jsilva --set-password
```

## `fluigcli user activate <login>` / `deactivate <login>`

Reativa ou desativa um usuário. **Não há exclusão de usuário na API** —
desativar é o caminho; o usuário desativado fica no estado **`BLOCKED`** (e
sai das buscas, a menos de `--inactive`).

```sh
fluigcli user deactivate jsilva
fluigcli user activate jsilva
```

Login inexistente → exit **4**. Em servidor `prod`, as operações de escrita
respeitam a trava de confirmação.
