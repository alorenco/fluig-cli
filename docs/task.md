# fluigcli task — tarefas de workflow

Este grupo lê a fila de tarefas. Ele lê as suas tarefas, as de outro usuário ou
as de todos. Ele é nativo. Ele usa a REST v2 `process-management`.

## `fluigcli task list [flags]`

Sem flags, este comando responde "**o que está comigo?**". Ele mostra as suas
tarefas em aberto.

```sh
fluigcli task list                          # minhas tarefas em aberto
fluigcli task list --assignee vanderli      # a fila de outro usuário
fluigcli task list --everyone --sla expired # tudo que está estourado, de todos
fluigcli task list --group TI               # paradas no pool do grupo TI
fluigcli task list --role controladoria     # paradas no pool do papel
fluigcli task list --process compras_solicitacao --status all
fluigcli task list --json                   # para agentes/CI
```

| Flag | Uso |
|---|---|
| `--assignee <login>` | responsável (default: **você**); aceita um código de pool (`Pool:Role:financeiro`) |
| `--everyone` | remove o filtro de responsável (todos os usuários) |
| `--group <código>` | tarefas paradas no pool do grupo — as que nenhum usuário assumiu |
| `--role <código>` | tarefas paradas no pool do papel — as que nenhum usuário assumiu |
| `--status s` | `not_completed` (default), `pending_consensus`, `completed`, `transferred`, `canceled` ou `all` |
| `--process <id>` | filtra pelo processo |
| `--requester <login>` | filtra pelo solicitante |
| `--sla s` | `on_time`, `warning` ou `expired` |
| `--limit N` | máximo de tarefas (default 50; 0 = todas) |

A tabela traz a solicitação, o processo, a etapa, o responsável, o solicitante,
o status (em aberto em verde), o SLA e o início. Use o número da coluna
Solicitação com o grupo `request` (`request show`, `request move`...).

### Tarefas paradas num grupo ou papel (`--group`/`--role`)

Uma atividade de workflow pode apontar para um grupo ou para um papel. A tarefa
fica "parada no pool" até um usuário assumir. Use `--group <código do grupo>`
ou `--role <código do papel>` para ver essas tarefas. O comando usa a mesma
consulta da central de tarefas do portal. Use `fluigcli role list` para
descobrir os códigos de papel.

O pool só tem tarefas em aberto e sem responsável. Por isso `--group` e
`--role` não combinam entre si nem com `--assignee`, `--everyone`,
`--requester`, `--sla` ou `--status`. O filtro `--process` funciona: a CLI
recorta o resultado pelo processo. O Fluig mostra apenas os pools que o seu
usuário enxerga na central de tarefas. Com `--group`/`--role`, a coluna Início
mostra o início da **solicitação**. Nos outros modos ela mostra o início da
tarefa.

Modo avançado: o `--assignee` também aceita um código completo de pool. Por
exemplo: `--assignee Pool:Role:financeiro`. Neste caso a busca é a REST v2.
Informe também o `--process`: sem ele, a busca por pool pode alcançar tarefas
órfãs de processo apagado, e o servidor responde com erro.

> ⚠️ Os **contadores** de tarefas (`/v2/tasks/count` e `/v2/tasks/resume`)
> ficaram fora. Essas rotas penduram a requisição no Fluig testado (Voyager
> 2.0.0) e chegaram a derrubar o servidor de homologação. A CLI vai reavaliá-las
> em versões futuras da plataforma.
