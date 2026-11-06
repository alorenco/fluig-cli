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

O usuário precisa de acesso à tela Painel de Controle > Serviços.

## Segredos

A CLI **nunca** mostra senha, token ou outro segredo do serviço. A saída humana
e o envelope `--json` trazem só o código, o tipo, a URL base, o tipo de
autenticação, o caminho de teste, o swagger, o usuário da autenticação basic e
os **nomes** dos headers da autenticação custom (campo `customHeaders`).

O segredo **nunca** vem de argumento de linha de comando:

- Senha basic: defina a variável `FLUIGCLI_SERVICE_PASSWORD`. Em modo
  interativo, a CLI pede a senha num prompt oculto.
- Headers custom: envie pelo stdin com `--custom-headers-stdin` ou defina a
  variável `FLUIGCLI_SERVICE_CUSTOM_HEADERS`.

> **Atenção.** O Fluig guarda a senha basic e os headers custom em **texto
> aberto**. A API de listagem e a tela de edição do Painel de Controle mostram
> esses valores. Por isso, quem acessa o Painel de Controle lê o segredo. A CLI
> só não mostra o valor.

## Tipos de autenticação

O Fluig aceita seis tipos de autenticação em serviço REST:

| Tipo | A CLI grava? | Uso |
|---|---|---|
| `none` | sim | sem autenticação no cadastro. O dataset envia o token no header. |
| `basic` | sim | usuário e senha no cadastro. |
| `fluigAPI` | sim | chamada autenticada à API do próprio Fluig. |
| `oauth1` | não | configure pelo Painel de Controle. |
| `oauth2` | não | configure pelo Painel de Controle. |
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

## `fluigcli service create <código> --url <URL base> [flags]`

Este comando cadastra um serviço REST.

| Flag | Uso |
|---|---|
| `--url <URL>` | URL base do serviço (**obrigatório**). Comece com `http://` ou `https://`. |
| `--description <texto>` | descrição do serviço (default: o código) |
| `--auth <tipo>` | `none`, `basic`, `fluigAPI` ou `custom` (default: `none`) |
| `--username <login>` | usuário da autenticação basic. Obrigatório com `--auth basic`. |
| `--custom-headers-stdin` | lê do stdin os headers da autenticação custom. Alternativa: `FLUIGCLI_SERVICE_CUSTOM_HEADERS`. |
| `--test-path <caminho>` | caminho de teste do serviço. Por exemplo: `/health`. |
| `--swagger <URL>` | URL do swagger do serviço |

```sh
fluigcli service create sabia_agentos --url https://agentos.empresa.com --test-path /health
FLUIGCLI_SERVICE_PASSWORD=... fluigcli service create erp_api --url https://erp.empresa.com \
  --auth basic --username integracao
printf 'Authorization: Bearer %s\n' "$(cat token.txt)" | \
  fluigcli service create painel_api --url https://painel.empresa.com --auth custom --custom-headers-stdin
```

O `--custom-headers-stdin` e o `--password-stdin` leem o mesmo stdin. Por isso,
o comando recusa as duas flags juntas. Neste caso, use
`FLUIGCLI_SERVICE_CUSTOM_HEADERS` para os headers.

Código repetido termina com exit **5**. Neste caso, use `service update`.

## `fluigcli service update <código> [flags]`

Este comando altera um serviço REST. Ele usa as mesmas flags do `create`. O
comando troca só os campos das flags informadas. Os outros campos ficam iguais.
Por exemplo, a troca da URL mantém a senha guardada.

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

O comando recusa serviço SOAP ou JDBC com exit **2**. Código inexistente termina
com exit **4**.

## `fluigcli service delete <código>...`

Este comando exclui um ou mais serviços. Os datasets e as widgets que usam o
serviço deixam de funcionar. Por isso, o comando pede confirmação. Em modo
não-interativo, confirme com `--yes`.

```sh
fluigcli service delete zz_servico_antigo --yes
```

Código inexistente termina com exit **4**. Num lote, uma falha termina com
exit **6**.

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

O comando recusa serviço SOAP, JDBC, OAuth 1 e OAuth 2 com exit **2**.

### `fluigcli service apply <arquivo.json> [--dry-run]`

Este comando cria o serviço quando ele não existe. Quando ele existe, o
comando altera só os campos que diferem da especificação. Rodar de novo não
muda nada.

O segredo segue estas regras:

- Na criação com basic ou custom, a variável do `secretEnv` é obrigatória.
  Sem ela, o comando para com exit **2** antes de gravar.
- Na alteração sem a variável, o segredo guardado no servidor fica igual.
- Na alteração com a variável, a CLI compara o valor com o segredo guardado,
  sem mostrar nenhum dos dois. Ela só grava quando o valor mudou.

Use `--dry-run` para ver as mudanças sem gravar. Um campo desconhecido no
arquivo termina com exit **2**. Por isso, um campo `password` no arquivo é
recusado.

## Produção

Os comandos `create`, `update`, `apply` e `delete` pedem confirmação num servidor de
produção, como os outros comandos de escrita. Em modo não-interativo, confirme
com `--yes`.
