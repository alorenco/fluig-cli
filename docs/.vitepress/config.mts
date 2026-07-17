import { defineConfig } from 'vitepress'

// Site de documentação do fluigcli — fonte única: os .md desta pasta (docs/).
// Publicado no GitHub Pages pelo workflow .github/workflows/docs.yml.
export default defineConfig({
  lang: 'pt-BR',
  title: 'fluigcli',
  description: 'CLI não oficial para desenvolvimento TOTVS Fluig — datasets, formulários, workflow e widgets direto do terminal',
  // O site vive em https://alorenco.github.io/fluig-cli/
  base: '/fluig-cli/',
  // O README.md é o índice para quem navega no GitHub; no site, a home é o index.md.
  srcExclude: ['README.md'],
  cleanUrls: true,
  lastUpdated: true,

  themeConfig: {
    nav: [
      { text: 'Início', link: '/' },
      { text: 'Instalação', link: '/instalacao' },
      { text: 'Comandos', link: '/server' },
    ],

    sidebar: [
      {
        text: 'Introdução',
        items: [
          { text: 'Instalação e quickstart', link: '/instalacao' },
        ],
      },
      {
        text: 'Desenvolvimento',
        items: [
          { text: 'server — servidores e sessão', link: '/server' },
          { text: 'dataset — datasets', link: '/dataset' },
          { text: 'event — eventos globais', link: '/event' },
          { text: 'mechanism — mecanismos', link: '/mechanism' },
          { text: 'form — formulários', link: '/form' },
          { text: 'workflow — scripts de processo', link: '/workflow' },
          { text: 'widget — widgets', link: '/widget' },
          { text: 'diff — conferir antes de publicar', link: '/diff' },
          { text: 'audit — conformidade com o style guide', link: '/audit' },
          { text: 'watch — publicar ao salvar', link: '/watch' },
          { text: 'dev — dev server com live reload', link: '/dev' },
        ],
      },
      {
        text: 'Operação',
        items: [
          { text: 'request — solicitações', link: '/request' },
          { text: 'task — fila de tarefas', link: '/task' },
          { text: 'document — GED', link: '/document' },
        ],
      },
      {
        text: 'Administração',
        items: [
          { text: 'user — usuários', link: '/user' },
          { text: 'group — grupos', link: '/group' },
          { text: 'role — papéis', link: '/role' },
          { text: 'replacement — substitutos', link: '/replacement' },
        ],
      },
      {
        text: 'CLI',
        items: [
          { text: 'skill — Skill para agentes de IA', link: '/skill' },
          { text: 'upgrade — atualização da CLI', link: '/upgrade' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/alorenco/fluig-cli' },
    ],

    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: 'Buscar', buttonAriaLabel: 'Buscar na documentação' },
          modal: {
            displayDetails: 'Mostrar detalhes',
            resetButtonTitle: 'Limpar busca',
            backButtonTitle: 'Voltar',
            noResultsText: 'Nenhum resultado para',
            footer: {
              selectText: 'selecionar',
              navigateText: 'navegar',
              closeText: 'fechar',
            },
          },
        },
      },
    },

    outline: { label: 'Nesta página' },
    docFooter: { prev: 'Anterior', next: 'Próxima' },
    lastUpdatedText: 'Atualizado em',
    darkModeSwitchLabel: 'Tema',
    sidebarMenuLabel: 'Menu',
    returnToTopLabel: 'Voltar ao topo',
    editLink: {
      pattern: 'https://github.com/alorenco/fluig-cli/edit/main/docs/:path',
      text: 'Sugerir mudança nesta página',
    },
    footer: {
      message: 'Projeto não oficial, sem qualquer vínculo com a TOTVS. "Fluig" e "TOTVS" são marcas de seus respectivos donos.',
      copyright: 'MIT © Alessandro Lorençone',
    },
  },
})
