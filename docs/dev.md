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
- `view.ftl`, `edit.ftl`, `.properties` e `application.info` são renderizados
  **no servidor**: mudar neles não recarrega (recarregar mentiria que a mudança
  apareceu) — sai um aviso pedindo `fluigcli widget export <code>`.

## Formulários

`/_dev/forms/` lista os formulários do projeto; cada um tem preview local
equivalente ao modo "novo registro". Como a origem é a mesma do proxy, os
caminhos absolutos que os formulários usam (`/style-guide/...`,
`/portal/resources/js/...`, `/webdesk/vcXMLRPC.js`) resolvem no servidor real
com a sessão injetada — **`DatasetFactory` funciona com dados reais**, sem
publicar nada. Para testar o formulário dentro do processo (bindings de card,
modos de edição), continue com o `fluigcli watch` + F5.

## Segurança (por design)

- **Escuta só em `127.0.0.1`** — o proxy carrega a SUA sessão autenticada;
  quem acessa a porta age como você no Fluig. Não exponha a porta
  (túnel/port-forward por sua conta e risco).
- O navegador **nunca vê os cookies do Fluig**: a sessão mora no proxy; os
  `Set-Cookie` do servidor são absorvidos pelo jar da CLI.
- **Só roda em servidor `dev` ou `hml`**, como o watch — produção é recusada
  sem exceção; servidor sem ambiente marcado idem
  (`fluigcli server update <name> --env hml`).

## Detalhes

- `--port <n>` (padrão `8787`): porta local.
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
