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
3. padrão do **projeto** (pessoal, em `.fluigcli/servers.local.json`)
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

A configuração do **projeto** separa o que é do time do que é seu: o
`servers.json` é versionável e guarda só a **conexão**; sua **identidade**
(usuário) e seu **padrão** ficam num arquivo pessoal, git-ignorado.

| Arquivo | Escopo | Conteúdo | Precedência |
|---|---|---|---|
| `<projeto>/.fluigcli/servers.json` | projeto (versionável em Git) | conexão do time (host, porta, ssl, companyId, env) | maior |
| `<projeto>/.fluigcli/servers.local.json` | projeto (**git-ignorado**) | sua identidade por servidor + seu padrão | — |
| `~/.config/fluigcli/servers.json` · `%APPDATA%\fluigcli\servers.json` (Windows) | global | servidor completo + padrão (pessoal) | menor |

Nenhum arquivo **jamais** contém senha. Como o `servers.json` do projeto não
carrega usuário nem padrão pessoal, é seguro commitá-lo: cada pessoa do time põe
o próprio usuário no `servers.local.json` (o `server add` já cria a entrada no
`.gitignore`). Arquivos no formato antigo (com usuário embutido) continuam sendo
lidos.

### Sua identidade num servidor compartilhado

Ao usar um servidor que veio do repositório (sem identidade local ainda), a CLI
resolve o **usuário** nesta ordem:

1. overlay local (`servers.local.json`) — gravado quando você informa uma vez;
2. servidor global de mesmo nome, se houver;
3. `FLUIGCLI_USERNAME` (útil em CI/não-interativo);
4. prompt interativo (a resposta é salva no overlay local);
5. sem nada disso em modo não-interativo → exit `2`, orientando a definir.

## Onde a senha fica (ordem de resolução)

0. **Sessão em cache válida dispensa senha** — se há uma sessão reaproveitável
   (ver abaixo), a CLI a usa direto, sem prompt nem env var. Exceção:
   `--password-stdin` pula essa etapa, porque quem manda a senha
   explicitamente quer vê-la validada por um login de verdade.
1. `--password-stdin` — senha lida do stdin (scripts e agentes)
2. `FLUIGCLI_PASSWORD` — variável de ambiente (vale para o servidor selecionado)
3. Keyring do SO (Windows Credential Manager, macOS Keychain, Secret Service no Linux),
   gravada pelo `server add` e chaveada por `baseURL|usuário`
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
- Se o projeto tem pastas em `forms/` ainda sem vínculo com o servidor
  cadastrado, o comando lembra de rodar `fluigcli form link` (o vínculo
  pasta↔formulário é por servidor — ver [form](form.md)). O `server test`
  dá a mesma dica.

### `fluigcli server list`

Lista os servidores visíveis (projeto + global, com o projeto sobrepondo nomes
repetidos), em tabela. O padrão aparece **primeiro** e marcado com `●`; sem
padrão definido, a saída orienta a fixar um com `server use`. No `--json`, o
campo `default` traz o nome do padrão.

### `fluigcli server use [<name>]`

Define o servidor padrão. Sem `<name>`, lista e deixa escolher (interativo).

```sh
fluigcli server use producao            # padrão pessoal do projeto (git-ignorado)
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

### `fluigcli server status [<name>]`

Mostra a saúde do servidor: **versão do Fluig**, estado do **fluigcliHelper**
(instalado + versão; helper antigo sem o endpoint de versão sai como
"versão desconhecida", com a dica de reinstalar), uptime, usuários conectados,
threads, memória da JVM e do SO, banco de dados (nome, versão, tamanho) e a
tabela de **monitores** de serviços (OK em verde; NONE esmaecido = serviço não
configurado). **Requer usuário com privilégio administrativo** — sem ele o
módulo `/environment` responde 401 (exit 3). A versão do produto (ex.: `Voyager
2.0.0` / `Crystal Mist 1.8.2`) vem do endpoint `/api/public/wcm/version`, que
não exige admin: se as estatísticas falharem por privilégio, a versão ainda é
identificada. No `--json`, o campo `helper` traz `{installed, version}`.

```sh
fluigcli server status homolog
fluigcli server status --json     # stats tipadas + monitores, para agentes/CI
```

### `fluigcli server test [<name>]`

Faz login, valida a sessão (ping) e busca os dados do usuário. Sem `<name>`,
usa `--server`/`FLUIGCLI_SERVER` ou oferece seleção interativa.

```sh
fluigcli server test homolog
echo "$SENHA" | fluigcli server test homolog --password-stdin --json
```

Também reporta se o componente auxiliar **fluigcliHelper** está instalado —
necessário para o deploy de scripts de processo e o `widget import` (ver
[workflow](workflow.md)). No `--json` vem o campo `helperInstalled`.

Exit codes: `0` ok · `3` autenticação/sessão · `4` servidor não cadastrado ·
`5` erro do servidor Fluig.

### `fluigcli server logout [<name>]`

Descarta a sessão em cache de um servidor (ou de todos com `--all`). Útil para
forçar novo login ou limpar credenciais de sessão gravadas.

### `fluigcli server install-helper [<name>]`

Instala o componente auxiliar `fluigcliHelper` — o WAR vai embutido no
binário da CLI (pré-requisito dos scripts de processo e do `widget import`).
Detalhes em [workflow](workflow.md).
