# fluigcli service — cadastro de serviços

O grupo `service` consulta e altera o cadastro de **serviços** do Fluig. Este
cadastro fica na tela Painel de Controle > Serviços. Os datasets chamam um
serviço REST pelo código. Por exemplo:

```js
fluigAPI.getAuthorizeClientService().invoke({
  serviceCode: "sabia_agentos",
  endpoint: "/api/painel",
  method: "get",
});
```

O código do serviço (`serviceCode`) precisa existir no servidor. Use este grupo
para cadastrar o serviço na implantação, sem abrir a interface.

O grupo trata os três tipos de serviço do Fluig:

| Tipo | Uso | O que a CLI faz |
|---|---|---|
| `rest` | API REST externa, chamada pelo `getAuthorizeClientService()` | tudo: cadastro, autenticações, teste, log, cópia, export/apply |
| `soap` | web service SOAP, chamado pelo `ServiceManager` | cadastro (WSDL, pacote, autenticação none ou basic), export/apply |
| `jdbc` | banco de dados externo | cadastro (URL, driver, usuário e senha), teste de conexão, export/apply |

O usuário precisa de acesso à tela Painel de Controle > Serviços.

## Segredos

A CLI **nunca** mostra senha, token ou outro segredo do serviço. A saída humana
e o envelope `--json` trazem só o código, o tipo, a URL base, o tipo de
autenticação, o caminho de teste, o swagger, o usuário da autenticação basic,
os **nomes** dos headers da autenticação custom (campo `customHeaders`) e os
**nomes** dos campos OAuth preenchidos (campo `oauthFields`).

O segredo **nunca** vem de argumento de linha de comando:

- Senha basic: defina a variável `FLUIGCLI_SERVICE_PASSWORD`. Em modo
  interativo, a CLI pede a senha num prompt oculto.
- Headers custom: envie pelo stdin com `--custom-headers-stdin` ou defina a
  variável `FLUIGCLI_SERVICE_CUSTOM_HEADERS`.
- Campos OAuth: envie um objeto JSON pelo stdin com `--oauth-stdin` ou defina
  a variável `FLUIGCLI_SERVICE_OAUTH`.

> **Atenção.** O Fluig guarda a senha basic, os headers custom e os campos
> OAuth em **texto aberto**. A API de listagem e a tela de edição do Painel de Controle mostram
> esses valores. Por isso, quem acessa o Painel de Controle lê o segredo. A CLI
> só não mostra o valor.

## Tipos de autenticação

O Fluig aceita seis tipos de autenticação em serviço REST:

| Tipo | A CLI grava? | Uso |
|---|---|---|
| `none` | sim | sem autenticação no cadastro. O dataset envia o token no header. |
| `basic` | sim | usuário e senha no cadastro. |
| `fluigAPI` | sim | chamada autenticada à API do próprio Fluig. |
| `oauth1` | sim | OAuth 1. Os campos chegam como objeto JSON. |
| `oauth2` | sim | OAuth 2. Os campos chegam como objeto JSON. |
| `custom` | sim | headers fixos com segredo. O Fluig envia os headers em toda chamada. |

A CLI lista e mostra os seis tipos. O `update` altera a URL e os outros campos
de qualquer serviço REST. Neste caso, o segredo guardado no servidor continua
igual.

### Autenticação custom

A autenticação custom guarda headers fixos. Escreva uma linha por header, no
formato `Nome: valor`. Por exemplo: `Authorization: Bearer <token>`. A CLI
confere o formato antes de enviar:

- Cada linha tem `:` e um valor não vazio.
- O nome do header não tem espaço.
- O texto não tem linha vazia no meio.

O `fluigAPI.getAuthorizeClientService().invoke()` envia esses headers em toda
chamada do serviço. O dataset pode enviar outros headers na chamada. O dataset
**não** deve repetir um header que o cadastro já tem. Neste caso, o Fluig envia
o header duas vezes, e o servidor de destino pode recusar a chamada (medido:
HTTP 400).

### Autenticação OAuth 1 e OAuth 2

Os campos OAuth chegam como um objeto JSON de textos. A CLI aceita só os
campos que a tela do Fluig usa. Ela confere os campos obrigatórios antes de
enviar:

| Tipo | Campos | Obrigatórios |
|---|---|---|
| `oauth1` | `consumerKey`, `consumerSecret`, `tokenAccess`, `tokenSecret`, `accessTokenURL`, `requestTokenURL`, `authorizationURL` | todos |
| `oauth2` | `clientId`, `clientSecret`, `tokenAccess`, `refreshToken`, `refreshTokenURL` | `clientId`, `clientSecret` e `tokenAccess` |

