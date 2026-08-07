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
| 4 | `ExitNotFound` | `POOL_TASK_NOT_ASSIGNED` | `request move`: a tarefa corrente está num **pool** e ninguém a assumiu — a solicitação EXISTE (ver abaixo) |
| 4 | `ExitNotFound` | `NO_HUMAN_TASK` | `request move`: a solicitação está em **atividade automática** (service task) — não há tarefa humana para concluir |
| 5 | `ExitServer` | `SERVER_ERROR` | o servidor Fluig retornou erro |
| 5 | `ExitServer` | `TIMEOUT` | a requisição estourou o tempo limite do CLIENTE — **resultado desconhecido** (ver abaixo) |
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
  **Confira o `error.code` antes de concluir que o recurso não existe**: no
  `request move`, o servidor responde 404 também quando a tarefa não é sua, e
  a CLI desambigua. `POOL_TASK_NOT_ASSIGNED` = a solicitação existe e a tarefa
  está num pool sem dono (a mensagem traz o pool; NÃO é problema de permissão
  nem de id — assuma com `task assume <número>` se você pertence ao pool). `NO_HUMAN_TASK` = a
  solicitação está em atividade automática (aguarde o servidor ou olhe o
  `log tail`). Nos dois casos, repetir dá o mesmo 4 até o estado mudar.
- **exit 5 (servidor)**: erro do Fluig — pode ser **transitório**; um retry com
  pequeno backoff (1–2 tentativas) é razoável. Persistiu? Leia
  `error.message`. **Confira o `error.code` antes de repetir**: com `TIMEOUT` a
  regra é outra (abaixo).
- **exit 5 com `error.code == "TIMEOUT"`**: quem desistiu foi o CLIENTE, não o
  servidor. **O resultado é desconhecido.** O Fluig continua processando depois
  que a CLI para de esperar — em produção, um `request move` estourou o tempo
  limite e a movimentação foi concluída no servidor. **NUNCA repita uma escrita
  às cegas depois de um TIMEOUT** (repetir movimenta duas vezes). Faça assim:
  1. Leia `data.outcome` quando existir. O `request move` já relê o estado
     sozinho: `moved` = a tarefa alvo saiu de aberto (**não repita**),
     `not_moved` = a tarefa continua aberta (pode repetir com `--timeout`
     maior), `unknown` = a releitura falhou (verifique você).
  2. Sem `outcome`, verifique o estado antes de agir. O `request start` devolve
     `data.checkCommand` — o comando pronto que responde se a solicitação
     nasceu.
  3. Em leitura pura (`list`, `show`, `query`), repetir é seguro.

  ```sh
  out=$(fluigcli request move 230005 --movement 15 --json); rc=$?
  if [ "$rc" -eq 5 ] && [ "$(echo "$out" | jq -r '.error.code')" = "TIMEOUT" ]; then
    case "$(echo "$out" | jq -r '.data.outcome')" in
      moved)     echo "já movimentou — seguir em frente" ;;
      not_moved) fluigcli request move 230005 --movement 15 --timeout 5m --json ;;
      *)         fluigcli request show 230005 --json ;;   # decidir pelo estado
    esac
  fi
  ```
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
| `--project <dir>` | `FLUIGCLI_PROJECT` | raiz do projeto (default: descoberta automática, subindo do cwd — primeiro procurando `.fluigcli/`, que vence sempre; sem ele, as pastas convencionais). Rodar de DENTRO de `forms/<nome>/`, `datasets/` ou qualquer subpasta funciona. ⚠️ Rodando de FORA do projeto (scratchpad, /tmp), os servidores do `.fluigcli/servers.json` ficam invisíveis e o alvo dá `NOT_FOUND` — a mensagem diz que nenhum projeto foi descoberto; a correção é esta flag, não cadastrar o servidor de novo |
| `--password-stdin` | — | lê a senha do stdin (comandos de auth) |
| — | `FLUIGCLI_PASSWORD` | senha do servidor selecionado |
| `--timeout <dur>` | `FLUIGCLI_TIMEOUT` | timeout por requisição (ex.: `30s`, `1m`). Default 30s; **piso de 2m** nas operações de ESCRITA e nas LEITURAS PESADAS (`db query`, `db grants`, `dataset query`, `user audit`, `log tail --since/--until`). O valor informado aqui sempre vence, inclusive para baixo. Com `-v`, a CLI diz no stderr quando elevou. ⚠️ O timeout é do CLIENTE: o servidor segue executando depois que a CLI desiste |
| `--no-session-cache` | `FLUIGCLI_NO_SESSION_CACHE=1` | não reaproveita a sessão entre execuções |
| `--verbose` | — | loga as requisições HTTP no stderr (senha/cookies mascarados) |
| `--yes` / `-y` | — | assume "sim" em confirmações |
