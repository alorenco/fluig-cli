# fluigcli layout — layouts WCM

O grupo `layout` lista e publica layouts WCM. Um layout é a página-molde do
portal. Ele define os slots onde os widgets entram. O layout local é este:

```
wcm/layout/<código>/
└── src/main/
    ├── resources/          # application.info, layout.ftl, .properties → WEB-INF/classes no WAR
    ├── webapp/WEB-INF/     # web.xml, jboss-web.xml (context-root)
    └── webapp/resources/   # js, css, imagens
```

A estrutura é a mesma de um widget. A diferença está no `application.info`:
um layout declara `application.type=layout`.

- **list** mostra os layouts do servidor. O comando usa a API nativa de
  page-management.
- **export** envia o projeto local ao servidor (deploy). O comando é
  **nativo** (`uploadfile`), o mesmo caminho do `widget export`.

Esta versão não tem `layout import` nem `layout new`. O Fluig não informa o
arquivo `.war` de um layout pela API nativa. Por isso o import depende do
fluigcliHelper e fica para um ciclo futuro.

## `fluigcli layout list [--all]`

Este comando lista os layouts customizados do servidor.

```sh
fluigcli layout list --server homolog
```

- Por padrão, o comando mostra só os layouts **customizados**. Eles são os
  layouts que a CLI publica.
- Com `--all`, o comando inclui os layouts internos da plataforma (Amplo,
  Público, Constante...). A tabela ganha a coluna `Origem`, com os valores
  `customizado` e `plataforma`.
- No `--json`: `{layouts: [{code, title, internal}], all}`.

## `fluigcli layout export <código>`

Este comando empacota o WAR em memória (compressão STORE) a partir do layout
local. Ele publica via upload nativo. O servidor instala o layout de forma
**assíncrona**.

```sh
fluigcli layout export intranet --server homolog
```

Empacotamento (local → WAR):

| No projeto | No WAR |
|---|---|
| `src/main/webapp/WEB-INF/**` | `WEB-INF/**` |
| `src/main/resources/**` | `WEB-INF/classes/**` |
| `src/main/webapp/resources/**` | `resources/**` |

Antes de empacotar, a CLI confere o `application.info`:

- Pasta sem `src/main/resources/application.info` = exit code 2. O servidor
  aceitaria o WAR e falharia depois, em silêncio.
- `application.type` diferente de `layout` = exit code 2. A mensagem aponta o
  `widget export`. Isso evita publicar um widget como layout por engano.

Republicar um layout que já existe no servidor é a atualização normal. O
comando não pede `--force` para isso.

### Guarda de colisão com widget

Antes de publicar, a CLI verifica se o código do layout já existe no servidor
como **widget**. O deploy nativo do WCM identifica o destino só pelo nome do
arquivo (`<código>.war`). Por isso um layout com o código de um widget
**sobrescreve o WAR do widget**. Esta guarda é o espelho da
[guarda do `widget export`](widget.md#guarda-de-colisão-com-layout).

Neste caso o comando recusa a publicação com exit code 2:

```sh
$ fluigcli layout export alertas --server producao
ERRO: o código "alertas" já existe no servidor como WIDGET ("Alertas").
Publicar o layout sobrescreveria o WAR do widget. Renomeie o layout ou
publique com --force.
```

Escolha uma das duas saídas:

- **Renomeie o layout.** É a opção correta quando o código coincidiu por acaso.
- **Use `--force`.** Escolha esta opção só quando quiser substituir o artefato
  de propósito.

A verificação falha em aberto. Se o servidor não responder a consulta, o comando
avisa e publica. A guarda protege contra um erro conhecido. Ela não impede a
publicação por indisponibilidade.

## No `deploy --plan`

O passo `{"layout": "<código>"}` publica o layout dentro de um plano de
release. Ele aceita a opção `force`. O `--dry-run` confere a pasta, o
`application.type` e a colisão com widget. Veja [deploy](deploy.md).

## Exit codes

| Código | Quando |
|---|---|
| 0 | Layout enviado. A instalação segue no servidor |
| 2 | `application.info` ausente ou com tipo diferente de `layout`. Ou código que já é widget, sem `--force` |
| 4 | Pasta `wcm/layout/<código>` não existe |
| 5 | O servidor rejeitou o upload |
