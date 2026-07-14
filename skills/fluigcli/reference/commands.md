# Mapa de comandos do fluigcli

Convenção que vale para todos os recursos:
**`import` = servidor → local · `export` = local → servidor.**

Consulte sempre `fluigcli <grupo> <sub> --help` para as flags exatas — abaixo é
o mapa mental, não a referência completa.

## server — servidores e sessão

| comando | efeito |
|---|---|
| `server add` | cadastra um servidor (metadados; senha vai para o keyring, nunca para arquivo); `--env dev\|hml\|prod` marca o ambiente |
| `server list` | lista os servidores cadastrados (projeto + global); `*` = padrão |
| `server use [<name>]` | define o servidor padrão, usado quando `--server` não é informado |
| `server update <name>` | altera o cadastro (ex.: `--env prod`) preservando a senha no keyring |
| `server remove <name>` | remove o servidor (e a senha do keyring) |
| `server test [<name>]` | login + ping + dados do usuário; reporta se a fluiggersWidget está instalada |
| `server logout [<name>]` | descarta a sessão em cache (ou de todos com `--all`) |
| `server install-helper [<name>]` | instala a widget auxiliar fluiggersWidget (pré-requisito de `workflow export` e `widget import`; o `widget list` tem fallback nativo) |

Resolução do servidor alvo: `--server`/`FLUIGCLI_SERVER` > padrão do projeto >
padrão global > único cadastrado. ⚠️ Em servidor com `env=prod`, comandos de
escrita (`export`, `delete`, `install-helper`) **exigem `--yes`** em modo
não-interativo (sem ele: exit 2).

## dataset

| comando | direção | efeito |
|---|---|---|
| `dataset list [--custom-only] [--search t]` | — | lista os datasets do servidor (id, tipo, descrição, ativo) |
| `dataset import <id>... \| --all` | servidor → local | baixa datasets para arquivos locais |
| `dataset export <file>...` | local → servidor | envia datasets locais |
| `dataset query <id>` | — | consulta os dados de um dataset (`--order` aceita um único campo; sufixo `_DESC`) |
| `dataset enable\|disable <id>...` | — | reativa/desativa datasets no servidor (não há API de exclusão; disable é o caminho) |
| `dataset history <id> [--version N]` | — | histórico de versões; `--version N` imprime o código JS daquela versão |
| `dataset restore <id> <version>` | — | restaura o código de uma versão do histórico (cria versão nova; exige `--yes` em modo não-interativo) |

## event — eventos globais

| comando | direção | efeito |
|---|---|---|
| `event list` | — | lista os eventos globais |
| `event import <id>... \| --all` | servidor → local | baixa eventos globais |
| `event export <file>...` | local → servidor | envia eventos globais |
| `event delete <id>...` | — | exclui eventos globais no servidor |

## mechanism — mecanismos de atribuição

| comando | direção | efeito |
|---|---|---|
| `mechanism list` | — | lista os mecanismos customizados |
| `mechanism import <id>... \| --all` | servidor → local | baixa mecanismos |
| `mechanism export <file>...` | local → servidor | envia mecanismos |
| `mechanism delete <id>...` | — | exclui mecanismos no servidor |

## form — formulários

| comando | direção | efeito |
|---|---|---|
| `form list` | — | lista os formulários |
| `form import <documentId\|nome>... \| --all` | servidor → local | baixa formulários para pastas locais (com anexos e eventos) |
| `form export <pasta>` | local → servidor | envia um formulário local (cria nova versão) |
| `form link --auto` | — | vincula pastas locais aos forms do servidor por nome (grava em .fluigcli/forms.json, por servidor); sem `--auto` é interativo (não use como agente) |

## workflow — scripts de eventos de processo

| comando | efeito |
|---|---|
| `workflow list [--active-only]` | lista os processos do servidor (nativo) |
| `workflow version <processId>` | mostra a última versão do processo (nativo) |
| `workflow import <processId>... \| --all` | baixa os scripts de eventos para workflow/scripts/ (servidor → local; sobrescreve no lugar; nativo) |
| `workflow export <arquivo\|processId>` | atualiza scripts na versão corrente, sem criar versão (via fluiggersWidget) |
| `workflow publish <processId> [--no-release]` | deploy nativo: cria versão nova com os scripts locais e a libera |

## request — solicitações de workflow (operação)

| comando | efeito |
|---|---|
| `request list [--process id] [--status s] [--sla s] [--assignee login] [--requester login] [--limit N]` | busca solicitações (status: open/canceled/finalized; sla: on_time/warning/expired; limit 0 = todas) |
| `request show <número>` | detalhe da solicitação + histórico de movimentação (`--json` traz request e tasks) |
| `request start <processId> [--fields-file arq.json\|-] [--field k=v]... [--attach arq] [--target-state N] [--assignee login] [--comment s] [--no-send]` | inicia solicitação; --fields-file = objeto JSON plano (use `-` para stdin — o modo natural para agentes; --field sobrepõe); com --attach usa SOAP (a REST não sobe anexo) e requer --target-state; throw de evento vira exit 5 com a mensagem |
| `request move <número> [--target-state N] [--fields-file arq.json\|-] [--field k=v]... [--comment s]` | conclui a tarefa corrente (descoberta sozinha) e envia adiante; tarefa de outro usuário = 404 |
| `request assignees <número> [--target-state N]` | possíveis responsáveis da próxima atividade |
| `request attachments <número> [--download] [--seq N] [--dir pasta]` | lista/baixa os anexos (o "(formulário)" da lista não é baixado; --seq inexistente = exit 4) |

