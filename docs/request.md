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
fluigcli request list --assignee mjara --sla expired
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
preenche o formulário com os `--field`. Os eventos do processo e do formulário
rodam no servidor normalmente. Um `throw` de validação volta como mensagem de
erro (exit 5).

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
`--movement`, a CLI descobre a tarefa em aberto sozinha. Informe `--movement`
quando houver mais de uma tarefa. Flags: `--target-state`, `--assignee`,
`--comment`, `--field` (atualiza campos do formulário no movimento) e
`--movement`.

```sh
fluigcli request move 196542 --target-state 5 --comment "enviado via CLI"
fluigcli request move 196542 --target-state 13 --field aprNivel1=aprovado
```

⚠️ Você só movimenta **a sua** tarefa. A solicitação cuja tarefa aberta é de
outro usuário responde **404**. Neste caso, o servidor a esconde. Este é o
comportamento real.

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
