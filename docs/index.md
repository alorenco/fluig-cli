---
layout: home

hero:
  name: fluigcli
  text: TOTVS Fluig direto do terminal
  tagline: CLI não oficial para importar, implantar e automatizar os artefatos da plataforma — feita para desenvolvedores, agentes de IA e CI/CD.
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
    details: Datasets, formulários, eventos globais, mecanismos de atribuição, scripts de processo e widgets — import (servidor → local) e export (local → servidor), com diff antes de publicar.
    link: /dataset
    linkText: dataset · form · workflow · widget
  - icon: ⚡
    title: Dev loop de verdade
    details: fluigcli dev é um proxy local autenticado do portal — o JS/CSS das widgets sai do disco, formulários têm preview com simulação de processo, e o navegador recarrega ao salvar. Scaffolds prontos com widget new (classic, Vue 3, React 19, Vuetify 3).
    link: /dev
    linkText: dev · watch · widget new
  - icon: 🗂️
    title: Operação no dia a dia
    details: Inicie e movimente solicitações, consulte a fila de tarefas, navegue no GED e administre usuários, grupos, papéis e substitutos — sem abrir o portal.
    link: /request
    linkText: request · task · document · user
  - icon: 🤖
    title: Feita para agentes e CI
    details: Modo não-interativo, envelope --json estável, exit codes 0–7 documentados e uma Skill embutida para Claude Code e Codex. Senha nunca em argumento — keyring, env var ou stdin.
    link: /skill
    linkText: skill · contrato de saída
---

> ⚠️ **Projeto não oficial, sem qualquer vínculo com a TOTVS.**
> "Fluig" e "TOTVS" são marcas de seus respectivos donos.

## Todos os grupos de comandos

Convenção que vale para tudo: **import** = servidor → local · **export** =
local → servidor. Comandos e flags em inglês; mensagens em pt-BR.

| Grupo | O que faz |
|---|---|
| [server](./server) | servidores, credenciais, sessão, saúde e `install-helper` |
| [dataset](./dataset) | datasets: consulta, CRUD, ativação, histórico e restauração |
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
