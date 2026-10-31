# Como escrever testes

Esta página ensina a escrever os testes de um processo Fluig com o
`fluigcli test`. Ela mostra o fluxo de trabalho, exemplos completos, receitas e
as armadilhas conhecidas. A referência de cada comando, chave e exit code está
em [test](./test).

Os exemplos usam o processo fictício **`compras`**:

| Etapa | Nome | Tipo |
|---|---|---|
| 4 | Início | início |
| 5 | Aprovar Gestor | tarefa (campo `gestor` do formulário) |
| 9 | Valor alto? | gateway: `valor > 10000` vai para 12; o resto vai para 15 |
| 12 | Aprovar Diretoria | tarefa (pool do papel `diretoria`) |
| 15 | Integração ERP | service task |
| 18 | Corrigir Integração | tarefa (erro da service task) |
| 7 | Revisar Solicitação | tarefa (o solicitante) |
| 20 | Fim | fim |

Os campos do formulário: `valor`, `fornecedor`, `gestor`, `decisaoGestor`,
`comentarioGestor`, `decisaoDiretoria`, `controleEnvio` (hidden) e a tabela filha
`itens` (`itemDescricao___N`, `itemValor___N`).

## 1. Escolha a camada

Cada camada prova uma coisa. Use a mais barata que prova o que você quer.

| Você quer provar que… | Camada | Comando |
|---|---|---|
| uma função de script calcula certo em todos os ramos | unidade | `test unit` |
| o `validateForm` recusa cada caso inválido | unidade | `test unit` |
| o gateway leva para a etapa certa com os dados do card | processo | `test run` |
| um evento de processo (`beforeTaskSave`) recusa ou grava | processo | `test run` |
| a solicitação percorre o caminho certo até o fim | processo | `test run` |
| a etapa mostra só os campos certos editáveis | interface | `test run` com passo `ui` |
| o `form.js` (`beforeSendValidate`, máscara) faz o que deve | interface | `test run` com passo `ui` |
| os testes passam por todas as transições do diagrama | cobertura | `test coverage` |

Regra prática:

- Um ramo de lógica custa milissegundos no `test unit` e uma solicitação real
  no `test run`. Por isso, teste os ramos no `test unit`.
- Teste no `test run` o que só o motor BPM faz: gateway, atribuição, eventos de
  processo, service task.
- Use o passo `ui` só onde a tela importa. Ele custa segundos por passo.

## 2. Siga o fluxo de trabalho

1. Gere o esqueleto da suíte a partir do diagrama:

   ```sh
   fluigcli test new compras --from-request 1201 --split-service integracao
   ```

   O comando grava `tests/compras/compras.json`,
   `tests/compras/integracao/compras.json` e `tests/compras/fixtures/card_base.json`.

2. Revise a fixture `fixtures/card_base.json`. Ela tem dados reais da
   solicitação 1201. Troque nomes, documentos e e-mails por valores fictícios
   antes de versionar.

3. Complete os `TODO` das `description`. Cada `TODO` diz o que falta: o campo
   que faz o gateway escolher o caminho, o campo que atribui a etapa, o
   mecanismo de atribuição.

4. Valide sem abrir solicitação:

   ```sh
   fluigcli test run tests/compras/ --dry-run
   ```

5. Rode na homologação:

   ```sh
   fluigcli test run tests/compras/ --server homologacao
   ```

6. Meça o que ficou sem teste:

   ```sh
   fluigcli test coverage tests/compras/ --server homologacao
   ```

7. Escreva os testes de unidade dos scripts com muitos ramos em `tests/unit/`:

   ```sh
   fluigcli test unit
   ```

8. Rode tudo de novo a cada mudança no processo, no formulário ou nos scripts.

> ⚠️ O `test run` nunca roda em produção. Os cenários da pasta `integracao/`
> passam pela service task e gravam no sistema externo. O cancelamento no fim
> NÃO desfaz essa gravação. Rode essa pasta só com o OK de quem cuida do sistema
> externo.

## 3. Exemplo completo: suíte de processo

Arquivo `tests/compras/aprovacao.json`:

```json
{
  "process": "compras",
  "description": "Aprovação de compras: gestor, diretoria e devolução",
  "defaults": { "card": "fixtures/card_base.json" },
  "scenarios": [
    {
      "name": "valor baixo: gestor aprova e vai para a integração",
      "fields": { "valor": "500,00", "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5, "status": "OPEN" } },
        { "move": { "target": 9, "fields": { "decisaoGestor": "aprovado" } },
          "expect": { "state": 15, "path": { "visited": [4, 5, 9], "notVisited": [12] } } }
      ]
    },
    {
      "name": "valor alto: passa pela diretoria",
      "fields": { "valor": "25.000,00", "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5 } },
        { "move": { "target": 9, "fields": { "decisaoGestor": "aprovado" } },
          "expect": { "state": 12, "assignee": "Pool:Role:diretoria" } },
        { "assume": true },
        { "move": { "target": 15, "fields": { "decisaoDiretoria": "aprovado" } },
          "expect": { "state": 15, "card": { "decisaoDiretoria": "aprovado" } } }
      ]
    },
    {
      "name": "gestor reprova sem comentário: bloqueado",
      "fields": { "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5 } },
        { "move": { "target": 7, "fields": { "decisaoGestor": "reprovado", "comentarioGestor": "" } },
          "expect": { "error": "informe o motivo da reprovação", "state": 5 } }
      ]
    },
    {
      "name": "gestor devolve ao solicitante",
      "fields": { "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5 } },
        { "move": { "target": 7, "fields": { "decisaoGestor": "reprovado", "comentarioGestor": "falta orçamento" } },
          "expect": { "state": 7, "card": { "comentarioGestor": { "contains": "orçamento" } } } }
      ]
    }
  ]
}
```

O que cada parte faz:

- `defaults.card` é a fixture: os valores do formulário no `start`.
- `fields` do cenário sobrescreve campos da fixture só neste cenário.
- `{{actors.eu.code}}` é o seu usuário. O cenário atribui a etapa 5 a você, e
  por isso o `move` seguinte funciona sem `assume`.
- `move` com destino num gateway (9) para na etapa real. O `expect.state` confere
  essa etapa.
- `path.visited` confere as etapas por onde a solicitação passou, inclusive
  gateways.
- O `expect.error` confere a recusa de um evento e também que a solicitação
  NÃO avançou.
- No fim de cada cenário, o comando cancela a solicitação aberta.

## 4. Exemplo completo: teste de unidade

O `test unit` roda uma função de script na sua máquina, sem servidor. Ele
precisa do Java 11 ou mais novo. Os objetos do Fluig (`form`, `hAPI`,
`DatasetFactory`, `getValue`) são falsos que o arquivo declara.

Arquivo `tests/unit/compras.unit.json`:

```json
{
  "description": "Compras: validação e cálculo de prazo",
  "scripts": ["forms/frm_compras/events/validateForm.js"],
  "defaults": {
    "today": "2026-11-03T10:00",
    "wk": { "WKNumState": 5, "WKNextState": 9 },
    "user": { "login": "jsilva", "code": "jsilva", "fullName": "João Silva" },
    "form": { "card": "fixtures/card_valido.json", "children": { "itens": [1] } }
  },
  "cases": [
    {
      "name": "card válido passa",
      "event": "validateForm",
      "expect": { "returnType": "undefined" }
    },
    {
      "name": "sem fornecedor: recusa",
      "event": "validateForm",
      "form": { "fields": { "fornecedor": "" } },
      "expect": { "throws": "Fornecedor" }
    },
    {
      "name": "reprovado sem comentário: recusa",
      "event": "validateForm",
      "form": { "fields": { "decisaoGestor": "reprovado", "comentarioGestor": "" } },
      "expect": { "throws": ["motivo da reprovação"] }
    },
    {
      "name": "na abertura não exige a decisão do gestor",
      "event": "validateForm",
      "wk": { "WKNumState": 4, "WKNextState": 5 },
      "form": { "fields": { "decisaoGestor": "" } },
      "expect": { "returnType": "undefined" }
    },
    {
      "name": "prazo: fornecedor com dias úteis do dataset",
      "scripts": ["forms/frm_compras/events/inputFields.js"],
      "call": "calcularPrazo",
      "args": ["$form", "$children:itens"],
      "datasets": {
        "ds_dias_uteis": [
          { "when": { "dataFinal": "10/11/2026" }, "columns": ["STATUS", "DIAS"], "rows": [["OK", "5"]] }
        ]
      },
      "form": { "fields": { "itemPrazo___1": "10/11/2026" } },
      "expect": {
        "return": { "urgente": false, "dias": "5" },
        "datasetCalls": [{ "name": "ds_dias_uteis", "constraints": { "dataFinal": "10/11/2026" } }]
      }
    },
    {
      "name": "prazo: dataset falha e o script registra o erro",
      "scripts": ["forms/frm_compras/events/inputFields.js"],
      "call": "calcularPrazo",
      "args": ["$form", "$children:itens"],
      "datasets": { "ds_dias_uteis": null },
      "form": { "fields": { "itemPrazo___1": "10/11/2026" } },
      "expect": {
        "return": { "urgente": true, "dias": "" },
        "log": [{ "level": "error", "contains": "dias úteis" }]
      }
    }
  ]
}
```

