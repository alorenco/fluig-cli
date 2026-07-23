# Mapa de comandos do fluigcli

Convenção que vale para todos os recursos:
**`import` = servidor → local · `export` = local → servidor.**

Consulte sempre `fluigcli <grupo> <sub> --help` para as flags exatas — abaixo é
o mapa mental, não a referência completa.

## Por objetivo (índice rápido)

Comece por aqui: identifique a **intenção** e pule para o grupo certo.

| quero… | comando |
|---|---|
| começar num servidor existente com uma pasta vazia (baixar tudo) | `clone` |
| criar um widget novo do zero (esqueleto no padrão oficial) | `widget new <code>` |
| criar um artefato novo do zero (esqueleto local) | `dataset new` · `form new` · `event new` · `mechanism new` · `workflow new-script` |
| publicar um artefato local (dataset/form/evento/mecanismo/widget/script) | `<grupo> export` |
| baixar do servidor p/ inspecionar ou editar | `<grupo> import` |
| ver o que **mudaria** antes de publicar | `diff` |
| conferir se o código respeita o Style Guide 2.0 (tema fixo) | `audit` |
| consultar os dados de um dataset | `dataset query` |
| rodar SQL de diagnóstico (permissão, testar SQL, ver se objeto existe) | `db query` |
| consultar / iniciar / movimentar solicitações | `request` |
| ver a fila de tarefas (a minha ou de outros) | `task list` |
| navegar / baixar / subir documentos (GED) | `document` |
| criar / editar registros de um formulário | `form records` |
| ver tudo que um usuário fez num período (tarefas/solicitações/documentos) | `user audit <login>` |
| administrar usuários / grupos / papéis | `user` · `group` · `role` |
| definir substituto / delegar tarefas de um usuário | `replacement` |
| ver / acompanhar / baixar o log do servidor | `log tail` · `log files` · `log download` |
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
| `server test [<name>]` | login + ping + dados do usuário; reporta qual componente auxiliar está instalado |
| `server logout [<name>]` | descarta a sessão em cache (ou de todos com `--all`) |
| `server status [<name>]` | saúde do servidor: versão, helper (instalado/versão), uptime, memória, banco e monitores (requer admin) |
| `server install-helper [<name>]` | instala o componente auxiliar fluigcliHelper, embutido no binário (pré-requisito de `workflow export`, `widget import` e do grupo `log`; o `widget list` tem fallback nativo); `--force` reenvia = atualiza o helper |

Resolução do servidor alvo: `--server`/`FLUIGCLI_SERVER` > padrão do projeto >
padrão global > único cadastrado. ⚠️ Em servidor com `env=prod`, comandos de
escrita (`export`, `delete`, `install-helper`) **exigem `--yes`** em modo
não-interativo (sem ele: exit 2).

## clone — onboarding de instância existente

`clone` traz para o projeto local tudo o que a CLI gerencia em um servidor já
em uso (o cenário: consultor chega no cliente com uma pasta vazia). Consulta o
inventário do servidor e importa os tipos selecionados — mesma semântica do
`import --all` de cada grupo.

| comando | efeito |
|---|---|
| `clone --all` | clona todos os tipos disponíveis (sem perguntar) |
| `clone --only forms,datasets` | clona só os tipos citados (`forms`, `datasets`, `workflows`, `events`, `mechanisms`, `widgets`) |
| `clone` | interativo: mostra o inventário e pergunta o que clonar |

- Em modo não-interativo (`--json`/CI) `--all` ou `--only` é obrigatório
  (exit 2 sem eles).
- **widgets exigem o fluigcliHelper**: com `--all` são pulados com aviso; com
  `--only widgets` sem o helper = exit 7.
- `workflows` = só os **scripts de eventos** (o diagrama fica no servidor);
  widget SPA vem o bundle publicado (sem fonte TS/Vue); páginas, comunidades e
  GED ficam fora do escopo.
- Re-executar sobrescreve os arquivos locais (commite antes). Falha parcial =
  exit 6 com `data.results` por tipo.

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

## db — SQL de diagnóstico (requer fluigcliHelper ≥ 0.6.0)

SQL de **leitura** contra um datasource JNDI do servidor. É SQL cru de
diagnóstico — NÃO é `dataset query` (que executa um dataset cadastrado). Só
`SELECT`/`WITH`; escrita e múltiplas instruções são recusadas no servidor.

| comando | efeito |
|---|---|
| `db datasources` | lista os datasources JNDI disponíveis (padrão `/jdbc/AppDS`) |
| `db query "<sql>" [--jndi X] [--param v]... [--max-rows N]` | executa o SELECT e mostra colunas+linhas; `?` recebe os `--param` na ordem |

