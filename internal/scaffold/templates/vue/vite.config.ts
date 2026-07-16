import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Código da widget: define o nome dos arquivos do bundle, que o
// application.info declara (application.resource.js/css).
const widgetCode = '[[.Code]]'

// Proxy do modo dev (npm run dev): aponta para o `fluigcli dev`, que injeta a
// sessão autenticada do Fluig — nenhuma credencial em .env. Suba-o antes, na
// raiz do projeto: `fluigcli dev` (porta padrão 8787).
const fluigcliDev = 'http://127.0.0.1:8787'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: '127.0.0.1',
    proxy: {
      '/api': fluigcliDev,
      '/style-guide': fluigcliDev,
      '/portal': fluigcliDev,
      '/webdesk': fluigcliDev,
      '/ecm': fluigcliDev,
    },
  },
  build: {
    // Emite direto na árvore que vai para o WAR (fluigcli widget export).
    outDir: 'src/main/webapp/resources',
    emptyOutDir: false, // a pasta também guarda images/ etc.
    copyPublicDir: false,
    // Um único arquivo CSS (o application.info declara css/<code>.css);
    // sem isto o Vite inlina o CSS no JS e o portal pediria um 404.
    cssCodeSplit: false,
    rollupOptions: {
      input: 'src/vue/main.ts',
      output: {
        // Um único JS (IIFE) + um único CSS, com o nome da widget — o global
        // real é definido no main.ts; imports dinâmicos são inlinados para
        // não gerar chunks que o application.info não conhece.
        format: 'iife',
        name: '[[.CamelCode]]Bundle',
        inlineDynamicImports: true,
        entryFileNames: `js/${widgetCode}.js`,
        assetFileNames: (asset) =>
          asset.name?.endsWith('.css') ? `css/${widgetCode}.css` : 'assets/[name][extname]',
      },
    },
  },
})