```sh
fluigcli service create crm_api --url https://crm.empresa.com --auth oauth2 --oauth-stdin <<'JSON'
{"clientId":"...","clientSecret":"...","tokenAccess":"...","refreshTokenURL":"https://crm.empresa.com/oauth/token"}
JSON
```

Sem um campo obrigatório, o Fluig responde "Erro ao criar/atualizar cliente
OAuth". Por isso, a CLI para antes com exit **2**. Um campo desconhecido também
termina com exit **2**.

## `fluigcli service list [flags]`

Este comando lista os serviços cadastrados numa tabela: código, tipo, URL base
e autenticação.

| Flag | Uso |
|---|---|
| `--type <tipo>` | filtra por tipo: `rest`, `soap` ou `jdbc`. Repita a flag ou separe por vírgula. |
| `--filter <trecho>` | filtra pelo trecho do código. O servidor faz a busca. |

```sh
fluigcli service list
fluigcli service list --type rest
fluigcli service list --filter n8n --json
```

## `fluigcli service show <código>`

Este comando mostra um serviço, sem os segredos. Código inexistente termina com
exit **4**.

```sh
fluigcli service show sabia_agentos
fluigcli service show sabia_agentos --json
```

## `fluigcli service create <código> --url <URL> [flags]`

Este comando cadastra um serviço. O tipo default é REST.

| Flag | Uso |
|---|---|
| `--type <tipo>` | `rest` (default), `soap` ou `jdbc` |
| `--url <URL>` | **obrigatório**. REST: URL base, com `http://` ou `https://`. SOAP: URL do WSDL. JDBC: URL `jdbc:`. |
| `--description <texto>` | descrição do serviço (default: o código) |
| `--auth <tipo>` | `none`, `basic`, `fluigAPI`, `custom`, `oauth1` ou `oauth2` (default: `none`) |
| `--username <login>` | usuário da autenticação basic. Obrigatório com `--auth basic`. |
| `--custom-headers-stdin` | lê do stdin os headers da autenticação custom. Alternativa: `FLUIGCLI_SERVICE_CUSTOM_HEADERS`. |
| `--oauth-stdin` | lê do stdin os campos OAuth, como objeto JSON. Alternativa: `FLUIGCLI_SERVICE_OAUTH`. |
| `--test-path <caminho>` | caminho de teste do serviço. Por exemplo: `/health`. |
| `--swagger <URL>` | URL do swagger do serviço |
| `--soap-package <pacote>` | SOAP: pacote Java das classes geradas do WSDL (opcional) |
| `--driver <driver>` | JDBC: driver do banco (**obrigatório**). Veja os drivers com `service drivers`. |

As flags `--test-path`, `--swagger`, `--custom-headers-stdin` e `--oauth-stdin`
valem só para REST. No SOAP, `--auth` aceita `none` ou `basic`. No JDBC,
`--username` é obrigatório e a senha vem de `FLUIGCLI_SERVICE_PASSWORD` ou do
prompt.

```sh
fluigcli service create sabia_agentos --url https://agentos.empresa.com --test-path /health
FLUIGCLI_SERVICE_PASSWORD=... fluigcli service create erp_api --url https://erp.empresa.com \
  --auth basic --username integracao
printf 'Authorization: Bearer %s\n' "$(cat token.txt)" | \
  fluigcli service create painel_api --url https://painel.empresa.com --auth custom --custom-headers-stdin
fluigcli service create erp_ws --type soap --url 'http://erp.empresa.com:8051/wsDataServer/MEX?wsdl'
FLUIGCLI_SERVICE_PASSWORD=... fluigcli service create erp_db --type jdbc \
  --url 'jdbc:sqlserver://db.empresa.com:1433;databaseName=erp' --driver sqlserver --username leitura
```

A criação de um serviço SOAP leva alguns segundos, porque o servidor gera as
classes do WSDL.

O `--custom-headers-stdin` e o `--password-stdin` leem o mesmo stdin. Por isso,
o comando recusa as duas flags juntas. Neste caso, use
`FLUIGCLI_SERVICE_CUSTOM_HEADERS` para os headers.

Código repetido termina com exit **5**. Neste caso, use `service update`.

## `fluigcli service update <código> [flags]`

Este comando altera um serviço REST, SOAP ou JDBC. Ele usa as mesmas flags do
`create`, menos `--type`: o tipo não muda. O comando troca só os campos das
flags informadas. Os outros campos ficam iguais. Por exemplo, a troca da URL
mantém a senha guardada.

