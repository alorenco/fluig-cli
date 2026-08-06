# fluigcli request — solicitações de workflow

O grupo `request` consulta, inicia e movimenta solicitações direto do terminal.
Uma solicitação é uma instância de processo. Este é o primeiro grupo de
**Operação** da CLI. Você usa a plataforma no dia a dia. Você não faz deploy de
artefatos aqui. Os comandos usam a REST v2 `process-management`. O start com
anexo usa o SOAP `startProcess`, pois a REST não tem upload de anexo.

## `fluigcli request list [flags]`

Este comando busca solicitações do servidor. Ele lista das mais recentes para
as mais antigas. A tabela mostra número, processo, etapa atual, status, SLA,
solicitante e início. O comando obtém a etapa atual da movimentação corrente.
As solicitações OPEN aparecem em verde.

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
fluigcli request list --assignee jsilva --sla expired
fluigcli request list --limit 0 --json          # todas, para agentes/CI
```

::: tip Compatibilidade Fluig 1.8 × 2.0
A CLI obtém a "etapa atual" de formas diferentes conforme a versão do servidor.
Ela detecta a versão por `/api/public/wcm/version`. No **Fluig 2.0+**, a etapa
vem do expand `currentMovements`. No **Fluig 1.8**, esse campo não existe na
API. Neste caso, a CLI usa o expand `activities` e considera a atividade ativa
(`active=true`). O resultado (`currentSteps` no `--json`) é idêntico nas duas
versões. Nada muda para quem consome o comando.
:::

## `fluigcli request show <número>`

Este comando mostra uma solicitação. Ele mostra processo/versão, status,
solicitante, período e etapa atual. Ele também mostra a **tabela de
movimentação**. Esta tabela é o histórico completo de tarefas. Ela traz
responsável, status e datas. A tarefa em aberto aparece em verde.

```sh
fluigcli request show 196522
fluigcli request show 196522 --json    # request + tasks estruturados
```

Solicitação inexistente → exit **4**.

## `fluigcli request start <processId> [flags]`

Este comando inicia uma solicitação. Ele abre e envia a solicitação. Ele
preenche o formulário com os `--field`. Os eventos do **processo** rodam no
servidor normalmente. Um `throw` de evento volta como mensagem de erro
(exit 5).

### ⚠️ Os eventos do formulário não rodam

A CLI grava o card pela API REST. Neste caminho, o servidor **não executa os
eventos do formulário**. O `displayFields` e o `validateForm` ficam de fora. O
`beforeSendValidate` é client-side e também não roda. Os eventos do processo,
esses sim, rodam.

O card recebe só os valores que você enviou. **Uma solicitação com os campos
obrigatórios vazios é aceita sem crítica.** Por isso, não use este comando para
testar a validação do formulário. O teste passa e você conclui, errado, que a
validação está correta. Para exercitar a validação, use o navegador ou o
`fluigcli dev`.

O mesmo vale para o `request move` e para o `form records create` e
`form records update` (ver [form](form.md)).

| Flag | Uso |
|---|---|
| `--field campo=valor` | campo do formulário (pode repetir; **sobrepõe** o `--fields-file`) |
| `--fields-file <arq \| ->` | campos em **JSON plano** `{"campo":"valor"}`; `-` lê do stdin |
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

### Campos por JSON (`--fields-file`)

Alguns formulários têm muitos campos. Nestes casos, passe os campos como um
**objeto JSON plano** em vez de repetir `--field`. Este formato também ajuda
agentes de IA e CI. A CLI converte valores numéricos e booleanos para a string
que a API espera. A CLI rejeita objetos e arrays aninhados com erro claro.

```sh
# 1. arquivo — bom para versionar a solicitação de teste no Git do projeto
cat > requisicao.json <<'EOF'
{
  "codColigada": 1,
  "codCCusto": "001.002.020.0001",
  "codEquipamento": 1084,
  "veiculoEquipamento": "(1084) FIAT UNO ECONOMY",
  "codMotorista": 206,
  "codPontoAbast": 2,
  "hodometro": 45210,
  "codMaterial": 3,
  "quantidade": 10
}
EOF
fluigcli request start compras_requisicao_abastecimento \
  --fields-file requisicao.json --attach hodometro.png --target-state 5

