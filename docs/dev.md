# fluigcli dev — servidor de desenvolvimento com live reload

O comando `fluigcli dev` sobe um proxy local **autenticado** do servidor
Fluig. O proxy serve do disco os arquivos que você edita. Você não publica
nada. Sem o `dev`, o ciclo é *editar, publicar, esperar o deploy e o cache,
recarregar*. Com o `dev`, o ciclo vira *salvar, e o navegador recarrega
sozinho*.

```
$ fluigcli dev
✓ Dev server de "homolog" (hml) no ar — Ctrl+C para parar.
Dashboard: http://127.0.0.1:8787/
17 widget(s) do disco · 35 formulário(s) · watch desligado — gerencie no dashboard
mudança em wcm/widget/ramais/resources/js/ramais.js — recarregando o navegador
```

O **dashboard** reúne o portal, o preview de formulários, a **consulta de
datasets**, as widgets servidas e as configurações. O dashboard é a raiz do
dev server.

## Dashboard

A raiz do dev server (`http://127.0.0.1:8787/`) é o **dashboard**. Só a raiz
exata é o dashboard. O portal e os demais caminhos seguem pelo proxy normal.
A página tem estas seções, de cima para baixo:

- **Servidor conectado**: um card compacto de duas linhas. O card mostra o
  ambiente em destaque (homologação ou desenvolvimento). A **produção sai em
  cor de alerta**. O `dev` em produção exige a confirmação da trava. O card
  mostra a mesma saúde do `fluigcli server status`: a **versão do Fluig**, o
  uptime, os usuários conectados, as threads, a memória JVM/SO e o banco. O
  detalhe de cada número aparece no **hint**. Passe o mouse para ver o hint.
  Os monitores de serviço viram pontos coloridos. O hint mostra o nome e o
  status de cada monitor. Só os monitores em **FAILURE** aparecem nomeados. O
  card também mostra o estado do **fluigcliHelper** (`helper vX.Y.Z`). O helper
  ausente ou desatualizado aparece em destaque, com a orientação no hint. A
  versão do Fluig aparece sempre. As estatísticas e os monitores exigem usuário
  admin. Sem o privilégio, o painel avisa e continua a funcionar. O card
  atualiza a cada 60 s.
