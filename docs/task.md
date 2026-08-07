# fluigcli task — tarefas de workflow

Este grupo trabalha a fila de tarefas. Ele lê as suas tarefas, as de outro
usuário ou as de todos — e **assume para você** uma tarefa parada num pool.
Ele usa a REST v2 `process-management` e, no `assume`, o SOAP nativo.

| Comando | O que faz |
|---|---|
| `task list` | lista tarefas (as suas, de outro usuário, de todos ou de um pool) |
| `task summary` | resumo da central de tarefas: contadores e pools visíveis |
| `task assume` | assume para você a tarefa de pool de uma solicitação |

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

## `fluigcli task summary [flags]`

Este comando mostra o resumo da central de tarefas — a mesma visão do painel do
portal. Ele responde "**quanto tem em cada fila?**".

```sh
fluigcli task summary                 # o seu resumo
fluigcli task summary --user jsilva   # o resumo de outro usuário
fluigcli task summary --json          # para agentes/CI
```

| Flag | Uso |
|---|---|
| `--user <login>` | usuário do resumo (default: **você**) |

A tabela traz uma linha por categoria: tarefas a concluir, tarefas em pool
(grupo e papel), minhas solicitações e tarefas sob gerência. Cada pool aparece
como uma linha filha, com o código e a contagem. Use o código com `task list
--group` ou `--role` para abrir o pool.

O resumo usa a consulta da central de tarefas do portal. Os contadores da REST
v2 seguem fora (veja o aviso acima). O Fluig monta o resumo quando o usuário
abre a central no portal. Por isso a consulta de um usuário que nunca abriu a
central responde vazio. Isso não é erro: a CLI mostra uma mensagem e aponta o
`task list --assignee` como alternativa.

## `fluigcli task assume <número> [flags]`

Este comando assume, para você, a **tarefa de pool** de uma solicitação (Pool
Papel / Pool Grupo). Ele é o desbloqueio do teste de processo por CLI: uma
tarefa de pool sem responsável **não pode ser movimentada** — o `request move`
responde `POOL_TASK_NOT_ASSIGNED`. Depois do `assume`, o `request move` segue
normalmente.

```sh
fluigcli task assume 230702
# tarefa da solicitação 230702 (etapa "Acompanhar Retornos") assumida por Alessandro Lorençone (alorenco)
fluigcli request move 230702 --target-state 24
```

Você precisa **pertencer ao papel ou grupo do pool**. Fora dele, o servidor
recusa e a CLI repassa a mensagem (exit 5).

A CLI confirma o resultado: ela relê as tarefas da solicitação e mostra a
etapa e o novo responsável. Com `--json`, os mesmos dados saem em
`data.{movement, stateName, assignee}`.

| Flag | Uso |
|---|---|
| `--thread N` | ramo **paralelo** do diagrama (threadSequence); no fluxo único o padrão `0` serve |

⚠️ **Não existe devolução ao pool.** A plataforma não expõe API para devolver
uma tarefa assumida (o `releaseProcess` do SOAP, apesar do nome, **libera
versão de processo** — outra coisa). Assuma com critério. Para passar a tarefa
adiante, conclua com `request move` ou transfira pelo portal.

Situações que o servidor recusa (exit 5, com a mensagem dele):

- a tarefa em aberto já está com uma pessoa (inclusive você): "Tarefa não
  encontrada" — só tarefa **de pool sem dono** pode ser assumida;
- você não pertence ao papel/grupo do pool.
