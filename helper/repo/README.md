# SDK do Fluig vendorizado

Repositório Maven local com o SDK que o `fluigcliHelper` compila contra. O
`pom.xml` aponta para cá (`file://${project.basedir}/repo`).

## Por que os jars estão no repositório

O Nexus público da TOTVS (`nexus.fluig.com`) passou a **exigir autenticação** em
2026 — responde 401 até em GET de artefato. Sem os jars aqui, ninguém consegue
buildar o helper a partir de um clone limpo, e o job de CI que confere o WAR
contra a fonte (`helper/verify-war.sh`) não rodaria.

## Proveniência

Os jars foram **extraídos do WAR do
[fluig-widget-helper](https://github.com/fluiggers/fluig-widget-helper)** da
comunidade Fluiggers, distribuído sob licença MIT. Os `.pom` ao lado foram
escritos à mão, com o mínimo para o Maven resolver as dependências.

⚠️ **Não há checksum oficial da TOTVS para conferir.** O Nexus fechado impede a
verificação contra a origem. Os hashes abaixo servem para detectar alteração
**dentro deste repositório**, não para atestar autenticidade na origem.

| Artefato | sha256 |
|---|---|
| `com/fluig/fluig-sdk-api/1.8.2/fluig-sdk-api-1.8.2.jar` | `1b50d2943f68ee5feac44a328a88000480a27c530a37fb6dabfa25da1396a763` |
| `com/fluig/fluig-sdk-common/1.8.2/fluig-sdk-common-1.8.2.jar` | `a86c07740cfb5423f9f799ec77e84cc0c7ff6bb640cfe17496011438fb48311f` |

Registrados em 2026-07-27 (ROADMAP §2.11-F). Para conferir:

```sh
sha256sum helper/repo/com/fluig/*/*/*.jar
```

## Escopo no WAR publicado

Os dois jars vão **dentro** do WAR (`WEB-INF/lib/`), porque o SDK não é provido
pelo servidor no classloader do webapp. Toda outra dependência do helper é
`provided` (javaee-api, slf4j) e vem do WildFly.
