# [[.Title]]

Widget Fluig criada com `fluigcli widget new --template vue[[if .Vuetify]] --vuetify[[end]]`: uma SPA
**Vue 3 + TypeScript + Vite[[if .Vuetify]] + Vuetify 3[[end]]** dentro da casca oficial de widget. Para o
servidor Fluig ela é uma widget comum (WAR com `application.info` + FTLs);
para você é um projeto Vue moderno com hot reload e build de um comando.

## Pré-requisitos

- **Node.js** (versão no `.nvmrc`). Com [nvm](https://github.com/nvm-sh/nvm):
  `nvm install && nvm use` nesta pasta. Sem nvm: instale o Node LTS de
  <https://nodejs.org>.
- **fluigcli** configurado no projeto (`fluigcli server list` deve mostrar o
  servidor de desenvolvimento).
- Java/Maven **não** são necessários.

## Estrutura

```
[[.Code]]/
├── README.md, package.json, vite.config.ts, tsconfig.json, .nvmrc
├── index.html               ← página do dev local (npm run dev) — NÃO vai pro WAR
├── src/vue/                 ← o código da SPA — NÃO vai pro WAR
│   ├── main.ts              ← ponte SuperWidget ↔ Vue (monta 1 app por instância)
│   ├── App.vue              ← componente raiz (exemplo com [[if .Vuetify]]Vuetify[[else]]style guide[[end]] + dataset)
│   ├── composables/useDataset.ts
│   └── fluig/               ← kit: dataset.ts, fluigc.ts (toast/loading), i18n.ts
└── src/main/                ← SÓ ISTO entra no WAR (fluigcli widget export)
    ├── resources/           ← application.info, view.ftl, edit.ftl, .properties
    └── webapp/
        ├── WEB-INF/         ← jboss-web.xml (context-root)
        └── resources/
            ├── js/[[.Code]].js    ← SAÍDA do build (versionada de propósito)
            ├── css/[[.Code]].css  ← SAÍDA do build
            └── images/icon.png
```

Como o empacotamento só olha `src/main/`, o toolchain inteiro fica fora do
WAR por construção. A saída do build é **versionada** para permitir
`fluigcli widget export` sem Node (ex.: numa esteira de CI).

## Desenvolvimento

Instale as dependências uma vez:

```sh
npm install
```

### Modo 1 — SPA isolada com hot reload (o dia a dia)

```sh
fluigcli dev     # terminal 1, na raiz do projeto (porta 8787)
npm run dev      # terminal 2, nesta pasta
```

Abra a URL que o Vite mostrar. A página simula o container do portal com o
**style guide real** e as chamadas `/api/*` passam pelo `fluigcli dev`, que
injeta a **sessão autenticada** — datasets e APIs respondem de verdade e
nenhuma credencial fica em arquivo. Salvou um `.vue`, a tela atualiza na hora.

### Modo 2 — dentro do portal real

```sh
fluigcli dev --npm-watch    # na raiz do projeto: um comando só
```

O dev server sobe o `npm run watch` de cada widget SPA do projeto (log
prefixado com o código da widget), serve o bundle do disco e recarrega o
navegador ao salvar. Navegue no portal pela porta local para validar a
widget no contexto real da página (outras widgets, tema, permissões). Sem a
flag, rode `npm run watch` você mesmo em outro terminal.

## Preferências por instância (edit.ftl)

No modo de edição da página o portal renderiza o `edit.ftl` (formulário
clássico, sem Vue). O botão Salvar envia os campos nomeados via
`WCMSpaceAPI.PageService.UPDATEPREFERENCES` e o `view.ftl` devolve o JSON no
atributo `data-configs` — a SPA o recebe pronto na prop `configs` do
`App.vue`. Para criar uma preferência nova: acrescente o campo no `edit.ftl`
e leia `props.configs.<nome>` no Vue (exemplo: `customTitle`).

## i18n

- O que o **servidor** renderiza (título na galeria, edit.ftl) usa os
  `.properties` de `src/main/resources` — texto não-ASCII precisa de escape
  `\uXXXX` (padrão java.util.Properties).
- O que a **SPA** mostra usa `src/vue/fluig/i18n.ts` (o idioma vem de
  `WCMAPI.getLocale()` no portal).

[[- if .Vuetify]]
## Visual: Vuetify 3

A UI vem do **Vuetify 3 via npm** — a amarra de versão das widgets antigas
(Vuetify por CDN + Vue global) não existe aqui. O `vite-plugin-vuetify` faz
tree-shaking: só os componentes usados entram no bundle. Ícones por
**@mdi/font**: strings `mdi-*` funcionam como nas widgets Vuetify antigas
(bom para conversões); a fonte vai no WAR e é servida pelo próprio Fluig,
sem CDN. Pontos de atenção:

- O **tema do Vuetify não segue o tema do portal** (o dark mode do Fluig não
  escurece o Vuetify sozinho) — se precisar, configure
  `createVuetify({ theme: ... })` no `main.ts`.
- Peso de referência do exemplo gerado: JS ~190 KB (68 gzip) + CSS ~390 KB
  (61 gzip) + fonte de ícones ~400 KB (woff2) — bem servível na intranet,
  mas maior que o template vue puro (69 KB); prefira o vue puro para widgets
  novas simples.
- O style guide do portal continua na página (`FLUIGC.toast` do kit etc.).
[[- else]]
## Visual: Fluig Style Guide

O portal carrega o CSS do style guide e o `FLUIGC` em toda página — use as
classes dele (`panel`, `form-group`, `btn`, `table`, utilitários `fs-*`) e as
variáveis `--fs-color-*` em vez de bibliotecas de UI: o **dark mode e o tema
do portal funcionam sozinhos**. Referência: `{host}/style-guide/`. Estilos
próprios ficam no `<style scoped>` dos componentes (não vazam para o portal).
[[- end]]

## Regras de ouro

- A widget pode aparecer **mais de uma vez por página** — nunca use id fixo
  no DOM; o `main.ts` já monta um app por `instanceId`. Nos FTLs, todo id
  leva `${instanceId}`.
- Chamadas à API usam a **sessão do usuário logado** — nunca coloque
  credencial, token ou URL de outro ambiente no código.
- `src/main/resources` e os FTLs raramente mudam; quando mudarem, é preciso
  republicar (`widget export`) — o live reload cobre só JS/CSS.

## Deploy

```sh
npm run build                    # typecheck + bundle em src/main/webapp/resources
fluigcli widget export [[.Code]] # empacota o WAR e publica no servidor
```

A instalação é assíncrona no Fluig (acompanhe na Central de Componentes).
Depois adicione a widget a uma página: no editor, ela aparece na categoria
**[[.Category]]** com o título "[[.Title]]" — a galeria lista por título,
não pelo código.
