# Contrato de saída do fluigcli

Feito para consumo programático. Duas garantias estáveis: o **envelope JSON** e
os **exit codes**. Mudá-los é breaking change.

## Envelope `--json`

Com `--json`, o **stdout recebe exatamente um** documento JSON; todo o resto
(logs, avisos, progresso) vai para o **stderr**. Estrutura:

```json
{
  "ok": true,
  "command": "dataset list",
  "server": "homolog",
  "data": { "...": "conteúdo específico do comando" },
  "error": null
}
```

Em caso de falha:

```json
{
  "ok": false,
  "command": "dataset export",
  "server": "homolog",
  "data": null,
  "error": { "code": "SERVER_ERROR", "message": "mensagem humana em pt-BR" }
}
```

Em falha parcial de lote (`ok:false`, exit 6), `data` traz o resultado por item
e `error.code` é `PARTIAL_FAILURE` — inspecione cada item.

Regras para o agente:
- Parse **só** o stdout como JSON; nunca misture stderr.
- Ramifique pelo **exit code** e por `error.code` (estável, em inglês), **não**
  pelo texto de `error.message` (humano, pt-BR, pode mudar).

## Exit codes

| código | constante | code (JSON) | quando ocorre |
|---|---|---|---|
| 0 | `ExitOK` | — | sucesso total |
| 1 | `ExitGeneric` | `INTERNAL_ERROR` | erro inesperado |
| 2 | `ExitUsage` | `USAGE_ERROR` | flag/argumento inválido; faltou argumento em modo não-interativo |
| 3 | `ExitAuth` | `AUTH_FAILED` | login/sessão falhou |
| 4 | `ExitNotFound` | `NOT_FOUND` | dataset/form/processo/servidor inexistente |
| 5 | `ExitServer` | `SERVER_ERROR` | o servidor Fluig retornou erro |
| 6 | `ExitPartial` | `PARTIAL_FAILURE` | operação em lote com alguns itens falhos |
| 7 | `ExitMissingHelper` | `HELPER_NOT_INSTALLED` | falta a widget fluiggersWidget no servidor |

## Exemplo de consumo (bash)

```sh
out=$(fluigcli dataset list --json --server homolog) ; rc=$?
case $rc in
  0) echo "$out" | jq -r '.data.datasets[].name' ;;
  3) echo "auth falhou" >&2 ;;
  7) fluigcli server install-helper homolog --json ;;
  *) echo "$out" | jq -r '.error.message' >&2 ;;
esac
```

## Flags globais

| flag | env var | efeito |
|---|---|---|
| `--json` | — | envelope JSON em stdout (implica não-interativo) |
| `--non-interactive` | `FLUIGCLI_NON_INTERACTIVE=1` | falha em vez de perguntar |
| `--server <name>` | `FLUIGCLI_SERVER` | servidor alvo |
| `--project <dir>` | `FLUIGCLI_PROJECT` | raiz do projeto (default: descoberta automática) |
| `--password-stdin` | — | lê a senha do stdin (comandos de auth) |
| — | `FLUIGCLI_PASSWORD` | senha do servidor selecionado |
| `--timeout <dur>` | `FLUIGCLI_TIMEOUT` | timeout por requisição (ex.: `30s`, `1m`) |
| `--no-session-cache` | `FLUIGCLI_NO_SESSION_CACHE=1` | não reaproveita a sessão entre execuções |
| `--verbose` | — | loga as requisições HTTP no stderr (senha/cookies mascarados) |
| `--yes` / `-y` | — | assume "sim" em confirmações |
