---
layout: home

hero:
  name: fluigcli
  text: TOTVS Fluig direto do terminal
  tagline: CLI não oficial para importar, implantar e automatizar os artefatos da plataforma. Feita para desenvolvedores, agentes de IA e CI/CD.
  actions:
    - theme: brand
      text: Começar
      link: /instalacao
    - theme: alt
      text: Comandos
      link: /server
    - theme: alt
      text: GitHub
      link: https://github.com/alorenco/fluig-cli

features:
  - icon: 🧩
    title: Artefatos da plataforma
    details: Datasets, formulários, eventos globais, mecanismos de atribuição, scripts de processo e widgets. Use import para trazer do servidor para o local. Use export para enviar do local para o servidor. Confira o diff antes de publicar.
    link: /dataset
    linkText: dataset · form · workflow · widget
  - icon: ⚡
    title: Dev loop de verdade
    details: O fluigcli dev é um proxy local autenticado do portal. O JS e o CSS das widgets saem do disco. Os formulários têm preview com simulação de processo. O navegador recarrega ao salvar. O widget new gera scaffolds prontos em classic, Vue 3, React 19 e Vuetify 3.
    link: /dev
    linkText: dev · watch · widget new
  - icon: 🗂️
    title: Operação no dia a dia
    details: Inicie e movimente solicitações. Consulte a fila de tarefas. Navegue no GED. Administre usuários, grupos, papéis e substitutos. Faça tudo sem abrir o portal.
    link: /request
    linkText: request · task · document · user
  - icon: 🤖
    title: Feita para agentes e CI
    details: Modo não-interativo, envelope --json estável e exit codes de 0 a 7 documentados. Uma Skill embutida para o Claude Code e o Codex. Nunca passe a senha em argumento. Use o keyring, uma env var ou o stdin.
    link: /skill
    linkText: skill · contrato de saída
---

> ⚠️ **Projeto não oficial, sem qualquer vínculo com a TOTVS.**
> "Fluig" e "TOTVS" são marcas de seus respectivos donos.

## Todos os grupos de comandos

Uma convenção vale para tudo. **import** = servidor → local. **export** =
local → servidor. Os comandos e as flags estão em inglês. As mensagens estão
em pt-BR.

| Grupo | O que faz |
|---|---|
| [server](./server) | servidores, credenciais, sessão, saúde e `install-helper` |
| [dataset](./dataset) | datasets: consulta, CRUD, ativação, histórico e restauração |
| [db](./db) | SQL de leitura de diagnóstico via datasource JNDI |
| [event](./event) | eventos globais |
| [mechanism](./mechanism) | mecanismos de atribuição |
| [form](./form) | formulários e registros (CRUD de cards) |
| [workflow](./workflow) | scripts de eventos de processo, publish nativo |
| [widget](./widget) | widgets: scaffold, import e deploy nativo |
| [diff](./diff) | comparar local × servidor antes de publicar |
| [audit](./audit) | auditar forms/widgets contra o Style Guide 2.0 |
| [watch](./watch) | publicar automaticamente ao salvar (dev/hml) |
| [dev](./dev) | dev server com live reload e preview de formulários |
| [request](./request) | solicitações: consultar, iniciar, movimentar, anexos |
| [task](./task) | fila de tarefas (a sua e a dos outros) |
| [document](./document) | GED: navegar, baixar e publicar documentos |
| [log](./log) | logs do servidor: tail com filtros, follow e download |
| [user](./user) | usuários da plataforma (requer admin) |
| [group](./group) | grupos e membros (requer admin) |
| [role](./role) | papéis e usuários (requer admin) |
| [replacement](./replacement) | substitutos de usuário / delegação (requer admin) |
| [skill](./skill) | Skill para agentes de IA (Claude Code / Codex) |
| [upgrade](./upgrade) | atualização da própria CLI |
