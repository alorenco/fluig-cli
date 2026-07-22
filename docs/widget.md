# fluigcli widget — widgets

O grupo `widget` empacota, publica e importa widgets. O layout local é este:

```
wcm/widget/<NomeWidget>/
├── pom.xml
└── src/main/
    ├── resources/          # .ftl, .properties, application.info  → WEB-INF/classes no WAR
    ├── webapp/WEB-INF/*.xml
    └── webapp/resources/   # js, css, imagens
```

- **new** cria o esqueleto local. Nada vai ao servidor.
- **export** envia o projeto local ao servidor (deploy). O comando é
  **nativo** (`uploadfile`).
- **import** traz o servidor para o projeto local. O comando usa o
  **fluigcliHelper**. O Fluig não expõe o download do pacote da widget
  nativamente. Isso foi confirmado na Voyager 2.0.0.

## `fluigcli widget new <code>`

Este comando cria `wcm/widget/<code>/` com o esqueleto completo. O esqueleto
segue o padrão oficial do Fluig. Ele é o mesmo dos samples da TOTVS. Ele traz
`application.info`, `view.ftl`/`edit.ftl` e properties de i18n (base +
pt_BR/en_US/es). Ele traz `jboss-web.xml` com o context-root. Ele traz JS no
padrão `SuperWidget` com um binding de exemplo. Ele traz CSS, ícone e um
`README.md`. O README tem o passo a passo de desenvolvimento e deploy. O README
fica na raiz da widget e **não** entra no WAR.

```sh
fluigcli widget new meu_painel --title "Meu Painel"
fluigcli widget export meu_painel   # publica quando quiser
```

- O `<code>` vira context-root, id de DOM e global JS. Use minúsculas, dígitos
  e `_`. Comece por letra. Por exemplo, `meu_painel` → global `MeuPainel`.
- Flags: `--title` (padrão: o código), `--category` (padrão: `SYSTEM`) e
  `--template`. A categoria é **texto livre**. Ela vira uma aba própria na
  galeria do editor de páginas. A galeria lista por **título**, nunca pelo
  código.
- A pasta não pode existir. Código ou template inválidos = exit 2, sem criar
  nada.
- No `--json`: `{widget, template, dir, files}`.

### Templates

- **`classic`** (padrão) — o esqueleto oficial puro, sem toolchain: FTL +
  JS `SuperWidget` + CSS. Você não instala nada. Edite e publique.
- **`react`** — igual ao `vue`, com outra camada de framework: SPA
  **React 19 + TypeScript + Vite**. As fontes ficam em `src/react/`. O
  `main.tsx` monta um root por instância. O kit vem em hooks (por exemplo,
  `useDataset`). Os estilos ficam em `app.css`, sempre prefixados com o
  container. O React não tem CSS com escopo automático. Todo o resto (casca,
  dev, build e deploy) é idêntico ao template vue abaixo.
- **`vue`** — SPA **Vue 3 + TypeScript + Vite** dentro da casca oficial:

  ```sh
  fluigcli widget new meu_painel --template vue --title "Meu Painel"
  cd wcm/widget/meu_painel
  npm install && npm run build
  fluigcli widget export meu_painel
  ```

  - O código da SPA fica em `src/vue/`, fora do WAR. O build emite **1 JS +
    1 CSS** com o nome da widget direto em `src/main/webapp/resources/`. A
    saída é versionada. Por isso, o `widget export` funciona sem Node (por
    exemplo, em CI).
  - **Dois modos de dev**. Use `npm run dev` (Vite com HMR). A página simula o
    portal com o style guide real. O proxy aponta para o `fluigcli dev`, que
    injeta a **sessão autenticada**. Nenhuma credencial fica no `.env`. Ou use
    `npm run watch` + `fluigcli dev`. A widget roda dentro do portal real, com
    live reload.
  - A SPA é multi-instância por construção. A ponte em `src/vue/main.ts` monta
    um app Vue por `instanceId`.
  - As preferências por instância vêm prontas: `edit.ftl` (formulário clássico)
    → `UPDATEPREFERENCES` → prop `configs` no `App.vue`.
  - O kit vem incluído: `useDataset` (consulta de datasets), wrappers de
    `FLUIGC.toast`/`loading` e i18n da SPA (`WCMAPI.getLocale()`).
  - O visual usa as classes do **Fluig Style Guide**. O portal já carrega o
    CSS. O dark mode funciona sozinho. Não há UI kit embutido.
  - O `README.md` gerado na widget traz o passo a passo completo:
    pré-requisitos, dev, build e deploy.
  - O deploy sai em um comando. O `fluigcli widget export <code> --build` roda
    o `npm run build` antes de empacotar. Falha de build = exit 2, e nada é
    enviado. Sem `--build`, o export avisa quando o bundle está desatualizado
    em relação à fonte.
