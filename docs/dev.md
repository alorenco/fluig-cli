# fluigcli dev — servidor de desenvolvimento com live reload

Sobe um proxy local **autenticado** do servidor Fluig que serve do disco os
arquivos que você está editando — sem publicar nada. O ciclo
*editar → publicar → esperar deploy/cache → recarregar* vira só
*salvar → o navegador recarrega sozinho*:

```
$ fluigcli dev
✓ Dev server de "homolog" (hml) no ar — Ctrl+C para parar.
Dashboard: http://127.0.0.1:8787/
17 widget(s) do disco · 35 formulário(s) · watch desligado — gerencie no dashboard
mudança em wcm/widget/ramais/resources/js/ramais.js — recarregando o navegador
```

Portal, preview de formulários, **consulta de datasets**, widgets servidas e
configurações estão no **dashboard** (a raiz do dev server).

## Dashboard

A raiz do dev server (`http://127.0.0.1:8787/`) é o **dashboard**. Só a raiz
exata — o portal e todos os demais caminhos seguem pelo proxy normalmente.
De cima para baixo:

- **Servidor conectado**: o ambiente em destaque (homologação/desenvolvimento
  — produção sairia em cor de alerta, mas o `dev` nem inicia apontando para
  produção) e a mesma saúde do `fluigcli server status`, num card compacto de
  duas linhas: **versão do Fluig**, uptime, usuários conectados, threads,
  memória JVM/SO e banco, com o detalhe de cada número no **hint** (passe o
  mouse). Os monitores de serviço viram pontos coloridos (hint = nome/status);
  só os em **FAILURE** aparecem nomeados. O card também mostra o estado do
  **fluigcliHelper** (`helper vX.Y.Z`; ausente ou desatualizado sai em
  destaque com a orientação no hint). A versão aparece sempre; as
  estatísticas e os monitores exigem usuário admin — sem o privilégio, o
  painel avisa e segue funcionando. Atualiza a cada 60 s.
