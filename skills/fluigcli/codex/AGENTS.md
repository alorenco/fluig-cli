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
servidor · `6` falha parcial (ver `data`) · `7` falta a widget fluiggersWidget
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

Grupos: `server` (add|list|use|update|remove|test|logout|install-helper),
`dataset` (list|import|export|query), `event` (list|import|export|delete),
`mechanism` (list|import|export|delete), `form` (list|import|export),
`workflow` (version|export), `widget` (list|import|export), `diff` (local vs.
servidor, read-only — use antes de um export). O `watch` (publica ao salvar) é
interativo e não é indicado para agentes — prefira `diff` + `export`.

Sem `--server`, vale o servidor padrão (`server use`). ⚠️ Servidor com
`env=prod` exige `--yes` nos comandos de escrita em modo não-interativo.
