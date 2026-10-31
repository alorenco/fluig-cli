# Testes de processo Fluig com `fluigcli test` — guia para agentes

Leia este arquivo antes de escrever ou rodar testes. Guia humano completo, com
mais exemplos: `docs/test-guia.md`. Referência de cada chave: `docs/test.md`.

## 1. Decisão: qual camada

| O que provar | Camada | Arquivo |
|---|---|---|
| ramos de uma função de script; cada regra do `validateForm` | unidade (`test unit`, sem servidor, ms por caso) | `tests/unit/*.unit.json` |
| gateway, atribuição, eventos de processo, service task, caminho | processo (`test run`, solicitação real) | `tests/<processId>/*.json` |
| campos editáveis/bloqueados, painéis, `form.js`, celular | interface (passo `ui` no `test run`) | mesma suíte do processo |
| transições do diagrama sem teste | cobertura (`test coverage`) | — |

Regra: cada ramo de lógica vai para o `test unit`. O `test run` só prova o que
o motor BPM faz. O passo `ui` só onde a tela importa (custa segundos).

## 2. Fluxo (sempre com `--json --non-interactive`)

```sh
fluigcli test new <processId> --from-request <N> --split-service integracao --json   # esqueleto + fixture
# 1) sanitize fixtures/card_base.json (dados REAIS da solicitação N)
# 2) complete os "TODO:" das description (campo do gateway, atribuição)
fluigcli test run tests/<processId>/ --dry-run --json      # valida contra o diagrama; não escreve
fluigcli test run tests/<processId>/ --json                # roda na homologação (recusa produção: exit 2)
fluigcli test coverage tests/<processId>/ --json           # arestas sem teste
fluigcli test unit --json                                  # tests/unit/ (Java 11+)
fluigcli test unit --self-check --json                     # confere o motor Rhino local
```

Decida pelo exit code: 0 passou; 1 `TEST_FAILED` (relatório em `data`); 2 suíte
inválida ou produção; 3 autenticação; 4 arquivo/processo/script ausente; 7
`HELPER_NOT_INSTALLED` (coverage) / `JAVA_NOT_FOUND` / `RHINO_UNAVAILABLE`
(unit) / `BROWSER_NOT_FOUND` (suíte com passo `ui`).

NÃO rode `tests/<processId>/integracao/` sem OK humano: a service task grava no
sistema externo e o cancelamento não desfaz.

## 3. Modelo: suíte de processo

```json
{
  "process": "compras",
  "defaults": { "card": "fixtures/card_base.json" },
  "scenarios": [
    {
      "name": "valor alto passa pela diretoria",
      "fields": { "valor": "25.000,00", "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5, "status": "OPEN" } },
        { "move": { "target": 9, "fields": { "decisaoGestor": "aprovado" } },
          "expect": { "state": 12, "assignee": "Pool:Role:diretoria", "path": { "visited": [4, 5, 9] } } },
        { "assume": true },
        { "move": { "target": 15, "fields": { "decisaoDiretoria": "aprovado" } },
          "expect": { "state": 15, "card": { "decisaoDiretoria": "aprovado" } } },
        { "wait": { "status": "FINALIZED", "timeout": "3m" } }
      ]
    },
    {
      "name": "reprovar sem comentário é bloqueado",
      "fields": { "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true, "expect": { "state": 5 } },
        { "move": { "target": 7, "fields": { "decisaoGestor": "reprovado", "comentarioGestor": "" } },
          "expect": { "error": "informe o motivo", "state": 5 } }
      ]
    }
  ]
}
```

Passos: `start`, `move` (`target`; destino pode ser gateway), `assume` (pool),
`manager` (modo gestor; sem `fields` — use `setFields` antes), `setFields`
(card inteiro: lê, mescla, grava; roda o `validateForm`), `check`, `wait`
(`state`/`status`), `cancel`, `ui`.
Asserções: `state`, `stateName`, `status`, `error` (trecho; confere que NÃO
avançou), `card` (`eq`/`ne`/`in`/`empty`/`notEmpty`/`contains`/`matches`),
`path` (`visited`/`notVisited`), `assignee`.
Expressões: `{{today}}`, `{{today(+N)}}`, `{{unique}}`, `{{var.x}}` (`--var
x=v`), `{{actors.eu.login|code}}`, `{{requestId}}`, `{{steps.<id>.nextState}}`.

## 4. Modelo: teste de unidade

```json
{
  "scripts": ["forms/frm_compras/events/validateForm.js"],
  "defaults": {
    "today": "2026-11-03T10:00",
    "wk": { "WKNumState": 5, "WKNextState": 9, "WKMobile": false },
    "user": { "login": "jsilva", "code": "jsilva" },
    "form": { "card": "fixtures/card_valido.json", "children": { "itens": [1] } }
  },
  "cases": [
    { "name": "card válido passa", "event": "validateForm", "expect": { "returnType": "undefined" } },
    { "name": "sem fornecedor", "event": "validateForm",
      "form": { "fields": { "fornecedor": "" } }, "expect": { "throws": "Fornecedor" } },
    { "name": "prazo com dataset", "scripts": ["forms/frm_compras/events/inputFields.js"],
      "call": "calcularPrazo", "args": ["$form", "$children:itens"],
      "form": { "fields": { "itemPrazo___1": "10/11/2026" } },
      "datasets": { "ds_dias_uteis": [ { "when": { "dataFinal": "10/11/2026" }, "columns": ["STATUS", "DIAS"], "rows": [["OK", "5"]] } ] },
      "expect": { "return": { "urgente": false, "dias": "5" },
                  "datasetCalls": [ { "name": "ds_dias_uteis", "constraints": { "dataFinal": "10/11/2026" } } ] } }
  ]
}
```

