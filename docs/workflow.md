# fluigcli workflow — scripts de processo

O grupo `workflow` gerencia os scripts de processo. Ele lista os processos do
servidor. Ele consulta a versão de um processo. Ele baixa os scripts de eventos
para o projeto com o comando `import`. Ele faz o deploy cirúrgico dos scripts.
Este deploy não reimporta o processo inteiro. Ele compara o local com o
servidor (`diff`) e publica uma versão nova completa (`publish`). Os arquivos locais ficam em:

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

Processo inexistente → exit **4**. Quando existe um processId parecido, a
mensagem de erro o sugere. Por exemplo, um erro de digitação em "Compras"
recebe `talvez: "Compras"`.

## `fluigcli workflow versions <processId>`

Este comando lista todas as versões do processo em tabela: número, ativa
(publicada) e em edição. Ele mostra onde uma edição caiu e qual versão está no
ar. O comando é nativo (REST v2 `process-management`). Ele não depende de nada
instalado.

```sh
fluigcli workflow versions Compras --server homolog
fluigcli workflow versions Compras --json
```

Diferente do `version` (singular), que só imprime o número da última versão.
Combine com o `workflow import --version <n>` para ler os scripts de uma versão
específica.

## `fluigcli workflow import <processId>... | --all`

Este comando baixa os scripts de eventos dos processos do servidor para
`workflow/scripts/<Processo>.<evento>.js` (servidor → local). Ele é o espelho
do `export`. O comando é nativo (export do processo via SOAP). Ele não depende
do componente auxiliar.

```sh
fluigcli workflow import Compras --server homolog        # um processo
fluigcli workflow import Compras Financeiro              # vários
fluigcli workflow import --all                           # todos os processos do servidor
fluigcli workflow import Compras --events beforeTaskSave # só um evento
fluigcli workflow import Compras --stdout                # imprime, não grava
```

| Flag | Uso |
|---|---|
| `--all` | importa os scripts de todos os processos do servidor |
| `--events a,b` | importa só os eventos indicados |
| `--stdout` | imprime os scripts no terminal, sem gravar arquivo |
| `--version N` | lê os scripts de uma versão específica (default: a mais recente) |

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

A opção `--stdout` imprime os scripts no terminal e não toca no repositório. Use
essa opção para conferir o que está publicado sem sobrescrever os arquivos
locais. Com mais de um evento, o comando separa cada script com um cabeçalho
`// ==> <processId>.<evento>.js`. Com `--json`, os scripts vão no envelope
(`data.scripts[]`). Para comparar local e servidor, prefira o `workflow diff`.

```sh
# só o script publicado de um evento, para um arquivo separado
fluigcli workflow import "Adiantamento ao Fornecedor" --events servicetask88 --stdout > /tmp/publicado.js
```

A opção `--version` lê uma versão anterior, e não só a mais recente. A leitura da
versão usa o export XML daquela versão. Liste as versões com `workflow versions`.
A opção `--version` não combina com `--all` (o número de versão é por processo).

```sh
# confere um evento como estava na versão 143, sem tocar no repositório
fluigcli workflow import "Adiantamento ao Fornecedor" --version 143 --events servicetask88 --stdout
```

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
| `--process-id ID` | processId de destino no servidor, quando diferente do prefixo do arquivo local |

Por convenção, o prefixo do arquivo é o `processId` no servidor. Alguns
processos quebram essa convenção. Por exemplo, o arquivo é
`SolicitacaoAdiantamento.servicetask88.js`, mas o `processId` publicado é
`Adiantamento ao Fornecedor`. Use `--process-id` nesse caso. O alvo (arquivo ou
prefixo) continua a identificar os scripts locais. A flag troca apenas o
processo de destino no servidor.

```sh
fluigcli workflow export workflow/scripts/SolicitacaoAdiantamento.servicetask88.js \
    --process-id "Adiantamento ao Fornecedor" --server homolog
```

Quando o processo não é encontrado, a mensagem sugere os processIds mais
próximos e lembra da flag `--process-id`. Assim, o caso comum de o nome do
arquivo diferir do processId no servidor vira um conserto direto.

**Limitação:** o comando só atualiza eventos de um processo existente (criado no
Fluig Studio). Ele não cria processos. Ele não sobe diagramas `.process`. Para o
deploy com versão nova e liberação, use `workflow publish` (nativo).

### Checagem local antes de publicar

Antes de enviar, o comando audita os scripts com as regras do
[`audit`](audit.md). Um achado de nível **ERRO** em **qualquer** script aborta o
comando, com exit code 1. Nada é enviado ao servidor.

O aborto é total de propósito. O `publish` cria uma versão nova com todos os
scripts, e o `export` aplica todos os scripts do alvo. Publicar metade deixaria o
processo num estado que ninguém pediu. É a mesma regra que já valia para script
de evento inexistente.

Use `--no-audit` para pular a checagem.

