# fluigcli clone — onboarding de instância existente

O comando `clone` copia os artefatos de um servidor Fluig **já em uso** para o
projeto local. O `clone` consulta o servidor. O comando mostra o inventário de 
cada tipo. Em seguida, o comando importa os tipos que você seleciona. 
A semântica é a mesma do `<grupo> import --all` de cada comando.

```sh
mkdir cliente && cd cliente
fluigcli server add --name homolog --host fluig-hml.cliente.com.br --username consultor --env hml

fluigcli clone                       # interativo: inventário + escolha do que clonar
fluigcli clone --all                 # tudo o que estiver disponível, sem perguntar
fluigcli clone --only forms,datasets # só os tipos citados
git init && git add -A && git commit -m "estado inicial do servidor"
```

## O que é clonado

| tipo | vai para | observação |
|---|---|---|
| `forms` | `forms/<Nome>/` | anexos e `events/*.js`. O vínculo pasta↔servidor fica em `.fluigcli/forms.json` |
| `datasets` | `datasets/<id>.js` | **só os customizados**. Os nativos não têm código a versionar |
| `workflows` | `workflow/scripts/<Processo>.<evento>.js` | **só os scripts de eventos**. O diagrama do processo fica no servidor |
| `events` | `events/<id>.js` | eventos globais |
| `mechanisms` | `mechanisms/<id>.js` | mecanismos de atribuição customizados |
| `widgets` | `wcm/widget/<code>/` | requer o [fluigcliHelper](./server#fluigcli-server-install-helper-name). O widget SPA vem como o **bundle publicado**, sem o fonte TS/Vue |

A CLI não gerencia estes itens: páginas e layouts, comunidades, parâmetros da
plataforma e documentos do GED. Para documentos avulsos, use
[`document`](./document).

## Seleção

- **Interativo** (terminal, sem flags): a tabela de inventário mostra cada
  tipo com a contagem. Responda com números ou nomes (`1,3` ou
  `forms,widgets`). Pressione Enter para clonar todos os tipos disponíveis.
- **Não-interativo / CI / `--json`**: informe `--all` ou `--only <tipos>`. Sem
  uma destas flags, o comando termina com exit 2. O `--only` aceita os nomes no
  plural ou no singular.

## Widgets e o fluigcliHelper

O download de widgets precisa do componente auxiliar
[fluigcliHelper](./server#fluigcli-server-install-helper-name) instalado no servidor:

- Com `--all` e sem o helper, o comando **pula as widgets com aviso**. Os demais
  tipos seguem normalmente.
- Com `--only widgets` e sem o helper, o comando termina com exit 7. O comando
  mostra a orientação do `server install-helper`.

## Re-execução e segurança

- Rodar de novo **sobrescreve os arquivos locais** com o estado do servidor.
  Esta é a mesma regra dos `import`. Faça o commit antes de re-executar. O
  [`diff`](./diff) mostra o que difere sem tocar em nada.
- O clone **só lê** o servidor. O comando não publica nada. Por isso, ele
  funciona sem a trava de produção.
- A falha em itens individuais não interrompe o restante. O comando termina com
  exit 6 (sucesso parcial). Cada falha aparece em `data.results`.

## Saída `--json`

```json
{
  "ok": true,
  "command": "clone",
  "server": "homolog",
  "data": {
    "root": "/home/consultor/cliente",
    "available": {"forms": 40, "datasets": 12, "workflows": 31, "events": 5, "mechanisms": 8, "widgets": 34},
    "selected": ["forms", "datasets"],
    "results": {
      "forms": [{"id": "Solicitação de Compras", "action": "imported", "success": true}],
      "datasets": [{"id": "ds_clientes", "action": "created", "success": true}]
    }
  },
  "error": null
}
```

Sem o fluigcliHelper, o campo `data.unavailable.widgets` explica a ausência. O
campo `available` não conta widgets.
