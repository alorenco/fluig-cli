# fluigcli log — logs do servidor

O grupo `log` lê os logs do servidor de aplicação do Fluig. Ele lê o
`server.log` do WildFly e os arquivos rotacionados. Você faz isso do terminal,
sem acesso SSH. Você filtra as entradas e acompanha o log ao vivo.

Estes comandos precisam do componente auxiliar **fluigcliHelper 0.3.0 ou
superior** no servidor. A janela de tempo pede 0.5.0 e o OU de vários `--grep`
pede 0.8.0. Instale ou atualize o helper com o comando
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

Este comando lista os arquivos do diretório de log, **do mais recente para o
mais antigo**. A lista mostra o tamanho e a data de modificação de cada arquivo.

```sh
fluigcli log files                              # os 20 arquivos de log mais recentes
fluigcli log files --pattern 'chrono.log*'      # filtra por nome (glob)
fluigcli log files --all --limit 0              # tudo, sem limite
fluigcli log files --json
```

| Flag | Uso |
|---|---|
| `--all` | inclui os arquivos que não são de log, por exemplo os CSVs de monitoramento |
| `--pattern <glob>` | filtra por nome. O padrão manda: ele também alcança os CSVs |
| `--limit N` | número máximo de arquivos. O valor padrão é 20. Use `0` para não limitar |

O diretório de log do Fluig é grande. Na homologação ele tem **622 arquivos**, e
**395 são CSVs de monitoramento** (por exemplo
`CustomizationManagerImpl.invokeFunction.*.csv`). Estes arquivos poluem a
leitura. Por isso a listagem padrão traz somente os arquivos de log, ou seja
`server.log`, as rotações (`server.log.2026-07-17`) e os outros `*.log`
(`chrono.log`, `audit.log`, `conversion.log`).

A CLI sempre informa quantos arquivos ficaram de fora. **Nada é omitido em
silêncio.** Na homologação, a listagem padrão gasta 1,9 KB. Com `--all --limit 0`
ela gasta 72 KB.

Com `--json`, o envelope traz `{files[], total, omitted}`. O campo `total` é a
quantidade de arquivos no diretório e `omitted` é quanto ficou fora da listagem.

Você lê qualquer um destes arquivos com a opção `--file` do `tail` e do
`download`, inclusive os CSVs.

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
  isso, ele encontra o texto também dentro de um stack trace. **Repita a opção
  para procurar vários textos**: a entrada passa quando casa com qualquer um
  deles (OU).

  ```sh
  fluigcli log tail --grep "Session expired" --grep "AlertAppSender"
  ```

  O servidor aplica o filtro. Este é o ponto importante: o servidor corta a
  resposta em 5000 entradas ou 2 MB. Um filtro no lado da CLI rodaria **depois**
  do corte e perderia entradas em silêncio. Por isso vários `--grep` exigem o
  fluigcliHelper **0.8.0 ou maior**. Com uma versão anterior o comando sai com
  exit 7 e orienta a atualização. Um `--grep` só continua funcionando em
  qualquer versão.
- `--skip` — pula as N entradas mais recentes. Use esta opção para ver as
  entradas mais antigas. Por exemplo, `--skip 100 -n 100` mostra a página
  anterior.
- `-f, --follow` — acompanha o arquivo ao vivo, como o `tail -f`. O comando lê
  o arquivo a cada 2 segundos. Quando o servidor rotaciona o arquivo, o comando
  recomeça do início. Este modo é contínuo. Este modo **não aceita `--json`**.

### Janela de tempo (`--since` e `--until`)

Use `--since` e `--until` para procurar um momento específico. O comando busca
as entradas dentro da janela, em vez das últimas entradas.

```sh
fluigcli log tail --since 30m                             # os últimos 30 minutos
fluigcli log tail --since 18:19 --until 18:30             # hoje, entre 18:19 e 18:30
fluigcli log tail --since 2026-07-24 --until 2026-07-24   # o dia 24 inteiro
fluigcli log tail --since "2026-07-24T18:19" --level error
```

Os formatos aceitos são:

- **duração** — `30m`, `2h`, `1h30m`. O comando volta esse tanto no tempo.
- **hora de hoje** — `18:19` ou `18:19:05`.
- **data** — `2026-07-24`. No `--since` a janela começa às 00:00. No `--until` a
  janela termina no fim do dia.
- **data e hora** — `2026-07-24T18:19` ou `"2026-07-24 18:19:30"`.

⚠️ `18h` é uma **duração** (18 horas atrás), não a hora 18:00. Para a hora do
dia, escreva `18:00`.

Os horários são os do log, ou seja, a hora local do **servidor**. O timestamp do
`server.log` não tem fuso. Por isso, a CLI pergunta o fuso ao fluigcliHelper
(0.4.0 ou maior) e resolve as durações e as horas de hoje nesse fuso. Quando o
helper não informa o fuso, a CLI usa o horário desta máquina e avisa.

