# fluigcli audit — Style Guide e APIs de script

Linter estático do projeto Fluig, em duas famílias de regras:

- **SG*** — conformidade com o **Fluig Style Guide 2.0**: varre `forms/` e
  `wcm/widget/` e aponta o que briga com o tema fixo da plataforma (a partir
  do Fluig 2.0 o tema não é mais personalizável).
- **FL*** — chamadas às **APIs de script do Fluig** (`hAPI`, `getValue`,
  `form.*`, `FLUIGC`, `DatasetFactory`, `docAPI`, `WCMAPI`…) validadas contra
  a referência `fluig.d.ts` embutida; cobre também `datasets/`, `events/`,
  `mechanisms/` e `workflow/scripts/`. Um typo de método vira aviso com a
  sugestão do nome mais próximo — antes de o servidor devolver um erro
  criptico (ou `null` em silêncio) em produção.

Nada é enviado ao servidor; os arquivos só mudam com `--fix`.

```sh
fluigcli audit                       # projeto inteiro (todas as pastas convencionais)
fluigcli audit forms/MeuFormulario   # só um formulário
fluigcli audit --fix                 # aplica as correções determinísticas
fluigcli audit --sync                # atualiza o catálogo do servidor antes
fluigcli audit --fail-on none --json # só relatório (CI/agentes leem o data)
```

## Regras

| Regra | Sev | O que pega | Sugestão / correção |
|---|---|---|---|
| `SG001` | aviso | referência ao CSS legado `fluig-style-guide.min.css` (404 no 2.0) | trocar para o `-flat` — **`--fix` aplica** |
| `SG002` | erro | recurso externo: `<script src>`/`<link href>`/`@import`/`url()` de CDN, Google Fonts etc. | servir do próprio WAR/servidor (nos templates SPA a dependência vem por npm) |
| `SG003` | erro | cor fixa (hex ou `rgb()`) em CSS, `<style>` embutido ou `style=` inline | a **variável do tema**: valor idêntico → variável exata (**`--fix` aplica** nos hex); cinza → a neutra mais próxima (mesmo mapa do "Check color" oficial) |
| `SG004` | aviso | `!important` em regra cujo seletor usa classe do style guide (em classe própria não é apontado) | compor com o tema numa classe própria |
| `SG005` | aviso | estilo inline (`style=`) | mover para o CSS próprio ou utilitárias `fs-*` |
| `SG006` | aviso | classe `fs-*` que não existe no catálogo do servidor (typo) | a classe mais parecida do catálogo |
| `SG007` | aviso | `alert()`/`confirm()`/`prompt()` nativos em JS de widget/form e `<script>` (eventos de formulário, que rodam no servidor, ficam de fora) | `FLUIGC.toast` / `FLUIGC.message.*` |
| `FL001` | aviso | método `hAPI.*` que não existe na referência (provável typo) | o método mais parecido do `fluig.d.ts` |
| `FL002` | aviso | variável `WK*` desconhecida em `getValue()` — o Fluig devolve `null` **em silêncio** | a variável mais parecida (`WKNumState`, `WKUser`…) |
| `FL003` | aviso | método `form.*` que não existe no FormController (só nos eventos de formulário, onde `form` é garantido) | o método mais parecido |
| `FL004` | aviso | membro inexistente em `FLUIGC`, `DatasetFactory`, `DatasetBuilder`, `docAPI`, `WCMAPI`, `fluigAPI`, `customHTML` (inclui os aninhados, ex.: `FLUIGC.message.*`) | o membro mais parecido |

As regras FL* usam a referência `fluig.d.ts` embutida (fork do
[fluig-declaration-type](https://github.com/fluiggers/fluig-declaration-type)
da comunidade, completado pelo fluigcli com APIs validadas no produto). Como
nenhuma referência é exaustiva, os achados FL* são **avisos**: um typo de
verdade se corrige no código; uma API real que falte na referência é caso de
silenciar via `severity`/`ignore` — e de abrir issue para entrar no arquivo.

## `--fix` (correções determinísticas)

Aplica **apenas** o que não tem ambiguidade: SG001 (caminho legado → flat) e
os SG003 de **hex com valor idêntico** a uma variável do tema (o render em
light não muda; o dark passa a funcionar). Cinzas aproximados, `rgb()` e o
resto continuam manuais — o relatório pós-fix mostra o que sobrou. Cada
achado corrigível traz o campo `fix` no `--json`. Confira com `git diff`.

Por que cor fixa é erro: o tema 2.0 troca os valores das variáveis entre os
modos light/dark — um `#fff` fixo fica branco nos dois e quebra o dark mode.

## Catálogo

As classes válidas (~2.500) e as variáveis de tema (`--fs-color-*`) vêm
**embutidas no binário**, extraídas do CSS real de um Fluig 2.0. Com `--sync`
o catálogo é atualizado do servidor alvo na hora (o style guide é público,
não requer login); se o servidor não responder, cai no embutido com aviso.

## Exceções (`.fluigcli/audit.json`)

Arquivos vendorados minificados (`*.min.*` ou linha única gigante) e os
bundles gerados de widgets SPA (`widget new --template vue/react`) são
ignorados automaticamente. Para excluir outros caminhos:

```json
{
  "ignore": [
    "wcm/widget/legado_terceiro/",
    "forms/Formulario Congelado/",
    "*.snapshot.css"
  ],
  "severity": {
    "SG005": "off",
    "SG001": "error"
  }
}
```

Cada entrada de `ignore` casa por caminho exato, prefixo de pasta (termina em
`/`) ou glob no caminho/nome do arquivo; `severity` muda o nível por regra
(`error`, `warning`) ou a desliga (`off`). O `--json` lista o que foi ignorado.

## No preview do `dev`

O [`fluigcli dev`](dev.md) roda esta auditoria automaticamente no preview de
cada formulário (botão **🎨** da barra: verde/amarelo/vermelho) e reexecuta a
cada salvamento — os achados aparecem na tela, com as mesmas sugestões.

## Exit code e CI

Por padrão a auditoria **reprova com exit 1** quando há achados de nível
`error` (`--fail-on error`). `--fail-on warning` é o modo estrito;
`--fail-on none` sempre retorna 0 (só relatório). No `--json`, o envelope
reprovado vem com `error.code = AUDIT_FAILED` e o `data` completo
(`findings[]` com regra/arquivo/linha/sugestão, `counts`, `scanned`,
`ignored`) — ideal para agentes de IA corrigirem em loop e para gates de CI.
