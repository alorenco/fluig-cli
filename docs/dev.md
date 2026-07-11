# fluigcli dev — servidor de desenvolvimento com live reload

Sobe um proxy local **autenticado** do servidor Fluig que serve do disco os
arquivos que você está editando — sem publicar nada. O ciclo
*editar → publicar → esperar deploy/cache → recarregar* vira só
*salvar → o navegador recarrega sozinho*:

```
$ fluigcli dev
✓ Dev server de "homolog" em http://127.0.0.1:8787 — Ctrl+C para parar.
Portal via proxy:       http://127.0.0.1:8787/portal/p/1/home
Preview de formulários: http://127.0.0.1:8787/_dev/forms/
Widgets servidas do disco (17):
  /ramais → wcm/widget/ramais/src/main/webapp
  ...
mudança em wcm/widget/ramais/resources/js/ramais.js — recarregando o navegador
```

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
  cobre `setVisibleById`, `getChildrenIndexes`, `getCardData` e o interop
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
- O live reload observa `forms/` e `wcm/widget/` (SSE injetado nas páginas
  HTML; nada é alterado no servidor).
- Redirects e URLs absolutas que o portal embute (ex.: `WCMAPI.serverURL`)
  são reescritos para a origem local — a navegação não "escapa" do proxy.
- `--json` não é suportado: dev é um modo interativo de longa duração.
- `dev` e `watch` se complementam: dev = feedback instantâneo local (widgets,
  layout de forms); watch = publica de verdade ao salvar (datasets, eventos,
  mecanismos, scripts de processo, forms no contexto real).
