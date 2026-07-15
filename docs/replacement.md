# fluigcli replacement — substitutos de usuário

Consulta e definição de **substituições de usuário** (delegação de tarefas). O
**titular** é quem será substituído; o **substituto** assume as tarefas de
workflow e/ou GED no período informado. **Requer usuário com privilégio
administrativo** (exit 3 sem ele). Alias: `substitute`.

Os argumentos de usuário são sempre **logins** — a CLI resolve para o `userCode`
internamente (o serviço compara pelo código; login inexistente vira exit **4**,
nunca um filtro silenciosamente ignorado).

Duas APIs sustentam o comando:

- **Leitura global** (`list`): REST v2 `process-management/api/v2/user-replacements`.
- **Escrita e leitura por usuário** (`show`/`create`/`update`/`delete`): SOAP
  `ECMColleagueReplacementService`. O SOAP expõe as flags de escopo
  (workflow/GED) e inclui as vigências **expiradas**; o REST não traz as flags.

## `fluigcli replacement list [flags]`

Lista as substituições cadastradas (titular, substituto, período).

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
**Workflow** e **GED**. Com `--valid-only`, só as vigentes na data atual.

```sh
fluigcli replacement show jsilva
fluigcli replacement show jsilva --valid-only --json
```

> O nome do substituto é enriquecido pela listagem REST quando disponível; para
> substituições expiradas (fora do REST) aparece o `userCode`.

## `fluigcli replacement create <titular> <substituto> --end <YYYY-MM-DD> [flags]`

Define um substituto para o titular no período informado.

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

Um par (titular, substituto) para o **mesmo período** não pode ser duplicado →
exit **5** (`Já existe uma Substituição …`). Titular ou substituto inexistente →
exit **4**.

## `fluigcli replacement update <titular> <substituto> [flags]`

Altera uma substituição existente. Faz **merge**: os campos não informados são
preservados. Identifica a substituição pelo par (titular, substituto).

```sh
fluigcli replacement update jsilva msouza --end 2026-09-30   # estende o prazo
fluigcli replacement update jsilva msouza --ged-tasks        # passa a cobrir GED
```

Aceita `--start`, `--end`, `--workflow-tasks` e `--ged-tasks` (ao menos um). Par
inexistente → exit **4**.

## `fluigcli replacement delete <titular> <substituto>`

Remove a substituição. Par inexistente → exit **4**.

```sh
fluigcli replacement delete jsilva msouza
```

## Observações

- As datas são **dia a dia** (sem hora): a CLI envia a data sem fuso e o
  servidor a interpreta no próprio fuso, preservando o dia.
- Em servidor `prod`, as operações de escrita (`create`/`update`/`delete`)
  respeitam a trava de confirmação (`--yes`).
- O contrato `--json` traz o objeto `replacements` (list/show) ou `replacement`
  (create/update); as flags de escopo (`workflowTasks`/`gedTasks`) só vêm no
  caminho SOAP (show/create/update), não no `list`.
