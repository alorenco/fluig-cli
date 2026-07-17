# Fluig Style Guide 2.0 — como escrever código conforme

A partir do Fluig 2.0 o **tema é fixo** (não personalizável) e alterna entre
light e dark trocando os valores das **variáveis CSS**. Todo formulário e
widget que você escrever deve seguir estas regras — e ser conferido com
`fluigcli audit <path> --json` (corrija pelas `suggestion`/`fix` dos
`data.findings[]` e repita até exit 0).

## Regras de ouro

1. **NUNCA cor fixa** (`#hex`, `rgb()`) em CSS, `<style>` ou `style=` — sempre
   `var(--fs-color-...)`. Um `#fff` fixo fica branco também no dark mode.
2. **NUNCA recurso externo** (CDN, Google Fonts, unpkg): tudo sai do próprio
   servidor/WAR. Em widget SPA (`widget new --template vue|react`),
   dependência entra por npm e vai no bundle.
3. **Não usar** `style-guide/css/fluig-style-guide.min.css` (legado, 404 no
   2.0) — o certo é `fluig-style-guide-flat.min.css`, e em widget nem isso: o
   portal já carrega o CSS do tema em toda página.
4. **Evite `!important` sobre classes do style guide** (`panel`, `btn`,
   `fs-*`…) e **estilo inline** — componha com o tema em classes próprias.
5. **Diálogos e avisos são do FLUIGC**, nunca nativos: `FLUIGC.toast`,
   `FLUIGC.message.alert/confirm`, `FLUIGC.modal`, `FLUIGC.loading` (em vez
   de `alert()`/`confirm()`/`prompt()`).
6. Layout/utilitários: use o grid (`row`, `col-md-*`) e as classes `fs-*` do
   catálogo (~850) antes de inventar CSS.

## Variáveis do tema (valores light → dark)

Neutras (grayscale — o `fluigcli audit` sugere a mais próxima para cada cinza
fixo que encontrar):

| Variável | Light | Dark |
|---|---|---|
| `--fs-color-neutral-light-00` | `#ffffff` | `#1c1c1c` |
| `--fs-color-neutral-light-05` | `#eeeeee` | `#202020` |
| `--fs-color-neutral-light-10` | `#d9d9d9` | `#2b2b2b` |
| `--fs-color-neutral-light-20` | `#c1c1c1` | `#3b3b3b` |
| `--fs-color-neutral-light-30` | `#a1a1a1` | `#5a5a5a` |
| `--fs-color-neutral-mid-40` | `#7c7c7c` | `#7c7c7c` |
| `--fs-color-neutral-mid-60` | `#5a5a5a` | `#a1a1a1` |
| `--fs-color-neutral-dark-70` | `#3b3b3b` | `#c1c1c1` |
| `--fs-color-neutral-dark-80` | `#2b2b2b` | `#d9d9d9` |
| `--fs-color-neutral-dark-90` | `#202020` | `#eeeeee` |
| `--fs-color-neutral-dark-95` | `#1c1c1c` | `#ffffff` |
| `--fs-color-neutral-white` / `-black` | fixas | não invertem |

Marca e ação (a família brand inverte no dark; as action apontam para brand):

| Variável | Light | Dark |
|---|---|---|
| `--fs-color-brand-01-base` | `#0079b8` | `#0079b8` |
| `--fs-color-brand-01-light/lighter/lightest` | mais claro | vira escuro |
| `--fs-color-brand-01-dark/darker/darkest` | mais escuro | vira claro |
| `--fs-color-action-default` | brand-01-base | brand-01-dark |
| `--fs-color-action-hover` | brand-01-dark | brand-01-darker |
| `--fs-color-action-pressed/focus/disabled` | ver css.html | — |

Feedback (cada família tem a escala lightest…darkest): `--fs-color-positive-*`
(`base #107048`), `--fs-color-negative-*` (`#be3e37`), `--fs-color-warning-*`
(`#efba2a`), `--fs-color-info-*` (`#23489f`). Sombras: `--fs-shadow-sm/md/lg/xl`.

## Substituições que mais aparecem em código legado

| Encontrou | Escreva |
|---|---|
| `#fff` / `#ffffff` / `rgb(255,255,255)` | `var(--fs-color-neutral-light-00)` |
| `#000` (texto) | `var(--fs-color-neutral-dark-95)` |
| `#eee` / `#efefef` / `#f3f3f3` | `var(--fs-color-neutral-light-05)` |
| `#ccc` / `#c1c1c1` (bordas) | `var(--fs-color-neutral-light-20)` |
| `#333` | `var(--fs-color-neutral-dark-70)` |
| azul de link/botão da casa | `var(--fs-color-action-default)` (+ `-hover`) |
| verde/vermelho/amarelo de status | `var(--fs-color-positive/negative/warning-base)` |
| `<link>` de Google Fonts | remova — a fonte do tema vem de `--fs-font-family` |
| Vuetify/Vue/axios por CDN | `widget new --template vue [--vuetify]` (npm no bundle) |
| `alert('...')` | `FLUIGC.toast({message:'...', type:'success\|danger'})` |
| `confirm('...')` | `FLUIGC.message.confirm({message, title, labelYes, labelNo}, cb)` |

## Referência viva

A documentação canônica é o próprio servidor (público, sem login):
`{host}/style-guide/index.html` (css, javascript, components, chart,
miscellaneous). O catálogo que o `fluigcli audit` usa vem de
`{host}/style-guide/css/fluig-style-guide-flat.min.css` (`--sync` atualiza).