O que cada parte faz:

- `scripts` lista os scripts carregados em cada caso. Os caminhos são relativos à
  raiz do projeto. O caso pode trocar a lista.
- `event` chama o evento com os argumentos certos. `call` + `args` chama
  qualquer função.
- `"$form"` é o falso do formulário. `"$children:itens"` é o `Integer[]` da
  tabela filha, como o `getChildrenIndexes` devolve.
- `today` congela o `new Date()`. Sem ele, o teste depende do dia em que roda.
- `datasets` declara o retorno de cada dataset. Uma chamada sem falso termina o
  caso com erro "falso ausente". Assim, uma consulta esquecida não passa
  despercebida.
- `wk` aceita o tipo do servidor: número para `WKNumState`, `true`/`false` para
  `WKMobile`.

## 5. Exemplo completo: a tela da etapa

O passo `ui` abre a tarefa num navegador (Chrome, Chromium ou Edge) e confere a
tela. A API leva a solicitação até a etapa. O navegador abre só no passo `ui`.

Arquivo `tests/compras/tela.json`:

```json
{
  "process": "compras",
  "description": "Tela da Aprovar Diretoria",
  "defaults": { "card": "fixtures/card_base.json" },
  "scenarios": [
    {
      "name": "diretoria vê só a decisão editável e precisa de comentário",
      "fields": { "valor": "25.000,00", "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5 } },
        { "move": { "target": 9, "fields": { "decisaoGestor": "aprovado" } }, "expect": { "state": 12 } },
        { "assume": true },
        { "ui": { "open": "task" },
          "expect": {
            "visible": ["painelDiretoria"],
            "hidden": ["painelAnexos"],
            "enabled": ["decisaoDiretoria", "comentarioDiretoria"],
            "disabled": ["valor", "fornecedor", "itemValor___1", "decisaoGestor"],
            "value": { "valor": "25.000,00" },
            "text": { "resumoGestor": { "contains": "aprovado por" } }
          } },
        { "ui": { "fill": { "decisaoDiretoria": "reprovado", "comentarioDiretoria": "" }, "send": 7 },
          "expect": { "message": "informe o motivo", "changed": ["controleEnvio"], "state": 12 } },
        { "ui": { "fill": { "comentarioDiretoria": "fora do orçamento" }, "send": 7 },
          "expect": { "state": 7, "card": { "decisaoDiretoria": "reprovado" } } }
      ]
    },
    {
      "name": "a mesma tela no celular",
      "fields": { "valor": "25.000,00", "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true },
        { "move": 9, "expect": { "state": 12 } },
        { "assume": true },
        { "ui": { "open": "task", "viewport": "mobile" },
          "expect": { "visible": ["painelDiretoria"], "enabled": ["decisaoDiretoria"] } }
      ]
    }
  ]
}
```

O que cada parte faz:

- `open: "task"` abre a tarefa como você. Na etapa de pool, faça `assume` antes.
- `enabled` e `disabled` conferem o `displayFields`. Um campo bloqueado pelo
  `form.setEnabled(campo, false)` conta como `disabled`.
- `fill` + `send` preenche e envia pela tela. O `form.js` roda de verdade.
- `message` confere a recusa do envio. Ela pode vir do `beforeSendValidate` (no
  navegador) ou de um evento do servidor.
- `changed` confere que um campo mudou. No envio, o comando compara com o valor
  que foi ENVIADO ao servidor.
- Um passo `ui` que falha grava a tela em PNG. O relatório mostra o caminho.

## 6. Receitas

### Etapa de pool

A etapa atribuída a um papel ou grupo fica "no pool". Assuma antes de mover:

```json
{ "assume": true }
```

O `--dry-run` confere se você está no papel ou no grupo da etapa.

### Gateway

Mova para o gateway e confira a etapa onde a solicitação parou:

```json
{ "move": { "target": 9, "fields": { "valor": "25.000,00" } }, "expect": { "state": 12 } }
```

O gateway lê os valores que o formulário gravou. Um `hAPI.setCardValue` do
`beforeTaskSave` NÃO chega ao gateway no mesmo movimento. Calcule o campo no
`inputFields`.

### Service task

A service task roda DEPOIS da resposta do `move`. Espere o estado final:

```json
{ "wait": { "status": "FINALIZED", "timeout": "3m" } }
```

Ponha os cenários que passam por service task numa pasta separada
(`--split-service integracao` no `test new`).

### Erro esperado de um evento

