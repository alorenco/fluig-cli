# fluigcli audit — conformidade com o Style Guide

Linter estático do **Fluig Style Guide 2.0**: varre `forms/` e `wcm/widget/`
e aponta o que briga com o tema fixo da plataforma (a partir do Fluig 2.0 o
tema não é mais personalizável). **Read-only** — nada é alterado nem enviado
ao servidor.

```sh
fluigcli audit                       # projeto inteiro (forms/ + wcm/widget/)
fluigcli audit forms/MeuFormulario   # só um formulário
fluigcli audit --sync                # atualiza o catálogo do servidor antes
fluigcli audit --fail-on none --json # só relatório (CI/agentes leem o data)
```

## Regras (fase 1)

| Regra | Sev | O que pega | Sugestão |
|---|---|---|---|
| `SG001` | aviso | referência ao CSS legado `fluig-style-guide.min.css` (404 no 2.0) | trocar para o `-flat` (no render de solicitação o servidor reescreve sozinho — fora dele, não) |
| `SG002` | erro | recurso externo: `<script src>`/`<link href>`/`@import`/`url()` de CDN, Google Fonts etc. | servir do próprio WAR/servidor (nos templates SPA a dependência vem por npm) |
| `SG003` | erro | cor fixa (hex ou `rgb()`) em CSS, `<style>` embutido ou `style=` inline | a **variável do tema**: valor idêntico → variável exata; cinza → a neutra mais próxima (mesmo mapa do "Check color" oficial) |
| `SG006` | aviso | classe `fs-*` que não existe no catálogo do servidor (typo) | a classe mais parecida do catálogo |

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
  ]
}
```

Cada entrada casa por caminho exato, prefixo de pasta (termina em `/`) ou
glob no caminho/nome do arquivo. O `--json` lista o que foi ignorado.

## Exit code e CI

Por padrão a auditoria **reprova com exit 1** quando há achados de nível
`error` (`--fail-on error`). `--fail-on warning` é o modo estrito;
`--fail-on none` sempre retorna 0 (só relatório). No `--json`, o envelope
reprovado vem com `error.code = AUDIT_FAILED` e o `data` completo
(`findings[]` com regra/arquivo/linha/sugestão, `counts`, `scanned`,
`ignored`) — ideal para agentes de IA corrigirem em loop e para gates de CI.