```sh
fluigcli service update sabia_agentos --url https://agentos2.empresa.com
fluigcli service update erp_api --auth none          # remove usuário e senha
FLUIGCLI_SERVICE_PASSWORD=... fluigcli service update erp_api   # troca só a senha
printf 'Authorization: Bearer %s\n' "$(cat token.txt)" | \
  fluigcli service update painel_api --auth custom --custom-headers-stdin   # troca para custom
```

A CLI relê a senha basic em três casos: na troca para `basic`, na troca do
usuário e quando `FLUIGCLI_SERVICE_PASSWORD` está definida. A CLI relê os
headers custom em três casos: na troca para `custom`, com
`--custom-headers-stdin` e quando `FLUIGCLI_SERVICE_CUSTOM_HEADERS` está
definida. A troca de tipo apaga a credencial do tipo antigo. Por exemplo, a
troca para `none` apaga o usuário, a senha e os headers custom.

No SOAP basic, a alteração mantém a senha guardada, como no REST.

O JDBC é a exceção: o Fluig **não** devolve a senha do JDBC. Uma alteração
sem a senha apagaria a senha guardada. Por isso, toda alteração de um serviço
JDBC pede a senha (`FLUIGCLI_SERVICE_PASSWORD` ou prompt). Sem ela, o comando
para com exit **2**.

Código inexistente termina com exit **4**.

## `fluigcli service drivers [--all]`

Este comando lista os drivers JDBC do servidor: o tipo, se está disponível e o
modelo de URL. Só um driver disponível serve para `service create --type jdbc`.
Com `--all`, a lista inclui os drivers não habilitados no servidor.

## `fluigcli service delete <código>...`

Este comando exclui um ou mais serviços. Os datasets e as widgets que usam o
serviço deixam de funcionar. Por isso, o comando pede confirmação. Em modo
não-interativo, confirme com `--yes`.

```sh
fluigcli service delete zz_servico_antigo --yes
```

Código inexistente termina com exit **4**. Num lote, uma falha termina com
exit **6**.

## `fluigcli service test <código> [--full]`

No serviço JDBC, este comando abre uma conexão de teste com o banco. É o botão
"Testar conexão" do Painel de Controle. A conexão aceita termina com exit
**0**. A conexão recusada termina com exit **5**, com a mensagem do driver. O
serviço SOAP não tem teste (exit **2**).

No serviço REST, este comando chama o caminho de teste do serviço (`--test-path`) pelo próprio
servidor Fluig, com a autenticação do cadastro. É o botão "Testar" do Painel de
Controle. Por isso, o teste confere a URL, a rede do servidor e a autenticação
de uma vez.

```sh
fluigcli service test sabia_agentos
```

A saída mostra o status HTTP do destino e o corpo da resposta. O corpo sai
truncado em 2000 caracteres. Use `--full` para o corpo inteiro.

| Resultado | Exit |
|---|---|
| o destino respondeu 2xx | **0** |
| o destino respondeu outro status | **5** |
| a chamada não saiu do Fluig (por exemplo, host inexistente) | **5** |
| código inexistente | **4** |

Sem caminho de teste, a chamada vai para a raiz da URL base, e o comando
avisa. O Fluig não aceita outro caminho na hora do teste. Para testar outro
caminho, troque o caminho com `service update <código> --test-path <caminho>`.

## `fluigcli service log [<código>] [flags]`

Este comando trata o log de requisições do serviço. Sem flags, ele mostra o
resumo: se o log está ligado, até quando, e as contagens de chamadas com
sucesso e com falha.

| Flag | Uso |
|---|---|
| `--enable` | liga o log |
| `--hours N` | horas de log ligado, de 1 a 24 (default 1). Só vale com `--enable`. |
| `--disable` | desliga o log na hora |
| `--download` | baixa o log de **todos** os serviços com log ligado, num `.zip`. O código é opcional. |
| `--output <arquivo>` | arquivo do `--download` (default: `services-log-<data>.zip`) |

```sh
fluigcli service log sabia_agentos --enable --hours 2
fluigcli service test sabia_agentos
fluigcli service log --download -o log-servicos.zip
fluigcli service log sabia_agentos --disable
```

O Fluig desliga o log sozinho no fim do prazo.

> **Atenção.** O log traz as mensagens completas, inclusive os headers
> enviados. Num serviço custom, o segredo está no arquivo. Por isso, a CLI
> grava o log só em arquivo, com permissão 0600. Apague o arquivo quando não
> precisar mais dele.

## Levar o serviço para outro ambiente

Use um arquivo de especificação versionado no Git. O fluxo é este:

1. Grave a especificação a partir do servidor de origem:

   ```sh
   fluigcli service export sabia_agentos --server homolog
   ```

   O comando grava `services/sabia_agentos.json`.
