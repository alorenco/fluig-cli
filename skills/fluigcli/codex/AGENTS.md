## fluigcli — desenvolvimento TOTVS Fluig (CLI para agentes)

`fluigcli` é uma CLI não-interativa para TOTVS Fluig. Use-a para datasets,
formulários, eventos globais, mecanismos de atribuição, scripts de evento de
processo e widgets. Dirija-a por flags, leia o envelope JSON e **decida pelo
exit code** — não pelo texto (humano, pt-BR).

Regras de ouro:
- Sempre `--json` e `--non-interactive`.
- **Nunca** passe senha em argumento (vaza em `ps`). Use `FLUIGCLI_PASSWORD` ou
  `--password-stdin`. A sessão é reaproveitada entre execuções.
- Direção dos verbos: `import` = servidor → local · `export` = local → servidor.
- Consulte `fluigcli <cmd> --help` para as flags exatas.

Exit codes: `0` ok · `2` uso · `3` auth · `4` não encontrado · `5` erro do
servidor · `6` falha parcial (ver `data`) · `7` falta o componente auxiliar (fluigcliHelper)
(rode `fluigcli server install-helper <name>`).

⚠️ Exit `5` com `error.code == "TIMEOUT"` = quem desistiu foi o CLIENTE; o
resultado da operação é **desconhecido** e o servidor pode ter concluído.
**NUNCA repita uma escrita às cegas nesse caso** (repetir um `request move`
movimenta duas vezes). O `move` já relê o estado e devolve `data.outcome`:
`moved` (não repita) · `not_moved` (pode repetir com `--timeout` maior) ·
`unknown` (verifique com `request show`). O `start` devolve
`data.checkCommand`. Leitura pura pode repetir sem medo. A escrita usa piso de
2m de tempo limite; `--timeout` sempre vence.

Envelope (stdout recebe só isto; logs vão para stderr):
`{ "ok": bool, "command": str, "server": str, "data": any, "error": {"code","message"}|null }`
Ramifique por exit code e por `error.code` (estável, inglês), nunca por `message`.

Setup e uso:
```sh
echo "$SENHA" | fluigcli server add --name homolog --host HOST --port 443 --ssl \
  --username USER --company-id 1 --password-stdin --json
echo "$SENHA" | fluigcli server test homolog --password-stdin --json
export FLUIGCLI_SERVER=homolog FLUIGCLI_PASSWORD="$SENHA"
fluigcli dataset list --json
fluigcli dataset export datasets/ds_x.js --json     # publica (local → servidor)
```

Grupos: `server` (add|list|use|update|remove|test|status|logout|install-helper),
`dataset` (new|list|import|export|query|enable|disable|history|restore|delete — `delete` = hard-delete via helper;
o `export` AUDITA antes de enviar: erro de audit barra o arquivo com exit 1 e ele
não chega ao servidor — corrija pelo achado ou envie com `--no-audit`. O MESMO
gate vale em `event export`, `mechanism export`, `workflow export|publish` (nestes
o aborto é total: publicação atômica) e `form export` (só `RHINO*`/`FL*` barram;
`SG*` de tema visual não)),
`db` (query|grants|datasources — SQL de LEITURA de diagnóstico via datasource JNDI, requer o fluigcliHelper;
`query --file script.sql` roda o script instrução por instrução — `--list` só lista,
`--statement N` roda uma, falha parcial = exit 6 com `data.statements[]`),
`event` (new|list|import|export|delete),
`mechanism` (new|list|import|export|delete), `form` (new|list|import|export|link|records — CRUD de registros;
`records show` traz as linhas das tabelas filhas agrupadas por `tableId`, use `--no-children` para só o pai),
`workflow` (new-script|list|version|versions|import|export|publish|diff — `--process-id` desacopla arquivo do processId do servidor),
`widget` (new|list|import|export — o `export` RECUSA com exit 2 se o código já
existir no servidor como LAYOUT, porque o upload sobrescreveria o WAR do layout;
renomeie o widget ou publique com `--force`),
`request` (list|show|start|move|assignees|attachments — solicitações de workflow),
`task` (list — fila de tarefas; sem flags = as suas em aberto),
`log` (files|tail|download — server.log do servidor via helper; `--grep` repetível
= OU, filtrado no servidor (2+ padrões exigem helper ≥ 0.8.0); `tail --since 30m`
ou `--since 18:19 --until 18:30` recorta uma janela de tempo na hora do SERVIDOR
e exige helper ≥ 0.5.0; o `--json` traz `records[]` já decomposto em
`{timestamp,level,logger,thread,message,stack}` além do `entries[]` cru;
`tail --follow --ndjson --until-match "txt" --for 2m` é o MONITOR de agente —
uma entrada JSON por linha, começa agora, exit 0 se apareceu e **exit 4** se
o tempo acabou antes),
`document` (list|download|upload|mkdir|delete — GED),
`user` (list|show|create|update|activate|deactivate — requer admin; senha do
novo usuário só via FLUIGCLI_NEW_USER_PASSWORD/prompt), `group` e `role` (CRUD
+ users|add-user|remove-user; requerem admin), `replacement` (list|show|create|
update|delete — substituto/delegação de tarefas; requer admin), `diff` (local
vs. servidor, read-only — use antes de um export; artefato quebrado no servidor
vira `status:"error"` no item e exit 6, o resto segue comparado),
`deploy` (`--plan release.json` executa um release na ordem: passos `dataset`/
`event`/`mechanism`/`form`/`widget`/`workflow` (publish)/`db`; para no 1º erro e marca o
resto `skipped`, retome com `--from N`; `--dry-run` valida tudo sem escrever —
inclusive se cada evento local EXISTE no processo; audita todos os scripts antes
de começar), `audit` (linter do Style
Guide 2.0 em forms/widgets; exit 1 = reprovado, corrija pelas `suggestion`
dos `data.findings[]` e repita). Os `new`/`new-script` são
scaffolds **locais** (nada vai ao servidor; nunca sobrescrevem; o
`workflow new-script <pid> <evento>` gera a assinatura correta do evento — o
catálogo está no `--help`). O `watch` (publica ao salvar) e
o `dev` (dev server local com live reload) são interativos e não são indicados
para agentes — prefira `diff` + `export`.

Sem `--server`, vale o servidor padrão (`server use`). ⚠️ Servidor com
`env=prod` exige `--yes` nos comandos de escrita em modo não-interativo.
