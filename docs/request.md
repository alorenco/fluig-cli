# fluigcli request — solicitações de workflow

Consulta, inicia e movimenta solicitações (instâncias de processo) direto do
terminal. Primeiro grupo de **Operação** da CLI — uso da plataforma no dia a
dia, não deploy de artefatos. Nativo: REST v2 `process-management` (o start
com anexo usa o SOAP `startProcess`, pois a REST não tem upload de anexo).

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

## `fluigcli request start <processId> [flags]`

Inicia (abre e envia) uma solicitação, preenchendo o formulário com os
`--field`. Os eventos do processo e do formulário rodam no servidor
normalmente — um `throw` de validação volta como mensagem de erro (exit 5).

| Flag | Uso |
|---|---|
| `--field campo=valor` | campo do formulário (pode repetir) |
| `--attach <arquivo>` | anexa o arquivo à solicitação (pode repetir) |
| `--comment "..."` | comentário do movimento |
| `--target-state N` | etapa de destino (sequence); com `--attach`/`--no-send` informe-a |
| `--assignee <login>` | responsável pela próxima atividade (precisa ser apto pelo mecanismo) |
| `--no-send` | cria **sem enviar** — fica na atividade inicial, com você |

```sh
fluigcli request start compras_solicitacao --field descricao="Teclado novo" --comment "via CLI"
fluigcli request start compras_requisicao_abastecimento \
  --field codEquipamento=1084 --field quantidade=10 ... \
  --attach hodometro.png --target-state 5
```

⚠️ **Anexos**: a REST v2 não tem upload de anexo de solicitação (só download)
— processos que exigem anexo no início (ex.: `hAPI.listAttachments()` no
`beforeTaskSave`) **só** iniciam com `--attach` (a CLI troca para o SOAP
`startProcess` automaticamente). Se a atividade seguinte exigir escolha de
responsável (HTTP 412), a CLI lista as opções e pede `--assignee`.

## `fluigcli request move <número> [flags]`

Conclui a tarefa corrente e envia a solicitação adiante. Sem `--movement`, a
CLI descobre a tarefa em aberto sozinha (obrigatório quando houver mais de
uma). Flags: `--target-state`, `--assignee`, `--comment`, `--field` (atualiza
campos do formulário no movimento), `--movement`.

```sh
fluigcli request move 196542 --target-state 5 --comment "enviado via CLI"
fluigcli request move 196542 --target-state 13 --field aprNivel1=aprovado
```

⚠️ Você só movimenta **a sua** tarefa: solicitação cuja tarefa aberta é de
outro usuário responde **404** (o servidor a esconde — comportamento real).

## `fluigcli request assignees <número> [--target-state N]`

Lista quem pode assumir a próxima atividade. Quando o diagrama tem mais de um
destino, o servidor exige a etapa — informe `--target-state`.

## Status e SLA (valores da API)

- `status`: `OPEN` (em andamento), `CANCELED`, `FINALIZED`.
- `slaStatus`: `ON_TIME`, `WARNING` (perto do prazo), `EXPIRED` (estourado).
- Status de tarefa (no `show`): `NOT_COMPLETED` (em aberto),
  `PENDING_CONSENSUS`, `COMPLETED`, `TRANSFERRED`, `CANCELED`.

As flags aceitam os valores em minúsculas; a CLI valida antes de consultar.