```json
{ "move": { "target": 7, "fields": { "comentarioGestor": "" } }, "expect": { "error": "informe o motivo", "state": 5 } }
```

O texto é um trecho, sem caixa e sem HTML. O comando confere também que a
solicitação não avançou.

### Etapa de outra pessoa

Grave os campos e mova pelo modo gestor:

```json
{ "setFields": { "decisaoGestor": "aprovado" } },
{ "manager": { "target": 9, "comment": "fluigcli test" }, "expect": { "state": 15 } }
```

O `manager` não aceita `fields`. Use `setFields` antes.

### Datas relativas e valores únicos

```json
{ "fields": { "dataEntrega": "{{today(+10)}}", "fornecedor": "zz_teste {{unique}}" } }
```

`{{today}}` é a data do servidor. `{{unique}}` muda a cada cenário.

### Linha de tabela filha

Use o sufixo `___N` (três sublinhados):

```json
{ "fields": { "itemDescricao___1": "Papel", "itemValor___1": "10,00", "itemDescricao___2": "Caneta", "itemValor___2": "5,00" } }
```

## 7. Armadilhas conhecidas

Estas armadilhas foram medidas num servidor Fluig real.

| Armadilha | Consequência | O que fazer |
|---|---|---|
| Campo não enviado no `start` ou no `move` | o campo chega NULO aos eventos e pode quebrar com `TypeError` | mande o campo vazio (`""`) em vez de omitir |
| Etapa de pool sem `assume` | o `move` falha com `POOL_TASK_NOT_ASSIGNED` | ponha `{ "assume": true }` antes |
| Conferir a etapa logo depois de uma service task | a etapa ainda é a anterior | use `wait` |
| `hAPI.setCardValue` no `beforeTaskSave` para o gateway | o gateway não vê o valor no mesmo movimento | calcule no `inputFields` |
| `const` no corpo de um laço (script) | o valor fica o da 1ª volta, sem erro | use `let` (o `test unit` reproduz) |
| `const` com o mesmo nome em blocos irmãos | o script não compila | renomeie |
| `getValue("campo")` de dataset comparado com `===` | sempre `false` (é String Java) | use `==` ou `String(x) ===` |
| String Java vazia num `if (x)` | conta como verdadeira | compare com `== ""` |
| `dataset.values[0].coluna` | quebra no servidor | use `getValue(0, "coluna")` |
| Radio do Style Guide clicado no input | o clique é interceptado pelo label | o passo `ui` já clica no label |
| Cenário da pasta `integracao/` | grava no sistema externo e o cancelamento não desfaz | rode só com OK de quem cuida do sistema |

## 8. Como ler uma falha

Uma asserção que falha mostra o esperado e o obtido:

```
✗ 2 · valor alto: passa pela diretoria
  passo 2 (move → 9): falhou
    state: esperado 12, obtido 15
```

Leia assim: o gateway 9 mandou para a 15, e o teste esperava a 12. Confira o
campo que o gateway lê e o valor da fixture.

| Sinal | Causa provável |
|---|---|
| `state` diferente logo depois de um gateway | o campo da condição não tem o valor esperado |
| `POOL_TASK_NOT_ASSIGNED` | falta `assume` |
| `error`: "esperava erro, mas o passo foi aceito" | o evento não recusou: a regra mudou ou o campo do teste está certo demais |
| `unchanged`: "o servidor rejeitou, mas a solicitação avançou" | o evento gravou e depois recusou |
| passo `errored` com `TIMEOUT` | servidor lento ou service task longa: aumente `--step-timeout` ou o `timeout` do `wait` |
| `test unit`: "falso ausente" | o script chama um dataset ou uma global que o caso não declarou |
| `test unit`: "não compila no Rhino" | erro de sintaxe ou `const` em blocos irmãos; a mensagem traz a linha |
| passo `ui`: `disabled` com "o campo não existe na página" | nome do campo errado |

Com `--json`, o relatório completo vem em `data` (também na falha, com exit 1).

## 9. Lista de conferência

Antes de dar um teste por pronto, confira:

- [ ] A fixture não tem dados reais de pessoas.
- [ ] Cada cenário tem `expect` nos passos que importam. Um passo sem `expect` só
      confere que o servidor aceitou.
- [ ] Os cenários com service task estão na pasta `integracao/`.
- [ ] O `--dry-run` passa sem problema.
- [ ] Os ramos de lógica dos scripts estão no `test unit`, não no `test run`.
- [ ] Os testes de unidade usam `today` quando o script usa a data.
- [ ] O `test coverage` mostra as transições sem teste, e cada uma tem um motivo.
