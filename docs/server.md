# fluigcli server — gerenciamento de servidores

Este grupo cadastra e testa os servidores Fluig. Os demais comandos usam esses
servidores.

## Servidor padrão e ambientes

Marque cada servidor com um ambiente. Os ambientes são `dev`, `hml` ou `prod`.
Marque um servidor como **padrão**. A CLI usa o servidor padrão quando você não
informa `--server`. Este comportamento segue a org padrão da CLI do Salesforce.
O primeiro servidor cadastrado vira padrão automaticamente. Troque o padrão com
`server use`.

A CLI resolve o servidor alvo nesta ordem:

1. argumento posicional (`server test homolog`)
2. `--server <nome>` ou `FLUIGCLI_SERVER`
3. padrão do **projeto** (pessoal, em `.fluigcli/servers.local.json`)
4. padrão **global** (preferência pessoal)
5. único servidor cadastrado
6. seleção interativa. A seleção oferece fixar a escolha como padrão.

### ⚠️ Trava de produção

Em servidor marcado `prod`, os comandos que **escrevem** pedem confirmação. Os
comandos de escrita são `export`, `delete` e `install-helper`. A confirmação
aparece antes de a CLI tocar no servidor:

```
O servidor "producao" é de PRODUÇÃO — publicar datasets mesmo assim? (s/N)
```

Em modo não-interativo (CI, agentes, `--json`), a CLI bloqueia a operação com
exit `2`. Para liberar, informe `--yes`. Assim, o deploy consciente em produção
continua a um flag de distância. O deploy acidental fica bloqueado.

O [`fluigcli dev`](dev.md) passa pela mesma trava ao **subir** apontando para
produção. Ele carrega sua sessão num proxy local. Neste modo, o watch integrado
fica indisponível. Publicar pelo painel exige confirmação própria. O `watch`
standalone recusa produção sem exceção.

## Onde a configuração fica

A configuração do **projeto** separa o que é do time do que é seu. O
`servers.json` é versionável e guarda só a **conexão**. Sua **identidade**
(usuário) e seu **padrão** ficam num arquivo pessoal, git-ignorado.

| Arquivo | Escopo | Conteúdo | Precedência |
|---|---|---|---|
| `<projeto>/.fluigcli/servers.json` | projeto (versionável em Git) | conexão do time (host, porta, ssl, companyId, env) | maior |
| `<projeto>/.fluigcli/servers.local.json` | projeto (**git-ignorado**) | sua identidade por servidor + seu padrão | — |
| `~/.config/fluigcli/servers.json` · `%APPDATA%\fluigcli\servers.json` (Windows) | global | servidor completo + padrão (pessoal) | menor |

Nenhum arquivo contém senha. O `servers.json` do projeto não carrega usuário
nem padrão pessoal. Por isso, é seguro commitá-lo. Cada pessoa do time põe o
próprio usuário no `servers.local.json`. O `server add` cria a entrada no
`.gitignore`. A CLI ainda lê os arquivos no formato antigo (com usuário
embutido).

### Sua identidade num servidor compartilhado

Ao usar um servidor que veio do repositório, você pode não ter identidade local
ainda. Neste caso, a CLI resolve o **usuário** nesta ordem:

1. overlay local (`servers.local.json`). A CLI grava quando você informa uma vez.
2. servidor global de mesmo nome, se houver;
3. `FLUIGCLI_USERNAME`. Use em CI e em modo não-interativo.
4. prompt interativo. A CLI salva a resposta no overlay local.
5. nada disso em modo não-interativo → exit `2`. A mensagem orienta a definir.

## Onde a senha fica (ordem de resolução)

0. **Sessão em cache válida dispensa senha.** Se há uma sessão reaproveitável
   (ver abaixo), a CLI a usa direto, sem prompt nem env var. Exceção:
   `--password-stdin` pula esta etapa. Quem manda a senha explícita quer vê-la
   validada por um login real.
1. `--password-stdin` — a CLI lê a senha do stdin (scripts e agentes).
2. `FLUIGCLI_PASSWORD` — variável de ambiente. Vale para o servidor selecionado.
3. Keyring do SO (Windows Credential Manager, macOS Keychain, Secret Service no
   Linux). O `server add` grava a senha. A CLI chaveia por `baseURL|usuário`.
4. Prompt interativo. A CLI oferece salvar no keyring, quando ele existe.
5. Nenhuma disponível em modo não-interativo → exit `3`.

Alguns ambientes não têm keyring. Um exemplo é o Linux headless. Neste caso, a
CLI não pergunta se quer salvar e não emite avisos. Use `FLUIGCLI_PASSWORD` ou
`--password-stdin`.

## ⚠️ HTTP vs HTTPS

A CLI aceita servidores `--ssl=false` (HTTP). O HTTP é comum em on-premise. Mas
o HTTP **não criptografa** o tráfego. A senha (no login) e os **cookies de
sessão** vão em texto claro. Quem está na rede pode capturá-los. A sessão é a
credencial de acesso completa. Por isso, **prefira HTTPS** sempre que possível.
Use HTTP apenas em redes confiáveis.

## Cache de sessão

