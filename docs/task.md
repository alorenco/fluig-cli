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
fluigcli task list --process compras_solicitacao --status all
fluigcli task list --json                   # para agentes/CI
```

| Flag | Uso |
|---|---|
| `--assignee <login>` | responsável (default: **você**) |
| `--everyone` | remove o filtro de responsável (todos os usuários) |
| `--status s` | `not_completed` (default), `pending_consensus`, `completed`, `transferred`, `canceled` ou `all` |
| `--process <id>` | filtra pelo processo |
| `--requester <login>` | filtra pelo solicitante |
| `--sla s` | `on_time`, `warning` ou `expired` |
| `--limit N` | máximo de tarefas (default 50; 0 = todas) |

A tabela traz a solicitação, o processo, a etapa, o responsável, o solicitante,
o status (em aberto em verde), o SLA e o início. Use o número da coluna
Solicitação com o grupo `request` (`request show`, `request move`...).

> ⚠️ Os **contadores** de tarefas (`/v2/tasks/count` e `/v2/tasks/resume`)
> ficaram fora. Essas rotas penduram a requisição no Fluig testado (Voyager
> 2.0.0) e chegaram a derrubar o servidor de homologação. A CLI vai reavaliá-las
> em versões futuras da plataforma.
