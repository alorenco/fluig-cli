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

- **export** = projeto local → servidor (deploy). **Nativo** (`uploadfile`).
- **import** = servidor → projeto local. Via **fluiggersWidget** (o Fluig não
  expõe o download do pacote da widget nativamente — confirmado na Voyager 2.0.0).

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