2. Revise o arquivo e faça o commit.
3. Aplique no servidor de destino:

   ```sh
   export FLUIGCLI_SECRET_SABIA_AGENTOS="Authorization: Bearer $(cat token.txt)"
   fluigcli service apply services/sabia_agentos.json --server producao
   ```

Você também pode pôr o arquivo num plano de [`deploy`](deploy.md), com o passo
`service`.

### `fluigcli service copy <código> --to <servidor> [--as <código-novo>]`

Este comando copia um serviço REST do servidor de origem (`--server`) para o
servidor de destino (`--to`). Ele cria o serviço no destino ou alinha o
serviço que já existe. A cópia leva a URL, a descrição, o caminho de teste, o
swagger, o tipo de autenticação e os segredos de qualquer tipo: basic,
custom, OAuth 1 e OAuth 2. Os segredos vão de servidor para servidor e não
aparecem na tela nem em arquivo.

```sh
fluigcli service copy sabia_agentos --server homolog --to producao
fluigcli service copy sabia_agentos --server homolog --to homolog --as sabia_agentos_v2
```

Quando o destino já é igual à origem, inclusive nos segredos, a cópia não
grava nada. Neste caso, o resultado é `unchanged`.

A cópia trata só serviços REST. Para SOAP e JDBC, use o `export` e o
`apply`.

Use `--as` para gravar com outro código. No destino de produção, a cópia pede
confirmação. Em modo não-interativo, confirme com `--yes`. O comando não
aceita `--password-stdin`, porque autentica em dois servidores. Use a sessão
em cache ou a variável de senha.

Use o `copy` para uma cópia avulsa. Para um fluxo versionado no Git, use o
`export` e o `apply`.

### `fluigcli service export <código> [--output <arquivo>]`

Este comando grava a especificação de um serviço REST em
`services/<código>.json`. Com `--output`, ele grava no arquivo informado.

O arquivo **nunca** guarda segredo. Na autenticação basic ou custom, o campo
`secretEnv` traz o **nome** da variável de ambiente que o `apply` lê. O nome
sugerido é `FLUIGCLI_SECRET_<CÓDIGO>`. Você pode trocar o nome no arquivo.

```json
{
  "code": "sabia_agentos",
  "description": "AgentOS do SabIA",
  "url": "https://agentos.empresa.com",
  "auth": "custom",
  "secretEnv": "FLUIGCLI_SECRET_SABIA_AGENTOS"
}
```

Na autenticação OAuth, a variável do `secretEnv` traz o objeto JSON dos campos
OAuth.

O export também trata SOAP e JDBC. O arquivo ganha o campo `type`, mais
`soapPackage` (SOAP) ou `driver` (JDBC). No SOAP basic e no JDBC, o
`secretEnv` nomeia a variável da senha.

```json
{
  "code": "erp_db",
  "type": "jdbc",
  "driver": "sqlserver",
  "url": "jdbc:sqlserver://db.empresa.com:1433;databaseName=erp",
  "username": "leitura",
  "secretEnv": "FLUIGCLI_SECRET_ERP_DB"
}
```

### `fluigcli service apply <código | arquivo.json> [--dry-run]`

Este comando cria o serviço quando ele não existe. Quando ele existe, o
comando altera só os campos que diferem da especificação. Rodar de novo não
muda nada.

Informe o caminho do arquivo ou só o código. O código lê o arquivo
`services/<código>.json` do projeto. É o mesmo arquivo que o `export` grava.
Por exemplo, `fluigcli service apply sabia_agentos` lê
`services/sabia_agentos.json`.

O segredo segue estas regras:

- Na criação com basic, custom, oauth1 ou oauth2, a variável do `secretEnv` é
  obrigatória.
  Sem ela, o comando para com exit **2** antes de gravar.
- Na alteração sem a variável, o segredo guardado no servidor fica igual.
- Na alteração com a variável, a CLI compara o valor com o segredo guardado,
  sem mostrar nenhum dos dois. Ela só grava quando o valor mudou.

No JDBC, a CLI não confere a senha, porque o Fluig não a devolve. Ela envia a
senha na criação e quando outro campo muda. Neste caso, a variável é
obrigatória. Sem outra mudança, o apply diz "já está igual" e avisa que a
senha não foi conferida. O apply não troca o tipo de um serviço existente.

Use `--dry-run` para ver as mudanças sem gravar. Um campo desconhecido no
arquivo termina com exit **2**. Por isso, um campo `password` no arquivo é
recusado.

## Produção

Os comandos `create`, `update`, `apply` e `delete` pedem confirmação num servidor de
produção, como os outros comandos de escrita. Em modo não-interativo, confirme
com `--yes`.
