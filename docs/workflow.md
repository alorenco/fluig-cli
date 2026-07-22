# fluigcli workflow — scripts de processo

O grupo `workflow` gerencia os scripts de processo. Ele lista os processos do
servidor. Ele consulta a versão de um processo. Ele baixa os scripts de eventos
para o projeto com o comando `import`. Ele faz o deploy cirúrgico dos scripts.
Este deploy não reimporta o processo inteiro. Os arquivos locais ficam em:

```
workflow/scripts/<Processo>.<evento>.js
# ex.: workflow/scripts/Compras.beforeTaskSave.js  → processId "Compras", evento "beforeTaskSave"
```

## `fluigcli workflow new-script <processId> <evento>`

Este comando cria `workflow/scripts/<processId>.<evento>.js` com a assinatura
correta do evento. O arquivo traz os parâmetros e um lembrete das APIs `hAPI` e
`getValue` disponíveis. Assim, você não copia de outro processo. O comando
valida o evento contra o catálogo. Ele aceita qualquer caixa e grava a forma
canônica. A opção `--help` lista todos os eventos com a assinatura e o momento
em que rodam. O comando trabalha só no projeto local. Publique depois com
`workflow export` (cirúrgico) ou `workflow publish` (nativo).

```sh
fluigcli workflow new-script Compras beforeTaskSave
fluigcli workflow new-script Compras validateAvailableStates
fluigcli workflow new-script --help    # catálogo completo de eventos
```

## `fluigcli workflow list [--active-only]`

Este comando lista os processos do servidor em tabela (ID, descrição,
categoria, ativo). O comando é nativo (REST v2 `process-management`). Ele não
depende de nada instalado.

```sh
fluigcli workflow list --server homolog
fluigcli workflow list --active-only          # só os processos ativos
fluigcli workflow list --json                 # para agentes/CI
```

A primeira coluna traz o `processId`. Os demais comandos usam esse valor. São
eles `workflow version`, `workflow import` e `workflow export`. A convenção de
arquivos também usa esse valor
(`workflow/scripts/<processId>.<evento>.js`).

## `fluigcli workflow version <processId>`

Este comando mostra a última versão do processo no servidor. O comando é nativo
(SOAP `ECMWorkflowEngineService`). Ele não depende de nada instalado.

```sh
fluigcli workflow version Compras --server homolog
```

Processo inexistente → exit **4**.

## `fluigcli workflow import <processId>... | --all`

Este comando baixa os scripts de eventos dos processos do servidor para
`workflow/scripts/<Processo>.<evento>.js` (servidor → local). Ele é o espelho
do `export`. O comando é nativo (export do processo via SOAP). Ele não depende
do componente auxiliar.

```sh
fluigcli workflow import Compras --server homolog        # um processo
fluigcli workflow import Compras Financeiro              # vários
fluigcli workflow import --all                           # todos os processos do servidor
```

| Flag | Uso |
|---|---|
| `--all` | importa os scripts de todos os processos do servidor |

Comportamento:

- O comando sobrescreve no lugar um script local existente do mesmo evento. Ele
  faz isso mesmo que o script esteja em subpasta de `workflow/scripts/`. Sem
  arquivo local, o comando cria o script em
  `workflow/scripts/<processId>.<evento>.js`.
- O comando traz os eventos da versão mais recente do processo. Eventos sem
  código (registro vazio no export) não viram arquivo.
- Processo inexistente → exit **4**. Em lote, falhas parciais → exit **6**. Nesse
  caso, o comando importa os demais processos normalmente.
- A opção `--all` faz um export por processo. Por isso, ela pode demorar em
  servidores com muitos processos.

## `fluigcli workflow export <arquivo|processId> [flags]`

Este comando atualiza os scripts de eventos de um processo. Ele não faz o
redeploy do processo todo.

> **Pré-requisito:** a atualização cirúrgica de scripts não tem API nativa no
> Fluig. Nem o SOAP nem a REST v2 oferecem essa operação. Ambos só reimportam o
> processo inteiro. Por isso, o comando usa o componente auxiliar
> **fluigcliHelper**. Sem o helper instalado, o comando falha com exit **7** e
> orienta: `fluigcli server install-helper`.

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

**Limitação:** o comando só atualiza eventos de um processo existente (criado no
Fluig Studio). Ele não cria processos. Ele não sobe diagramas `.process`. Para o
deploy com versão nova e liberação, use `workflow publish` (nativo).

## `fluigcli workflow publish <processId> [--no-release]`

Este comando faz o deploy do processo. Ele cria uma versão nova no servidor com
os scripts locais (`workflow/scripts/<processId>.*.js`) aplicados. Ele libera
essa versão para uso. O servidor desativa a versão anterior. O comando é nativo
(REST v2 `process-management`). Ele não depende do componente auxiliar.

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

- O publish não cria eventos nem processos. Um script local de um evento que não
  existe no processo interrompe o comando antes de qualquer mudança no servidor.
  Crie o evento no Fluig Studio. Eventos do servidor sem script local ficam como
  estão.
- A liberação pode falhar (por exemplo, um diagrama sem início ou fim). Neste
  caso, a versão nova fica criada em edição. A mensagem de erro avisa. Corrija no
  Fluig Studio ou repita com `--no-release`.
- O diagrama e as demais configurações da versão nova vêm do estado atual do
  servidor. O publish exporta a última versão, troca só os scripts e reimporta.

## `fluigcli server install-helper [<name>]`

Este comando instala o `fluigcliHelper` no servidor. O WAR vai embutido no
binário da CLI. O comando publica o WAR pelo upload nativo de widget. A
instalação é assíncrona no servidor.

```sh
fluigcli server install-helper homolog
fluigcli server install-helper homolog --war ./fluigcliHelper.war    # WAR custom
```
