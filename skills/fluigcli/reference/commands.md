# Mapa de comandos do fluigcli

Convenção que vale para todos os recursos:
**`import` = servidor → local · `export` = local → servidor.**

Consulte sempre `fluigcli <grupo> <sub> --help` para as flags exatas — abaixo é
o mapa mental, não a referência completa.

## Por objetivo (índice rápido)

Comece por aqui: identifique a **intenção** e pule para o grupo certo.

| quero… | comando |
|---|---|
| criar um widget novo do zero (esqueleto no padrão oficial) | `widget new <code>` |
| criar um artefato novo do zero (esqueleto local) | `dataset new` · `form new` · `event new` · `mechanism new` · `workflow new-script` |
| publicar um artefato local (dataset/form/evento/mecanismo/widget/script) | `<grupo> export` |
| baixar do servidor p/ inspecionar ou editar | `<grupo> import` |
| ver o que **mudaria** antes de publicar | `diff` |
| consultar os dados de um dataset | `dataset query` |
| consultar / iniciar / movimentar solicitações | `request` |
| ver a fila de tarefas (a minha ou de outros) | `task list` |
| navegar / baixar / subir documentos (GED) | `document` |
| criar / editar registros de um formulário | `form records` |
| administrar usuários / grupos / papéis | `user` · `group` · `role` |
| definir substituto / delegar tarefas de um usuário | `replacement` |
| conferir acesso e saúde do servidor | `server test` · `server status` |

Direção dos verbos (o contrário de "git"): **`import` = servidor→local**,
**`export` = local→servidor**.

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
| `server status [<name>]` | saúde do servidor: uptime, memória, banco e monitores (requer admin) |
| `server install-helper [<name>]` | instala a widget auxiliar fluiggersWidget (pré-requisito de `workflow export` e `widget import`; o `widget list` tem fallback nativo) |

Resolução do servidor alvo: `--server`/`FLUIGCLI_SERVER` > padrão do projeto >
padrão global > único cadastrado. ⚠️ Em servidor com `env=prod`, comandos de
escrita (`export`, `delete`, `install-helper`) **exigem `--yes`** em modo
não-interativo (sem ele: exit 2).

## dataset

| comando | direção | efeito |
|---|---|---|
| `dataset new <name>` | local | cria `datasets/<name>.js` com o esqueleto de dataset customizado (nada vai ao servidor; publique com export --new) |
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
| `event new <name>` | local | cria `events/<name>.js` (o nome é o id do evento global; ajuste os parâmetros da função) |
| `event list` | — | lista os eventos globais |
| `event import <id>... \| --all` | servidor → local | baixa eventos globais |
| `event export <file>...` | local → servidor | envia eventos globais |
| `event delete <id>...` | — | exclui eventos globais no servidor |

## mechanism — mecanismos de atribuição

| comando | direção | efeito |
|---|---|---|
| `mechanism new <name>` | local | cria `mechanisms/<name>.js` com o esqueleto (devolver userCodes, não logins) |
| `mechanism list` | — | lista os mecanismos customizados |
| `mechanism import <id>... \| --all` | servidor → local | baixa mecanismos |
| `mechanism export <file>...` | local → servidor | envia mecanismos |
| `mechanism delete <id>...` | — | exclui mecanismos no servidor |

## form — formulários

| comando | direção | efeito |
|---|---|---|
| `form new <name> [--title t]` | local | cria `forms/<name>/` (HTML com `<form>` + events/ comuns, prontos para o preview do dev) |
| `form list` | — | lista os formulários |
| `form import <documentId\|nome>... \| --all` | servidor → local | baixa formulários para pastas locais (com anexos e eventos) |
| `form export <pasta>` | local → servidor | envia um formulário local (cria nova versão) |
| `form link --auto` | — | vincula pastas locais aos forms do servidor por nome (grava em .fluigcli/forms.json, por servidor); sem `--auto` é interativo (não use como agente) |
| `form records list <form> [--fields a,b] [--filter "campo eq 'v'"] [--limit N]` | — | registros (dados) do formulário; `--json` traz todos os campos |
| `form records show <form> <cardId>` | — | registro completo com linhas filhas |
| `form records create <form> --field k=v... \| --fields-file` | — | cria registro (eventos do form NÃO rodam) |
| `form records update <form> <cardId> --field k=v...` | — | atualiza (MESCLA campos; cria versão nova do registro) |
| `form records delete <form> <cardId>...` | — | exclui registros (exige `--yes` em modo não-interativo) |