A janela é um recorte fechado. Ela não aceita `--follow`, `-n` nem `--skip`. Os
filtros `--level`, `--grep` e `--file` continuam valendo. Com `--json`, o
envelope traz `{file, from, to, entries[], records[], truncated}` — não traz
`size`, porque a janela não usa o offset do arquivo.

A janela exige o fluigcliHelper **0.5.0 ou maior**. Com uma versão anterior, o
comando sai com exit 7 e orienta a atualização.

O servidor aplica os filtros. Por isso, apenas as entradas necessárias trafegam
pela rede. No terminal, o comando mostra as entradas ERROR e FATAL em vermelho.
O comando mostra as entradas WARN em amarelo. As cores aparecem quando há um
TTY e a variável `NO_COLOR` não está definida.

### Monitor para agente (`--follow --ndjson`)

O `--follow` sozinho é interativo e não aceita `--json`, porque o envelope JSON
é único por execução. Para um monitor programático, use `--follow --ndjson`.
Neste modo cada linha do stdout é **um objeto JSON completo** de uma entrada, no
mesmo formato do `records[]`. As mensagens humanas vão para o stderr. Assim o
stdout fica exclusivo dos dados.

⚠️ Neste modo o stream começa **agora**. O histórico (as últimas `-n` entradas)
não entra. Esta regra evita um falso positivo: um `--until-match` casaria com uma
entrada antiga e o monitor terminaria antes do evento que você espera.

Combine o modo com uma condição de parada. Sem nenhuma, o comando espera
`Ctrl+C`.

| Flag | Efeito |
|---|---|
| `--until-match <texto>` | encerra na primeira entrada que contém o texto (exit **0**) |
| `--for <duração>` | acompanha por no máximo esse tempo |
| `--idle-timeout <duração>` | encerra depois desse tempo sem entrada nova |
| `--max-entries <n>` | encerra depois de emitir N entradas |

Com `--until-match`, o resultado é um veredicto:

- o texto apareceu → exit **0**;
- o tempo ou o limite acabou antes → exit **4** (não encontrado). O agente
  distingue "vi o que esperava" de "desisti de esperar".

As entradas vistas no caminho saem no stream nos dois casos. O monitor não perde
dado.

```sh
# dispare a ação e espere o efeito no log, com teto de tempo
fluigcli log tail --follow --ndjson --until-match "ZZTOKEN" --for 2m
echo "exit=$?"   # 0 = apareceu · 4 = não apareceu no tempo

# só os erros, no máximo 20 entradas
fluigcli log tail --follow --ndjson --level error --max-entries 20
```

O acompanhamento é por **entrada**, não por linha. Um stack trace nunca sai
partido. Uma entrada só fecha quando a próxima começa, ou quando o log fica em
silêncio por um ciclo de leitura. Por isso a última entrada aparece com alguns
segundos de atraso.

### Saída `--json`

Com `--json` (e sem `--follow`), o envelope traz
`{file, size, entries[], records[], truncated}`. O valor `truncated=true`
indica que a resposta chegou ao limite de tamanho. Neste caso, o servidor cortou
a lista. Para reduzir o resultado, use `--grep` ou `--level`, ou diminua o valor
de `-n`.

O campo `entries[]` traz o texto cru de cada entrada. O campo `records[]` traz a
mesma entrada **decomposta em campos**. Os dois têm o mesmo tamanho e a mesma
ordem, então `records[i]` corresponde a `entries[i]`. Cada record tem:

| campo | conteúdo |
|---|---|
| `timestamp` | data e hora, no formato `2026-07-25T17:58:16.089` |
| `level` | a severidade, como veio no log (`INFO`, `WARN`, `ERROR`…) |
| `logger` | a classe entre colchetes |
| `thread` | a thread entre parênteses |
| `message` | o texto da primeira linha |
| `stack` | as linhas de continuação, por exemplo o stack trace |
| `raw` | a entrada inteira, **somente** quando a CLI não reconhece o cabeçalho |

⚠️ O `timestamp` é a hora local do **servidor**. O `server.log` não registra o
fuso. Use `server status` para ver o fuso do servidor.

O formato do cabeçalho é configurável em cada servidor. Quando a CLI não
reconhece o cabeçalho, ela devolve a entrada em `raw` e deixa os outros campos
vazios. A CLI nunca descarta conteúdo.

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
| `2` | uso incorreto (`--level` inválido, `--follow` com `--json`, `--ndjson` com `--json`, `--ndjson`/condição de parada sem `--follow`, `--since` com `--follow`/`-n`/`--skip`, valor de janela irreconhecível) |
| `4` | o arquivo de log não existe; ou `--until-match` não apareceu antes da condição de parada |
| `7` | fluigcliHelper ausente ou **desatualizado** (< 0.3.0 sem as rotas de log; < 0.5.0 sem a janela de tempo; < 0.8.0 sem o OU de vários `--grep`). Atualize com `server install-helper <name> --force`. |
