# fluigcli test — teste de processo por cenários

O comando testa um processo no motor BPM real. Ele usa a API do Fluig e não
abre navegador. Uma **suíte** descreve **cenários**. Cada cenário abre uma
solicitação, executa **passos** e confere o estado depois de cada passo.

Use o comando para repetir um roteiro de teste manual. Por exemplo: "abra a
solicitação, aprove na revisão, reprove na diretoria e confira que ela volta
para o solicitante". Escreva o roteiro uma vez. Depois rode a suíte a cada
mudança no processo.

```sh
fluigcli test run --server homologacao              # roda as suítes de tests/*/
fluigcli test run tests/Compras/aprovacao.json      # roda uma suíte
fluigcli test run tests/Compras/ --dry-run          # valida, sem abrir solicitação
fluigcli test run --scenario "aprovação simples"    # roda um cenário
```

> ⚠️ O comando **nunca** roda em servidor de produção. Um servidor com
> `--env prod` recusa o comando com exit 2. Não existe opção para liberar.

## O que o teste cobre

O teste de processo fica na camada do meio da pirâmide de testes:

| Camada | O que testa | Onde fica |
|---|---|---|
| Unidade | um script isolado | fora do `fluigcli` |
| **Processo** | **caminho no motor, gateways, eventos de processo, valores gravados no card** | **`fluigcli test`** |
| Interface | `displayFields`, `form.js`, campos bloqueados na tela | fora do `fluigcli` |

Os eventos de processo rodam no servidor normalmente. Os eventos de formulário
`inputFields` e `validateForm` também rodam no `start`, no `move` e no
`setFields`. O `displayFields` não roda, porque ele só desenha a tela.

## A pasta `tests/`

Guarde as suítes no projeto Fluig, junto do processo que elas testam:

```
tests/
└── Compras/
    ├── aprovacao.json        ← suíte
    └── fixtures/
        └── card_base.json    ← fixture (valores do formulário)
```

- Sem alvo, o comando roda todas as suítes de `tests/*/`.
- Com uma pasta, o comando lê os arquivos `*.json` da pasta, sem recursão. A
  subpasta `fixtures/` fica de fora.
- Com um arquivo, o comando roda só aquele arquivo.

## A suíte

O formato é **JSON**. As chaves ficam em inglês.

```json
{
  "process": "Compras",
  "server": "homologacao",
  "description": "Aprovação de compras acima do limite",
  "vars": { "limite": "5000,00" },
  "defaults": { "card": "fixtures/card_base.json", "cleanup": ["cancel"] },
  "scenarios": [
    {
      "name": "compra acima do limite vai para a diretoria",
      "fields": { "valorTotal": "{{var.limite}}" },
      "steps": [
        { "start": true, "expect": { "state": 5, "status": "OPEN" } },
        { "assume": true },
        { "move": { "target": 9, "fields": { "decisaoGerente": "aprovado" } },
          "expect": { "state": 12, "card": { "nivelAprovacao": "diretoria" } } }
      ]
    }
  ]
}
```

| Chave | Obrigatória | Descrição |
|---|---|---|
| `process` | sim | O `processId` no servidor. |
| `server` | não | O servidor alvo. A opção `--server` vence este valor. |
| `description` | não | Texto livre. |
| `vars` | não | Variáveis da suíte (texto). A opção `--var` vence este valor. |
| `defaults` | não | Valores herdados por todos os cenários: `card` e `cleanup`. |
| `scenarios` | sim | A lista de cenários. A suíte precisa de pelo menos um. |

Uma chave desconhecida em qualquer nível é erro (exit 2). A mensagem mostra o
caminho da chave. Por isso um erro de digitação, como `expcet`, não vira um teste
que passa sem conferir nada.

### O cenário

| Chave | Obrigatória | Descrição |
|---|---|---|
| `name` | sim | O nome do cenário. Ele é único na suíte e é o alvo do `--scenario`. |
| `description` | não | Texto livre. |
| `card` | não | O caminho da fixture, relativo ao arquivo da suíte. Herda de `defaults.card`. |
| `fields` | não | Valores que sobrescrevem a fixture. |
| `steps` | sim | Os passos, em ordem. O primeiro passo é sempre `start`. |
| `cleanup` | não | A limpeza do cenário. Herda de `defaults.cleanup`. O padrão é `["cancel"]`. |

