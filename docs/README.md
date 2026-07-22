# Documentação do fluigcli

Guia de cada grupo de comandos. O [README](../README.md) tem a visão geral e o
uso por agentes de IA.

- [server](server.md) — servidores, credenciais e `install-helper`
- [clone](clone.md) — onboarding. Clona os artefatos de um servidor existente para o projeto
- [dataset](dataset.md) — datasets (list/import/export/query e enable/disable/history/restore)
- [event](event.md) — eventos globais
- [mechanism](mechanism.md) — mecanismos de atribuição
- [form](form.md) — formulários
- [workflow](workflow.md) — scripts de eventos de processo
- [widget](widget.md) — widgets
- [request](request.md) — solicitações de workflow. Consulta, inicia, movimenta e trata anexos
- [task](task.md) — tarefas de workflow. A sua fila e a dos outros
- [document](document.md) — GED. Navega, baixa e publica documentos
- [log](log.md) — logs do servidor. Tail com filtros, follow e download (requer o fluigcliHelper)
- [user](user.md) — usuários da plataforma (requer admin)
- [group](group.md) — grupos da plataforma e membros (requer admin)
- [role](role.md) — papéis da plataforma e usuários (requer admin)
- [replacement](replacement.md) — substitutos de usuário e delegação de tarefas (requer admin)
- [diff](diff.md) — compara artefatos locais com o servidor antes de publicar
- [audit](audit.md) — audita formulários e widgets contra o Fluig Style Guide 2.0
- [watch](watch.md) — publica ao salvar (só dev/hml)
- [dev](dev.md) — dev server local com live reload. Serve widgets sem deploy e dá preview de formulários com simulação de processo (só dev/hml)
- [skill](skill.md) — Skill para agentes de IA (Claude Code / Codex)
- [upgrade](upgrade.md) — atualização da própria CLI e o aviso de versão nova

Convenção: **import** = servidor → local. **export** = local → servidor. Os
comandos e as flags ficam em inglês. As mensagens ficam em pt-BR.
