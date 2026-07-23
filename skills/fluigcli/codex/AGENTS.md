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
`dataset` (new|list|import|export|query|enable|disable|history|restore),
`db` (query|datasources — SQL de LEITURA de diagnóstico via datasource JNDI, requer o fluigcliHelper),
`event` (new|list|import|export|delete),
`mechanism` (new|list|import|export|delete), `form` (new|list|import|export|link|records — CRUD de registros),
`workflow` (new-script|list|version|versions|import|export|publish|diff — `--process-id` desacopla arquivo do processId do servidor),
`widget` (new|list|import|export),
`request` (list|show|start|move|assignees|attachments — solicitações de workflow),
`task` (list — fila de tarefas; sem flags = as suas em aberto),
`document` (list|download|upload|mkdir|delete — GED),
`user` (list|show|create|update|activate|deactivate — requer admin; senha do
novo usuário só via FLUIGCLI_NEW_USER_PASSWORD/prompt), `group` e `role` (CRUD
+ users|add-user|remove-user; requerem admin), `replacement` (list|show|create|
update|delete — substituto/delegação de tarefas; requer admin), `diff` (local
vs. servidor, read-only — use antes de um export), `audit` (linter do Style
Guide 2.0 em forms/widgets; exit 1 = reprovado, corrija pelas `suggestion`
dos `data.findings[]` e repita). Os `new`/`new-script` são
scaffolds **locais** (nada vai ao servidor; nunca sobrescrevem; o
`workflow new-script <pid> <evento>` gera a assinatura correta do evento — o
catálogo está no `--help`). O `watch` (publica ao salvar) e
o `dev` (dev server local com live reload) são interativos e não são indicados
para agentes — prefira `diff` + `export`.

Sem `--server`, vale o servidor padrão (`server use`). ⚠️ Servidor com
`env=prod` exige `--yes` nos comandos de escrita em modo não-interativo.
