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
autenticação, o caminho de teste, o swagger e o usuário da autenticação basic.

A senha da autenticação basic **nunca** vem de argumento de linha de comando.
Defina a senha na variável `FLUIGCLI_SERVICE_PASSWORD`. Em modo interativo, a
CLI pede a senha num prompt oculto.

## Tipos de autenticação

O Fluig aceita seis tipos de autenticação em serviço REST:

| Tipo | A CLI grava? | Uso |
|---|---|---|
| `none` | sim | sem autenticação no cadastro. O dataset envia o token no header. |
| `basic` | sim | usuário e senha no cadastro. |
| `fluigAPI` | sim | chamada autenticada à API do próprio Fluig. |
| `oauth1` | não | configure pelo Painel de Controle. |
| `oauth2` | não | configure pelo Painel de Controle. |
| `custom` | não | configure pelo Painel de Controle. |

A CLI lista e mostra os seis tipos. O `update` altera a URL e os outros campos
de qualquer serviço REST. Neste caso, o segredo guardado no servidor continua
igual.

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
| `--auth <tipo>` | `none`, `basic` ou `fluigAPI` (default: `none`) |
| `--username <login>` | usuário da autenticação basic. Obrigatório com `--auth basic`. |
| `--test-path <caminho>` | caminho de teste do serviço. Por exemplo: `/health`. |
| `--swagger <URL>` | URL do swagger do serviço |

```sh
fluigcli service create sabia_agentos --url https://agentos.empresa.com --test-path /health
FLUIGCLI_SERVICE_PASSWORD=... fluigcli service create erp_api --url https://erp.empresa.com \
  --auth basic --username integracao
```

Código repetido termina com exit **5**. Neste caso, use `service update`.

## `fluigcli service update <código> [flags]`

Este comando altera um serviço REST. Ele usa as mesmas flags do `create`. O
comando troca só os campos das flags informadas. Os outros campos ficam iguais.
Por exemplo, a troca da URL mantém a senha guardada.

```sh
fluigcli service update sabia_agentos --url https://agentos2.empresa.com
fluigcli service update erp_api --auth none          # remove usuário e senha
FLUIGCLI_SERVICE_PASSWORD=... fluigcli service update erp_api   # troca só a senha
```

A CLI relê a senha basic em três casos: na troca para `basic`, na troca do
usuário e quando `FLUIGCLI_SERVICE_PASSWORD` está definida. A troca para
`none` ou `fluigAPI` apaga o usuário e a senha do cadastro.

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

## Produção

Os comandos `create`, `update` e `delete` pedem confirmação num servidor de
produção, como os outros comandos de escrita. Em modo não-interativo, confirme
com `--yes`.