# 2. stdin — o modo natural para pipelines e agentes
echo '{"descricao":"Teclado novo","quantidade":"1"}' | \
  fluigcli request start compras_solicitacao --fields-file -

# 3. template + variação: o arquivo é a base e o --field sobrepõe um campo
fluigcli request start compras_requisicao_abastecimento \
  --fields-file requisicao.json --field quantidade=20 --attach h.png --target-state 5
```

### Linhas de tabela-filha (`campo___N`)

O JSON continua **plano** quando o formulário tem tabela-filha. Escreva o nome
do campo com o sufixo `___<linha>` (**três** underscores). O índice da linha
começa em `1`. Esta é a convenção do próprio Fluig. A API usa o mesmo sufixo
quando você **lê** o card (ver [form](form.md)).

```json
{
  "numeroNotificacaoAuto": "TESTE-001",
  "anxNotificacaoFileId___1": "1247035",
  "anxNotificacaoNome___1": "Notificação nº TESTE-001.pdf",
  "anxNotificacaoFileId___2": "1247099",
  "anxNotificacaoNome___2": "Anexo 2.pdf"
}
```

Cada índice cria uma linha. O exemplo acima cria duas linhas na tabela. Você não
informa o nome da tabela: o Fluig descobre a tabela pelo nome do campo.

Confira o resultado com `form records show <form> <cardId>`. As linhas saem
agrupadas por `tableId`, com o `rowId` que casa com o índice que você enviou.

Este é o caminho para testar um processo que exige anexo em tabela-filha.

O `request move` aceita as mesmas flags (`--fields-file`/`--field`). Use-as
para atualizar campos do formulário no movimento.

```sh
echo '{"aprNivel1":"aprovado","comentarioNivel1":"ok"}' | \
  fluigcli request move 196542 --target-state 13 --fields-file -
