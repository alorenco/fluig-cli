# fluigcli server — gerenciamento de servidores

Cadastra e testa os servidores Fluig usados pelos demais comandos.

## Servidor padrão e ambientes

Cada servidor pode ser marcado com um ambiente — `dev`, `hml` ou `prod` — e um
deles pode ser o **padrão**, usado quando `--server` não é informado (como a
org padrão da CLI do Salesforce). O primeiro servidor cadastrado vira padrão
automaticamente; troque com `server use`.

A ordem de resolução do servidor alvo é:

1. argumento posicional (`server test homolog`)
2. `--server <nome>` ou `FLUIGCLI_SERVER`
3. padrão do **projeto** (`.fluigcli/servers.json`, versionável — o time
   compartilha)
4. padrão **global** (preferência pessoal)
5. único servidor cadastrado
6. seleção interativa (que oferece fixar a escolha como padrão)

### ⚠️ Trava de produção

Em servidor marcado `prod`, os comandos que **escrevem** (`export`, `delete`,
`install-helper`) pedem confirmação antes de tocar no servidor:

```
O servidor "producao" é de PRODUÇÃO — publicar datasets mesmo assim? (s/N)
```

Em modo não-interativo (CI, agentes, `--json`), a operação é bloqueada com exit
`2` a menos que venha `--yes` — o deploy consciente em produção continua a um
flag de distância, mas o acidental morre na praia.

## Onde a configuração fica

| Arquivo | Escopo | Precedência |
|---|---|---|
| `<projeto>/.fluigcli/servers.json` | por projeto (versionável em Git) | maior |
| `~/.config/fluigcli/servers.json` (Linux/macOS) · `%APPDATA%\fluigcli\servers.json` (Windows) | global | menor |

O arquivo **nunca** contém senha — apenas metadados (host, porta, usuário,
companyId). Por isso é seguro commitá-lo no repositório do projeto.

## Onde a senha fica (ordem de resolução)

0. **Sessão em cache válida dispensa senha** — se há uma sessão reaproveitável
   (ver abaixo), a CLI a usa direto, sem prompt nem env var. Exceção:
   `--password-stdin` pula essa etapa, porque quem manda a senha
   explicitamente quer vê-la validada por um login de verdade.
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

- `--env dev|hml|prod` marca o ambiente (apelidos como `homolog` e `producao`
  são aceitos e normalizados). `prod` ativa a trava de produção.
- `--default` define o servidor como padrão já no cadastro (o primeiro
  cadastrado vira padrão automaticamente).
- `--password-stdin` lê a senha do stdin e grava no keyring (uso não-interativo).
- `--global` grava na configuração global em vez da do projeto.
- A senha **nunca** é aceita como argumento de linha de comando (vazaria em `ps`
  e no histórico do shell).

### `fluigcli server list`

Lista os servidores visíveis (projeto + global, com o projeto sobrepondo nomes
repetidos). O `*` marca o padrão; a segunda coluna é o ambiente. No `--json`,
o campo `default` traz o nome do padrão.

### `fluigcli server use [<name>]`

Define o servidor padrão. Sem `<name>`, lista e deixa escolher (interativo).

```sh
fluigcli server use producao            # padrão do projeto (vai para o git)
fluigcli server use homolog --global    # preferência pessoal, fora do projeto
```

### `fluigcli server update <name>`

Altera campos do cadastro sem remover o servidor — a senha no keyring é
preservada. O nome não muda (é a chave); para renomear, remova e cadastre de novo.

```sh
fluigcli server update producao --env prod
fluigcli server update homolog --host novo-host.empresa.com.br --port 8080 --ssl=false
```

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