## workflow — scripts de eventos de processo

| comando | efeito |
|---|---|
| `workflow new-script <processId> <evento>` | cria `workflow/scripts/<processId>.<evento>.js` com a assinatura correta do evento (catálogo no `--help`; local) |
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

## user — usuários da plataforma (administração; requer admin)

| comando | efeito |
|---|---|
| `user list [--search prefixoDeNome] [--role r] [--inactive] [--limit N]` | lista usuários (--search é PREFIXO de nome, não substring) |
| `user show <login>` | detalhe com papéis e grupos |
| `user create <login> --email --first-name --last-name [--code] [--full-name]` | cria usuário; senha via FLUIGCLI_NEW_USER_PASSWORD ou prompt (NUNCA por flag) |
| `user update <login> [--email] [--first-name] [--last-name] [--full-name] [--set-password]` | mescla os campos informados |
| `user activate\|deactivate <login>` | ativa/desativa (desativado = state BLOCKED; não há exclusão na API) |

## group — grupos da plataforma (administração; requer admin)

Grupo = `code` + `description` + `type` (`user` = administrado; `community` =
automático das comunidades, prefixos MODERATOR_/MEMBER_). Não há campo "nome".

| comando | efeito |
|---|---|
| `group list [--type user\|community] [--search texto] [--limit N]` | lista grupos (--type/--search filtram NO CLIENTE; o servidor ignora os params) |
| `group show <code>` | detalhe do grupo + membros |
| `group create <code> --description <texto> [--type user\|community]` | cria grupo (descrição obrigatória; type default user); duplicado = exit 5 |
| `group update <code> --description <texto>` | atualiza a descrição (PUT mescla) |
| `group delete <code>` | exclui o grupo |
| `group users <code>` | lista os usuários membros |
| `group add-user <code> <login>` | adiciona usuário (valida grupo+login antes; a API não valida) |
| `group remove-user <code> <login>` | remove usuário (não-membro = exit 4) |

## role — papéis da plataforma (administração; requer admin)

Papel = `code` + `description` (sem "type"). É a via para dar/tirar um papel de
um usuário. `user show` lista os papéis; `user list --role` filtra por papel.

| comando | efeito |
|---|---|
| `role list [--search texto] [--limit N]` | lista papéis (--search filtra NO CLIENTE) |
| `role show <code>` | detalhe do papel + usuários vinculados |
| `role create <code> [--description <texto>]` | cria papel (descrição opcional = o código); duplicado = exit 5 |
| `role update <code> --description <texto>` | atualiza a descrição (PUT mescla) |
| `role delete <code>` | exclui o papel |
| `role users <code>` | lista os usuários com o papel |
| `role add-user <code> <login>` | dá o papel a um usuário (valida papel+login; exit 4 se faltar) |
| `role remove-user <code> <login>` | tira o papel (não-vinculado = exit 4) |

## replacement — substitutos de usuário (administração; requer admin)

Delegação de tarefas: o TITULAR é quem será substituído; o SUBSTITUTO assume as
tarefas de workflow/GED no período. Os argumentos de usuário são **logins**
(resolvidos para userCode internamente; login inexistente = exit 4, nunca filtro
ignorado). Alias: `substitute`.

| comando | efeito |
|---|---|
| `replacement list [--user login] [--replaced-by login] [--limit N]` | lista as substituições (filtros por titular/substituto) |
| `replacement show <login> [--valid-only]` | substituições de um usuário, com escopo Workflow/GED; `--valid-only` = só as vigentes hoje |
| `replacement create <titular> <substituto> --end YYYY-MM-DD [--start YYYY-MM-DD] [--workflow-tasks] [--ged-tasks]` | define um substituto (--start default hoje; par+período duplicado = exit 5) |
| `replacement update <titular> <substituto> [--start] [--end] [--workflow-tasks] [--ged-tasks]` | altera a substituição (merge; par inexistente = exit 4) |
| `replacement delete <titular> <substituto>` | remove a substituição (inexistente = exit 4) |

