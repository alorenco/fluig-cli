# fluigcliHelper — componente auxiliar do fluigcli

WAR de widget publicado no Fluig pelo `fluigcli server install-helper`. Expõe,
sob `/fluigcliHelper/api/*`, endpoints que não existem na API nativa da
plataforma e que a CLI consome:

| Rota | Uso na CLI |
|---|---|
| `GET /api/ping` | detecção (`server test`, resolução do helper) |
| `GET /api/widgets` | `widget list` (fonte primária) |
| `GET /api/widgets/{arquivo}.war` | `widget import` (download do pacote) |
| `GET /api/workflows/{processId}/version` | reservado (a CLI usa o SOAP nativo) |
| `PUT /api/workflows/{processId}/{version}/events` | `workflow export` (update cirúrgico de eventos) |

Segurança: o container exige sessão do portal em `/api/*` (security-domain
`TOTVSTech`) e o `BaseController` restringe a administradores do tenant.

Baseado no [fluig-widget-helper](https://github.com/fluiggers/fluig-widget-helper)
da comunidade Fluiggers (MIT) — mesmos endpoints e semântica; o fluigcli
prefere este helper e mantém fallback para a fluiggersWidget já instalada.

## Build

Requer JDK 11+ e Maven. O SDK do Fluig (`com.fluig:fluig-sdk-{common,api}`)
é resolvido do repositório local vendorizado em `repo/` (o Nexus público da
TOTVS passou a exigir autenticação em 2026; os jars vieram do WAR MIT da
fluiggers).

```sh
mvn -f helper/pom.xml package
cp helper/target/fluigcliHelper.war helper/fluigcliHelper.war  # artefato versionado
```

O WAR **buildado é versionado no Git** (`helper/fluigcliHelper.war`) e
embutido no binário via `go:embed` — o release da CLI não precisa de
toolchain Java (mesmo padrão dos bundles das widgets SPA). Ao alterar
qualquer fonte em `helper/src`, rebuilde e atualize o WAR versionado; o
teste `TestHelperWARAtualizado` acusa drift.