- Use para conferir permissão (`select has_perms_by_name('dbo.T','OBJECT','INSERT')`), login do datasource (`select suser_sname()`), ou testar um SQL antes do dataset.
- `--json`: `{columns[],rows[],rowCount,truncated}`; `rows` posicional; null do banco = `null`.
- Erro de SQL ou consulta que não é de leitura = exit 5 com a mensagem do banco.

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
| `workflow versions <processId>` | lista TODAS as versões (número, ativa, em edição) em tabela |
| `workflow import <processId>... \| --all` | baixa os scripts de eventos para workflow/scripts/ (servidor → local; sobrescreve no lugar; nativo) |
| `workflow import <processId> --stdout [--events X]` | imprime os scripts publicados SEM gravar no repo (read-only) |
| `workflow import <processId> --version <n>` | baixa os scripts de uma versão específica (não só a corrente) |
| `workflow export <arquivo\|processId> [--process-id <id>]` | atualiza scripts na versão corrente, sem criar versão (via componente auxiliar) |
| `workflow diff <arquivo\|processId> [--process-id <id>]` | compara o script local com o publicado (read-only; aceita `--events`/`--all-events`) |
| `workflow publish <processId> [--no-release] [--process-id <id>]` | deploy nativo: cria versão nova com os scripts locais e a libera |

`--process-id` desacopla o arquivo local (que dá o evento/script) do processId no servidor — use quando o processId publicado difere do prefixo do arquivo (ex.: arquivo `SolicitacaoAdiantamento.*.js`, processId `"Adiantamento ao Fornecedor"`). Um `NOT_FOUND` de processo sugere ids próximos e lembra da flag.

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
| `user audit <login> [--day dd/mm/aaaa \| --from … --to …] [--only tasks,requests,documents] [-o arq.txt\|arq.xlsx]` | atuação do usuário no período: tarefas que concluiu (com horário), solicitações que abriu e documentos que criou. Sem data = HOJE |

`user audit` é uma CONSULTA operacional (não exige admin como o resto do grupo):
resolve login→userCode e cruza `task list` (ordenado por conclusão), `request
list` (por abertura) e o dataset `document` (autor + createDate, por criação). A
coluna Processo usa a DESCRIÇÃO; documentos mostram o tipo legível. `-o`/`--output`
salva em `.txt` (texto puro) ou `.xlsx` (abas Resumo + Tarefas/Solicitações/
Documentos). Login inexistente = exit 4. ⚠️ Documentos têm só a DATA de criação
(sem hora — o Fluig não expõe). ⚠️ Login/logout e rastreamento por log NÃO entram
(o log do servidor só retém ~4 dias e não há dado estruturado de sessão).

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

## log — logs do servidor (operação; requer fluigcliHelper ≥ 0.3.0)

| comando | efeito |
|---|---|
| `log files` | lista os arquivos do diretório de log do servidor (server.log + rotacionados) |
| `log tail [-n N] [--file arq] [--level nível] [--grep texto] [--skip N]` | últimas N ENTRADAS (stack trace conta como uma e vem inteiro); `--level warn` = severidade mínima (warn+error+fatal); `--grep` = substring case-insensitive na entrada completa |
| `log tail --follow` | acompanha o log ao vivo (como `tail -f`; interativo — recusa `--json`) |
| `log download [--file arq] [-o caminho]` | baixa o arquivo inteiro (streaming) |

Sem o helper → exit 7 (`server install-helper`); helper antigo sem as rotas de
log → exit 7 orientando `server install-helper <name> --force`. Arquivo
inexistente → exit 4. Com `--json`, o tail devolve
`{file, size, entries[], truncated}`.

## widget

| comando | direção | efeito |
|---|---|---|
| `widget new <code>` | local | cria o esqueleto de um widget em `wcm/widget/<code>` (`--title`, `--category`, `--template classic\|vue\|react`; código minúsculo `[a-z][a-z0-9_]*`; a pasta não pode existir; templates vue/react = SPA Vue 3/React 19 + Vite, build com `npm run build` antes do export; `--template vue --vuetify` = variante Vuetify 3 via npm, para converter widgets Vuetify antigas) |
| `widget list` | — | lista os widgets do servidor (componente auxiliar; sem ele usa a API nativa, que pode omitir itens) |
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
export nativo do processo — não requerem o componente auxiliar.

## audit — Style Guide 2.0 e APIs de script

| comando | efeito |
|---|---|
| `audit [<path>...]` | linter das pastas convencionais (forms/, wcm/widget/, datasets/, events/, mechanisms/, workflow/scripts/): tema fixo do Fluig 2.0 (SG*), chamadas de API inexistentes (FL*, sobre o fluig.d.ts) e footguns do Rhino (RHINO*, só JS server-side); `--sync` atualiza o catálogo do servidor; `--fix` aplica as correções determinísticas; `--fail-on error\|warning\|none` (default error → exit 1 reprova) |

