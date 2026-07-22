# fluigcli audit — Style Guide e APIs de script

O comando `audit` é o linter estático do projeto Fluig. Ele tem duas famílias
de regras:

- **SG*** — conformidade com o **Fluig Style Guide 2.0**. Estas regras varrem
  `forms/` e `wcm/widget/`. Elas apontam o que conflita com o tema fixo da
  plataforma. A partir do Fluig 2.0 o tema não é mais personalizável.
- **FL*** — chamadas às **APIs de script do Fluig** (`hAPI`, `getValue`,
  `form.*`, `FLUIGC`, `DatasetFactory`, `docAPI`, `WCMAPI` e outras). O comando
  valida estas chamadas contra a referência `fluig.d.ts` embutida. Estas regras
  cobrem também `datasets/`, `events/`, `mechanisms/` e `workflow/scripts/`. Um
  typo de método vira aviso. O aviso traz a sugestão do nome mais próximo. Assim
  você corrige o typo antes de o servidor devolver um erro críptico ou um `null`
  em silêncio em produção.

O comando não envia nada ao servidor. Os arquivos só mudam com `--fix`.

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

As regras FL* usam a referência `fluig.d.ts` embutida. Esta referência é um fork
do [fluig-declaration-type](https://github.com/fluiggers/fluig-declaration-type)
da comunidade. O fluigcli completou o fork com APIs validadas no produto. Nenhuma
referência é exaustiva. Por isso os achados FL* são **avisos**. Corrija no código
o typo de verdade. Uma API real que falte na referência é caso de silenciar via
`severity`/`ignore`. Neste caso, abra uma issue para a API entrar no arquivo.

## `--fix` (correções determinísticas)

O `--fix` aplica **apenas** o que não tem ambiguidade. Ele corrige o SG001
(caminho legado → flat). Ele corrige também os SG003 de **hex com valor idêntico**
a uma variável do tema. Neste caso, o render em light não muda e o dark passa a
funcionar. Os cinzas aproximados, os `rgb()` e o resto continuam manuais. O
relatório pós-fix mostra o que sobrou. Cada achado corrigível traz o campo `fix`
no `--json`. Confira o resultado com `git diff`.

A cor fixa é erro por este motivo: o tema 2.0 troca os valores das variáveis entre
os modos light e dark. Um `#fff` fixo fica branco nos dois modos. Por isso ele
quebra o dark mode.

## Catálogo

As classes válidas (~2.500) e as variáveis de tema (`--fs-color-*`) vêm
**embutidas no binário**. O fluigcli extrai estes dados do CSS real de um Fluig
2.0. Com `--sync` o comando atualiza o catálogo do servidor alvo na hora. O style
guide é público e não requer login. Quando o servidor não responde, o comando cai
no catálogo embutido com um aviso.

## Exceções (`.fluigcli/audit.json`)

O comando ignora automaticamente os arquivos vendorados minificados (`*.min.*` ou
linha única gigante) e os bundles gerados de widgets SPA
(`widget new --template vue/react`). Para excluir outros caminhos:

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

Cada entrada de `ignore` casa por caminho exato, por prefixo de pasta (termina em
`/`) ou por glob no caminho ou no nome do arquivo. O `severity` muda o nível por
regra (`error`, `warning`) ou desliga a regra (`off`). O `--json` lista o que o
comando ignorou.

## No preview do `dev`

O [`fluigcli dev`](dev.md) roda esta auditoria automaticamente no preview de cada
formulário. Use o botão **🎨** da barra (verde/amarelo/vermelho). O comando
reexecuta a auditoria a cada salvamento. Os achados aparecem na tela, com as
mesmas sugestões.

## Exit code e CI

Por padrão a auditoria **reprova com exit 1** quando há achados de nível `error`
(`--fail-on error`). O `--fail-on warning` é o modo estrito. O `--fail-on none`
sempre retorna 0 (só relatório). No `--json`, o envelope reprovado vem com
`error.code = AUDIT_FAILED` e o `data` completo (`findings[]` com
regra/arquivo/linha/sugestão, `counts`, `scanned`, `ignored`). Este formato é
ideal para agentes de IA corrigirem em loop e para gates de CI.