Após o primeiro login, a CLI reaproveita os cookies de sessão **entre
execuções**. A CLI valida a sessão por ping. Assim, ela não faz login a cada
comando. Isto é útil para agentes e CI que rodam vários comandos. Os cookies
ficam em `<cache do usuário>/fluigcli/sessions.json` (arquivo `0600`). São
credenciais de sessão. Eles nunca vão para o projeto.

- Desativar: `--no-session-cache` ou `FLUIGCLI_NO_SESSION_CACHE=1`.
- Descartar: `fluigcli server logout [<name>]` (ou `--all`).

## Comandos

### `fluigcli server add`

Este comando cadastra um servidor. Sem flags, ele pergunta os dados de forma
interativa.

```sh
fluigcli server add --name homolog --host fluig-homolog.empresa.com.br \
  --port 443 --ssl --username admin.deploy --company-id 1
```

- `--env dev|hml|prod` marca o ambiente. A CLI aceita e normaliza apelidos como
  `homolog` e `producao`. O ambiente `prod` ativa a trava de produção.
- `--default` define o servidor como padrão já no cadastro. O primeiro servidor
  cadastrado vira padrão automaticamente.
- `--password-stdin` lê a senha do stdin e grava no keyring. Use em modo
  não-interativo.
- `--global` grava na configuração global em vez da configuração do projeto.
- A CLI **nunca** aceita a senha como argumento de linha de comando. A senha
  vazaria em `ps` e no histórico do shell.
- O projeto pode ter pastas em `forms/` sem vínculo com o servidor cadastrado.
  Neste caso, o comando lembra de rodar `fluigcli form link`. O vínculo
  pasta↔formulário é por servidor. Ver [form](form.md). O `server test` dá a
  mesma dica.

### `fluigcli server list`

Este comando lista os servidores visíveis, em tabela. Ele mostra o projeto e o
global. O projeto sobrepõe nomes repetidos. O padrão aparece **primeiro** e
marcado com `●`. Sem padrão definido, a saída orienta a fixar um com
`server use`. No `--json`, o campo `default` traz o nome do padrão.

### `fluigcli server use [<name>]`

Este comando define o servidor padrão. Sem `<name>`, ele lista e deixa você
escolher (interativo).

```sh
fluigcli server use producao            # padrão pessoal do projeto (git-ignorado)
fluigcli server use homolog --global    # preferência pessoal, fora do projeto
```

### `fluigcli server update <name>`

Este comando altera campos do cadastro sem remover o servidor. A CLI preserva a
senha no keyring. O nome não muda, porque é a chave. Para renomear, remova e
cadastre de novo.

```sh
fluigcli server update producao --env prod
fluigcli server update homolog --host novo-host.empresa.com.br --port 8080 --ssl=false
```

### `fluigcli server remove <name>`

Este comando remove o servidor e a senha correspondente do keyring. Ele pede
confirmação. Informe `--yes` para pular.

### `fluigcli server status [<name>]`

Este comando mostra a saúde do servidor. Ele mostra a **versão do Fluig**, o
estado do **fluigcliHelper** (instalado + versão), o uptime, os usuários
conectados, as threads, a memória da JVM e do SO, o banco de dados (nome,
versão, tamanho) e a tabela de **monitores** de serviços. Um helper antigo não
tem o endpoint de versão. Neste caso, ele sai como "versão desconhecida", com a
dica de reinstalar. Na tabela de monitores, OK sai em verde. NONE sai esmaecido
e indica serviço não configurado.

Este comando **requer usuário com privilégio administrativo**. Sem ele, o módulo
`/environment` responde 401 (exit 3). A versão do produto (ex.: `Voyager 2.0.0`
/ `Crystal Mist 1.8.2`) vem do endpoint `/api/public/wcm/version`, que não exige
admin. Por isso, se as estatísticas falharem por privilégio, a CLI ainda
identifica a versão. No `--json`, o campo `helper` traz `{installed, version}`.

```sh
fluigcli server status homolog
fluigcli server status --json     # stats tipadas + monitores, para agentes/CI
```

### `fluigcli server test [<name>]`

Este comando faz login, valida a sessão (ping) e busca os dados do usuário. Sem
`<name>`, ele usa `--server`/`FLUIGCLI_SERVER` ou oferece seleção interativa.

```sh
fluigcli server test homolog
echo "$SENHA" | fluigcli server test homolog --password-stdin --json
```

O comando também reporta se o componente auxiliar **fluigcliHelper** está
instalado. O helper é necessário para o deploy de scripts de processo, o
`widget import` e os comandos de [log](log.md). Ver [workflow](workflow.md). No
`--json` vem o campo `helperInstalled`.

Exit codes: `0` ok · `3` autenticação/sessão · `4` servidor não cadastrado ·
`5` erro do servidor Fluig.

### `fluigcli server logout [<name>]`

Este comando descarta a sessão em cache de um servidor. Use `--all` para todos.
Use este comando para forçar novo login ou limpar credenciais de sessão
gravadas.

### `fluigcli server install-helper [<name>]`

Este comando instala o componente auxiliar `fluigcliHelper`. O WAR vai embutido
no binário da CLI. O helper é pré-requisito dos scripts de processo, do
`widget import` e do grupo [log](log.md). Com `--force`, a CLI reenvia mesmo que
já exista uma versão instalada. É assim que você **atualiza** o helper. Ver mais
detalhes em [workflow](workflow.md).
