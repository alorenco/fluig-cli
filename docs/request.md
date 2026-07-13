# fluigcli request — solicitações de workflow

Consulta e acompanha solicitações (instâncias de processo) direto do terminal.
Primeiro grupo de **Operação** da CLI — uso da plataforma no dia a dia, não
deploy de artefatos. Tudo nativo (REST v2 `process-management`).

## `fluigcli request list [flags]`

Busca solicitações do servidor, das mais recentes para as mais antigas. A
tabela mostra número, processo, etapa atual (expandida da movimentação
corrente), status, SLA, solicitante e início; solicitações OPEN aparecem em
verde.

| Flag | Uso |
|---|---|
| `--process <id>` | filtra pelo processo (`processId` do `workflow list`) |
| `--status s` | `open`, `canceled` ou `finalized` |
| `--sla s` | `on_time`, `warning` ou `expired` |
| `--assignee <login>` | responsável atual pela tarefa |
| `--requester <login>` | solicitante |
| `--limit N` | máximo de solicitações (default 50; 0 = todas) |

```sh
fluigcli request list --process compras_solicitacao --status open
fluigcli request list --assignee mjara --sla expired
fluigcli request list --limit 0 --json          # todas, para agentes/CI
```

## `fluigcli request show <número>`

Mostra uma solicitação: processo/versão, status, solicitante, período, etapa
atual e a **tabela de movimentação** (histórico completo de tarefas, com
responsável, status e datas; a tarefa em aberto aparece em verde).

```sh
fluigcli request show 196522
fluigcli request show 196522 --json    # request + tasks estruturados
```

Solicitação inexistente → exit **4**.

## Status e SLA (valores da API)

- `status`: `OPEN` (em andamento), `CANCELED`, `FINALIZED`.
- `slaStatus`: `ON_TIME`, `WARNING` (perto do prazo), `EXPIRED` (estourado).
- Status de tarefa (no `show`): `NOT_COMPLETED` (em aberto),
  `PENDING_CONSENSUS`, `COMPLETED`, `TRANSFERRED`, `CANCELED`.

As flags aceitam os valores em minúsculas; a CLI valida antes de consultar.