- **Acessos**: portal pelo proxy, preview de formulários e o
  [Dataset Lab](#dataset-lab).
- **Widgets SPA** (só aparece quando o projeto tem widget vue/react): estado
  do bundle por widget — **bundle desatualizado** quando a fonte é mais nova
  que o compilado (o portal serviria o js velho) — e do `npm run watch`. O
  **toggle "npm watch" liga e desliga os watchers sem reiniciar o dev** (o
  `--npm-watch` da linha de comando vira só o estado inicial); ao desligar,
  os processos são encerrados na hora.
- **Style guide**: o resumo do [`fluigcli audit`](audit.md) no projeto
  inteiro — total de erros/avisos e as regras mais violadas (o **hint** de
  cada regra explica o que ela aponta). Recalcula ao salvar qualquer arquivo
  de `forms/` ou `wcm/widget/`; detalhes por arquivo ficam no comando
  `fluigcli audit` ou no botão 🎨 do preview de cada formulário.
- **Watch integrado (publicar ao salvar)**: liga o comportamento do
  `fluigcli watch` dentro do `dev`, com **escolha por tipo de artefato**
  (datasets, eventos globais, mecanismos, formulários, scripts de processo).
  Mesmas garantias do watch: publica só no **servidor conectado**, nunca
  cria artefato, formulários com a versão mantida e scripts de processo
  cirúrgicos; salvamento sem mudança não vai à rede. A escolha fica
  persistida em `.fluigcli/dev.json` (git-ignorado) e as últimas publicações
  aparecem num feed na própria página (e no terminal, como sempre).
- **Live reload**: pausar/retomar e ajustar o debounce sem reiniciar o dev.
- **Limpar caches do painel**: zera contexto/processos/etapas/usuários da
  simulação e as conexões de publicação.

## Widgets (o grande ganho)

Navegue no **portal real** pela porta local: página, dados, sessão e WCMAPI
são os do servidor — mas o JS/CSS das widgets do projeto
(`wcm/widget/<code>/src/main/webapp/resources/`) é servido da sua máquina.
Salvou, recarregou, mudou. **Sem deploy de WAR, sem esperar o servidor
descompactar, sem limpeza de cache.** Vale também para widgets-biblioteca
(JS/CSS compartilhados por outras widgets).

- O context-root vem do `jboss-web.xml` da widget (fallback: nome da pasta).
- A query de cache-busting (`?v=…`) e o sufixo de idioma que o portal
  acrescenta (`ramais_pt_BR.js` → `ramais.js`) são resolvidos.
- Arquivo que não existe localmente (ex.: bundles gerados pelo servidor)
  segue para o servidor — o portal nunca quebra por causa do map-local.
- **O markup do `view.ftl` também é local**: o portal envelopa cada instância
  com `id="_instance_<id>_" appcode="<code>"`, e o proxy troca a saída
  renderizada pela do seu `view.ftl` (substituindo `${instanceId}` e
  removendo comentários `<#-- -->`). Salvou o `.ftl` → recarregou → mudou,
  sem deploy. Limite: template com FreeMarker de verdade (outras `${…}`,
  `<#if>`, `<@macro>`) mantém o render do servidor, com um aviso — o
  renderizador local não executa FreeMarker.
- `edit.ftl`, `.properties` e `application.info` seguem server-side: mudar
  neles não recarrega (recarregar mentiria que a mudança apareceu) — sai um
  aviso pedindo `fluigcli widget export <code>`.

### Widgets SPA (templates vue/react do `widget new`)

Widget com `package.json` na raiz é tratada como SPA compilada:

- **`--npm-watch`**: o dev server roda o `npm run watch` de cada widget SPA
  do projeto (saída no log com o prefixo `[<code>]`) e encerra tudo junto no
  Ctrl+C — desenvolvimento no portal real com **um comando só**. Widget sem
  `node_modules/` é pulada com aviso (rode `npm install` e reinicie); sem a
  flag, rode o `npm run watch` você mesmo.
- Editar a **fonte** da SPA (`src/vue/`, `src/react/`, `vite.config.ts`...)
  não dispara reload nem aviso: quem recarrega é a escrita do **bundle** em
  `src/main/webapp/resources/` feita pelo Vite (js + css na mesma rajada =
  um reload só, pelo debounce).
- Na largada, o dev avisa **bundle ausente ou desatualizado** (fonte mais
  nova que o js compilado) — sem isso o portal serviria o js velho e a
  mudança "não apareceria".
- `node_modules/` fica fora da observação (um `npm install` com o dev no ar
  não estoura o limite de watches do SO nem gera reloads).

## Formulários

`/_dev/forms/` lista os formulários do projeto; cada um tem preview local
equivalente ao modo "novo registro". Como a origem é a mesma do proxy, os
caminhos absolutos que os formulários usam (`/style-guide/...`,
`/portal/resources/js/...`, `/webdesk/vcXMLRPC.js`) resolvem no servidor real
com a sessão injetada — **`DatasetFactory` funciona com dados reais**, sem
publicar nada.

O preview **emula o render de formulários do Fluig 2.0**: ao servir um
formulário numa solicitação, o servidor 2.0 reescreve o style guide legado
(`fluig-style-guide.min.css`, descontinuado — responde 404) para o tema novo
(`fluig-style-guide-flat.min.css`) e injeta no `<head>` o runtime
(`forms.js`) e os CSS do tema (flat + animalia-icons + fluig-icons) — por
isso formulários **não migrados** renderizam certo no portal. O preview
aplica as mesmas transformações, condicionadas ao servidor ter o tema novo
(num Fluig 1.x nada é alterado). É só apresentação: **os arquivos locais não
são tocados**.

### Tema claro/escuro no preview (botão 🌓)

O botão **🌓** da barra alterna o tema do preview como o portal 2.0 faz —
trocando a classe `theme-dark` no `<html>`, que vira todas as variáveis
`--fs-*` do CSS flat. É o jeito mais rápido de **ver** o que o audit aponta:
cores fixas (`#hex`) não acompanham a troca e ficam "presas" no tema antigo.
Sem preferência salva, o preview segue o tema do sistema; a escolha persiste
entre recarregamentos (localStorage). A barra e as janelas do fluigcli usam
sempre o **tema oposto** ao do formulário (form claro → painel escuro, e
vice-versa) — contraste constante, sem se confundir com o conteúdo.

### Auditoria de Style Guide no preview (botão 🎨)

Cada preview roda automaticamente o [`fluigcli audit`](audit.md) na pasta do
formulário e mostra o veredito no botão **🎨** da barra: ponto **verde**
(nenhuma pendência), **amarelo** (só avisos) ou **vermelho** (erros). O clique
abre o painel com os achados — regra, arquivo:linha, mensagem e a sugestão de
correção (inclusive qual variável do tema usar no lugar de uma cor fixa, e o
que o `audit --fix` resolve sozinho). Como salvar um arquivo recarrega o
preview, **a auditoria reexecuta a cada salvamento** — dá para corrigir e ver
a lista encolher em tempo real. Respeita o `.fluigcli/audit.json` do projeto.

### Simulação de processo (painel flutuante)

Formulário de processo costuma "sumir" em preview: o `events/displayFields.js`
— que roda **no servidor** quando o portal renderiza o form numa solicitação —
lê as variáveis de workflow (`getValue("WKNumState")`, `WKUser`, …) e preenche
campos que o JS do formulário usa para mostrar/esconder as seções de cada
etapa. Sem processo, nada disso acontece.

O preview resolve executando o **displayFields local no navegador**, com a API
server-side emulada:

- `getValue("WK…")` lê do **painel de simulação** (botão flutuante no canto da
  página): etapa (`WKNumState`), modo (`ADD`/`MOD`/`VIEW`), usuário (`WKUser`,
  um seletor com todos os usuários do servidor pelo nome — ativos primeiro,
  o seu já selecionado; a lista é carregada uma vez e fica em cache) e
  variáveis extras (`CHAVE=valor`).
- `form.setValue/getValue/getFormMode/setEnabled` operam no DOM do preview;
  `DatasetFactory` usa os **datasets reais** pela sessão do proxy.
- **Com o formulário vinculado** (`fluigcli form link`), o processo é
  detectado sozinho (pelo `formId` das versões) e as **etapas reais aparecem
  pelo nome** — escolha "Revisar Justificativa" em vez de decorar o número.
  Sem vínculo, escolha o processo na lista ou digite o número direto.
- A escolha fica salva por formulário (localStorage) e sobrevive ao reload —
  inclusive o live reload ao salvar o próprio `displayFields.js`.
- O painel mostra o que o evento fez (leituras, `setValue`, avisos). Além do
  básico (`setValue/getValue/getFormMode/setEnabled/getMobile`), a emulação
  cobre `setVisibleById`, `getChildrenIndexes`, `getCardData`, o `log.*`
  (vai para o console do navegador e para o relatório do painel), o
  `fluigAPI.getUserService().getCurrent()` (dados reais do usuário simulado,
  via dataset `colleague`) e o interop
  Java comum dos eventos (`new java.util.HashMap()`/`ArrayList`,
  `keySet().iterator()`, `importClass` das classes simuladas). Classe Java
  fora disso (ex.: `java.text.SimpleDateFormat`) não roda no navegador: o
  painel mostra o erro com o nome da classe e o form fica como no preview
  cru.
- **Tabelas pai×filho funcionam no preview** (`wdkAddChild`/
  `fnWdkRemoveChild`): o preview replica a transformação do render (marca a
  linha-modelo de cada `table[tablename]`) e carrega a **máquina real do
  servidor** (`wdkdetail.js`) pelo proxy — incluir/remover linhas se comporta
  como no portal. Em servidor sem o arquivo (Fluig antigo), entra uma
  emulação local equivalente.
- **Selects declarativos de dataset são populados** (`<select
  dataset="dsX" datasetkey="..." datasetvalue="..." addBlankLine="true">`):
  o preview consulta o dataset real (via proxy) e gera os `<option>` como o
  render do servidor — opção vazia primeiro quando `addBlankLine`, depois as
  linhas na ordem do dataset. Falha na consulta vira aviso no painel.
- **Barra de ferramentas do preview** (canto da página; tooltips em todos os
  botões; posição direita/centro/esquerda alternável no botão ⇔ e
  persistida):
  - **⚙ Simulação** — o cartão de contexto do processo (acima), incluindo o
    toggle `getMobile() = celular` para eventos que se adaptam ao app.
  - **💾 Salvar** — um clique valida na hora: roda o
    `events/validateForm.js` local sobre os valores preenchidos no preview
    (incluindo linhas de tabelas pai×filho) e mostra o resultado num
    diálogo: o `throw` da validação aparece como no portal (HTML
    renderizado); sucesso informa que o Fluig gravaria. **Nada é gravado.**
  - **▶ Enviar etapa** — pergunta a **próxima etapa (WKNextState)** (etapas
    reais pelo nome ou número) e valida o envio; se o formulário definir
    `beforeSendValidate(numState, nextState)` client-side, ele roda antes,
    como no portal.
  - **🚀 Publicar** — o `fluigcli form export` em forma de diálogo, no
    padrão do Fluig: escolha o **servidor** (qualquer um cadastrado; o
    conectado vem selecionado) e o **formulário de destino** — o do vínculo
    (forms.json) vem pré-selecionado. **Atualizando**, o diálogo mostra o
    dataset e o campo descritor do servidor (ajustáveis) e a versão —
    *manter a atual* é o padrão. **Criando**, sem escolha de versão: nome
    (sugerido = pasta), dataset sugerido como `ds_{{nome}}`, **pasta do GED
    navegável** (seletor com as pastas reais do servidor; o id também pode
    ser digitado) e armazenamento (tabelas de banco — recomendado — ou
    tabela única). Nos dois casos o **campo descritor é um seletor com os
    campos do próprio formulário**. O vínculo local é atualizado após
    publicar. Credencial: sessão em cache/keyring/env — sem nenhuma, o
    diálogo pede a senha (que trafega apenas até o dev server local).
    **Produção** exige digitar o nome exato do servidor, como a trava do CLI.
  - **📱 Tela** — alterna num clique: livre → celular (375px) → tablet
    (768px). O formulário abre numa **moldura de dispositivo com iframe**
    (`?screen=phone|tablet`) — iframe tem viewport próprio, então as media
    queries do grid disparam e as colunas quebram linha de verdade (limitar
    por container não dispara). Largura visual apenas — o `getMobile()` é
    simulado na Simulação, pois re-executa o evento.
  - **↗ Abrir no Fluig** — abre numa aba o render **real** do formulário
    (streamcontrol, via proxy autenticado) para comparar com o preview;
    requer o processo escolhido na Simulação.
  - **⌂ Índice** e **🧹 Limpar** — volta a `/_dev/forms/` / recarrega o
    preview zerando os campos.

  Eventos de processo (`beforeTaskSave` etc.) ficam fora da simulação — para
  o ciclo real, use `fluigcli watch`.

Para testar o formulário dentro do processo de verdade (bindings de card,
anexos, movimentação), continue com o `fluigcli watch` + F5.

## Datasets

`/_dev/datasets/` (tile **Datasets** no dashboard) é a bancada de consulta de
datasets — o ciclo *criei/editei um dataset → quero ver o que ele traz*, sem
sair do navegador. É **só leitura**, direto no servidor conectado pelo proxy
(mesma sessão autenticada); nada é publicado.

- **Escolha o dataset** num campo com busca (id ou descrição; badge de tipo,
  `CUSTOM` em verde, `inativo` em vermelho). O ↻ recarrega a lista.
- **Consultar** executa e mostra o resultado numa tabela com cabeçalho fixo;
  o rodapé traz o **nº de linhas/colunas** e a **duração** da consulta. Célula
  nula aparece como `null` esmaecido (distinta de texto vazio).
- **Ver como Tabela ou JSON** — o JSON é a lista de objetos `campo: valor`
  (bom para quem consome o dataset por API).
- **Copiar** joga o resultado como TSV na área de transferência (cola direto
  no Excel); **Exportar CSV** baixa um `.csv` UTF-8 (abre no Excel em PT-BR).
- **Configurar parâmetros**:
  - **Campos** — escolha as colunas do resultado (todas por padrão).
  - **Ordenar** — um campo (a API aceita só um) + ascendente/descendente.
  - **Limite** — nº de linhas no resultado (padrão `100`; `0` = sem limite,
    cuidado com datasets grandes). Quando o resultado bate o limite, sai o
    aviso "limite atingido — pode haver mais".
  - **Filtros** — quantos quiser: campo, valor inicial/final, tipo
    (Must / Must Not / Should) e "usa Like".
  - **sqlLimit** — o limite no nível do SQL (para datasets SQL), com
    inicial/final/tipo/Like, como na extensão do Studio.
- A última configuração de cada dataset (campos, ordenação, limite, filtros)
  fica salva no navegador (localStorage) e volta ao reabrir o dataset.

As colunas para os seletores vêm de uma consulta-sonda (uma linha). Datasets
que exigem um filtro obrigatório (ex.: `sqlLimit`) podem não revelar as colunas
na sonda — nesse caso o painel avisa e as colunas aparecem após a primeira
consulta.

## Segurança (por design)

- **Por padrão escuta só em `127.0.0.1`** — o proxy carrega a SUA sessão
  autenticada; quem acessa a porta age como você no Fluig. Desenvolvendo em
  servidor remoto (via SSH), use `--listen` com um endereço de rede privada
  sua — ex.: o IP da máquina na tailnet (`fluigcli dev --listen 100.x.y.z`)
  ou um túnel SSH (`ssh -L 8787:127.0.0.1:8787`). **Nunca** um IP público:
  a CLI avisa sempre que o bind sai do loopback.
- O navegador **nunca vê os cookies do Fluig**: a sessão mora no proxy; os
  `Set-Cookie` do servidor são absorvidos pelo jar da CLI.
- **Só roda em servidor `dev` ou `hml`**, como o watch — produção é recusada
  sem exceção; servidor sem ambiente marcado idem
  (`fluigcli server update <name> --env hml`).

## Detalhes

- `--port <n>` (padrão `8787`): porta do dev server.
- `--listen <addr>` (padrão `127.0.0.1`): endereço de escuta (ver Segurança).
  A reescrita de URLs usa o Host de cada requisição — acessar pelo IP da
  tailnet funciona sem configuração extra.
- `--debounce <dur>` (padrão `500ms`): espera após o salvamento antes de
  recarregar, agrupando rajadas do editor.
- `--npm-watch`: roda o `npm run watch` das widgets SPA (ver a seção acima).
- O live reload observa `forms/` e `wcm/widget/` (SSE injetado nas páginas
  HTML; nada é alterado no servidor).
- Redirects e URLs absolutas que o portal embute (ex.: `WCMAPI.serverURL`)
  são reescritos para a origem local — a navegação não "escapa" do proxy.
- `--json` não é suportado: dev é um modo interativo de longa duração.
- `dev` e `watch` se complementam: dev = feedback instantâneo local (widgets,
  layout de forms); watch = publica de verdade ao salvar (datasets, eventos,
  mecanismos, scripts de processo, forms no contexto real).