## task — fila de tarefas (operação)

| comando | efeito |
|---|---|
| `task list [--assignee login \| --everyone] [--status s\|all] [--process id] [--requester login] [--sla s] [--limit N]` | sem flags = SUAS tarefas em aberto; status default not_completed |

## document — GED (operação)

| comando | efeito |
|---|---|
| `document list [<folderId>]` | sem arg = pastas raiz; com id = conteúdo (pastas/arquivos/artigos) |
| `document download <id>... [--dir pasta]` | baixa pelo id (nome vem dos metadados; byte a byte) |
| `document upload <file>... --folder <id>` | publica na pasta (upload + publish em uma etapa) |
| `document mkdir <parentId> <nome>` | cria pasta |
| `document delete <id>...` | envia para a lixeira (exige `--yes` em modo não-interativo) |

## widget

| comando | direção | efeito |
|---|---|---|
| `widget list` | — | lista os widgets do servidor (fluiggersWidget; sem ela usa a API nativa, que pode omitir itens) |
| `widget import <code>... \| --all` | servidor → local | baixa widgets para o projeto |
| `widget export <NomeWidget>` | local → servidor | empacota e publica um widget (deploy nativo) |

## diff — conferir antes de publicar

| comando | efeito |
|---|---|
| `diff` | compara datasets, eventos, mecanismos, formulários e scripts de processo locais com o servidor; aponta `only-server` |
| `diff <path>...` | compara só os arquivos (ou pastas de formulário) informados |

Read-only (não dispara a trava de produção). No `--json`, cada artefato vem com
`status` (`equal`\|`modified`\|`only-local`\|`only-server`) e o diff unificado.
Use antes de um `export` para saber o que mudaria. Em formulários, um arquivo
`only-server` seria **removido** por um `form export` da pasta; anexos binários
são comparados byte a byte (sem diff textual). Scripts de processo usam o
export nativo do processo — não requerem a fluiggersWidget.

## watch — publicar ao salvar (interativo)

| comando | efeito |
|---|---|
| `watch` | observa datasets/, events/, mechanisms/, forms/ e workflow/scripts/ e publica a cada salvamento |

Só roda em servidor `dev`/`hml` (prod e servidor sem env são recusados); nunca
cria artefato nem versão (forms sempre com a versão mantida); sem `--json` —
para automação, use os comandos `export`. Não é indicado para agentes:
prefira `diff` + `export`.

## dev — dev server com live reload (interativo)

| comando | efeito |
|---|---|
| `dev` | proxy local autenticado do portal: serve JS/CSS **e o markup do view.ftl** das widgets do disco (sem deploy), preview de formulários em `/_dev/forms/` — com painel de simulação de processo (executa o `events/displayFields.js` local com WKNumState/WKUser/formMode escolhidos; form vinculado por `form link` ganha as etapas reais pelo nome) — e recarrega o navegador ao salvar |

Só roda em servidor `dev`/`hml`; escuta em `127.0.0.1` por padrão (`--listen`
muda, com aviso — ex.: IP de tailnet em servidor remoto); sem `--json`.
Não publica nada no servidor. Não é indicado para agentes — é para o humano
ver o resultado no navegador; agentes usam `diff` + `export`.

## Utilitários

| comando | efeito |
|---|---|
| `version` | versão do fluigcli |
| `upgrade` | atualiza o próprio fluigcli para a última release (`--check` só consulta) |
| `completion <bash\|zsh\|fish\|powershell>` | script de autocompletar |
| `skill install \| show` | instala/mostra esta skill de agente |

## Receitas

**Publicar um dataset editado**
```sh
export FLUIGCLI_SERVER=homolog FLUIGCLI_PASSWORD="$SENHA"
fluigcli dataset export datasets/ds_clientes.js --json ; echo "exit=$?"
```

**Baixar um formulário inteiro (com anexos) para inspecionar**
```sh
fluigcli form import "Solicitação de Compras" --json --server homolog
```

**Iniciar uma solicitação com os campos em JSON (via stdin)**
```sh
echo '{"descricao":"Teclado novo","quantidade":"1"}' | \
  fluigcli request start compras_solicitacao --fields-file - --json ; echo "exit=$?"
```

**Atualizar o script de um evento de processo**
```sh
# requer a fluiggersWidget (exit 7 → server install-helper)
fluigcli workflow export workflow/MeuProcesso.beforeTaskSave.js --json --server homolog
```

**Sincronizar tudo do servidor para o local (leitura)**
```sh
for g in dataset event mechanism form widget ; do
  fluigcli "$g" import --all --json --server homolog || echo "falha em $g" >&2
done
```