- Chamada: `event` (`validateForm`/`inputFields`/`enableFields` = `(form)`;
  `displayFields` = `(form, customHTML)`; outro evento + `eventArgs`) OU `call` +
  `args`. Tokens: `"$form"`, `"$hAPI"`, `"$customHTML"`, `"$children:<tabela>"`,
  `{"$java": "x"}`.
- Falsos: `form.card`/`form.fields`/`form.children`, `wk` (tipo do JSON: inteiro
  = Integer, `true`/`false` = Boolean — use para `WKMobile`/`WKManagerMode`),
  `user`, `today`, `datasets` (`null` | `{columns,rows}` | regras com `when`),
  `returns` (`"hAPI.metodo"`).
- `expect` obrigatório: `return` (1 ≠ "1"), `returnType` (`js-string`,
  `java-string`, `undefined`…), `throws` (texto(s) ou `true`), `form`/`card`
  (`null` = não gravado; `"$only": true`), `log`, `datasetCalls` (`[]` =
  nenhuma), `calls`, `html`.
- Dataset ou global sem falso = caso com erro "falso ausente" (declare o falso).

## 5. Modelo: passo `ui` (tela real)

```json
{
  "process": "compras",
  "defaults": { "card": "fixtures/card_base.json" },
  "scenarios": [
    {
      "name": "tela da diretoria",
      "fields": { "valor": "25.000,00", "gestor": "{{actors.eu.code}}" },
      "steps": [
        { "start": true },
        { "move": 9, "expect": { "state": 12 } },
        { "assume": true },
        { "ui": { "open": "task" },
          "expect": { "enabled": ["decisaoDiretoria", "comentarioDiretoria"],
                      "disabled": ["valor", "fornecedor", "itemValor___1"],
                      "visible": ["painelDiretoria"], "hidden": ["painelAnexos"] } },
        { "ui": { "fill": { "decisaoDiretoria": "reprovado", "comentarioDiretoria": "" }, "send": 7 },
          "expect": { "message": "informe o motivo", "changed": ["controleEnvio"], "state": 12 } },
        { "ui": { "fill": { "comentarioDiretoria": "fora do orçamento" }, "send": 7 },
          "expect": { "state": 7 } }
      ]
    }
  ]
}
```

- Uma ação por passo: `open` (`task`|`view`), `fill`, `click`, `send: <sequência>`,
  `save`; só `fill` combina com `send`/`save`. `viewport`: `desktop`|`mobile`|`{w,h}`.
- `disabled` = existe e não edita (disabled, readonly ou bloqueado pelo
  `setEnabled`); campo ausente REPROVA. `message` = recusa do
  `beforeSendValidate` ou do servidor; sem `message`, a recusa reprova o passo.
  `changed` no envio compara com o valor ENVIADO. `error` não vale no `ui`.
- Falha grava screenshot: `data.suites[].scenarios[].steps[].screenshot`.

## 6. Armadilhas medidas (siga sempre)

1. Mande `""` em vez de omitir um campo: campo não enviado chega NULO aos eventos.
2. `assume` antes de mover etapa de pool (`POOL_TASK_NOT_ASSIGNED`).
3. `wait` depois de mover para service task: ela roda depois da resposta.
4. Gateway não vê `hAPI.setCardValue` do `beforeTaskSave` no mesmo movimento.
5. Script: `const` em corpo de laço congela o 1º valor; `const` homônimo em
   blocos irmãos não compila. Use `let`.
6. Script: String Java (`getValue`, `form.getValue`, dataset) com `===` dá
   `false`; vazia é verdadeira num `if`. Use `==` ou `String(x)`.
7. Script: `values[0].coluna` quebra no servidor; use `getValue(0, "coluna")`.
8. Fixture do `test new --from-request` tem dados reais: sanitize antes de versionar.
9. `manager` não aceita `fields`: use `setFields` antes.
10. Nunca rode `test run` em produção (a CLI recusa).

## 7. Ler a falha (`--json`)

- `data.suites[].scenarios[].result`: `passed`|`failed`|`errored`|`skipped`.
- Passo que falhou: `steps[]` com `result != "passed"`; `error`/`errorCode` e
  `assertions[]` com `passed: false` (`type`, `field`, `expected`, `actual`,
  `message`).
- `errored` = infraestrutura (sessão, rede, `TIMEOUT`): repita ou aumente
  `--step-timeout`. `failed` = o processo não fez o esperado: corrija o teste ou o
  processo.
- `test unit`: `data.files[].cases[]` com `status` e `assertions[]`; `error` traz
  "falso ausente", "não compila no Rhino (linha N)" ou a exceção com a linha.