- **Acessos**: o portal pelo proxy, o preview de formulários, o
  [Dataset Lab](#datasets), os [logs do servidor](#logs-do-servidor), o
  [Explorador de Processos](#processos) e a subtela [Pessoas](#pessoas).
- **Widgets SPA**: esta seção só aparece quando o projeto tem widget vue ou
  react. A seção mostra o estado do bundle de cada widget. O **bundle está
  desatualizado** quando a fonte é mais nova que o compilado. Neste caso, o
  portal serviria o js velho. A seção também mostra o estado do `npm run
  watch`. O **toggle "npm watch" liga e desliga os watchers sem reiniciar o
  dev**. Neste modo, o `--npm-watch` da linha de comando vira só o estado
  inicial. Ao desligar, o dev encerra os processos na hora.
- **Style guide**: o resumo do [`fluigcli audit`](audit.md) no projeto
  inteiro. A seção mostra o total de erros e avisos e as regras mais violadas.
  O **hint** de cada regra explica o que ela aponta. O dev recalcula o resumo
  ao salvar qualquer arquivo de `forms/` ou `wcm/widget/`. Os detalhes por
  arquivo ficam no comando `fluigcli audit` ou no botão 🎨 do preview de cada
  formulário.
- **Watch integrado (publicar ao salvar)** *(indisponível em produção)*: liga
  o comportamento do `fluigcli watch` dentro do `dev`. Você **escolhe o tipo
  de artefato**: datasets, eventos globais, mecanismos, formulários e scripts
  de processo. As garantias são as mesmas do watch. O watch publica só no
  **servidor conectado**. O watch nunca cria artefato. O watch mantém a versão
  dos formulários e aplica os scripts de processo de forma cirúrgica. O
  salvamento sem mudança não vai à rede. O dev grava a escolha em
  `.fluigcli/dev.json` (git-ignorado). As últimas publicações aparecem num feed
  na própria página. Elas também aparecem no terminal, como sempre.
- **Live reload**: pause e retome o live reload e ajuste o debounce sem
  reiniciar o dev.
- **Limpar caches do painel**: zera o contexto, os processos, as etapas e os
  usuários da simulação. Também zera as conexões de publicação.

## Widgets (o grande ganho)

Navegue no **portal real** pela porta local. A página, os dados, a sessão e o
WCMAPI são os do servidor. Mas o dev serve o JS/CSS das widgets do projeto
(`wcm/widget/<code>/src/main/webapp/resources/`) da sua máquina. Você salva, o
navegador recarrega, a mudança aparece. Você **não faz deploy de WAR, não
espera o servidor descompactar e não limpa o cache**. Isso também vale para as
widgets-biblioteca. Estas widgets compartilham JS/CSS com outras widgets.

- O context-root vem do `jboss-web.xml` da widget. O fallback é o nome da
  pasta.
- O dev resolve a query de cache-busting (`?v=…`). O dev também resolve o
  sufixo de idioma que o portal acrescenta (`ramais_pt_BR.js` → `ramais.js`).
- Um arquivo que não existe localmente segue para o servidor. Um exemplo é o
  bundle gerado pelo servidor. Por isso, o portal nunca quebra por causa do
  map-local.
- **O markup do `view.ftl` também é local**: o portal envelopa cada instância
  com `id="_instance_<id>_" appcode="<code>"`. O proxy troca a saída
  renderizada pela do seu `view.ftl`. O proxy substitui `${instanceId}` e
  remove os comentários `<#-- -->`. Você salva o `.ftl`, o navegador recarrega,
  a mudança aparece. Você não faz deploy. Há um limite. Um template com
  FreeMarker de verdade (outras `${…}`, `<#if>`, `<@macro>`) mantém o render do
  servidor, com um aviso. O renderizador local não executa FreeMarker.
- O `edit.ftl`, o `.properties` e o `application.info` seguem server-side. A
  mudança neles não recarrega o navegador. Recarregar mentiria que a mudança
  apareceu. Neste caso, o dev pede o `fluigcli widget export <code>` num aviso.

### Widgets SPA (templates vue/react do `widget new`)

O dev trata uma widget com `package.json` na raiz como SPA compilada:

- **`--npm-watch`**: o dev server roda o `npm run watch` de cada widget SPA do
  projeto. A saída vai para o log com o prefixo `[<code>]`. O dev encerra tudo
  junto no Ctrl+C. Assim você desenvolve no portal real com **um comando só**.
  O dev pula a widget sem `node_modules/` com um aviso. Neste caso, rode `npm
  install` e reinicie. Sem a flag, rode o `npm run watch` você mesmo.
- Editar a **fonte** da SPA (`src/vue/`, `src/react/`, `vite.config.ts`...) não
  dispara reload nem aviso. Quem recarrega é a escrita do **bundle** em
  `src/main/webapp/resources/`. O Vite faz essa escrita. O Vite escreve o js e
  o css na mesma rajada. O debounce agrupa a rajada num reload só.
- Na largada, o dev avisa **bundle ausente ou desatualizado**. O bundle está
  desatualizado quando a fonte é mais nova que o js compilado. Sem esse aviso,
  o portal serviria o js velho e a mudança não apareceria.
- O dev não observa o `node_modules/`. Assim, um `npm install` com o dev no ar
  não estoura o limite de watches do SO nem gera reloads.

## Formulários

O caminho `/_dev/forms/` lista os formulários do projeto. Cada formulário tem
preview local. O preview equivale ao modo "novo registro". A origem é a mesma
do proxy. Por isso, os caminhos absolutos que os formulários usam
(`/style-guide/...`, `/portal/resources/js/...`, `/webdesk/vcXMLRPC.js`)
resolvem no servidor real com a sessão injetada. Assim, o **`DatasetFactory`
funciona com dados reais**, sem publicar nada.

O preview **emula o render de formulários do Fluig 2.0**. Ao servir um
formulário numa solicitação, o servidor 2.0 reescreve o style guide legado
(`fluig-style-guide.min.css`) para o tema novo
(`fluig-style-guide-flat.min.css`). O style guide legado é descontinuado e
responde 404. O servidor também injeta no `<head>` o runtime (`forms.js`) e os
CSS do tema (flat, animalia-icons e fluig-icons). Por isso, os formulários
**não migrados** renderizam certo no portal. O preview aplica as mesmas
transformações. O preview só as aplica quando o servidor tem o tema novo. Num
Fluig 1.x, o preview não altera nada. Isso é só apresentação. O preview **não
toca nos arquivos locais**.

### Tema claro/escuro no preview (botão 🌓)

O botão **🌓** da barra alterna o tema do preview como o portal 2.0 faz. O
botão troca a classe `theme-dark` no `<html>`. Essa classe vira todas as
variáveis `--fs-*` do CSS flat. Este é o jeito mais rápido de **ver** o que o
audit aponta. As cores fixas (`#hex`) não acompanham a troca e ficam presas no
tema antigo. Sem preferência salva, o preview segue o tema do sistema. A
escolha persiste entre recarregamentos (localStorage). A barra e as janelas do
fluigcli usam sempre o **tema oposto** ao do formulário. Assim, um formulário
claro tem o painel escuro, e vice-versa. O contraste fica constante. Você não
confunde a barra com o conteúdo.

### Auditoria de Style Guide no preview (botão 🎨)

Cada preview roda o [`fluigcli audit`](audit.md) na pasta do formulário. O
preview mostra o veredito no botão **🎨** da barra. O ponto **verde** indica
nenhuma pendência. O ponto **amarelo** indica só avisos. O ponto **vermelho**
indica erros. O clique abre o painel com os achados: regra, arquivo:linha,
mensagem e a sugestão de correção. A sugestão inclui qual variável do tema usar
no lugar de uma cor fixa. A sugestão também mostra o que o `audit --fix`
resolve sozinho. Salvar um arquivo recarrega o preview. Por isso, **a auditoria
reexecuta a cada salvamento**. Assim, você corrige e vê a lista encolher em
tempo real. O preview respeita o `.fluigcli/audit.json` do projeto.

### Simulação de processo (painel flutuante)

Um formulário de processo costuma sumir no preview. A causa é o
`events/displayFields.js`. Este evento roda **no servidor** quando o portal
renderiza o form numa solicitação. O evento lê as variáveis de workflow
(`getValue("WKNumState")`, `WKUser`, …). O evento preenche os campos que o JS
do formulário usa para mostrar ou esconder as seções de cada etapa. Sem
processo, nada disso acontece.

O preview resolve isso. O preview executa o **displayFields local no
navegador**, com a API server-side emulada:

- O `getValue("WK…")` lê do **painel de simulação**. Este painel é o botão
  flutuante no canto da página. O painel define a etapa (`WKNumState`), o modo
  (`ADD`/`MOD`/`VIEW`), o usuário (`WKUser`) e as variáveis extras
  (`CHAVE=valor`). O `WKUser` é um seletor com todos os usuários do servidor
  pelo nome. Os usuários ativos vêm primeiro. O seu usuário já vem
  selecionado. O dev carrega a lista uma vez e a mantém em cache.
- O `form.setValue/getValue/getFormMode/setEnabled` opera no DOM do preview. O
  `DatasetFactory` usa os **datasets reais** pela sessão do proxy.
- **Com o formulário vinculado** (`fluigcli form link`), o dev detecta o
  processo sozinho pelo `formId` das versões. As **etapas reais aparecem pelo
  nome**. Escolha "Revisar Justificativa" em vez de decorar o número. Sem
  vínculo, escolha o processo na lista ou digite o número direto.
- O dev salva a escolha por formulário (localStorage). A escolha sobrevive ao
  reload. Ela sobrevive também ao live reload ao salvar o próprio
  `displayFields.js`.
- O painel mostra o que o evento fez: as leituras, os `setValue` e os avisos.
  A emulação cobre o básico (`setValue/getValue/getFormMode/setEnabled/
  getMobile`). A emulação também cobre o `setVisibleById`, o
  `getChildrenIndexes`, o `getCardData`, o `log.*` (vai para o console do
  navegador e para o relatório do painel), o
  `fluigAPI.getUserService().getCurrent()` (dados reais do usuário simulado,
  via dataset `colleague`) e o interop Java comum dos eventos
  (`new java.util.HashMap()`/`ArrayList`, `keySet().iterator()`, `importClass`
  das classes simuladas). Uma classe Java fora disso não roda no navegador. Um
  exemplo é o `java.text.SimpleDateFormat`. Neste caso, o painel mostra o erro
  com o nome da classe e o form fica como no preview cru.
- **As tabelas pai×filho funcionam no preview** (`wdkAddChild`/
  `fnWdkRemoveChild`). O preview replica a transformação do render. O preview
  marca a linha-modelo de cada `table[tablename]`. O preview carrega a
  **máquina real do servidor** (`wdkdetail.js`) pelo proxy. Assim, incluir e
  remover linhas se comporta como no portal. Num servidor sem o arquivo (Fluig
  antigo), entra uma emulação local equivalente.
- **O preview popula os selects declarativos de dataset** (`<select
  dataset="dsX" datasetkey="..." datasetvalue="..." addBlankLine="true">`). O
  preview consulta o dataset real via proxy. O preview gera os `<option>` como
  o render do servidor. A opção vazia vem primeiro quando há `addBlankLine`.
  Depois vêm as linhas na ordem do dataset. Uma falha na consulta vira um aviso
  no painel.
- **Barra de ferramentas do preview**: a barra fica no canto da página. Todos
  os botões têm tooltip. O botão ⇔ alterna a posição da barra entre direita,
  centro e esquerda. O dev persiste a posição.
  - **⚙ Simulação** — o cartão de contexto do processo (acima). O cartão inclui
    o toggle `getMobile() = celular` para eventos que se adaptam ao app.
  - **💾 Salvar** — um clique valida na hora. O botão roda o
    `events/validateForm.js` local sobre os valores preenchidos no preview,
    inclusive as linhas de tabelas pai×filho. O botão mostra o resultado num
    diálogo. O `throw` da validação aparece como no portal (HTML renderizado).
    O sucesso informa que o Fluig gravaria. **Nada é gravado.**
  - **▶ Enviar etapa** — o botão pergunta a **próxima etapa (WKNextState)**,
    por nome ou número, e valida o envio. Se o formulário definir
    `beforeSendValidate(numState, nextState)` client-side, esse código roda
    antes, como no portal.
  - **🚀 Publicar** — o `fluigcli form export` em forma de diálogo, no padrão
    do Fluig. Escolha o **servidor** (qualquer um cadastrado; o conectado vem
    selecionado) e o **formulário de destino**. O do vínculo (forms.json) vem
    pré-selecionado. Ao **atualizar**, o diálogo mostra o dataset e o campo
    descritor do servidor (ajustáveis) e a versão. O padrão é *manter a atual*.
    Ao **criar**, não há escolha de versão. Você define o nome (sugerido = a
    pasta), o dataset (sugerido como `ds_{{nome}}`), a **pasta do GED
    navegável** (um seletor com as pastas reais do servidor; você também pode
    digitar o id) e o armazenamento (tabelas de banco, recomendado, ou tabela
    única). Nos dois casos o **campo descritor é um seletor com os campos do
    próprio formulário**. O dev atualiza o vínculo local após publicar. A
    credencial vem da sessão em cache, do keyring ou da env var. Sem nenhuma, o
    diálogo pede a senha. Essa senha trafega apenas até o dev server local. A
    **produção** exige digitar o nome exato do servidor, como a trava do CLI.
  - **📱 Tela** — o botão alterna num clique entre livre, celular (375px) e
    tablet (768px). O formulário abre numa **moldura de dispositivo com
    iframe** (`?screen=phone|tablet`). O iframe tem viewport próprio. Por isso,
    as media queries do grid disparam e as colunas quebram linha de verdade.
    Limitar por container não dispara as media queries. Isto é largura visual
    apenas. O `getMobile()` é simulado na Simulação, porque ele re-executa o
    evento.
  - **↗ Abrir no Fluig** — o botão abre numa aba o render **real** do
    formulário (streamcontrol, via proxy autenticado) para comparar com o
    preview. O botão requer o processo escolhido na Simulação.
  - **⌂ Índice** e **🧹 Limpar** — o ⌂ volta a `/_dev/forms/`. O 🧹 recarrega
    o preview e zera os campos.

  Os eventos de processo (`beforeTaskSave` etc.) ficam fora da simulação. Para
  o ciclo real, use o `fluigcli watch`.

Para testar o formulário dentro do processo de verdade (bindings de card,
anexos, movimentação), continue com o `fluigcli watch` mais o F5.

## Datasets

O caminho `/_dev/datasets/` é a bancada de consulta de datasets. O tile
**Datasets** no dashboard abre essa bancada. Use a bancada no ciclo *criei ou
editei um dataset, quero ver o que ele traz*, sem sair do navegador. A bancada
é **só leitura**. A bancada consulta direto no servidor conectado pelo proxy,
na mesma sessão autenticada. Ela não publica nada.

- **Escolha o dataset** num campo com busca (por id ou descrição). O campo tem
  um badge de tipo. O tipo `CUSTOM` sai em verde. O estado `inativo` sai em
  vermelho. O ↻ recarrega a lista.
- **Consultar** executa a consulta e mostra o resultado numa tabela com
  cabeçalho fixo. O rodapé traz o **número de linhas e colunas** e a
  **duração** da consulta. Uma célula nula aparece como `null` esmaecido. Isto
  distingue a célula nula de um texto vazio.
- **Ver como Tabela ou JSON** — o JSON é a lista de objetos `campo: valor`.
  Este formato é bom para quem consome o dataset por API.
- **Copiar** joga o resultado como TSV na área de transferência. Você cola
  direto no Excel. **Exportar CSV** baixa um `.csv` UTF-8. Este arquivo abre no
  Excel em PT-BR.
- **Configurar parâmetros**:
  - **Campos** — escolha as colunas do resultado. O padrão é todas.
  - **Ordenar** — um campo (a API aceita só um) mais ascendente ou
    descendente.
  - **Limite** — o número de linhas no resultado. O padrão é `100`. O valor `0`
    significa sem limite. Tenha cuidado com datasets grandes. Quando o
    resultado bate o limite, sai o aviso "limite atingido — pode haver mais".
  - **Filtros** — quantos você quiser. Cada filtro tem campo, valor inicial e
    final, tipo (Must / Must Not / Should) e a opção "usa Like".
  - **sqlLimit** — o limite no nível do SQL, para datasets SQL. O sqlLimit tem
    inicial, final, tipo e Like, como na extensão do Studio.
- O dev salva a última configuração de cada dataset (campos, ordenação, limite,
  filtros) no navegador (localStorage). A configuração volta ao reabrir o
  dataset.

As colunas dos seletores vêm de uma consulta-sonda (uma linha). Um dataset que
exige um filtro obrigatório (por exemplo, `sqlLimit`) pode não revelar as
colunas na sonda. Neste caso, o painel avisa e as colunas aparecem após a
primeira consulta.

## Logs do servidor

O caminho `/_dev/logs/` acompanha o `server.log` **ao vivo** num console no
navegador. O tile **Logs do servidor** no dashboard abre esse console. Este é
a versão visual do [`fluigcli log tail --follow`](log.md). O console requer o
**fluigcliHelper ≥ 0.3.0** no servidor conectado. Sem o helper, o painel
orienta o `fluigcli server install-helper`.

- **Arquivo**: um seletor com todos os arquivos do diretório de log do servidor
  (server.log, rotacionados e CSVs de telemetria). O seletor mostra o tamanho
  de cada arquivo.
- **Filtros locais**: a severidade mínima (`DEBUG+` … `ERROR+`) e a
  palavra-chave. Cada aba filtra por conta própria, sobre as linhas já
  recebidas e as novas. A decisão é por entrada. O stack trace acompanha o
  ERROR que o abriu.
- **Colorização**: ERROR/FATAL em vermelho, WARN em amarelo, DEBUG esmaecido.
  O console é escuro nos dois temas.
- **Entradas grandes recolhidas**: uma entrada extensa mostra só a mensagem
  principal. Um botão **▸ mostrar +N linhas** expande o restante e recolhe de
  volta. Recolhem o **ERROR/FATAL** (o stack trace Java, sempre) e o
  **WARN/INFO** quando passam de algumas linhas de continuação. O **DEBUG/TRACE**
  fica sempre integral.
- **Fuso dos horários** (🕓): o timestamp do log é a hora de parede do
  servidor, **sem offset**. Com o `fluigcliHelper` 0.4.0+ (que reporta o fuso
  da JVM), o botão **🕓 servidor ⇄ 🕓 navegador** converte os horários para o
  fuso do navegador. O dev salva a preferência. O botão não aparece quando o
  servidor está no mesmo fuso do navegador. O botão também não aparece com um
  helper antigo, que não reporta o fuso. Nestes casos, não há o que converter.
- **Buscar intervalo** (📅): o botão abre uma janela flutuante com "início" e
  "fim" (data e hora). Use a janela para resgatar o log de um momento
  específico do arquivo selecionado. Ao aplicar, o painel troca do ao vivo para
  um **snapshot** do intervalo. Sai o banner "Intervalo · N entradas · voltar
  ao vivo". Os filtros de nível e palavra e o 🕓 continuam valendo. O painel
  interpreta as datas no fuso que está na tela (o mesmo do 🕓). O botão requer o
  `fluigcliHelper` 0.5.0+. O botão some com um helper mais antigo. A busca é só
  no arquivo do seletor. Para um log de dias atrás, escolha o arquivo
  rotacionado na lista.
- **Pausar/retomar**: o botão congela a rolagem para você ler com calma. As
  linhas continuam a chegar. O contador mostra quantas linhas esperam. As
  linhas aparecem ao retomar. A **auto-rolagem** e o **limpar** completam a
  barra. Os botões de pausa (⏸), limpar (🧹) e baixar (⬇) são só-ícone. A
  descrição fica no tooltip. Assim, a barra cabe numa linha.
- **⬇ baixar** entrega o arquivo inteiro pelo próprio proxy autenticado.

Por trás, um poller no dev server consulta o helper por offset (a cada 2 s). O
poller só consulta **enquanto houver navegador conectado**. O poller retransmite
as linhas por SSE. Várias abas compartilham a mesma consulta. O dev detecta a
rotação do arquivo no servidor. O acompanhamento recomeça sozinho.

## Processos

O caminho `/_dev/processes/` reúne, **só para leitura**, tudo que você precisa
saber de um processo enquanto escreve o formulário ou os scripts. Assim, você
não abre o Fluig Studio nem o portal. O tile **Processos** no dashboard abre
essa tela. Um combobox buscável escolhe o processo (por id ou nome). Um seletor
ao lado troca a versão. A URL aceita `?process=<id>` (compartilhável). O dev
salva o último processo visto no navegador.

Ao selecionar, a tela monta quatro blocos a partir do **export XML nativo** do
processo. Esta é a mesma fonte do `workflow publish` e do `diff`. Não há
configuração nova no servidor. A tela funciona apontando para produção:

- **Cabeçalho** — a descrição, o `processId` (clique para copiar), a versão
  atual (de N), o autor e o **formulário vinculado**. O formulário mostra o id
  e o nome no servidor. Quando há vínculo local (`.fluigcli/forms.json`), o
  nome vira um **link que abre o [preview](#formularios) em nova aba**. Assim,
  você não perde a tela do processo.
- **Etapas** — a tabela central. Ela tem estas colunas. **Nº** é o `sequence`,
  que é o `WKNumState` que os eventos leem. Depois vêm o nome, o tipo
  (Início/Atividade/Automação/Gateway/Fim) e **quem atua**. A coluna "quem
  atua" mostra o mecanismo mais o alvo real: papel, grupo, campo do formulário
  ou usuário. O usuário aparece pelo **nome**, com o código no tooltip. O alvo
  também pode ser o **executor de uma atividade anterior**, mostrado pelo nome
  da etapa. As últimas colunas são o prazo e o fluxo (`→` etapas de destino).
  Os eventos intermediários (boundary de erro etc.) não entram na lista. Eles
  não são etapas em que o formulário roda. As etapas vêm na **ordem aproximada
  do fluxo**, a partir do início, seguindo as transições. O botão **Fluxo / Nº**
  alterna para a ordem por número. **Clique numa linha para copiar** um snippet
  pronto. Por exemplo: `getValue("WKNumState") == 17 // Faturar Documento`. Os
  gateways mostram as condições (campo/operador/valor → etapa de destino). Há
  filtro por nome. **Copiar constantes** copia de uma vez o bloco de `const` de
  todas as etapas. Os nomes vêm do rótulo. O bloco inclui o `ETAPA_NOVO = 0` e
  usa um sufixo de número quando o nome repete. O bloco fica pronto para colar
  no topo de um `validateForm.js` ou `displayFields.js`.
- **Diagrama** — o desenho oficial do processo, exatamente como no Studio. O
  SVG vem pronto do servidor. O diagrama abre **ajustado à largura**. O
  processo inteiro cabe na tela. Os botões **− / + / ajustar** dão zoom. Com
  zoom, você **arrasta** para navegar.
- **Scripts** — os eventos do processo em duas listas (globais e service
  tasks). Cada evento mostra o tamanho do código no servidor. O evento tem um
  selo **local ✓** com o caminho quando o arquivo existe em
  `workflow/scripts/`. Sem o arquivo local, o evento aparece como **só no
  servidor**, com a dica do [`fluigcli workflow import`](workflow.md).

O dev cacheia o detalhe por processo e versão. O ↻ recarrega ignorando o cache.
O dev reconfere a presença local dos scripts a cada abertura. Assim, ela
reflete o que você acabou de salvar.

## Pessoas

O caminho `/_dev/people/` mostra os **usuários, grupos e papéis** da
plataforma. A tela também mostra, principalmente, **quem participa** de cada
grupo e papel. O tile **Pessoas** no dashboard abre essa tela. A tela é o
complemento do [Explorador de Processos](#processos). Quando você vê que uma
etapa é atribuída ao papel `faturista` ou ao grupo `Compras`, um clique no chip
de atribuição abre esta tela já no participante certo. E o chip de papel ou
grupo lá vira link para cá.

A tela tem três abas com busca instantânea. A busca é **no cliente**, sobre a
lista já carregada:

- **Usuários** — o nome, o login, o e-mail e o estado (ativo ou bloqueado).
  Clique num usuário para ver a **visão reversa**: de quais grupos e papéis ele
  participa. Cada item é clicável e leva ao detalhe correspondente.
- **Grupos** — o código, a descrição e o tipo. Os grupos de **comunidade**
  (`MODERATOR_*`/`MEMBER_*`, criados automaticamente) ficam ocultos por padrão.
  Um checkbox os exibe.
- **Papéis** — o código e a descrição.

No detalhe de um grupo ou papel você vê a **lista de membros**. Os membros
**bloqueados** aparecem em destaque. Um aprovador inativo num grupo de
atribuição costuma ser um bug. O detalhe tem estes atalhos:

- **Incluir-me / Remover-me** — inclui ou remove **o seu próprio usuário** (o
  que está logado no `dev`) do grupo ou papel, com uma confirmação. Este é o
  caso de uso central. Assim, você testa rápido uma atribuição de processo,
  entrando e saindo do papel. O dev só escreve o seu usuário. Para administrar
  outros usuários, use os comandos [`group`](group.md)/[`role`](role.md)/
  [`user`](user.md).
- **Onde é usado?** — o atalho varre os processos do servidor e lista as
  **etapas** em que aquele grupo ou papel atua (processo, etapa, `WKNumState` e
  mecanismo), com link para o Explorador. A primeira varredura lê o export de
  cada processo. Esta é a mesma fonte, cacheada e compartilhada com o
  Explorador. Por isso, a primeira varredura demora alguns segundos. Depois ela
  fica em cache.
- **Copiar código** e **Copiar membros (TSV)**.

A URL aceita `?user=<login>`, `?group=<código>` e `?role=<código>`
(compartilhável). A tela usa as APIs administrativas do Fluig
(`/admin/api/v1/...`). Essas APIs **exigem um usuário com o papel admin** no
servidor. Sem esse privilégio, um aviso explica o que falta. O ↻ recarrega as
listas ignorando o cache.

## Segurança (por design)

- **Por padrão, o dev escuta só em `127.0.0.1`**. O proxy carrega a SUA sessão
  autenticada. Quem acessa a porta age como você no Fluig. Para desenvolver em
  servidor remoto (via SSH), use `--listen` com um endereço de rede privada
  sua. Por exemplo, o IP da máquina na tailnet (`fluigcli dev --listen
  100.x.y.z`) ou um túnel SSH (`ssh -L 8787:127.0.0.1:8787`). **Nunca** use um
  IP público. A CLI avisa sempre que o bind sai do loopback.
- O navegador **nunca vê os cookies do Fluig**. A sessão mora no proxy. O jar
  da CLI absorve os `Set-Cookie` do servidor.
- **A produção exige escolha consciente**. Em servidor `prod`, o `dev` só sobe
  após a confirmação da trava de produção (`s/N`; use `--yes` em script). Isto
  é útil para **inspecionar** os logs, os datasets, o status e o portal pelo
  proxy. O card do servidor sai em **cor de alerta**. O **watch integrado fica
  indisponível**. A auto-publicação ao salvar continua proibida em produção,
  como no `watch` standalone. A publicação pelo painel 🚀 segue exigindo o nome
  do servidor digitado. O dev recusa um servidor **sem ambiente marcado**
  (`fluigcli server update <name> --env hml`).

## Detalhes

- `--port <n>` (padrão `8787`): a porta do dev server.
- `--listen <addr>` (padrão `127.0.0.1`): o endereço de escuta (ver Segurança).
  A reescrita de URLs usa o Host de cada requisição. Assim, acessar pelo IP da
  tailnet funciona sem configuração extra.
- `--debounce <dur>` (padrão `500ms`): a espera após o salvamento antes de
  recarregar. A espera agrupa as rajadas do editor.
- `--npm-watch`: roda o `npm run watch` das widgets SPA (ver a seção acima).
- O live reload observa `forms/` e `wcm/widget/`. O dev injeta o SSE nas
  páginas HTML. O dev não altera nada no servidor.
- O dev reescreve os redirects e as URLs absolutas que o portal embute (por
  exemplo, `WCMAPI.serverURL`) para a origem local. Assim, a navegação não
  escapa do proxy.
- O `--json` não é suportado. O dev é um modo interativo de longa duração.
- O `dev` e o `watch` se complementam. O `dev` dá feedback instantâneo local
  (widgets, layout de forms). O `watch` publica de verdade ao salvar
  (datasets, eventos, mecanismos, scripts de processo e forms no contexto
  real).