Cada cenário abre a **própria** solicitação. Os cenários rodam em série.

### A fixture

A fixture é um arquivo JSON plano, igual ao `--fields-file` do `request start`:

```json
{
  "fornecedor": "Fornecedor Exemplo",
  "dataEmissao": "{{today}}",
  "numeroDocumento": "{{unique}}",
  "aprovador": "{{actors.eu.code}}",
  "itemValor___1": "100,00",
  "itemVencimento___1": "{{today(+10)}}"
}
```

Uma linha de tabela filha usa a chave `campo___N`. O card enviado no `start` é
a fixture, depois os `fields` do cenário e depois os `fields` do passo `start`.
Cada nível sobrescreve chave a chave.

> 💡 Monte a fixture a partir de uma solicitação real de homologação. Use
> `fluigcli form records show <formId> <cardId>` e troque os valores variáveis
> por expressões.

## Os passos

Cada passo tem **exatamente uma** chave de ação.

| Ação | O que faz | Forma curta |
|---|---|---|
| `start` | Abre a solicitação com o card do cenário. | `{ "start": true }` |
| `move` | Conclui a tarefa corrente e envia para a etapa `target`. | `{ "move": 110 }` |
| `assume` | Assume a tarefa de pool corrente para o usuário da sessão. | `{ "assume": true }` |
| `manager` | Conclui a tarefa em modo gestor (tarefa de outro usuário). | `{ "manager": 110 }` |
| `setFields` | Grava campos no card, sem movimentar. | — |
| `check` | Não age. Só confere o `expect`. | `{ "check": true }` |
| `wait` | Relê a solicitação até ela chegar numa etapa ou status. | — |
| `cancel` | Cancela a solicitação no meio do cenário. | `{ "cancel": true }` |

Chaves comuns a todo passo:

| Chave | Descrição |
|---|---|
| `name` | O rótulo do passo no relatório. Sem ele, o comando gera um rótulo (ex.: `move → 110`). |
| `expect` | As asserções depois do passo. |
| `id` | Um identificador para a expressão `{{steps.<id>.nextState}}`. |

### `start` e `move`

```json
{ "start": { "target": 5, "fields": { "urgente": "S" }, "comment": "aberto pelo teste" } }
{ "move": { "target": 9, "fields": { "decisao": "aprovado" }, "comment": "ok" } }
```

- `target` é a etapa de destino. Ela pode ser um gateway. Neste caso, a
  solicitação para na etapa que o gateway escolher.
- `fields` grava os campos junto com o movimento. Os eventos de formulário
  rodam com esses valores.
- `assignee` escolhe o responsável quando o servidor pede essa escolha.
- `movement` escolhe o movimento em ramos paralelos (só no `move`).
- Uma tarefa de pool sem dono recusa o `move`. Neste caso, use um passo
  `assume` antes.

### `manager`

```json
{ "manager": { "target": 13, "comment": "devolvido pelo teste" } }
```

O modo gestor **não aceita** `fields`. Para gravar campos, use um passo
`setFields` antes. O usuário da sessão precisa ser gestor do processo.

### `setFields`

```json
{ "setFields": { "decisao": "reprovado", "comentario": "falta a nota fiscal" } }
```

O passo lê o card inteiro, aplica os campos e grava o card inteiro. O
servidor roda o `validateForm` nessa gravação. Os eventos de processo não
rodam. Uso típico: preparar o card antes de um `manager`.

### `wait`

```json
{ "wait": { "state": 52, "status": "FINALIZED", "timeout": "3m", "interval": "5s" } }
```

As service tasks rodam **depois** da resposta do `move`. Por exemplo: o
`move` volta em 2 s, e a cadeia de integrações termina 30 s depois. Use um
`wait` antes de conferir a etapa que vem depois de uma service task.

- O passo espera **todas** as condições informadas (`state` e `status`).
- O padrão do `timeout` é `3m`, e o padrão do `interval` é `3s`.
- Um tempo esgotado falha o passo e mostra o último estado lido.

## As asserções (`expect`)

O passo passa só se **todas** as asserções passarem. O comando avalia todas as
asserções, mesmo depois da primeira falha. Por isso o relatório mostra todas as
diferenças de uma vez.

