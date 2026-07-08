# fluigcli server — gerenciamento de servidores

Cadastra e testa os servidores Fluig usados pelos demais comandos.

## Onde a configuração fica

| Arquivo | Escopo | Precedência |
|---|---|---|
| `<projeto>/.fluigcli/servers.json` | por projeto (versionável em Git) | maior |
| `~/.config/fluigcli/servers.json` (Linux/macOS) · `%APPDATA%\fluigcli\servers.json` (Windows) | global | menor |

O arquivo **nunca** contém senha — apenas metadados (host, porta, usuário,
companyId). Por isso é seguro commitá-lo no repositório do projeto.

## Onde a senha fica (ordem de resolução)

1. `--password-stdin` — senha lida do stdin (scripts e agentes)
2. `FLUIGCLI_PASSWORD` — variável de ambiente (vale para o servidor selecionado)
3. Keyring do SO (Windows Credential Manager, macOS Keychain, Secret Service no Linux),
   gravada pelo `server add`
4. Prompt interativo (com oferta de salvar no keyring, quando ele existe)
5. Nenhuma disponível em modo não-interativo → exit `3`

Em ambientes sem keyring (ex.: Linux headless), a CLI não pergunta se quer
salvar e não emite avisos — use `FLUIGCLI_PASSWORD` ou `--password-stdin`.

## ⚠️ HTTP vs HTTPS

Servidores `--ssl=false` (HTTP) são aceitos (comum em on-premise), mas o tráfego
**não é criptografado**: a senha (no login) e os **cookies de sessão** vão em
texto claro e podem ser capturados por quem está na rede — e a sessão é a
credencial de acesso completa. **Prefira HTTPS** sempre que possível; use HTTP
apenas em redes confiáveis.

## Cache de sessão

Após o primeiro login, os cookies de sessão são reaproveitados **entre
execuções** (validados por ping), evitando relogar a cada comando — útil para
agentes/CI que rodam vários comandos. Ficam em `<cache do usuário>/fluigcli/
sessions.json` (arquivo `0600`; são credenciais de sessão, nunca vão para o
projeto).

- Desativar: `--no-session-cache` ou `FLUIGCLI_NO_SESSION_CACHE=1`.
- Descartar: `fluigcli server logout [<name>]` (ou `--all`).

## Comandos

### `fluigcli server add`

Cadastra um servidor. Sem flags, pergunta os dados interativamente.

```sh
fluigcli server add --name homolog --host fluig-homolog.empresa.com.br \
  --port 443 --ssl --username admin.deploy --company-id 1
```

- `--password-stdin` lê a senha do stdin e grava no keyring (uso não-interativo).
- `--global` grava na configuração global em vez da do projeto.
- A senha **nunca** é aceita como argumento de linha de comando (vazaria em `ps`
  e no histórico do shell).

### `fluigcli server list`

Lista os servidores visíveis (projeto + global, com o projeto sobrepondo nomes
repetidos).

### `fluigcli server remove <name>`

Remove o servidor e a senha correspondente do keyring. Pede confirmação
(`--yes` pula).

### `fluigcli server test [<name>]`

Faz login, valida a sessão (ping) e busca os dados do usuário. Sem `<name>`,
usa `--server`/`FLUIGCLI_SERVER` ou oferece seleção interativa.

```sh
fluigcli server test homolog
echo "$SENHA" | fluigcli server test homolog --password-stdin --json
```

Também reporta se a widget auxiliar **fluiggersWidget** está instalada
(necessária para o deploy de scripts de processo — ver
[workflow](workflow.md)); no `--json` vem o campo `helperInstalled`.

Exit codes: `0` ok · `3` autenticação/sessão · `4` servidor não cadastrado ·
`5` erro do servidor Fluig.

### `fluigcli server logout [<name>]`

Descarta a sessão em cache de um servidor (ou de todos com `--all`). Útil para
forçar novo login ou limpar credenciais de sessão gravadas.

### `fluigcli server install-helper [<name>]`

Instala a widget auxiliar `fluiggersWidget` (pré-requisito dos scripts de
processo). Detalhes em [workflow](workflow.md).
