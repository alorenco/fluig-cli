# Documentação do fluigcli

Guia de cada grupo de comandos. Visão geral e uso por agentes de IA no
[README](../README.md).

- [server](server.md) — servidores, credenciais, `install-helper`
- [dataset](dataset.md) — datasets (list/import/export/query + enable/disable/history/restore)
- [event](event.md) — eventos globais
- [mechanism](mechanism.md) — mecanismos de atribuição
- [form](form.md) — formulários
- [workflow](workflow.md) — scripts de eventos de processo
- [widget](widget.md) — widgets
- [request](request.md) — solicitações de workflow (consultar, iniciar, movimentar, anexos)
- [task](task.md) — tarefas de workflow (a sua fila e a dos outros)
- [diff](diff.md) — comparar artefatos locais com o servidor antes de publicar
- [watch](watch.md) — publicar automaticamente ao salvar (só dev/hml)
- [dev](dev.md) — dev server local com live reload: widgets sem deploy e preview de formulários com simulação de processo (só dev/hml)
- [skill](skill.md) — Skill para agentes de IA (Claude Code / Codex)
- [upgrade](upgrade.md) — atualização da própria CLI (e o aviso de versão nova)

Convenção: **import** = servidor → local; **export** = local → servidor.
Comandos e flags em inglês; mensagens em pt-BR.