| Chave | Exemplo | Passa quando |
|---|---|---|
| `state` | `"state": 12` | A solicitação está na etapa 12. Com ramos paralelos, uma das etapas correntes é 12. |
| `stateName` | `"stateName": "Aprovação Diretoria"` | O nome da etapa corrente é igual. |
| `status` | `"status": "OPEN"` | O status é `OPEN`, `FINALIZED` ou `CANCELED`. |
| `error` | `"error": "obrigatório informar o motivo"` | O passo **falhou** e a mensagem contém o texto. |
| `card` | `"card": { "urgente": "S" }` | Cada campo cumpre o comparador. |
| `path` | `"path": { "visited": [5, 9], "notVisited": [7] }` | O histórico inclui `visited` e não inclui `notVisited`. Gateways e service tasks contam. |
| `assignee` | `"assignee": "Pool:Role:compras"` | O responsável da tarefa aberta é este login, código ou pool. |

### O erro esperado

```json
{ "move": { "target": 9, "fields": { "decisao": "reprovado" } },
  "expect": { "error": "obrigatório informar o motivo", "state": 5 } }
```

- O comando compara o texto por **trecho**, sem diferenciar maiúsculas. Antes,
  ele remove o HTML da mensagem do servidor.
- Forma longa: `"error": { "contains": "...", "code": "SERVER_ERROR" }` ou
  `"error": { "matches": "expressão regular" }`.
- Depois de um erro esperado, o comando confere que a solicitação **não mudou
  de etapa**. Se ela mudou, o passo falha com "o servidor rejeitou, mas a
  solicitação avançou".
- Um passo com `error` que o servidor aceita falha.

### Os comparadores de campo (`card`)

A forma curta `"campo": "valor"` é igualdade exata. A forma longa tem
exatamente um operador:

```json
"card": {
  "urgente": { "eq": "S" },
  "diasUteis": { "in": ["1", "2", "3"] },
  "decisaoFinanceiro": { "empty": true },
  "aprovadorNome": { "notEmpty": true },
  "comentario": { "contains": "teste" },
  "dataAprovacao": { "matches": "^\\d{2}/\\d{2}/\\d{4}" },
  "status": { "ne": "erro" }
}
```

- `empty` passa para campo ausente e para campo vazio.
- Um campo que não existe no card falha com "campo X não existe no card",
  exceto com `empty`. Isso pega erro de digitação.
- Uma linha de tabela filha usa `campo___N`.

## As expressões `{{...}}`

O comando resolve as expressões na hora do envio. Uma expressão desconhecida é
erro antes de conectar (exit 2).

| Expressão | Resultado |
|---|---|
| `{{today}}` | A data de hoje, `dd/MM/yyyy`. |
| `{{today(+N)}}` / `{{today(-N)}}` | Hoje mais ou menos N dias corridos. |
| `{{unique}}` | Um texto único por execução e cenário (ex.: `zztm3k9a1b21`). Use em campos com regra de duplicidade. |
| `{{var.<nome>}}` | Uma variável de `vars` ou da opção `--var`. |
| `{{actors.eu.login}}` | O login do usuário da sessão. |
| `{{actors.eu.code}}` | O código (userCode) do usuário da sessão. Use em campos de aprovador. |
| `{{requestId}}` | O número da solicitação do cenário. |
| `{{steps.<id>.nextState}}` | A etapa que o passo com aquele `id` devolveu. |

- "Hoje" é a data do **servidor** Fluig, não da sua máquina. O fuso vem de
  `--var tz=America/Sao_Paulo`. Sem essa variável, o comando usa o fuso local.
- Para escrever `{{` literal, use `{{{{`.
- `{{today(+N)}}` conta dias corridos. Uma regra de dias úteis precisa de uma
  data que fique longe de feriados. Neste caso, passe a data por `--var`.

## A limpeza

No fim de cada cenário, o comando roda a limpeza, mesmo depois de uma falha.

| Ação | Efeito |
|---|---|
| `"cancel"` | Cancela a solicitação se ela ainda estiver `OPEN`. O motivo gravado é `fluigcli test: <suíte> / <cenário>`. |
| `"none"` | Não limpa. |

- `--keep` desliga a limpeza de todos os cenários. O relatório lista as
  solicitações mantidas.
- Uma falha na limpeza não muda o resultado do cenário. O comando mostra um
  aviso com o número da solicitação que ficou aberta.
- Só o solicitante ou o gestor do processo cancela uma solicitação.

> ⚠️ O cancelamento **não desfaz** uma integração externa. Por exemplo: uma
> service task que gravou um lançamento no ERP. Pare o cenário antes da etapa
> de integração ou aceite o registro no ERP de homologação.

