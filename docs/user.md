# fluigcli user — usuários da plataforma

O grupo `user` consulta e gerencia usuários pelo módulo administrativo
(`/admin/api/v1`). Estes comandos precisam de um usuário com privilégio
administrativo. Sem esse privilégio, a API responde 401 (exit 3).

## `fluigcli user list [flags]`

Este comando lista os usuários. A lista mostra o login, o nome, o e-mail e o
estado. O comando mostra os usuários ativos em verde.

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

Este comando mostra um usuário. A saída traz o e-mail, o código (userCode) e a
data da última atualização. A saída traz também as listas de **papéis** e
**grupos**.

```sh
fluigcli user show alorenco
fluigcli user show alorenco --json    # inclui roles/groups estruturados
```

Login inexistente → exit **4**.

## `fluigcli user create <login> --email … --first-name … --last-name …`

Este comando cria um usuário. A **senha nunca vai por argumento**. Esta é uma
regra de segurança. A senha vem da variável `FLUIGCLI_NEW_USER_PASSWORD`. Em
modo interativo, a senha vem de um prompt oculto com confirmação. O código
(userCode) usa o login quando você não informa `--code`.

```sh
# interativo (pede a senha, oculta)
fluigcli user create jsilva --email joao@empresa.com --first-name João --last-name Silva

# automação (senha por env)
FLUIGCLI_NEW_USER_PASSWORD='Seg@redo123' fluigcli user create jsilva \
  --email joao@empresa.com --first-name João --last-name Silva --json
```

Login já existente → exit **5** (`já existe um usuário com o login …`).

## `fluigcli user update <login> [flags]`

Este comando atualiza só os campos que você informa. O comando **preserva** os
demais campos, porque o PUT mescla. A opção `--set-password` redefine a senha.
A senha vem de `FLUIGCLI_NEW_USER_PASSWORD` ou do prompt. A senha nunca vai por
argumento.

| Flag | Uso |
|---|---|
| `--email`, `--first-name`, `--last-name`, `--full-name` | novos valores (só os informados mudam) |
| `--set-password` | redefine a senha (env ou prompt) |

```sh
fluigcli user update jsilva --email joao.silva@empresa.com
fluigcli user update jsilva --set-password
```

## `fluigcli user activate <login>` / `deactivate <login>`

Estes comandos reativam ou desativam um usuário. **A API não exclui usuário.**
Por isso, desativar é o caminho. O usuário desativado fica no estado
**`BLOCKED`**. O usuário desativado sai das buscas. Para incluí-lo, use
`--inactive`.

```sh
fluigcli user deactivate jsilva
fluigcli user activate jsilva
```

Login inexistente → exit **4**. Em servidor `prod`, as operações de escrita
respeitam a trava de confirmação.

## `fluigcli user audit <login> [flags]`

Este comando reúne a **atuação de um usuário num período**. Ele serve para
acompanhamento e auditoria. Por exemplo, "o que o João fez em produção no dia
03/07/2026". O comando reúne três dimensões. Todas trazem **data e horário**:

- **Tarefas atuadas** — tarefas de workflow que o usuário **concluiu** no
  período. A saída traz a data de conclusão e a hora em que a tarefa chegou até
  ele.
- **Solicitações abertas** — solicitações que o usuário **iniciou** no período.
- **Documentos criados** — documentos que o usuário **criou no GED** no período.

Este comando é uma **consulta**. Por isso, ele não exige privilégio de
administrador, diferente do resto do grupo `user`. O comando resolve o login
para o `userCode` internamente. Login inexistente → exit **4**.

| Flag | Uso |
|---|---|
| `--day <dd/mm/aaaa \| aaaa-mm-dd>` | audita um único dia |
| `--from <data>` / `--to <data>` | intervalo (só `--from` ou só `--to` = um dia) |
| `--only tasks,requests,documents` | restringe as dimensões (default: todas) |
| `-o, --output <arquivo>` | salva a auditoria em arquivo `.txt` (texto puro) ou `.xlsx` (Excel) |

Sem `--day`, `--from` ou `--to`, o comando audita **hoje**.

As tarefas saem **em ordem de conclusão**. As solicitações saem **em ordem de
abertura**. Os documentos saem **em ordem de criação**. Todas as listas são
cronológicas. A coluna "Processo" mostra a **descrição**, não o nome técnico.
Nos documentos, a coluna "Tipo" traz o rótulo legível. Por exemplo, *Anexo de
processo*, *Registro de formulário*, *Arquivo* ou *Pasta*.

```sh
fluigcli user audit jsilva --day 03/07/2026 --server producao
fluigcli user audit jsilva --from 01/07/2026 --to 07/07/2026 --only tasks,documents
fluigcli user audit jsilva --day 2026-07-03 --json

# salvar em arquivo (formato pela extensão)
fluigcli user audit jsilva --day 03/07/2026 -o jsilva_03-07.xlsx --server producao
fluigcli user audit jsilva --day 03/07/2026 -o jsilva_03-07.txt  --server producao
```

O `.xlsx` sai com uma aba **Resumo** e uma aba por dimensão (Tarefas,
Solicitações, Documentos). O comando gera o arquivo com a biblioteca padrão.
Não há dependência externa.

Notas:

- A dimensão **documentos** consulta o dataset builtin `document`. O comando
  filtra pelo autor (`colleagueId`) e ordena por data de criação. O comando faz
  uma **parada antecipada** ao cruzar o início do período. Puxar todo o
  histórico de um autor de alto volume seria inviável. O comando filtra o
  `createDate` no cliente, porque esse campo não é pesquisável no dataset.
  Documentos com várias versões contam **uma vez**.
- **Documentos têm só a DATA de criação, sem horário.** O Fluig não expõe a hora
  de criação de documento. O `createDate` guarda só a data. O
  `GET /v2/documents/{id}` não devolve timestamp. Tarefas e solicitações têm
  data e hora normalmente.
- O comando **não** inclui login/logout nem rastreamento pelo log do servidor. O
  Fluig não tem dado estruturado de sessão. O `server.log` retém só poucos dias.
  Por isso, esse rastreamento é inviável para datas antigas.
