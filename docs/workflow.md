# fluigcli workflow — scripts de processo

Consulta a versão de um processo e faz o deploy **cirúrgico** dos scripts de
eventos (sem reimportar o processo inteiro). Arquivos locais em:

```
workflow/scripts/<Processo>.<evento>.js
# ex.: workflow/scripts/Compras.beforeTaskSave.js  → processId "Compras", evento "beforeTaskSave"
```

## `fluigcli workflow version <processId>`

Mostra a última versão do processo no servidor. **Nativo** (SOAP
`ECMWorkflowEngineService`) — não depende de nada instalado.

```sh
fluigcli workflow version Compras --server homolog
```

Processo inexistente → exit **4**.

## `fluigcli workflow export <arquivo|processId> [flags]`

Atualiza os scripts de eventos de um processo **sem redeploy do processo todo**.

> **Pré-requisito:** a atualização cirúrgica de scripts **não tem API nativa** no
> Fluig — nem no SOAP nem na REST v2 (ambos só reimportam o processo inteiro).
> Ela usa a widget auxiliar **fluiggersWidget**. Se ela não estiver instalada,
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
Studio). Não cria processos nem sobe diagramas `.process`. O deploy do processo
inteiro (estilo Fluig Studio) via API nativa é um item de roadmap.

## `fluigcli server install-helper [<name>]`

Instala a `fluiggersWidget` no servidor (baixa o WAR do repositório oficial da
widget e publica via upload nativo). A instalação é **assíncrona** no servidor.

```sh
fluigcli server install-helper homolog
fluigcli server install-helper homolog --war ./fluiggersWidget.war   # offline/custom
```
