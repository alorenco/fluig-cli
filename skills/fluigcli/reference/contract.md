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
| 7 | `ExitMissingHelper` | `HELPER_NOT_INSTALLED` | falta o componente auxiliar (fluigcliHelper) no servidor |

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

## Estratégia por exit code (o que o agente faz)

- **exit 2 (uso)**: você errou a flag/argumento — **conserte o comando**, não
  reenvie igual. Consulte `--help` do subcomando.
- **exit 3 (auth)**: sessão/senha — confira `FLUIGCLI_PASSWORD`/`FLUIGCLI_USERNAME`
  e `server test`. Não adianta repetir sem mudar a credencial.
- **exit 4 (não encontrado)**: id/nome/login inexistente — **corrija o
  identificador; NÃO repita** o mesmo comando (o retry dá o mesmo 4).
- **exit 5 (servidor)**: erro do Fluig — pode ser **transitório**; um retry com
  pequeno backoff (1–2 tentativas) é razoável. Persistiu? Leia
  `error.message`.
- **exit 6 (lote parcial)**: alguns itens falharam. `data.results[]` traz
  `{id, action, success, error}` — **reprocesse só os que falharam**:

  ```sh
  out=$(fluigcli dataset export datasets/*.js --json --server homolog); rc=$?
  if [ "$rc" -eq 6 ]; then
    echo "$out" | jq -r '.data.results[] | select(.success==false) | "\(.id): \(.error)"'
    # corrija esses e reenvie só eles
  fi
  ```
- **exit 7 (dependência)**: falta o componente auxiliar — rode
  `fluigcli server install-helper <name>` uma vez e repita o comando.

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