- **`vue` + `--vuetify`** — variante do template vue com **Vuetify 3 via
  npm**. O `vite-plugin-vuetify` faz tree-shaking. Só os componentes usados
  entram no bundle. Os ícones vêm do **@mdi/font**. As strings `mdi-*`
  funcionam como nas widgets Vuetify antigas. Este é o caminho para
  **converter** widgets Vuetify presas em stack velha por CDN. Ele mantém o
  visual.

  ```sh
  fluigcli widget new meu_painel --template vue --vuetify --title "Meu Painel"
  ```

  Tudo do template vue vale aqui (dev, build e deploy). Mudam o `App.vue` de
  exemplo (componentes Vuetify) e as dependências. As fontes de ícone vão no
  WAR. O próprio Fluig as serve. As URLs no CSS são relativas, sem CDN. Pesos
  de referência: JS ~190 KB (68 gzip) + CSS ~640 KB (91 gzip) + fonte ~400 KB
  (woff2). ⚠️ O tema do Vuetify não segue o dark mode do portal sozinho.
  Configure `createVuetify({theme})` se precisar. Para widget nova sem legado
  Vuetify, prefira o template vue puro (69 KB).

## `fluigcli widget list`

Este comando lista os widgets customizados do servidor.

- Com o **fluigcliHelper** instalado, o comando usa a listagem dele. Esta
  listagem é completa. Ela traz o arquivo `.war` de cada widget. O `widget
  import` usa esse arquivo.
- Sem o helper, o comando cai para a **API nativa**
  (`page-management/applications`) com um aviso. A listagem funciona, mas
  **pode omitir widgets**. Isso foi validado na homologação: 3 de 28 não
  aparecem, embora instaladas. A API nativa também não traz o arquivo do
  import. No `--json`, o campo `source` indica qual fonte respondeu
  (`fluigcliHelper` ou `native`).

## `fluigcli widget import <code>... | --all`

Este comando baixa e desempacota widgets em `wcm/widget/<code>/`. Ele segue
este mapa:

| No WAR | No projeto |
|---|---|
| `resources/**` | `src/main/webapp/resources/**` |
| `WEB-INF/classes/<arq>` | `src/main/resources/<arq>` |
| `WEB-INF/classes/<pkg>/**` | `src/main/java/<pkg>/**` |
| `WEB-INF/<arq>` | `src/main/webapp/WEB-INF/<arq>` |
| `pom.xml` | `pom.xml` |

O comando preserva os arquivos binários (imagens, fontes) byte a byte.

## `fluigcli widget export <NomeWidget>`

Este comando empacota o WAR em memória (compressão STORE) a partir do layout
local. Ele publica via upload nativo. O servidor instala a widget de forma
**assíncrona**.

```sh
fluigcli widget export minhaWidget --server homolog
```

Empacotamento (local → WAR):

| No projeto | No WAR |
|---|---|
| `src/main/webapp/WEB-INF/**` | `WEB-INF/**` |
| `src/main/resources/**` | `WEB-INF/classes/**` |
| `src/main/webapp/resources/**` | `resources/**` |
