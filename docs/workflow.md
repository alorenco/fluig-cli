# fluigcli workflow — scripts de processo

Lista os processos do servidor, consulta a versão de um processo, baixa os
scripts de eventos para o projeto (`import`) e faz o deploy **cirúrgico** dos
scripts (sem reimportar o processo inteiro). Arquivos locais em:

```
workflow/scripts/<Processo>.<evento>.js
# ex.: workflow/scripts/Compras.beforeTaskSave.js  → processId "Compras", evento "beforeTaskSave"
```

## `fluigcli workflow new-script <processId> <evento>`

Cria `workflow/scripts/<processId>.<evento>.js` com a **assinatura correta do
evento** (parâmetros e um lembrete das APIs `hAPI`/`getValue` disponíveis) —
sem copiar de outro processo. O evento é validado contra o catálogo (aceita
qualquer caixa e grava a forma canônica); o `--help` lista todos os eventos com
assinatura e quando rodam. **Só local** — publique depois com `workflow export`
(cirúrgico) ou `workflow publish` (nativo).

```sh
fluigcli workflow new-script Compras beforeTaskSave
fluigcli workflow new-script Compras validateAvailableStates
fluigcli workflow new-script --help    # catálogo completo de eventos
```

## `fluigcli workflow list [--active-only]`

Lista os processos do servidor em tabela (ID, descrição, categoria, ativo).
**Nativo** (REST v2 `process-management`) — não depende de nada instalado.

```sh
fluigcli workflow list --server homolog
fluigcli workflow list --active-only          # só os processos ativos
fluigcli workflow list --json                 # para agentes/CI
```

O `processId` da primeira coluna é o que os demais comandos (`workflow
version`, `workflow import`, `workflow export`) e a convenção de arquivos
(`workflow/scripts/<processId>.<evento>.js`) usam.

## `fluigcli workflow version <processId>`

Mostra a última versão do processo no servidor. **Nativo** (SOAP
`ECMWorkflowEngineService`) — não depende de nada instalado.

```sh
fluigcli workflow version Compras --server homolog
```

Processo inexistente → exit **4**.

## `fluigcli workflow import <processId>... | --all`

Baixa os scripts de eventos de processos do servidor para
`workflow/scripts/<Processo>.<evento>.js` (**servidor → local** — o espelho do
`export`). **Nativo** (export do processo via SOAP) — não depende do
componente auxiliar.

```sh
fluigcli workflow import Compras --server homolog        # um processo
fluigcli workflow import Compras Financeiro              # vários
fluigcli workflow import --all                           # todos os processos do servidor
```

| Flag | Uso |
|---|---|
| `--all` | importa os scripts de todos os processos do servidor |

Comportamento:

- Um script local existente do mesmo evento é **sobrescrito no lugar**, mesmo
  que esteja em subpasta de `workflow/scripts/`; sem arquivo local, o script é
  criado em `workflow/scripts/<processId>.<evento>.js`.
- Vêm os eventos da **versão mais recente** do processo; eventos sem código
  (registro vazio no export) não viram arquivo.
- Processo inexistente → exit **4**; em lote, falhas parciais → exit **6** (os
  demais processos são importados normalmente).
- `--all` faz um export por processo — pode demorar em servidores com muitos
  processos.

## `fluigcli workflow export <arquivo|processId> [flags]`

Atualiza os scripts de eventos de um processo **sem redeploy do processo todo**.

> **Pré-requisito:** a atualização cirúrgica de scripts **não tem API nativa** no
> Fluig — nem no SOAP nem na REST v2 (ambos só reimportam o processo inteiro).
> Ela usa o componente auxiliar **fluigcliHelper**. Se ele não estiver
> instalado,
> o comando falha com exit **7** orientando: `fluigcli server install-helper`.

Alvos:

```sh
# um evento específico (pelo arquivo)
fluigcli workflow export workflow/scripts/Compras.beforeTaskSave.js --server homolog

# todos os eventos do processo
fluigcli workflow export Compras --all-events --server homolog

# eventos selecionados
fluigcli workflow export Compras --events beforeTaskSave,afterTaskComplete --server homolog
```

| Flag | Uso |
|---|---|
| `--all-events` | envia todos os `workflow/scripts/<processId>.*.js` |
| `--events a,b` | envia só os eventos indicados |
| `--process-version N` | versão do processo (default: a última do servidor) |

**Limitação:** só atualiza eventos de um processo **existente** (criado no Fluig
Studio). Não cria processos nem sobe diagramas `.process`. Para o deploy com
versão nova e liberação, use `workflow publish` (nativo).

## `fluigcli workflow publish <processId> [--no-release]`

Faz o **deploy** do processo: cria uma **versão nova** no servidor com os
scripts locais (`workflow/scripts/<processId>.*.js`) aplicados e a **libera
para uso** (a versão anterior é desativada). **Nativo** (REST v2
`process-management`) — não depende do componente auxiliar.

```sh
fluigcli workflow publish Compras --server homolog
fluigcli workflow publish Compras --no-release    # só cria a versão, sem liberar
```

| Flag | Uso |
|---|---|
| `--no-release` | cria a versão nova em modo de edição, sem liberá-la |

Quando usar `publish` vs `export`:

| | `workflow export` | `workflow publish` |
|---|---|---|
| Versão do processo | mantém (cirúrgico) | **cria nova** (sempre) |
| Liberação | não mexe | libera a nova (salvo `--no-release`) |
| Dependência | componente auxiliar | nenhuma (API nativa) |
| Uso típico | iterar em desenvolvimento | deploy |

Regras e limitações:

- O publish **não cria eventos nem processos**: script local de um evento que
  não existe no processo interrompe o comando **antes** de qualquer mudança no
  servidor (crie o evento no Fluig Studio). Eventos do servidor sem script
  local ficam como estão.
- Se a liberação falhar (ex.: diagrama sem início/fim), **a versão nova fica
  criada em edição** — a mensagem de erro avisa; corrija no Fluig Studio ou
  repita com `--no-release`.
- O diagrama e as demais configurações da versão nova vêm do estado atual do
  servidor (o publish exporta a última versão, troca só os scripts e reimporta).

## `fluigcli server install-helper [<name>]`

Instala o `fluigcliHelper` no servidor (o WAR vai **embutido no binário** da
CLI e é publicado pelo upload nativo de widget). A instalação é **assíncrona**
no servidor.

```sh
fluigcli server install-helper homolog
fluigcli server install-helper homolog --war ./fluigcliHelper.war    # WAR custom
```