## Iterar numa service task (reexecução)

Corrigir o script de uma service task não exige uma solicitação nova por
tentativa. O caminho de reexecução é o **fluxo de erro do próprio processo**:
a service task falha, a solicitação cai na **tarefa de correção** (em geral um
pool, ex.: Pool Grupo TI) e a transição dessa tarefa **volta para a service
task**. Movimentar a correção reexecuta o script.

O ciclo inteiro sai pela CLI:

```sh
# 1. corrija e publique o script
fluigcli workflow export workflow/scripts/meu_proc.servicetask7.js

# 2. assuma a tarefa de correção (é um pool — requer pertencer ao grupo/papel)
fluigcli task assume 230715

# 3. movimente de volta para a service task: o motor a reexecuta
fluigcli request move 230715 --target-state 7

# 4. acompanhe o resultado no log
fluigcli log tail --follow --grep "meu_proc" --for 2m
```

O número da etapa de destino (`--target-state`) é a sequence da service task
no diagrama. Consulte com `fluigcli audit --process <id>` (a sugestão lista as
etapas) ou no Explorador de Processos do `fluigcli dev`.

⚠️ Solicitação parada **na própria atividade automática** (service task em
execução ou pendurada, sem tarefa humana) não tem destrave pela CLI nem pela
API: o `request move` responde `NO_HUMAN_TASK`. Diagnostique pelo
`fluigcli log tail` e aguarde o motor — ou cancele com `request cancel` (se
você é o solicitante ou o gestor).

## Encoding: o que acontece com acentos e símbolos

O banco do Fluig guarda os scripts em colunas **CP-1252**. Na prática:

- acentos (`ç ã é`) e a pontuação tipográfica (`—`, `…`, `“aspas”`)
  **sobrevivem** ao export;
- caracteres fora do CP-1252 (`→`, `✓`, emoji) viram `?` **na gravação**,
  permanentemente — o `audit` avisa (regra `FL007`) antes do export;
- a leitura do `diff`/`import` usa o export UTF-8 da versão corrente, **fiel ao
  que está gravado**. (Versões antigas do fluigcli liam pelo zip SOAP, que o
  servidor serializa em ISO-8859-1 estrito — era isso que fazia um `—`
  publicado corretamente aparecer como `?` no diff, para sempre.)

## `fluigcli workflow diff <arquivo|processId> [flags]`

Este comando compara os scripts de eventos locais com o que está publicado no
servidor. Ele não altera nada. Ele é o companheiro do `export`/`publish`: confirma
se o que está no ar é igual ao local. A leitura usa o export nativo do processo.
Ele não depende do componente auxiliar.

```sh
# um evento (pelo arquivo)
fluigcli workflow diff workflow/scripts/Compras.beforeTaskSave.js --server homolog

# todos os eventos do processo
fluigcli workflow diff Compras --all-events --server homolog

# eventos selecionados
fluigcli workflow diff Compras --events beforeTaskSave,afterTaskComplete --server homolog
```

| Flag | Uso |
|---|---|
| `--all-events` | compara todos os `workflow/scripts/<processId>.*.js` |
| `--events a,b` | compara só os eventos indicados |
| `--process-id ID` | processId de destino no servidor, quando diferente do prefixo do arquivo local |

Os alvos são os mesmos do `export`. A flag `--process-id` tem o mesmo sentido:
o argumento identifica os scripts locais e a flag troca só o processo consultado
no servidor. Diferenças só de quebra de linha (CRLF/LF) não contam. Com `--json`,
o resultado sai no envelope (`data.artifacts[]` com `status` e `diff`, mais
`data.counts`).

## `fluigcli workflow publish <processId> [--no-release]`

Este comando faz o deploy do processo. Ele cria uma versão nova no servidor com
os scripts locais (`workflow/scripts/<processId>.*.js`) aplicados. Ele libera
essa versão para uso. O servidor desativa a versão anterior. O comando é nativo
(REST v2 `process-management`). Ele não depende do componente auxiliar.

O `publish` também audita os scripts antes de enviar. Um achado de nível ERRO
aborta o comando e nada é publicado. Ver
[Checagem local antes de publicar](#checagem-local-antes-de-publicar).

```sh
fluigcli workflow publish Compras --server homolog
fluigcli workflow publish Compras --no-release    # só cria a versão, sem liberar

# processId no servidor diferente do prefixo do arquivo local
fluigcli workflow publish SolicitacaoAdiantamento \
    --process-id "Adiantamento ao Fornecedor" --server homolog
```

| Flag | Uso |
|---|---|
| `--no-release` | cria a versão nova em modo de edição, sem liberá-la |
| `--process-id ID` | processId de destino no servidor, quando diferente do prefixo do arquivo local |

O argumento continua a identificar os scripts locais
(`workflow/scripts/<argumento>.*.js`). A flag `--process-id` troca apenas o
processo de destino no servidor.

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
