<div align="center">

<img src="docs/assets/banner.svg" width="400" alt="fluigcli — CLI não oficial para desenvolvimento TOTVS Fluig">

**TOTVS Fluig direto do terminal: importe, implante e automatize
os artefatos da plataforma.**

[![Release](https://img.shields.io/github/v/release/alorenco/fluig-cli?label=release&color=orange)](https://github.com/alorenco/fluig-cli/releases/latest)
[![CI](https://github.com/alorenco/fluig-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/alorenco/fluig-cli/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](go.mod)
[![Plataformas](https://img.shields.io/badge/plataformas-linux_·_macos_·_windows-5865a3)](https://github.com/alorenco/fluig-cli/releases)
[![Licença](https://img.shields.io/badge/licença-MIT-blue)](LICENSE)

</div>

> ⚠️ **Projeto não oficial, sem qualquer vínculo com a TOTVS.**
> "Fluig" e "TOTVS" são marcas de seus respectivos donos.

A CLI serve desenvolvedores, **agentes de IA** e pipelines de CI/CD. Ela tem
modo não-interativo, saída JSON e exit codes estáveis. A CLI cobre três
frentes:

- **Desenvolvimento.** Importe e publique datasets, formulários, eventos
  globais, mecanismos de atribuição, scripts de processo e widgets. Gere
  scaffolds com `new`. Clone um servidor existente para uma pasta vazia.
  Compare o local com o servidor com `diff`. Audite o projeto contra o Style
  Guide 2.0 e as APIs de script. Suba um dev server com live reload, preview de
  formulários com simulação de processo, explorador de processos e painel de
  logs.
- **Operação.** Consulte, inicie e movimente solicitações. Veja a fila de
  tarefas. Navegue no GED. Leia os logs do servidor sem SSH.
- **Administração.** Gerencie usuários, grupos, papéis e substitutos. Audite a
  atuação de um usuário por período. Consulte a saúde do servidor.

A lista de recursos continua a crescer.

📖 **Documentação completa:** <https://alorenco.github.io/fluig-cli/>

## Instalação

**Linux e macOS:**

```sh
curl -fsSL https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.ps1 | iex
```

O script detecta o sistema. Ele baixa a última versão de
[Releases](https://github.com/alorenco/fluig-cli/releases). Ele confere o
checksum e instala. Você também instala de forma manual. Baixe o binário da sua
plataforma direto de Releases e coloque no `PATH`. Ou compile do código-fonte
(Go ≥ 1.26):

```sh
go install github.com/alorenco/fluig-cli/cmd/fluigcli@latest
```

## Atualização

A CLI se atualiza sozinha. Ela baixa a última release, confere o checksum e
substitui o binário no lugar:

```sh
fluigcli upgrade
```

A CLI avisa ao fim de um comando quando sai uma versão nova. Ela faz isso uma
vez por dia no máximo, só em terminal interativo. Você desativa o aviso com
`FLUIGCLI_NO_UPDATE_CHECK=1`. Veja mais detalhes em
[docs/upgrade.md](docs/upgrade.md).

## Quickstart

```sh
# 1. Cadastre os servidores (a senha vai para o keyring do SO — nunca para arquivo).
#    O primeiro cadastrado vira o padrão; servidores "prod" ganham trava de escrita.
fluigcli server add --name homolog --host fluig-hml.empresa.com.br --username admin.deploy --env hml
fluigcli server add --name producao --host fluig.empresa.com.br --username admin.deploy --env prod

# 2. Teste o acesso (login + ping + dados do usuário + status da widget auxiliar)
fluigcli server test homolog

# 3. Traga os artefatos do servidor para o projeto (import = servidor → local).
#    Chegando num servidor já em uso com a pasta vazia? Clone tudo de uma vez:
fluigcli clone                # mostra o inventário e pergunta o que clonar (--all pula a pergunta)

#    ...ou pontualmente, por tipo:
fluigcli dataset import --all
fluigcli form import "Solicitação de Compras"
fluigcli event import --all
fluigcli workflow import Compras

# 4. Desenvolva com deploy automático: cada salvamento publica na homologação
#    (só roda em dev/hml; nunca cria artefato nem versão nova)
fluigcli watch

# 4b. Ou com feedback instantâneo, sem publicar nada: proxy local do portal
#     que serve o JS/CSS das widgets do disco e recarrega o navegador ao salvar
fluigcli dev

# 5. Ou no ritmo manual: confira o que mudaria e publique (export = local → servidor)
fluigcli diff
fluigcli dataset export datasets/ds_clientes.js
fluigcli workflow export workflow/scripts/Compras.beforeTaskSave.js

# 6. Na hora de ir para produção, a trava pede confirmação antes de escrever
fluigcli dataset export datasets/ds_clientes.js --server producao

# 7. Em scripts, CI e agentes de IA: --json + exit codes estáveis
fluigcli diff --json | jq '.data.counts'
```

## Comandos

**Desenvolvimento**. Importe, publique e acompanhe os artefatos da plataforma:

| Grupo | Comandos | Doc |
|---|---|---|
| `clone` | clona os artefatos de um servidor para o projeto local | [docs/clone.md](docs/clone.md) |
| `dataset` | `new` `list` `import` `export` `query` `enable` `disable` `history` `restore` `delete` | [docs/dataset.md](docs/dataset.md) |
| `db` | `query` `datasources`. SQL de leitura de diagnóstico (via fluigcliHelper) | [docs/db.md](docs/db.md) |
| `event` | `new` `list` `import` `export` `delete` | [docs/event.md](docs/event.md) |
| `mechanism` | `new` `list` `import` `export` `delete` | [docs/mechanism.md](docs/mechanism.md) |
| `form` | `new` `list` `import` `export` `link` `records` | [docs/form.md](docs/form.md) |
| `workflow` | `new-script` `list` `version` `versions` `import` `export` `publish` `diff` | [docs/workflow.md](docs/workflow.md) |
| `widget` | `new` `list` `import` `export` | [docs/widget.md](docs/widget.md) |
| `diff` | `diff [<path>...]`. Compara o local com o servidor | [docs/diff.md](docs/diff.md) |
| `audit` | linter do projeto Style Guide 2.0 e typos de API | [docs/audit.md](docs/audit.md) |
| `watch` | publica ao salvar. | [docs/watch.md](docs/watch.md) |
| `dev` | dev server com live reload. | [docs/dev.md](docs/dev.md) |

**Operação**. Use a plataforma no dia a dia. Faça consultas, automação e integrações:

| Grupo | Comandos | Doc |
|---|---|---|
| `request` | `list` `show` `start` `move` `assignees` `attachments` | [docs/request.md](docs/request.md) |
| `task` | `list` | [docs/task.md](docs/task.md) |
| `document` | `list` `download` `upload` `mkdir` `delete` | [docs/document.md](docs/document.md) |
| `log` | `files` `tail` (`--follow`, `--level`, `--grep`) `download` | [docs/log.md](docs/log.md) |

**Administração**. Gerencie a plataforma. Requer usuário com privilégio administrativo:

| Grupo | Comandos | Doc |
|---|---|---|
| `user` | `list` `show` `create` `update` `activate` `deactivate` `audit` | [docs/user.md](docs/user.md) |
| `group` | `list` `show` `create` `update` `delete` `users` `add-user` `remove-user` | [docs/group.md](docs/group.md) |
| `role` | `list` `show` `create` `update` `delete` `users` `add-user` `remove-user` | [docs/role.md](docs/role.md) |
| `replacement` | `list` `show` `create` `update` `delete` | [docs/replacement.md](docs/replacement.md) |

**Configuração**:

| Grupo | Comandos | Doc |
|---|---|---|
| `server` | `add` `list` `use` `update` `remove` `test` `status` `logout` `install-helper` | [docs/server.md](docs/server.md) |

**Adicionais**:

| Grupo | Comandos | Doc |
|---|---|---|
| `skill` | `install` `show` | [docs/skill.md](docs/skill.md) |
| — | `version` `upgrade` `completion` | [docs/upgrade.md](docs/upgrade.md) |


## Uso por agentes de IA e CI/CD

- `--json`: o stdout recebe **exatamente um** documento JSON com envelope fixo
  (`{ok, command, server, data, error}`). Todo log vai para o stderr.
- `--json` implica modo não-interativo. Fora de um TTY, o modo não-interativo é
  automático.
- Senha sem prompt: use a variável `FLUIGCLI_PASSWORD` ou a opção `--password-stdin`.
- A CLI reaproveita a sessão entre execuções (cache em disco). Por isso, rodar
  vários comandos em sequência não faz login a cada vez. Você desativa o cache
  com `--no-session-cache`.

Exit codes estáveis. A CLI os documenta e os cobre por teste:

| Código | Significado |
|---|---|
| 0 | Sucesso total |
| 1 | Erro genérico/inesperado |
| 2 | Uso incorreto (argumento faltando, flag inválida) |
| 3 | Falha de autenticação/sessão |
| 4 | Recurso não encontrado |
| 5 | Erro retornado pelo servidor Fluig |
| 6 | Sucesso parcial em lote (detalhes em `data.results[]`) |
| 7 | Dependência ausente no servidor (widget auxiliar) |

Veja um exemplo de fluxo dirigido por agente:

```sh
echo "$SENHA" | fluigcli dataset export datasets/ds_x.js --server homolog \
  --password-stdin --json
# → {"ok":true,"command":"dataset export","server":"homolog",
#    "data":{"results":[{"id":"ds_x","action":"updated","success":true}]},"error":null}
echo $?   # 0
```

A CLI tem estas flags globais: `--server` (`FLUIGCLI_SERVER`), `--project` (`FLUIGCLI_PROJECT`),
`--json`, `--yes`/`-y`, `--non-interactive` (`FLUIGCLI_NON_INTERACTIVE=1`),
`--verbose`/`-v`, `--timeout` (`FLUIGCLI_TIMEOUT`), `--no-session-cache`
(`FLUIGCLI_NO_SESSION_CACHE=1`).

### Skill para agentes (Claude Code / Codex)

O repositório traz uma Skill pronta. Ela ensina o agente a dirigir o fluigcli.
Ela cobre o contrato `--json`, os exit codes e o mapa de comandos. O conteúdo
canônico está em [`skills/fluigcli/`](skills/fluigcli/). A CLI embute esse
conteúdo no binário. Instale a Skill no seu projeto com:

```sh
fluigcli skill install --target all   # Claude Code (.claude/skills/) + Codex (AGENTS.md)
fluigcli skill install --target claude --global   # no diretório do usuário
```

A reinstalação é idempotente. Ela atualiza no lugar. Ela não duplica o bloco do
`AGENTS.md`. Ela não sobrescreve arquivos que você editou, salvo com `--force`.
Veja [docs/skill.md](docs/skill.md).

## Credenciais

A CLI **nunca** grava a senha em arquivo. A CLI **nunca** aceita a senha como
argumento de linha de comando. A ordem de resolução é: `--password-stdin` →
`FLUIGCLI_PASSWORD` → keyring do SO → prompt interativo (com oferta de salvar no
keyring).

No projeto, a **conexão** dos servidores é versionável
(`.fluigcli/servers.json`). Já a sua **identidade** (usuário) e o seu **padrão**
ficam num arquivo pessoal git-ignorado (`.fluigcli/servers.local.json`). Assim,
commitar a config não impõe o seu login ao time. Veja mais detalhes em
[docs/server.md](docs/server.md).

## Desenvolvimento

```sh
go build ./...
go test ./...
go test -tags=integration ./internal/fluig/   # integração (requer FLUIGCLI_TEST_*)
```

Veja o [CONTRIBUTING.md](CONTRIBUTING.md) e a documentação de cada comando em
[docs/](docs/).

## Inspirações e agradecimentos

- **[fluig-vscode-extension](https://github.com/fluiggers/fluig-vscode-extension)** —
  extensão VS Code para desenvolvimento Fluig.
- **[fluig-widget-helper](https://github.com/fluiggers/fluig-widget-helper)** —
  widget auxiliar da comunidade (MIT).
- **[fluig-declaration-type](https://github.com/fluiggers/fluig-declaration-type)** —
  declaração de tipos da comunidade (MIT).
- **[logfluig2.0](https://github.com/matheusnevoa/logfluig2.0)** — 
  visualizador de logs do servidor TOTVS Fluig.

## Autor e licença

Criado e mantido por **Alessandro Lorençone**
([@alorenco](https://github.com/alorenco)).

[MIT](LICENSE) — © 2026 Alessandro Lorençone e contribuidores do fluig-cli.
