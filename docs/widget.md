# fluigcli widget — widgets

Empacota/publica e importa widgets. Layout local:

```
wcm/widget/<NomeWidget>/
├── pom.xml
└── src/main/
    ├── resources/          # .ftl, .properties, application.info  → WEB-INF/classes no WAR
    ├── webapp/WEB-INF/*.xml
    └── webapp/resources/   # js, css, imagens
```

- **new** = scaffold local (cria o esqueleto; nada vai ao servidor).
- **export** = projeto local → servidor (deploy). **Nativo** (`uploadfile`).
- **import** = servidor → projeto local. Via **fluiggersWidget** (o Fluig não
  expõe o download do pacote da widget nativamente — confirmado na Voyager 2.0.0).

## `fluigcli widget new <code>`

Cria `wcm/widget/<code>/` com o esqueleto completo no padrão oficial do Fluig
(o mesmo dos samples da TOTVS): `application.info`, `view.ftl`/`edit.ftl`,
properties de i18n (base + pt_BR/en_US/es), `jboss-web.xml` com o
context-root, JS no padrão `SuperWidget` (com um binding de exemplo), CSS,
ícone e um `README.md` com o passo a passo de desenvolvimento e deploy — o
README fica na raiz da widget e **não** entra no WAR.

```sh
fluigcli widget new meu_painel --title "Meu Painel"
fluigcli widget export meu_painel   # publica quando quiser
```

- `<code>` vira context-root, id de DOM e global JS: minúsculas, dígitos e
  `_`, começando por letra (ex.: `meu_painel` → global `MeuPainel`).
- Flags: `--title` (padrão: o código), `--category` (padrão: `SYSTEM` —
  categoria é **texto livre** e vira uma aba própria na galeria do editor de
  páginas; a galeria lista por **título**, nunca pelo código) e `--template`.
- A pasta não pode existir; código/template inválidos = exit 2, sem criar nada.
- No `--json`: `{widget, template, dir, files}`.

### Templates

- **`classic`** (padrão) — o esqueleto oficial puro, sem toolchain: FTL +
  JS `SuperWidget` + CSS. Nada para instalar; edite e publique.
- **`vue`** — SPA **Vue 3 + TypeScript + Vite** dentro da casca oficial:

  ```sh
  fluigcli widget new meu_painel --template vue --title "Meu Painel"
  cd wcm/widget/meu_painel
  npm install && npm run build
  fluigcli widget export meu_painel
  ```

  - O código da SPA fica em `src/vue/` (fora do WAR); o build emite **1 JS +
    1 CSS** com o nome da widget direto em `src/main/webapp/resources/` — a
    saída é versionada, então o `widget export` funciona sem Node (ex.: CI).
  - **Dois modos de dev**: `npm run dev` (Vite com HMR; a página simula o
    portal com o style guide real e o proxy aponta para o `fluigcli dev`,
    que injeta a **sessão autenticada** — nenhuma credencial em `.env`) e
    `npm run watch` + `fluigcli dev` (a widget dentro do portal real, com
    live reload).
  - Multi-instância por construção: a ponte em `src/vue/main.ts` monta um
    app Vue por `instanceId`.
  - Preferências por instância prontas: `edit.ftl` (formulário clássico) →
    `UPDATEPREFERENCES` → prop `configs` no `App.vue`.
  - Kit incluído: `useDataset` (consulta de datasets), wrappers de
    `FLUIGC.toast`/`loading` e i18n da SPA (`WCMAPI.getLocale()`).
  - Visual: classes do **Fluig Style Guide** (o portal já carrega o CSS;
    dark mode funciona sozinho) — sem UI kit embutido.
  - O `README.md` gerado na widget traz o passo a passo completo
    (pré-requisitos, dev, build, deploy).

## `fluigcli widget list`

Lista os widgets customizados do servidor.

- Com a **fluiggersWidget** instalada, usa a listagem dela: completa e com o
  arquivo `.war` de cada widget (o que o `widget import` usa).
- Sem ela, cai para a **API nativa** (`page-management/applications`) com um
  aviso: a listagem funciona, mas **pode omitir widgets** (validado na
  homologação: 3 de 28 não aparecem, embora instaladas) e não traz o arquivo
  do import. No `--json`, o campo `source` indica qual fonte respondeu
  (`fluiggersWidget` ou `native`).

## `fluigcli widget import <code>... | --all`

Baixa e desempacota widgets em `wcm/widget/<code>/`, seguindo o mapa:

| No WAR | No projeto |
|---|---|
| `resources/**` | `src/main/webapp/resources/**` |
| `WEB-INF/classes/<arq>` | `src/main/resources/<arq>` |
| `WEB-INF/classes/<pkg>/**` | `src/main/java/<pkg>/**` |
| `WEB-INF/<arq>` | `src/main/webapp/WEB-INF/<arq>` |
| `pom.xml` | `pom.xml` |

Arquivos binários (imagens, fontes) são preservados byte a byte.

## `fluigcli widget export <NomeWidget>`

Empacota o WAR em memória (compressão STORE) a partir do layout local e publica
via upload nativo. A instalação da widget é **assíncrona** no servidor.

```sh
fluigcli widget export minhaWidget --server homolog
```

Empacotamento (local → WAR):

| No projeto | No WAR |
|---|---|
| `src/main/webapp/WEB-INF/**` | `WEB-INF/**` |
| `src/main/resources/**` | `WEB-INF/classes/**` |
| `src/main/webapp/resources/**` | `resources/**` |