Regras: SG001 CSS legado (aviso, `--fix` troca p/ flat) · SG002 recurso
externo/CDN (erro) · SG003 cor fixa hex/rgb (erro; hex com valor idêntico a
variável do tema ganha `fix` e o `--fix` aplica; o resto vem com a variável
sugerida em `suggestion`) · SG004 `!important` sobre classe do tema (aviso) ·
SG005 estilo inline (aviso) · SG006 classe `fs-*` inexistente (aviso) ·
SG007 alert/confirm/prompt nativos (aviso — use FLUIGC) · FL001 método
`hAPI.*` inexistente · FL002 variável `WK*` desconhecida em `getValue()`
(devolve null em silêncio!) · FL003 método `form.*` inexistente (eventos de
formulário) · FL004 membro inexistente em FLUIGC/DatasetFactory/docAPI/
WCMAPI/etc. As FL* são avisos com o nome mais próximo em `suggestion` —
typo se corrige no código; API real que falte na referência
([`fluig.d.ts`](fluig.d.ts)) é caso de completar o arquivo. RHINO001
(aviso, só JS server-side): `===`/`!==` entre um `java.lang.String`
(retorno de `getFieldName`/`getInitialValue`/`getString`/`getColleagueName`…,
inclusive via variável — `var campo = c.getFieldName()...; campo === 'x'`) e um
literal de texto — no Rhino do Fluig é SEMPRE `false` (o `!==` sempre `true`),
sem erro; corrija com `String(x.getFieldName()) === 'y'` ou `==` (`String(...)`
e concatenação com `+` já são reconhecidos como seguros). RHINO002 (**erro**, só
JS server-side): sintaxe ES6+ que o Rhino do Fluig (Voyager 2) não aceita e dá
`SyntaxError` no deploy — `class`, `import`/`export`, `async`/`await`, parâmetro
com valor default (`function f(x = 1)`), spread em array/chamada (`[...a, 3]`) e
propriedade computada (`{ [k]: v }`); corrija com o equivalente ES5 (default →
`if (y == null) y = 10;`; computada → `obj[k] = v;`). Suportado (não apontado):
template literal, `let`/`const`, arrow, `for...of`, destructuring, rest param,
`Map`/`Set`, `Array.includes/find`, `String.padStart`. RHINO003 (**erro**, só JS
server-side): `const` declarado no corpo de um laço (`for`/`while`/`do`) — o Rhino
não reinicializa a cada iteração, **congela o valor da 1ª volta** em silêncio
(bug de dados invisível); troque por `let`. Não aponta `const` em função aninhada
no laço nem no cabeçalho de `for (const x of …)`. No `--json`
reprovado: `error.code=AUDIT_FAILED` e `data.findings[]` completo — **rode
`audit --fix`, corrija o restante pelas sugestões e repita até exit 0**.
Config em `.fluigcli/audit.json`: `{"ignore":[globs],
"severity":{"SG005":"off"}}`; vendorado/minificado e bundle de SPA já são
ignorados sozinhos. Como escrever certo de primeira:
[`styleguide.md`](styleguide.md).

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

Roda em `dev`/`hml` direto; em `prod` exige a trava de produção (confirmação
ou `--yes`) e o watch integrado fica indisponível. Escuta em `127.0.0.1` por
padrão (`--listen` muda, com aviso — ex.: IP de tailnet em servidor remoto);
sem `--json`.
Não publica nada no servidor. Não é indicado para agentes — é para o humano
ver o resultado no navegador; agentes usam `diff` + `export`. O dashboard tem
ainda o Dataset Lab (`/_dev/datasets/`) e o painel de logs ao vivo
(`/_dev/logs/`, requer o fluigcliHelper) — agentes preferem `dataset query` e
`log tail`.

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

**Criar um artefato do zero e publicar** (os `new` são locais, nunca
sobrescrevem e já sugerem o export certo)
```sh
fluigcli dataset new ds_clientes --json
# edite datasets/ds_clientes.js e publique:
fluigcli dataset export datasets/ds_clientes.js --new --json --server homolog

fluigcli form new frm_pedido --title "Pedido de Compra" --json
fluigcli form export forms/frm_pedido --new --json --server homolog
```

**Criar o script de um evento de processo com a assinatura correta**
```sh
fluigcli workflow new-script --help          # catálogo: eventos + assinaturas + quando rodam
fluigcli workflow new-script Compras beforeTaskSave --json
# edite workflow/scripts/Compras.beforeTaskSave.js e publique:
fluigcli workflow export Compras --json --server homolog
```

**Criar uma widget SPA (vue/react) e publicar**
```sh
fluigcli widget new meu_painel --template vue --title "Meu Painel" --json
(cd wcm/widget/meu_painel && npm install)
# --build compila antes de empacotar; falha de build = exit 2, nada é enviado
fluigcli widget export meu_painel --build --json --server homolog
```

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
# requer o componente auxiliar (exit 7 → server install-helper)
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