## Os atores

Nesta versão só existe o ator `eu`: o usuário da sessão do servidor. Use os
passos `assume` (tarefa de pool) e `manager` (tarefa de outro usuário) para
agir nas etapas de outras pessoas. O usuário da sessão precisa estar nos
papéis dos pools e ser gestor do processo.

## `--dry-run`

O `--dry-run` valida as suítes **sem abrir solicitação**. Ele lê o diagrama da
versão ativa do processo e confere:

- cada etapa de destino existe;
- cada `move` e `manager` usa uma transição que existe a partir da etapa do
  `expect.state` do passo anterior;
- cada `expect.state` existe;
- cada `assume` cai numa etapa de pool, e o usuário está no papel ou no grupo.

Um passo anterior sem `expect.state` impede a conferência da transição. Neste
caso, o comando mostra um aviso.

## A saída

```
Suíte: tests/Compras/aprovacao.json
Processo: Compras (versão 31) · servidor: homologacao
┌───┬─────────────────────────────────────────────┬───────────┬─────────────┬────────┐
│ # │ Cenário                                     │ Resultado │ Solicitação │ Tempo  │
├───┼─────────────────────────────────────────────┼───────────┼─────────────┼────────┤
│ 1 │ compra acima do limite vai para a diretoria │ passou    │ 231044      │ 12,4 s │
│ 2 │ compra abaixo do limite vai direto          │ FALHOU    │ 231045      │  9,8 s │
└───┴─────────────────────────────────────────────┴───────────┴─────────────┴────────┘
✗ 2 · compra abaixo do limite vai direto
  passo 3 (move → 9): falhou
    state: esperado 15, obtido 12; etapa atual: 12 ("Aprovação Diretoria")
    card.nivelAprovacao: esperado "gerente", obtido "diretoria"

Resumo: 1 passaram · 1 falharam · 0 com erro · 0 pulados (22,2 s)
```

- Durante a execução, o stderr mostra o progresso de cada cenário.
- O detalhe da falha mostra **esperado × obtido** de cada asserção.
- `--verbose` mostra também os passos aprovados e todas as asserções.

Com `--json`, o stdout recebe um único envelope. O `data` traz `server`,
`summary` (`passed`, `failed`, `errored`, `skipped`, `durationMs`) e `suites`.
Cada suíte traz os `scenarios`. Cada cenário traz `result`, `requestId`,
`cleanup` e `steps`. Cada passo traz `result` e as `assertions`, inclusive as
que passaram (`passed: true`).

Os resultados são `passed`, `failed`, `errored` (erro de autenticação, rede ou
tempo limite), `skipped` (passo depois de uma falha) e `validated` (no
`--dry-run`).

## As opções do `test run`

| Opção | Efeito |
|---|---|
| `--scenario <nome>` | Roda só os cenários com este nome. Aceita repetição. A comparação é exata. |
| `--dry-run` | Valida as suítes sem abrir solicitação. |
| `--keep` | Não roda a limpeza. |
| `--var <chave=valor>` | Define uma variável. Aceita repetição. Vence o `vars` do arquivo. |
| `--step-timeout <duração>` | O tempo máximo de cada passo. O padrão é `5m`. |

## Exit codes

| Situação | Exit | Código |
|---|---|---|
| Todos os cenários passaram | 0 | — |
| Pelo menos um cenário `failed` ou `errored` | 1 | `TEST_FAILED` |
| Suíte inválida, expressão desconhecida, servidor de produção, problema no `--dry-run` | 2 | `USAGE_ERROR` |
| Falha de autenticação antes do primeiro cenário | 3 | `AUTH_FAILED` |
| Suíte, fixture ou processo inexistente | 4 | `NOT_FOUND` |

Um erro de infraestrutura **durante** a execução não troca o exit code para 3
ou 5. O cenário fica `errored`, e o exit é 1. Com `--json`, o envelope de falha
traz o relatório completo em `data`.

## Limites conhecidos

- O `displayFields` não roda. Os campos que ele preenche na tela não chegam ao
  card.
- O modo gestor (`manager`) não grava campos. Use `setFields` antes.
- As service tasks rodam depois da resposta do `move`. Use `wait`.
- O cancelamento não desfaz integração externa.
- Nesta versão só existe o ator `eu`.