```

⚠️ **Anexos**: a REST v2 não tem upload de anexo de solicitação. Ela só faz
download. Alguns processos exigem anexo no início. Um exemplo é o
`hAPI.listAttachments()` no `beforeTaskSave`. Estes processos **só** iniciam
com `--attach`. Neste caso, a CLI troca para o SOAP `startProcess`
automaticamente. A atividade seguinte pode exigir a escolha de responsável
(HTTP 412). Neste caso, a CLI lista as opções e pede `--assignee`.

## `fluigcli request move <número> [flags]`

Este comando conclui a tarefa corrente e envia a solicitação adiante. Sem
`--movement`, a CLI descobre a tarefa em aberto sozinha. Flags:
`--target-state`, `--assignee`, `--comment`, `--field` (atualiza campos do
formulário no movimento) e `--movement`.

⚠️ Como no `request start`, os eventos do formulário não rodam neste caminho.
Os eventos do processo rodam.

```sh
fluigcli request move 196542 --target-state 5 --comment "enviado via CLI"
fluigcli request move 196542 --target-state 13 --field aprNivel1=aprovado
```

⚠️ Você só movimenta **a sua** tarefa. A solicitação cuja tarefa aberta é de
outro usuário responde **404**. Neste caso, o servidor a esconde. Este é o
comportamento real.

### Quando o 404 não quer dizer "não existe"

O servidor responde **404** no `move` em três situações diferentes. A CLI
consulta as tarefas da solicitação e devolve um código próprio para cada uma.
O exit code é **4** nos três casos.

| Código no envelope | Significado | O que fazer |
|---|---|---|
| `POOL_TASK_NOT_ASSIGNED` | a tarefa está num pool e ninguém a assumiu | assuma a tarefa no portal |
| `NO_HUMAN_TASK` | a etapa corrente é automática (service task) | aguarde o servidor ou veja o log do evento |
| `NOT_FOUND` | a solicitação não existe, ou a tarefa é de outro usuário | confira o número |

A mensagem traz a etapa e o nome do pool:

```json
{"code":"POOL_TASK_NOT_ASSIGNED",
 "message":"a tarefa corrente da solicitação 230702 (etapa 21, \"Acompanhar Retornos\")
            está no pool Sucesso do Cliente (Pool:Role:sucesso_cliente) e ninguém a assumiu;
            assuma a tarefa no portal antes de movimentar"}
```

A consulta extra roda **só** quando o move falha. O caminho de sucesso não
muda.

### Quando a CLI pede `--movement`

A etapa corrente pode ter mais de uma tarefa no **mesmo movimento**. Um exemplo
é a tarefa do pool mais a tarefa do usuário que a assumiu. Isto é comum: numa
medição em produção, 53 de 200 solicitações abertas estavam assim. Neste caso
não existe ambiguidade. A CLI segue com esse movimento sem perguntar.

A CLI pede `--movement` somente quando existem movimentos **diferentes** em
aberto, ou seja, atividades paralelas. Neste caso ela lista as opções com
responsável e status, e sai com exit 2.

```
┌───────────┬─────────────────────┬─────────────────────┬────────────────────────────┬─────────┐
│ Movimento │ Etapa               │ Responsável         │ Status                     │ SLA     │
├───────────┼─────────────────────┼─────────────────────┼────────────────────────────┼─────────┤
│ 15        │ Corrigir Integração │ João Silva (jsilva) │ TRANSFERRED/NOT_COMPLETED  │ ON_TIME │
│ 16        │ Aprovar Diretoria   │ Maria Souza         │ NOT_COMPLETED              │ EXPIRED │
└───────────┴─────────────────────┴─────────────────────┴────────────────────────────┴─────────┘
```

O responsável e o status vêm das tarefas da solicitação. Uma tarefa de pool não
tem responsável. Neste caso a CLI mostra `(pool, sem responsável)`. Com `--json`,
as mesmas opções vão no envelope em `data.options[]`, com
`{movement, stateName, assignee, status, slaStatus}`. Assim um agente escolhe o
movimento sem ler texto.

### Tempo limite no move e no start

O Fluig continua processando depois que a CLI para de esperar. Uma movimentação
que salva um formulário grande passa de um minuto. Em produção, um `move` levou
~80 s, estourou o tempo limite do cliente e **a movimentação aconteceu**. Por
isso:

- As operações de escrita usam **no mínimo 2 minutos** de tempo limite. O
  `--timeout` que você informar sempre vence, para mais ou para menos.
- No tempo limite estourado, o comando sai com **exit 5** e
  `error.code = "TIMEOUT"`. O código é próprio porque o resultado é
  **desconhecido**.
- O `move` **relê o estado da solicitação** e diz o que encontrou. O campo
  `data.outcome` do `--json` traz o veredicto: `moved` (a tarefa alvo saiu de
  aberto — não repita), `not_moved` (a tarefa continua aberta — repita com
  `--timeout` maior) ou `unknown` (a releitura também falhou).
- O `start` não tem número de solicitação para conferir. Ele devolve o comando
  de verificação em `data.checkCommand`.

⚠️ **Nunca repita uma escrita às cegas depois de um TIMEOUT.** Verifique o
estado primeiro. Repetir um `move` que já passou movimenta a solicitação duas
vezes.

```sh
fluigcli request move 196542 --movement 15 --timeout 5m
```

## `fluigcli request assignees <número> [--target-state N]`

Este comando lista quem pode assumir a próxima atividade. O diagrama pode ter
mais de um destino. Neste caso, o servidor exige a etapa. Informe
`--target-state`.

## `fluigcli request attachments <número> [flags]`

Este comando lista os anexos de uma solicitação e baixa os arquivos. O próprio
**formulário** aparece na lista como `(formulário)`. O `--download` baixa
apenas os arquivos anexados. O download é byte a byte fiel ao que subiu via
`request start --attach`.

| Flag | Uso |
|---|---|
| `--download` | baixa todos os arquivos anexados (o formulário fica de fora) |
| `--seq N` | baixa só o anexo com esse sequence |
| `--dir <pasta>` | diretório de destino (default: o atual) |

```sh
fluigcli request attachments 196540                       # lista
fluigcli request attachments 196540 --download --dir ./anexos
fluigcli request attachments 196540 --seq 2               # um específico
```

Sequence inexistente → exit **4**. A CLI valida o sequence contra a lista antes
de baixar.

## Status e SLA (valores da API)

- `status`: `OPEN` (em andamento), `CANCELED`, `FINALIZED`.
- `slaStatus`: `ON_TIME`, `WARNING` (perto do prazo), `EXPIRED` (estourado).
- Status de tarefa (no `show`): `NOT_COMPLETED` (em aberto),
  `PENDING_CONSENSUS`, `COMPLETED`, `TRANSFERRED`, `CANCELED`.

As flags aceitam os valores em minúsculas. A CLI valida os valores antes de
consultar.
