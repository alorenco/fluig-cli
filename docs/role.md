# fluigcli role — papéis da plataforma

Consulta, gestão e usuários de papéis via módulo administrativo
(`/admin/api/v1`). **Requer usuário com privilégio administrativo** — sem ele a
API responde 401 (exit 3).

Um papel tem apenas **`code`** (identificador) e **`description`** (rótulo
humano). É a via para dar/tirar um papel de um usuário (não há endpoint para
alterar os papéis diretamente pelo lado do usuário).

## `fluigcli role list [flags]`

Lista os papéis com código e descrição.

| Flag | Uso |
|---|---|
| `--search <texto>` | substring em código **ou** descrição (case-insensitive) |
| `--limit N` | máximo (default 50; 0 = todos) |

> ⚠️ Como nos grupos, o servidor **ignora** filtros na query — `--search` é
> aplicado **no cliente**, sobre as páginas já buscadas.

```sh
fluigcli role list
fluigcli role list --search aprova
fluigcli role list --limit 0 --json
```

## `fluigcli role show <code>`

Mostra o papel (código, descrição) e a lista de **usuários** vinculados
(logins).

```sh
fluigcli role show aprovadores
fluigcli role show aprovadores --json   # inclui role + users estruturados
```

Código inexistente → exit **4**.

## `fluigcli role create <code> [--description <texto>]`

Cria um papel. A **descrição é opcional**; quando omitida, usa o próprio código
como descrição (evita papel com descrição em branco).

```sh
fluigcli role create aprovadores --description "Aprovadores de requisição"
fluigcli role create aprovadores           # descrição = "aprovadores"
```

Código já existente → exit **5** (`já existe um papel com o código …`).

## `fluigcli role update <code> --description <texto>`

Atualiza a descrição (o PUT **mescla** — o código é preservado).

```sh
fluigcli role update aprovadores --description "Aprovadores (nível 1)"
```

## `fluigcli role delete <code>`

Exclui o papel. Código inexistente → exit **4**.

## Usuários — `fluigcli role users|add-user|remove-user`

```sh
fluigcli role users aprovadores               # lista os usuários com o papel
fluigcli role add-user aprovadores jsilva      # dá o papel a um usuário
fluigcli role remove-user aprovadores jsilva   # tira o papel de um usuário
```

Ao contrário dos grupos, a API de vínculo de papel **valida** o papel e o login
(inexistente → erro), mas com a **mesma exceção genérica** para os dois. Para
apontar exatamente o que falta, a CLI **pré-valida** e devolve exit **4** com a
mensagem certa (`papel …` ou `usuário …`). Remover o papel de quem não o tem
também dá exit **4**.

Em servidor `prod`, as operações de escrita (`create`/`update`/`delete`/
`add-user`/`remove-user`) respeitam a trava de confirmação (`--yes`).

## Relação com `user`

`user show <login>` lista os papéis do usuário e `user list --role <papel>`
filtra por papel — ambos consomem o mesmo cadastro exposto aqui.
