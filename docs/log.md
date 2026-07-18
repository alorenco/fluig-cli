# fluigcli log — logs do servidor

Lê os logs do servidor de aplicação do Fluig (o `server.log` do WildFly e os
arquivos rotacionados) **remotamente, sem acesso SSH** — direto do terminal,
com filtros e acompanhamento ao vivo.

Requer o componente auxiliar **fluigcliHelper ≥ 0.3.0** publicado no servidor
(instale ou atualize com `fluigcli server install-helper <name> [--force]`).
O helper resolve o diretório de log pela propriedade `jboss.server.log.dir`
do próprio servidor de aplicação — funciona em qualquer caminho de instalação
e sistema operacional (Linux ou Windows), tanto em standalone (Fluig 2.x)
quanto em domain (1.8). Só o nome do arquivo trafega: o helper restringe os
caracteres aceitos e valida que o caminho canônico continua dentro do
diretório de log (anti-traversal).

Como toda rota do helper, exige **usuário administrador do tenant** — e cada
download fica registrado no próprio log do servidor (trilha de auditoria).

## `fluigcli log files`

Lista os arquivos do diretório de log, com tamanho e data de modificação.

```sh
fluigcli log files
fluigcli log files --json
```

Além do `server.log` e dos rotacionados (`server.log.2026-07-17`), o
diretório costuma conter outros arquivos do Fluig — como os CSVs de
telemetria de eventos (`CustomizationManagerImpl.invokeFunction.*.csv`) —
todos legíveis pelos demais comandos via `--file`.

## `fluigcli log tail`

Mostra as últimas entradas do log. Uma **entrada** é a linha com timestamp
mais as continuações dela — um stack trace inteiro conta como uma entrada só
e vem completo.

```sh
fluigcli log tail                        # últimas 100 entradas do server.log
fluigcli log tail -n 20                  # últimas 20
fluigcli log tail --level error          # só ERROR e FATAL
fluigcli log tail --grep "MeuProcesso"   # entradas que contêm o texto
fluigcli log tail --file server.log.2026-07-17 -n 50
fluigcli log tail -f                     # acompanha ao vivo (Ctrl+C sai)
```

- `-n, --lines` — número de entradas (default 100; máximo 5000).
- `--file` — outro arquivo do diretório (veja `log files`); default `server.log`.
- `--level` — severidade **mínima**: `trace`, `debug`, `info`, `warn`,
  `error` ou `fatal` (`--level warn` = WARN + ERROR + FATAL).
- `--grep` — substring, sem diferenciar maiúsculas, avaliada na entrada
  completa (pega o texto mesmo quando está no stack trace).
- `--skip` — pula as N entradas mais recentes (paginação para trás:
  `--skip 100 -n 100` é a "página anterior").
- `-f, --follow` — segue acompanhando o arquivo, como `tail -f` (polling a
  cada 2 s; rotação do arquivo é detectada e recomeça do zero). É um modo
  contínuo: **não suporta `--json`**.

Os filtros rodam **no servidor** — só as entradas que interessam trafegam.
No terminal, entradas ERROR/FATAL saem em vermelho e WARN em amarelo (quando
há TTY e sem `NO_COLOR`).

Com `--json` (sem `--follow`), o envelope traz
`{file, size, entries[], truncated}` — `truncated=true` indica que o limite
de tamanho da resposta cortou a lista (refine com `--grep`/`--level` ou
reduza `-n`).

## `fluigcli log download`

Baixa um arquivo de log inteiro (streaming — serve para os rotacionados
grandes).

```sh
fluigcli log download                          # server.log → ./server.log
fluigcli log download --file server.log.2026-07-17 -o /tmp/ontem.log
```

## Exit codes

| código | quando |
|---|---|
| `0` | sucesso |
| `2` | uso incorreto (`--level` inválido, `--follow` com `--json`) |
| `4` | arquivo de log inexistente |
| `7` | fluigcliHelper ausente — ou **desatualizado** (< 0.3.0, sem as rotas de log; atualize com `server install-helper <name> --force`) |
