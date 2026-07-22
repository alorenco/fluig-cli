# fluigcli replacement — substitutos de usuário

O grupo `replacement` consulta e define **substituições de usuário** (delegação
de tarefas). O **titular** é quem o substituto substitui. O **substituto** assume
as tarefas de workflow e de GED no período informado. Estes comandos precisam de
um usuário com privilégio administrativo. Sem esse privilégio, o comando termina
com exit 3. O alias é `substitute`.

Os argumentos de usuário são sempre **logins**. A CLI resolve o login para o
`userCode` internamente. O serviço compara pelo código. Login inexistente vira
exit **4**. A CLI nunca ignora o filtro em silêncio.

Duas APIs sustentam o comando:

- **Leitura global** (`list`): REST v2 `process-management/api/v2/user-replacements`.
- **Escrita e leitura por usuário** (`show`/`create`/`update`/`delete`): SOAP
  `ECMColleagueReplacementService`. O SOAP expõe as flags de escopo
  (workflow/GED) e inclui as vigências **expiradas**. O REST não traz as flags.

## `fluigcli replacement list [flags]`

Este comando lista as substituições cadastradas (titular, substituto, período).

| Flag | Uso |
|---|---|
| `--user <login>` | filtra pelo titular |
| `--replaced-by <login>` | filtra pelo substituto |
| `--limit N` | máximo (default 50; 0 = todas) |

```sh
fluigcli replacement list
fluigcli replacement list --user jsilva
fluigcli replacement list --replaced-by msouza --json
```

## `fluigcli replacement show <login> [--valid-only]`

Mostra as substituições de um usuário (titular), com as colunas de escopo
**Workflow** e **GED**. Com `--valid-only`, o comando mostra só as substituições
vigentes na data atual.

```sh
fluigcli replacement show jsilva
fluigcli replacement show jsilva --valid-only --json
```

> A listagem REST fornece o nome do substituto quando o dado está disponível. A
> substituição expirada fica fora do REST. Neste caso, aparece o `userCode`.

## `fluigcli replacement create <titular> <substituto> --end <YYYY-MM-DD> [flags]`

Este comando define um substituto para o titular no período informado.

| Flag | Uso |
|---|---|
| `--start <YYYY-MM-DD>` | início da vigência (default: **hoje**) |
| `--end <YYYY-MM-DD>` | fim da vigência (**obrigatório**) |
| `--workflow-tasks` | o substituto assume as tarefas de workflow (default **true**) |
| `--ged-tasks` | o substituto assume as tarefas de GED (default **false**) |

```sh
fluigcli replacement create jsilva msouza --end 2026-08-31
fluigcli replacement create jsilva msouza --start 2026-07-20 --end 2026-08-10 --ged-tasks
```

Você não pode duplicar um par (titular, substituto) no **mesmo período**. Este
caso vira exit **5** (`Já existe uma Substituição …`). Titular ou substituto
inexistente vira exit **4**.

## `fluigcli replacement update <titular> <substituto> [flags]`

Este comando altera uma substituição existente. Ele faz **merge**. O comando
preserva os campos que você não informa. O comando identifica a substituição
pelo par (titular, substituto).

```sh
fluigcli replacement update jsilva msouza --end 2026-09-30   # estende o prazo
fluigcli replacement update jsilva msouza --ged-tasks        # passa a cobrir GED
```

O comando aceita `--start`, `--end`, `--workflow-tasks` e `--ged-tasks`. Informe
ao menos uma destas flags. Par inexistente vira exit **4**.

## `fluigcli replacement delete <titular> <substituto>`

Este comando remove a substituição. Par inexistente vira exit **4**.

```sh
fluigcli replacement delete jsilva msouza
```

## Observações

- As datas são **dia a dia**, sem hora. A CLI envia a data sem fuso. O servidor
  interpreta a data no próprio fuso e preserva o dia.
- Em servidor `prod`, as operações de escrita (`create`/`update`/`delete`)
  respeitam a trava de confirmação (`--yes`).
- O contrato `--json` traz o objeto `replacements` (list/show) ou `replacement`
  (create/update). As flags de escopo (`workflowTasks`/`gedTasks`) vêm só no
  caminho SOAP (show/create/update). O `list` não traz essas flags.
