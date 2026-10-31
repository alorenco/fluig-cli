# fluigcli test — teste de processo e de unidade

O comando testa um processo no motor BPM real. Ele usa a API do Fluig e não
abre navegador. Uma **suíte** descreve **cenários**. Cada cenário abre uma
solicitação, executa **passos** e confere o estado depois de cada passo.

> Primeira vez? Comece pelo guia [Como escrever testes](./test-guia): ele
> mostra o fluxo de trabalho, exemplos completos e as armadilhas. Esta página é
> a referência de cada chave.

Use o comando para repetir um roteiro de teste manual. Por exemplo: "abra a
solicitação, aprove na revisão, reprove na diretoria e confira que ela volta
para o solicitante". Escreva o roteiro uma vez. Depois rode a suíte a cada
mudança no processo.

```sh
fluigcli test run --server homologacao              # roda as suítes de tests/*/
fluigcli test run tests/Compras/aprovacao.json      # roda uma suíte
fluigcli test run tests/Compras/ --dry-run          # valida, sem abrir solicitação
fluigcli test run --scenario "aprovação simples"    # roda um cenário
fluigcli test coverage tests/Compras/               # roda e mede a cobertura de caminhos
fluigcli test new Compras                           # gera o esqueleto de uma suíte
fluigcli test unit                                  # roda os testes de unidade de tests/unit/
```

> ⚠️ O `test run` **nunca** roda em servidor de produção. Um servidor com
> `--env prod` recusa o comando com exit 2. Não existe opção para liberar.

## O que o teste cobre

O teste de processo fica na camada do meio da pirâmide de testes:

| Camada | O que testa | Onde fica |
|---|---|---|
| Unidade | uma função de script, com falsos, sem servidor | `fluigcli test unit` ([abaixo](#teste-de-unidade-test-unit)) |
| **Processo** | **caminho no motor, gateways, eventos de processo, valores gravados no card** | **`fluigcli test`** |
| Interface | `displayFields`, `form.js`, campos bloqueados na tela | passo `ui` do `fluigcli test run` ([abaixo](#a-tela-da-tarefa-passo-ui)) |

Os eventos de processo rodam no servidor normalmente. Os eventos de formulário
`inputFields` e `validateForm` também rodam no `start`, no `move` e no
`setFields`. O `displayFields` não roda nos passos de API, porque ele só desenha
a tela. Para conferir a tela, use o passo `ui`.

## A pasta `tests/`

Guarde as suítes no projeto Fluig, junto do processo que elas testam:

```
tests/
└── Compras/
    ├── aprovacao.json        ← suíte
    └── fixtures/
        └── card_base.json    ← fixture (valores do formulário)
```

- Sem alvo, o comando roda todas as suítes de `tests/*/`. A pasta
  `tests/unit/` e os arquivos `*.unit.json` ficam de fora: eles são do
  `test unit`.
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
| `description` | Texto livre de orientação. Não muda a execução. O `test new` grava os TODO aqui. |

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
| `--browser <caminho>` | O navegador dos passos `ui`. |
| `--headed` | Mostra a janela do navegador (depuração, só no desktop). |
| `--ui-artifacts <pasta>` | A pasta dos screenshots de passo `ui` que falha. O padrão é uma pasta temporária. |

## Exit codes

| Situação | Exit | Código |
|---|---|---|
| Todos os cenários passaram | 0 | — |
| Pelo menos um cenário `failed` ou `errored` | 1 | `TEST_FAILED` |
| Suíte inválida, expressão desconhecida, servidor de produção, problema no `--dry-run` | 2 | `USAGE_ERROR` |
| Falha de autenticação antes do primeiro cenário | 3 | `AUTH_FAILED` |
| Suíte, fixture ou processo inexistente | 4 | `NOT_FOUND` |
| Suíte com passo `ui` e nenhum navegador na máquina | 7 | `BROWSER_NOT_FOUND` |

Um erro de infraestrutura **durante** a execução não troca o exit code para 3
ou 5. O cenário fica `errored`, e o exit é 1. Com `--json`, o envelope de falha
traz o relatório completo em `data`.

## A tela da tarefa (passo `ui`)

O passo `ui` abre a tarefa num navegador sem janela (headless) e confere o que
a tela mostra. Ele cobre o que a API não alcança:

- o `displayFields`: os campos editáveis e bloqueados, os painéis visíveis;
- o `form.js`: o `beforeSendValidate`, as máscaras e os campos de controle;
- os textos e as cores de painel;
- a largura de celular.

A API leva a solicitação até a etapa em segundos. O navegador abre só no
passo `ui`. A página é o render REAL da tarefa no servidor, não a simulação do
`fluigcli dev`.

```json
{ "assume": true },
{ "ui": { "open": "task" },
  "expect": {
    "visible": ["painelAprFinanceiro"],
    "hidden": ["painelTblAnexos"],
    "enabled": ["aprFinanceiro", "comentarioFinanceiro"],
    "disabled": ["codColigada", "parcelaValor___1", "aprDiretoria"],
    "text": { "aprovadoPorDiretoria": { "contains": "por " } },
    "css": { "painelAprFinanceiro": { "display": "block" } } } },
{ "ui": { "fill": { "aprFinanceiro": "reprovado", "comentarioFinanceiro": "" }, "send": 13 },
  "expect": { "message": "obrigatório incluir um COMENTÁRIO", "changed": ["controleEnvio"], "state": 112 } },
{ "ui": { "fill": { "comentarioFinanceiro": "sem verba" }, "send": 13 },
  "expect": { "state": 13, "changed": ["controleEnvio"] } }
```

### O navegador

O comando usa o Chrome, o Chromium ou o Edge instalado. Ele procura nesta
ordem: `--browser`, `$FLUIGCLI_BROWSER`, os nomes conhecidos no PATH, os
caminhos padrão do sistema e o Chromium do Playwright (`~/.cache/ms-playwright`).
A suíte com passo `ui` e sem navegador recusa com exit 7 (`BROWSER_NOT_FOUND`)
antes de abrir qualquer solicitação. A suíte sem passo `ui` não procura
navegador.

A sessão do `fluigcli` vale no navegador: o comando passa os cookies da sessão.
Por isso, o navegador não pede login nem 2FA. O comando também bloqueia a
pesquisa de satisfação (NPS) da TOTVS, porque o modal dela cobre a página.

No Linux, o navegador abre com `--no-sandbox`. O Ubuntu 23.10 e as versões
mais novas bloqueiam o sandbox do Chromium.

Uma janela atende todos os passos `ui` de um cenário e fecha no fim dele.

### As ações

Um passo `ui` faz uma ação. A única combinação é `fill` com `send` ou `save`.

| Ação | Efeito |
|---|---|
| `open: "task"` | Abre a tarefa corrente em modo de edição, como o usuário da sessão. Assuma a tarefa de pool antes (`assume`). |
| `open: "view"` | Abre os detalhes da solicitação (modo de visualização). |
| `fill: {campo: valor}` | Preenche os campos na ordem do arquivo: input, textarea, select (por valor), radio e checkbox (por valor). Dispara os eventos `input`, `change` e `blur`, então as máscaras e os eventos do `form.js` rodam. Um campo bloqueado recusa o passo. |
| `click: "elemento"` | Clica num elemento do formulário (botão de tabela filha, aba). |
| `send: <etapa>` | Clica em Enviar, escolhe a etapa (pela sequência) na janela "Movimentar Atividade" e confirma. |
| `save: true` | Clica em Salvar. |
| `viewport` | `"desktop"` (1366×900, o padrão), `"mobile"` (390×844, modo celular) ou `{ "w": 800, "h": 600 }`. Vale no passo ou em `defaults.ui.viewport`. |

Um elemento é um nome de campo, um id ou `css:<seletor>`. O nome de campo vence
o id.

### As asserções da tela

| Asserção | Confere |
|---|---|
| `visible` / `hidden` | O elemento aparece ou não. Um elemento ausente conta como oculto. |
| `enabled` | O campo existe, aparece e aceita edição. |
| `disabled` | O campo existe e não aceita edição: `disabled`, `readonly` ou bloqueado pelo `setEnabled(false)`. Um campo ausente reprova: um nome errado não passa como bloqueado. |
| `value` | O valor do campo na tela, com os comparadores do `card`. |
| `text` | O texto visível de um elemento, com os comparadores do `card`. |
| `css` | Propriedades de CSS calculadas (`getComputedStyle`). O espaço e a caixa não contam. |
| `message` | A mensagem de recusa do envio (`FLUIGC.message`), sem HTML e sem caixa. Vale para a recusa do `beforeSendValidate` (nada sai) e para a recusa do servidor (`validateForm`, `beforeTaskSave`). |
| `changed` / `unchanged` | Um campo (inclusive hidden) mudou ou não. O "antes" é a tela no início do passo. No envio, o "depois" é o valor ENVIADO ao servidor. |

As asserções da API (`state`, `status`, `card`, `path`, `assignee`) também
valem no passo `ui`. O comando lê a solicitação pela API depois da ação. O
`error` não vale no passo `ui`: use `message`.

Uma recusa do envio sem `message` no `expect` reprova o passo com o texto da
recusa.

Como o Fluig desenha um campo bloqueado pelo `setEnabled(false)` no modo de
edição: o valor vai num `input hidden` com o nome do campo, e a tela mostra uma
cópia `_campo` somente leitura. O comando entende os dois.

### O screenshot

Um passo `ui` que falha grava a tela em PNG. O relatório mostra o caminho
(`tela: ...`), e o `--json` traz `steps[].screenshot`. Escolha a pasta com
`--ui-artifacts`.

### O `--dry-run`

O `--dry-run` valida a forma do passo `ui` e confere a etapa do `send` contra o
diagrama. Ele não abre navegador.

## Cobertura de caminhos (`test coverage`)

O `test coverage` mostra quais **transições** do diagrama os testes
percorreram, e quais nenhum teste percorreu. É cobertura de caminhos do BPMN,
não de linhas de código.

```sh
fluigcli test coverage tests/Compras/                       # roda as suítes e mede
fluigcli test coverage --from-result resultado.json         # mede um test run --json anterior
fluigcli test coverage --process Compras --requests 1201,1202   # mede solicitações existentes
fluigcli test coverage tests/Compras/ --min 80              # falha abaixo de 80%
```

### Arestas e etapas

Uma **aresta** é uma seta do diagrama: origem → destino. Uma etapa pode ser
visitada por um caminho e mesmo assim ter uma saída que nenhum teste usou. Por
exemplo: todos os cenários passam pela etapa "Aprovação Diretoria", mas nenhum
segue a seta "Aprovação Diretoria → Revisar Solicitação". A cobertura de etapas
diz 100%, e a cobertura de arestas mostra a seta que falta. Por isso o
percentual principal conta **arestas**. O relatório também mostra as **etapas
humanas** que nenhum teste abriu.

O comando lê o caminho de cada solicitação no histórico do banco
(`HISTOR_PROCES`). Ele precisa do fluigcliHelper com a API de banco. Sem o
helper, o comando sai com exit 7.

### Fluxo principal e tratamento de erro

As arestas têm duas categorias:

- **`main`**: o fluxo principal.
- **`error`**: o tratamento de erro. São as setas que saem do evento de erro de
  uma service task e tudo que só é alcançável a partir dele (por exemplo, a
  tarefa "Corrigir Integração" e a volta dela para a service task).

O percentual principal conta só `main`. Motivo: um caminho de erro depende de
provocar falha numa integração. Ele quase nunca entra numa suíte rápida, e
somado ao percentual ele esconde o que importa. Use `--include-errors` para
somar as duas categorias.

> 💡 Um erro de service task não cria movimento no evento de erro. A
> solicitação vai direto da service task para a tarefa de correção. Por isso o
> relatório mostra essa aresta como "service task → tarefa de correção".

### As fontes

| Fonte | Como | Escreve no servidor? |
|---|---|---|
| executar | `<alvo>...` (como no `test run`) | sim: roda as suítes. Recusa produção. |
| resultado | `--from-result <arquivo.json>` (`-` lê do stdin) | não |
| lista | `--requests <n,n,...>` com `--process` | não |

- As fontes de leitura aceitam servidor de produção. Neste caso, o comando avisa
  no stderr. Medir os caminhos do uso real não escreve nada.
- O comando mede **um processo por vez**. Suítes de processos diferentes são
  erro de uso (exit 2).
- A referência é a versão **ativa** do processo, ou a versão de `--version`.
  Uma solicitação de outra versão conta, com um aviso. Uma transição que não
  existe no diagrama de referência vai para a lista `unknown` e fica fora do
  percentual.
- Uma solicitação inexistente gera um aviso. Se todas forem inexistentes, o
  comando sai com exit 4.

### A saída

```
Cobertura de Compras (versão 12) · servidor: homologacao
Fonte: 4 cenário(s) de tests/Compras/aprovacao.json (3 solicitações)

Fluxo principal: 6 de 9 arestas (66,7%) · etapas humanas: 3 de 4
┌──────────────────────┬──────────────────────┬─────────┬──────────────────────┐
│ Origem               │ Destino              │ Coberta │ Cenários             │
├──────────────────────┼──────────────────────┼─────────┼──────────────────────┤
│ 5 Aprovar Gerente    │ 7 Revisar Pedido     │ NÃO     │                      │
│ 9 Valor alto?        │ 12 Aprovar Diretoria │ NÃO     │                      │
│   condição: valor > 5000                      │         │                      │
│ 4 Início             │ 5 Aprovar Gerente    │ sim     │ 3 cenários           │
└──────────────────────┴──────────────────────┴─────────┴──────────────────────┘
Gateways:
  9 Valor alto?: 12 ✗  15 ✔
      → 12 quando valor > 5000
Etapas humanas nunca abertas: 12 Aprovar Diretoria
Tratamento de erro: 0 de 4 arestas (0,0%). Use --include-errors para somar.
```

- As arestas **não cobertas vêm primeiro**: a lista de trabalho fica no topo.
- A seção "Gateways" mostra cada saída com a condição completa.
- `--verbose` mostra também as arestas de erro.
- Na fonte "executar", o relatório do `test run` vem antes da cobertura.

Com `--json`, o `data` traz `process`, `version`, `source`, `summary` (`main`,
`error`, `states`, `headline`, `includeErrors`), `edges` (com `category`,
`conditions`, `covered` e `hits`), `gateways`, `uncoveredStates`, `unknown`,
`warnings` e, na fonte "executar", o relatório do `test run` em `run`.

### Exit codes do `test coverage`

| Situação | Exit | Código |
|---|---|---|
| Cobertura calculada (e acima do `--min`, se informado) | 0 | — |
| Algum cenário falhou (fonte "executar") | 1 | `TEST_FAILED` |
| Abaixo do `--min` | 1 | `COVERAGE_BELOW_MIN` |
| Fontes combinadas, `--process` faltando, processos misturados, `--min` fora de 0–100 | 2 | `USAGE_ERROR` |
| Processo ou versão inexistente; todas as solicitações inexistentes | 4 | `NOT_FOUND` |
| Helper sem a API de banco | 7 | `HELPER_NOT_INSTALLED` |

O `TEST_FAILED` vence o `COVERAGE_BELOW_MIN`.

## Gerar o esqueleto de uma suíte (`test new`)

O `test new` gera o ponto de partida de uma suíte a partir do diagrama da
versão ativa. Ele só lê o servidor.

```sh
fluigcli test new Compras                                       # grava tests/Compras/Compras.json
fluigcli test new Compras --split-service integracao            # separa os cenários com service task
fluigcli test new Compras --from-request 1201                   # grava também a fixture
fluigcli test new Compras --stdout > rascunho.json              # imprime em vez de gravar
```

O gerador escolhe caminhos que, juntos, percorrem **todas as arestas do fluxo
principal**. Ele usa poucos cenários e termina cada caminho no fim do processo
ou numa etapa humana.

- Um caminho que não precisa de service task para **antes** dela, numa etapa
  humana. A limpeza cancela a solicitação.
- Um caminho que precisa de service task segue até o fim. A `description` do
  cenário avisa: "pode gravar em sistema externo". Com
  `--split-service <subpasta>`, esses cenários vão para
  `<pasta>/<subpasta>/<processId>.json`.
- O mesmo diagrama gera sempre a mesma suíte.

O esqueleto **não** é uma suíte pronta. O JSON não tem comentário, por isso a
orientação vai na chave `description` do cenário e do passo. Um texto que
começa com `TODO:` pede ação:

- a condição de cada gateway: preencha os `fields` para cumprir a condição;
- a etapa atribuída por campo do formulário: preencha o campo com
  `{{actors.eu.code}}` na fixture, ou troque o passo por `setFields` + `manager`;
- a etapa com outro mecanismo: confira quem é o responsável.

O gerador já coloca o `assume` antes do `move` que sai de uma etapa de pool, e o
`wait` depois de um movimento que entra numa service task.

### A fixture (`--from-request`)

Com `--from-request N`, o comando grava `fixtures/card_base.json` com o card da
solicitação N. As linhas de tabela filha viram `campo___N`.

- A data de abertura da solicitação vira `{{today}}`.
- O seu código de usuário vira `{{actors.eu.code}}`, e o seu login vira
  `{{actors.eu.login}}`.
- Os campos de controle (`currentState`, `formMode` e os que começam com `WK`)
  ficam de fora.

> ⚠️ A fixture contém **dados reais** da solicitação. Revise o arquivo antes de
> versionar.

### Opções do `test new`

| Opção | Efeito |
|---|---|
| `--version <N>` | A versão do diagrama. O padrão é a versão ativa. |
| `--from-request <N>` | Grava também a fixture com o card da solicitação N. |
| `--out <pasta>` | A pasta de saída. O padrão é `tests/<processId>/`. |
| `--split-service <subpasta>` | Separa os cenários que passam por service task. |
| `--max-scenarios <N>` | O limite de cenários. O padrão é 30. |
| `--force` | Sobrescreve arquivos existentes. Sem ela, um arquivo existente é recusado (exit 2) e nada é gravado. |
| `--stdout` | Imprime a suíte no stdout. Não combina com `--from-request`, `--split-service` nem `--json`. |

Exit codes: 0 = gerado; 1 = falha ao gravar (`LOCAL_IO_ERROR`); 2 = arquivo
existente, opção incompatível ou solicitação de outro processo; 4 = processo,
versão ou solicitação inexistente.

## Teste de unidade (`test unit`)

O `test unit` roda uma função de script Fluig na sua máquina, sem servidor. O
`form`, o `hAPI`, o `DatasetFactory`, o `getValue`, o `fluigAPI` e o `log` são
**falsos**. O arquivo de teste declara o que cada falso devolve. Cada caso leva
milissegundos.

Use o `test unit` para a lógica com muitos ramos. Por exemplo: um
`validateForm` com dezenas de regras, ou um cálculo de prazo. No `test run`,
cada ramo custa uma solicitação real. No `test unit`, cada ramo custa um caso
no arquivo.

```sh
fluigcli test unit                                   # roda tests/unit/
fluigcli test unit tests/unit/compras.unit.json      # roda um arquivo
fluigcli test unit --filter fornecedor               # roda os casos com "fornecedor" no nome
fluigcli test unit --self-check                      # confere o motor
```

### O motor

O servidor Fluig roda os scripts no Rhino 1.7.15.1. O `test unit` usa o
**Rhino 1.7.15.1 oficial**, a mesma versão, em modo ES6. Por isso, os defeitos
conhecidos do Rhino do Fluig também aparecem no teste:

- O `const` no corpo de um laço guarda o valor da 1ª volta.
- O `const` com o mesmo nome em blocos irmãos não compila.
- A String Java vazia é verdadeira num `if`.
- O `===` entre uma String Java e um texto JS dá `false`.

O servidor usa uma compilação da TOTVS da mesma versão. Um dataset-canário
na homologação confirmou a configuração do servidor: ES6 (versão de linguagem
200), modo interpretado (`-opt -1`) e a WrapFactory padrão do Rhino. O
`test unit` usa a mesma configuração. As sondas do `--self-check` reproduzem o
comportamento medido no servidor.

O jar não vem dentro do `fluigcli`. Na 1ª execução, o comando baixa o
`rhino-1.7.15.1.jar` do Maven Central, confere o SHA-256 e guarda o jar no cache
local. As execuções seguintes não usam a rede. Para usar outro jar, informe
`--rhino-jar <arquivo>`. Neste caso, o comando não confere o hash. O jar
precisa ter o shell do Rhino (`org.mozilla.javascript.tools.shell.Main`).

O comando precisa do **Java 11** ou mais novo. Ele procura o Java nesta ordem:
`--java`, `$FLUIGCLI_JAVA`, `$JAVA_HOME/bin/java` e `java` no PATH. O fuso do
teste é `America/Sao_Paulo`.

### A pasta `tests/unit/`

```
tests/
└── unit/
    ├── compras.unit.json      ← arquivo de teste de unidade
    └── fixtures/
        └── card_vazio.json    ← fixture (valores do formulário)
```

- Sem alvo, o comando lê os `*.unit.json` de `tests/unit/` e das subpastas.
- Os caminhos de `scripts` são relativos à raiz do projeto.
- Os caminhos de fixture (`form.card`) são relativos ao arquivo de teste.

### O arquivo de teste

```json
{
  "description": "Pagamentos: urgência e validação",
  "scripts": ["forms/Pagamentos/events/inputFields.js"],
  "defaults": {
    "today": "2026-11-03T10:00",
    "wk": { "WKNumState": "23" },
    "user": { "login": "jsilva", "code": "jsilva", "fullName": "João Silva" },
    "form": { "children": { "tabelaParcelas": [1] } }
  },
  "cases": [
    {
      "name": "vence amanhã: urgente",
      "call": "calcularUrgencia",
      "args": ["$form", "$children:tabelaParcelas"],
      "form": { "fields": { "vencimento___1": "04/11/2026" } },
      "datasets": {
        "ds_dias_uteis": { "columns": ["STATUS", "DIAS"], "rows": [["OK", "1"]] }
      },
      "expect": { "return": { "urgente": true, "dias": "1" } }
    },
    {
      "name": "abertura sem fornecedor",
      "scripts": ["forms/Pagamentos/events/validateForm.js"],
      "event": "validateForm",
      "wk": { "WKNumState": "4", "WKNextState": "5" },
      "form": { "card": "fixtures/card_vazio.json" },
      "expect": { "throws": ["campos não foram preenchidos", "Fornecedor"] }
    }
  ]
}
```

O arquivo tem três chaves: `scripts` (os scripts carregados em cada caso),
`defaults` (os falsos comuns) e `cases`. O comando recusa chave desconhecida
com exit 2. Por isso, um erro de digitação não vira um caso que passa sem
conferir nada.

Cada caso roda num escopo novo. O comando carrega os scripts de novo em cada
caso. Por isso, um caso não vê o estado de outro. Os scripts de um caso dividem
o mesmo escopo, como os scripts de um processo no servidor.

### A chamada

Cada caso tem `call` ou `event`:

| Chave | Efeito |
|---|---|
| `call` + `args` | Chama a função `call` com os argumentos `args`. |
| `event: "validateForm"`, `"inputFields"`, `"enableFields"` | Chama o evento com `(form)`. |
| `event: "displayFields"` | Chama o evento com `(form, customHTML)`. |
| outro `event` + `eventArgs` | Chama o evento de processo com os argumentos `eventArgs`. Por exemplo: `beforeTaskSave` com `["jsilva", 5, []]`. |

Os textos especiais em `args` e em `eventArgs`:

| Texto | Valor |
|---|---|
| `"$form"` | O falso do `form`. |
| `"$hAPI"` | O falso do `hAPI`. |
| `"$customHTML"` | O falso do `customHTML`. |
| `"$children:<tabela>"` | O `Integer[]` da tabela filha, como o `getChildrenIndexes`. |
| `{ "$java": "texto" }` | Um `java.lang.String`. |
| `"$$texto"` | O texto `"$texto"`. |

Os outros valores JSON viram valores JS. Um texto em `args` vira texto JS.

### Os falsos

O caso sobrescreve os `defaults` campo a campo. As chaves `wk`, `datasets`,
`returns` e `form.fields` se mesclam chave a chave.

| Chave | Falso |
|---|---|
| `form.card` | A fixture do card: um JSON plano, no mesmo formato do `test new --from-request`. |
| `form.fields` | Os campos do card. Eles vencem a fixture. Linha de tabela filha: `campo___N`. |
| `form.children` | Os índices das tabelas filhas: `{ "tabela": [1, 2] }`. |
| `form.mode`, `form.mobile` | O retorno do `getFormMode()` (padrão `ADD`) e do `getMobile()`. |
| `wk` | As variáveis do `getValue("WK…")`. O tipo segue o JSON: texto, `true`/`false` ou número. Uma chave ausente devolve `null`. |
| `user` | O usuário do `fluigAPI.getUserService().getCurrent()`: `login`, `code`, `fullName`, `email`. |
| `today` | O relógio do `new Date()` e do `Date.now()`: `AAAA-MM-DD` ou `AAAA-MM-DDTHH:MM`. |
| `datasets` | O retorno do `DatasetFactory.getDataset`, por nome (veja abaixo). |
| `returns` | O retorno de outros métodos: `{ "hAPI.getActiveStates": [5] }`. |

Os falsos devolvem os mesmos tipos do servidor:

- O `form.getValue`, o `hAPI.getCardValue` e o `getValue` devolvem
  `java.lang.String`. Um campo ausente devolve `null`.
- No `wk`, `true`/`false` viram `java.lang.Boolean`, e um número inteiro vira
  `java.lang.Integer`. O `fluig.d.ts` declara como booleanas a `WKMobile`, a
  `WKManagerMode`, a `WKIsTransfer`, a `WKIsService` e a `WKPrivateDocument`.
  Informe essas variáveis com `true`/`false`, porque no Rhino `"false" ==
  false` é falso. Se uma delas vier como texto, o comando avisa. Atenção: um
  `java.lang.Boolean` com `false` é verdadeiro num `if (x)`, como a String
  Java vazia. O `x == false` funciona.
- O `getChildrenIndexes` devolve `Integer[]`.
- O `dataset.values` é um `Object[][]` Java, e cada célula é `java.lang.String`.
  O `values[0].coluna` e o `values[0]["coluna"]` quebram, como no servidor
  (regra `RHINO004` do `audit`). Use `getValue(linha, "coluna")`.
- O `getValue(linha, "coluna")` ignora a caixa do nome da coluna. Uma coluna
  inexistente devolve `null`. Uma linha fora do intervalo lança
  `IndexOutOfBoundsException`, como no servidor.
- O `wk.WKCompany` numérico vira `java.lang.Long`, como no servidor.
- O `hAPI` e o `form` têm os métodos do `fluig.d.ts`. Um método fora da lista
  dá `TypeError`, como no servidor. Os métodos sem falso próprio gravam a
  chamada (asserção `calls`) e devolvem `null` ou o valor de `returns`.
- O `hAPI.getCardValue` lê o mesmo card do `form`.

A fixture aceita `{{today}}`, `{{today(+N)}}` e `{{actors.eu.login}}` /
`{{actors.eu.code}}`. O "hoje" é o `today` do caso, e o "eu" é o `user`.

O comando **não** congela `java.util.Date`, `Calendar` nem `LocalDate`. Eles
usam o relógio real da máquina.

#### O falso de dataset

| Valor | `getDataset` devolve |
|---|---|
| `null` | `null` |
| `{ "columns": [...], "rows": [[...]] }` | um dataset com essas linhas |
| `[{ "when": {...}, ... }, ...]` | a 1ª regra cujo `when` casa com as constraints |
| `{ "null": true }` ou `{ "error": "texto" }` numa regra | `null`, ou lança o erro |

O `when` compara o valor inicial de cada constraint: `{ "dataFinal":
"05/11/2026" }`. Um dataset sem falso, ou sem regra que case, faz o caso
terminar com erro: "falso ausente". Assim, uma chamada esquecida não passa
despercebida.

### As asserções (`expect`)

O `expect` é obrigatório. O comando confere todas as asserções do caso, mesmo
depois da 1ª falha.

| Chave | Confere |
|---|---|
| `return` | O valor devolvido, em JSON. A comparação é profunda. Número e texto são diferentes: `1` ≠ `"1"`. |
| `returnType` | O tipo devolvido: `js-string`, `java-string`, `number`, `boolean`, `null`, `undefined`, `array`, `java-array`, `object`, `java-object`, `date`. |
| `throws` | A função lançou erro. Um texto ou uma lista de textos que a mensagem contém, sem HTML e sem caixa. `true` = qualquer erro. |
| `form` | Os campos gravados pelo `form.setValue`. `"campo": null` exige que o campo NÃO tenha sido gravado. `"$only": true` exige que só os campos listados tenham sido gravados. |
| `card` | Os campos gravados pelo `hAPI.setCardValue`, com as mesmas regras do `form`. |
| `log` | As linhas de log: `[{ "level": "warn", "contains": "texto" }]`. |
| `datasetCalls` | Os datasets chamados, em ordem: `[{ "name": "ds", "constraints": { "campo": "valor" } }]`. `[]` exige nenhuma chamada. |
| `calls` | As chamadas a outros métodos: `[{ "method": "hAPI.setTaskComments", "args": [...] }]`. Sem `args`, basta a chamada. |
| `html` | Os textos que o `customHTML.append` recebeu. |

Sem `throws`, um erro na função reprova o caso. O relatório mostra a mensagem,
o script e a linha. Uma variável global sem falso dá a dica "a global X não
existe no teste".

Um script que não compila faz o caso terminar com erro. O relatório mostra a
mensagem do Rhino e a linha. Por exemplo: `TypeError: redeclaration of const y`.

### A conferência do motor (`--self-check`)

O `--self-check` não roda casos. Ele roda 21 sondas no motor local e compara com
o comportamento medido no servidor: 13 construções ES6 que funcionam, 4 que dão
`SyntaxError` e os 4 defeitos do Rhino. Use o `--self-check` depois de trocar o
Java ou de informar um `--rhino-jar`. Uma divergência dá exit 1.

### A saída

```
tests/unit/pagamentos.unit.json
Pagamentos: urgência e validação
┌───┬─────────────────────────┬───────────┬────────┐
│ # │ Caso                    │ Resultado │ Tempo  │
├───┼─────────────────────────┼───────────┼────────┤
│ 1 │ vence amanhã: urgente   │ FALHOU    │ 4,1 ms │
│ 2 │ abertura sem fornecedor │ passou    │ 3,6 ms │
└───┴─────────────────────────┴───────────┴────────┘
✗ 1 · vence amanhã: urgente
    return: esperado {"dias":"1","urgente":true}, obtido {"dias":"1","urgente":false}

Motor: Rhino 1.7.15.1 (oficial, do cache), ES6, Java 17.0.19
Resumo: 1 passaram · 1 falharam · 0 com erro (0,8 s)
```

Com `--json`, o `data` traz `engine` (versão, origem do jar, Java), `files[]`
com `cases[]` (`name`, `status` = `passed`/`failed`/`errored`, `durationMs`,
`error`, `assertions[]`, `logs[]`) e `summary`.

### Opções do `test unit`

| Opção | Efeito |
|---|---|
| `--filter <texto>` | Roda só os casos cujo nome contém o texto, sem caixa. Repetível. |
| `--java <caminho>` | O executável do Java. |
| `--rhino-jar <arquivo>` | O jar do motor. Dispensa o download e não confere o hash. |
| `--self-check` | Confere o motor e não roda casos. |
| `--timeout <duração>` | O limite da execução inteira. O padrão é 60 s. Um laço infinito vira erro nos casos sem resultado. |

### Exit codes do `test unit`

| Situação | Exit | Código |
|---|---|---|
| Todos os casos passaram | 0 | — |
| Um caso falhou ou terminou com erro; `--self-check` divergente | 1 | `TEST_FAILED` |
| Arquivo inválido, opção inválida, `--filter` sem caso | 2 | `USAGE_ERROR` |
| Arquivo de teste, script, fixture ou `tests/unit/` ausente | 4 | `NOT_FOUND` |
| Java ausente ou anterior ao 11 | 7 | `JAVA_NOT_FOUND` |
| Jar fora do cache e sem download, hash diferente ou jar sem o shell | 7 | `RHINO_UNAVAILABLE` |

## Limites conhecidos

- O `displayFields` não roda nos passos de API. Os campos que ele preenche na
  tela não chegam ao card. O passo `ui` confere a tela.
- O modo gestor (`manager`) não grava campos. Use `setFields` antes.
- As service tasks rodam depois da resposta do `move`. Use `wait`.
- O cancelamento não desfaz integração externa.
- Nesta versão só existe o ator `eu`.
- O `test coverage` não mediu ramo paralelo: a homologação usada no
  desenvolvimento não tem processo com gateway paralelo.
- O `test unit` não congela `java.util.Date`, `Calendar` nem `LocalDate`.
- O `test unit` não executa SQL nem consulta banco: um dataset é testado como
  função, com falsos.
- O passo `ui` não escolhe o responsável na janela de envio. Para uma etapa que
  pede o responsável, use o `move` da API com `assignee`.
- O passo `ui` não usa o zoom (`fluig-zoom`) com busca.
