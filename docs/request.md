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

Para formulários com muitos campos (ou para agentes de IA e CI), passe os
campos como um **objeto JSON plano** em vez de repetir `--field`. Valores
numéricos/booleanos são convertidos para a string que a API espera; objetos e
arrays aninhados são rejeitados com erro claro.

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

O `request move` aceita as mesmas flags (`--fields-file`/`--field`) para
atualizar campos do formulário no movimento:

```sh
echo '{"aprNivel1":"aprovado","comentarioNivel1":"ok"}' | \
  fluigcli request move 196542 --target-state 13 --fields-file -
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

## `fluigcli request attachments <número> [flags]`

Lista os anexos de uma solicitação e baixa os arquivos. O próprio
**formulário** aparece na lista como `(formulário)` — o `--download` baixa
apenas os arquivos anexados (round-trip byte a byte com o que subiu via
`request start --attach`).

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

Sequence inexistente → exit **4** (validado contra a lista antes de baixar).

## Status e SLA (valores da API)

- `status`: `OPEN` (em andamento), `CANCELED`, `FINALIZED`.
- `slaStatus`: `ON_TIME`, `WARNING` (perto do prazo), `EXPIRED` (estourado).
- Status de tarefa (no `show`): `NOT_COMPLETED` (em aberto),
  `PENDING_CONSENSUS`, `COMPLETED`, `TRANSFERRED`, `CANCELED`.

As flags aceitam os valores em minúsculas; a CLI valida antes de consultar.
