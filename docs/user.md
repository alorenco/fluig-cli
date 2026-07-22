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

## `fluigcli user audit <login> [flags]`

Reúne a **atuação de um usuário num período** — útil para acompanhamento e
auditoria (ex.: "o que o Marlon fez em produção no dia 03/07/2026"). Reúne três
dimensões, todas com **data e horário**:

- **Tarefas atuadas** — tarefas de workflow que ele **concluiu** no período
  (data de conclusão + quando a tarefa chegou até ele).
- **Solicitações abertas** — solicitações que ele **iniciou** no período.
- **Documentos criados** — documentos que ele **criou no GED** no período.

Diferente do resto do grupo `user`, é uma **consulta** e não exige privilégio de
administrador. Resolve o login para o `userCode` internamente; login inexistente
→ exit **4**.

| Flag | Uso |
|---|---|
| `--day <dd/mm/aaaa \| aaaa-mm-dd>` | audita um único dia |
| `--from <data>` / `--to <data>` | intervalo (só `--from` ou só `--to` = um dia) |
| `--only tasks,requests,documents` | restringe as dimensões (default: todas) |
| `-o, --output <arquivo>` | salva a auditoria em arquivo `.txt` (texto puro) ou `.xlsx` (Excel) |

Sem `--day`/`--from`/`--to`, audita **hoje**.

As tarefas saem **em ordem de conclusão**, as solicitações **em ordem de
abertura** e os documentos **em ordem de criação** (todos cronológicos). A
coluna "Processo" mostra a **descrição** (não o nome técnico). Nos documentos, a
coluna "Tipo" traz o rótulo legível (ex.: *Anexo de processo*, *Registro de
formulário*, *Arquivo*, *Pasta*).

```sh
fluigcli user audit mjara --day 03/07/2026 --server producao
fluigcli user audit mjara --from 01/07/2026 --to 07/07/2026 --only tasks,documents
fluigcli user audit mjara --day 2026-07-03 --json

# salvar em arquivo (formato pela extensão)
fluigcli user audit mjara --day 03/07/2026 -o marlon_03-07.xlsx --server producao
fluigcli user audit mjara --day 03/07/2026 -o marlon_03-07.txt  --server producao
```

O `.xlsx` sai com uma aba **Resumo** + uma aba por dimensão (Tarefas,
Solicitações, Documentos). Nenhuma dependência externa — o arquivo é gerado com
a biblioteca padrão.

Notas:

- A dimensão **documentos** consulta o dataset builtin `document` filtrando pelo
  autor (`colleagueId`) e ordenando por data de criação, com **parada
  antecipada** ao cruzar o início do período — puxar todo o histórico de um autor
  de alto volume seria inviável. O `createDate` é filtrado no cliente (não é
  campo pesquisável no dataset). Documentos com várias versões contam **uma vez**.
- **Documentos têm só a DATA de criação, sem horário** — o Fluig não expõe a
  hora de criação de documento (o `createDate` é armazenado só como data, e o
  `GET /v2/documents/{id}` não devolve timestamp). Tarefas e solicitações têm
  data + hora normalmente.
- **Não** inclui login/logout nem rastreamento pelo log do servidor: não há dado
  estruturado de sessão no Fluig e o `server.log` só retém poucos dias — inviável
  para datas antigas.
