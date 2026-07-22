# fluigcli log — logs do servidor

O grupo `log` lê os logs do servidor de aplicação do Fluig. Ele lê o
`server.log` do WildFly e os arquivos rotacionados. Você faz isso do terminal,
sem acesso SSH. Você filtra as entradas e acompanha o log ao vivo.

Estes comandos precisam do componente auxiliar **fluigcliHelper 0.3.0 ou
superior** no servidor. Instale ou atualize o helper com o comando
`fluigcli server install-helper <name> [--force]`.

O helper encontra o diretório de log pela propriedade `jboss.server.log.dir`
do servidor. Por isso, os comandos funcionam em qualquer caminho de
instalação. Eles funcionam no Linux e no Windows. Eles funcionam no modo
standalone (Fluig 2.x) e no modo domain (Fluig 1.8).

O helper envia apenas o nome do arquivo. O helper aceita somente alguns
caracteres no nome. O helper verifica se o caminho fica dentro do diretório de
log. Esta verificação impede o acesso a outros diretórios (anti-traversal).

Estes comandos precisam de um usuário administrador do tenant. O servidor
registra cada download no log. Este registro é a trilha de auditoria.

O [`fluigcli dev`](dev.md#logs-do-servidor) tem uma versão visual. O painel
`/_dev/logs/` mostra o log ao vivo no navegador. O painel tem filtros, pausa e
cores.

## `fluigcli log files`

Este comando lista os arquivos do diretório de log. A lista mostra o tamanho e
a data de modificação de cada arquivo.

```sh
fluigcli log files
fluigcli log files --json
```

O diretório contém o `server.log` e os arquivos rotacionados (por exemplo,
`server.log.2026-07-17`). O diretório também contém outros arquivos do Fluig.
Um exemplo são os arquivos CSV de telemetria de eventos
(`CustomizationManagerImpl.invokeFunction.*.csv`). Você lê todos estes arquivos
com a opção `--file`.

## `fluigcli log tail`

Este comando mostra as últimas entradas do log. Uma **entrada** é a linha com
data e hora mais as linhas de continuação. Por exemplo, um stack trace é uma
entrada. O comando mostra o stack trace completo.

```sh
fluigcli log tail                        # últimas 100 entradas do server.log
fluigcli log tail -n 20                  # últimas 20
fluigcli log tail --level error          # só ERROR e FATAL
fluigcli log tail --grep "MeuProcesso"   # entradas que contêm o texto
fluigcli log tail --file server.log.2026-07-17 -n 50
fluigcli log tail -f                     # acompanha ao vivo (Ctrl+C sai)
```

- `-n, --lines` — o número de entradas. O valor padrão é 100. O valor máximo é
  5000.
- `--file` — o arquivo que você quer ler. Use `log files` para ver os
  arquivos. O valor padrão é `server.log`.
- `--level` — a severidade **mínima**: `trace`, `debug`, `info`, `warn`,
  `error` ou `fatal`. O comando mostra a severidade escolhida e as severidades
  maiores. Por exemplo, `--level warn` mostra as entradas WARN, ERROR e FATAL.
- `--grep` — o texto que você quer procurar. O comando não diferencia
  maiúsculas de minúsculas. O comando procura o texto na entrada completa. Por
  isso, ele encontra o texto também dentro de um stack trace.
- `--skip` — pula as N entradas mais recentes. Use esta opção para ver as
  entradas mais antigas. Por exemplo, `--skip 100 -n 100` mostra a página
  anterior.
- `-f, --follow` — acompanha o arquivo ao vivo, como o `tail -f`. O comando lê
  o arquivo a cada 2 segundos. Quando o servidor rotaciona o arquivo, o comando
  recomeça do início. Este modo é contínuo. Este modo **não aceita `--json`**.

O servidor aplica os filtros. Por isso, apenas as entradas necessárias trafegam
pela rede. No terminal, o comando mostra as entradas ERROR e FATAL em vermelho.
O comando mostra as entradas WARN em amarelo. As cores aparecem quando há um
TTY e a variável `NO_COLOR` não está definida.

Com `--json` (e sem `--follow`), o envelope traz
`{file, size, entries[], truncated}`. O valor `truncated=true` indica que a
resposta chegou ao limite de tamanho. Neste caso, o servidor cortou a lista.
Para reduzir o resultado, use `--grep` ou `--level`, ou diminua o valor de
`-n`.

## `fluigcli log download`

Este comando baixa um arquivo de log inteiro. O comando usa streaming. Por
isso, você baixa também os arquivos rotacionados grandes.

```sh
fluigcli log download                          # server.log → ./server.log
fluigcli log download --file server.log.2026-07-17 -o /tmp/ontem.log
```

## Exit codes

| código | quando |
|---|---|
| `0` | sucesso |
| `2` | uso incorreto (`--level` inválido, `--follow` com `--json`) |
| `4` | o arquivo de log não existe |
| `7` | fluigcliHelper ausente ou **desatualizado** (< 0.3.0, sem as rotas de log). Atualize com `server install-helper <name> --force`. |
