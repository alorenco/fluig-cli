# fluigcliHelper — componente auxiliar do fluigcli

WAR de widget publicado no Fluig pelo `fluigcli server install-helper`. Expõe,
sob `/fluigcliHelper/api/*`, endpoints que não existem na API nativa da
plataforma e que a CLI consome:

| Rota | Uso na CLI |
|---|---|
| `GET /api/ping` | detecção (`server test`, resolução do helper) |
| `GET /api/version` | versão do helper (`server status`, painel do dev) |
| `GET /api/widgets` | `widget list` (fonte primária) |
| `GET /api/widgets/{arquivo}.war` | `widget import` (download do pacote) |
| `GET /api/workflows/{processId}/version` | reservado (a CLI usa o SOAP nativo) |
| `PUT /api/workflows/{processId}/{version}/events` | `workflow export` (update cirúrgico de eventos) |
| `GET /api/logs` | `log files` (arquivos do `jboss.server.log.dir`) |
| `GET /api/logs/{arquivo}/tail?lines&skip&level&grep` | `log tail` (entradas agrupadas, filtro server-side) |
| `GET /api/logs/{arquivo}/read?from` | `log tail --follow` (polling por offset) |
| `GET /api/logs/{arquivo}/download` | `log download` (streaming octet-stream) |

Segurança: o container exige sessão do portal em `/api/*` (security-domain
`TOTVSTech`) e o `BaseController` restringe a administradores do tenant. Nas
rotas de log, o nome do arquivo passa por whitelist de caracteres + checagem
de containment do caminho canônico (anti-traversal), e cada download é
registrado no próprio log (auditoria).

Baseado no [fluig-widget-helper](https://github.com/fluiggers/fluig-widget-helper)
da comunidade Fluiggers (MIT) — mesmos endpoints e semântica. Desde 2026-07-18
este é o ÚNICO componente auxiliar que a CLI reconhece.

## Build

Requer JDK 11+ e Maven. O SDK do Fluig (`com.fluig:fluig-sdk-{common,api}`)
é resolvido do repositório local vendorizado em `repo/` (o Nexus público da
TOTVS passou a exigir autenticação em 2026; os jars vieram do WAR MIT da
fluiggers).

```sh
./helper/build.sh     # builda, copia o WAR versionado e atualiza o .srchash
```

Use **sempre o `build.sh`**. Ele faz o `mvn package`, copia o WAR para
`helper/fluigcliHelper.war` e atualiza o `.srchash` que o teste anti-drift
confere. Um `mvn` na mão deixa o hash velho e o teste
`TestHelperWARAtualizado` reprova.

O WAR **buildado é versionado no Git** (`helper/fluigcliHelper.war`) e
embutido no binário via `go:embed` — o release da CLI não precisa de
toolchain Java (mesmo padrão dos bundles das widgets SPA).

## Versão do helper

A versão vive num lugar só: o `<version>` do `helper/pom.xml`. O Maven resolve
esse valor no `application.version` do `src/main/resources/application.info`
(resource filtering restrito a esse arquivo — o `view.ftl` e o `edit.ftl` usam
`${...}` do FreeMarker e ficam fora do filtro). O `application.info` é o
manifesto que o Fluig lê e a fonte do `GET /api/version`.

Para subir a versão: mude o `<version>` do pom, rode `./helper/build.sh` e
commite o WAR.

⚠️ Antes de 2026-07-25 o número aparecia nos dois arquivos, e o helper 0.8.0
subiu anunciando 0.7.0 — a própria porta de versão da CLI recusou o recurso que
o servidor já tinha. Por isso a versão agora sai do pom.

A CLI lê a versão do WAR que ela embute (`helperwar.Version()`, direto do
`application.info` dentro do artefato) e a compara com a instalada no servidor
antes de publicar. Ver `docs/server.md` (install-helper).