`--workflow-tasks` (default true) e `--ged-tasks` (default false) definem o
escopo. `list` vem da REST v2 (sem as flags de escopo); `show` vem do SOAP (com
as flags, e inclui as vigências expiradas).

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
| `widget new <code>` | local | cria o esqueleto de um widget em `wcm/widget/<code>` (`--title`, `--category`, `--template classic\|vue\|react`; código minúsculo `[a-z][a-z0-9_]*`; a pasta não pode existir; templates vue/react = SPA Vue 3/React 19 + Vite, build com `npm run build` antes do export) |
| `widget list` | — | lista os widgets do servidor (fluiggersWidget; sem ela usa a API nativa, que pode omitir itens) |
| `widget import <code>... \| --all` | servidor → local | baixa widgets para o projeto |
| `widget export <NomeWidget>` | local → servidor | empacota e publica um widget (deploy nativo); `--build` roda `npm run build` antes (widgets vue/react; falha = exit 2 sem enviar) |

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
| `dev` | proxy local autenticado do portal: serve JS/CSS **e o markup do view.ftl** das widgets do disco (sem deploy), preview de formulários em `/_dev/forms/` — com painel de simulação de processo (executa o `events/displayFields.js` local com WKNumState/WKUser/formMode escolhidos; form vinculado por `form link` ganha as etapas reais pelo nome) — e recarrega o navegador ao salvar; `--npm-watch` roda o `npm run watch` das widgets SPA (vue/react) junto |

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

## Pegadinhas frequentes (evitam falha silenciosa)

- **Busca de usuário**: `user list --search` casa **PREFIXO do nome**
  (case-insensitive) — não é substring e **não** casa login. "Aless" acha o
  Alessandro; "lorenco" e "alorenco" não.
- **Busca de grupo/papel é client-side**: `group list --search`/`--type` e
  `role list --search` são filtrados **na CLI** (substring em código/descrição)
  porque o servidor **ignora** esses parâmetros na query. Sem `--limit 0`, o
  filtro roda só sobre as primeiras páginas.
- **Filtros de `task`/`request` usam userCode, não login**: `--assignee`/
  `--requester` esperam o userCode; a CLI converte um login automaticamente,
  mas **login inexistente = exit 4** (não "lista vazia"). Se vier vazio de
  verdade, o filtro está certo e não há itens.
- **`document list` sem argumento = pastas raiz**; para ver o conteúdo é
  preciso um **folderId válido** (id errado dá vazio/erro, não "sem itens").
- **Vínculos usuário↔grupo↔papel** só existem pelo lado do grupo/papel
  (`group add-user`, `role add-user`) — **não** há como editá-los pelo `user`.
- **Escrita em `prod`** exige `--yes` em modo não-interativo (sem ele: exit 2),
  para `export`/`delete`/`create`/`update`/`add-user`/`remove-user` etc.
- **Senha de usuário novo** (`user create`/`--set-password`) vem só de
  `FLUIGCLI_NEW_USER_PASSWORD` ou prompt — **nunca** por flag.

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

**Ver a minha fila de tarefas em aberto**
```sh
fluigcli task list --json --server homolog | jq -r '.data.tasks[] | "\(.requestId)\t\(.stateName)"'
```

**Consultar uma solicitação e seu histórico**
```sh
fluigcli request show 196540 --json --server homolog ; echo "exit=$?"   # exit 4 = número inexistente
```

**Baixar um documento do GED pelo id**
```sh
fluigcli document download 12345 --dir ./baixados --json --server homolog
```

**Criar um usuário (senha por env, nunca por flag)**
```sh
FLUIGCLI_NEW_USER_PASSWORD='Seg@redo123' fluigcli user create jsilva \
  --email joao@empresa.com --first-name João --last-name Silva --json --server homolog
```

**Dar um papel a um usuário / adicioná-lo a um grupo**
```sh
fluigcli role add-user aprovadores jsilva --json --server homolog     # papel
fluigcli group add-user Compras jsilva --json --server homolog        # grupo
# ambos: papel/grupo ou login inexistente = exit 4
```
