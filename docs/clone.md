# fluigcli clone — onboarding de instância existente

Clona os artefatos de um servidor Fluig **já em uso** para o projeto local — o
cenário clássico do consultor que chega num cliente com a instância rodando e
uma pasta vazia na máquina. O `clone` consulta o servidor, mostra o inventário
de cada tipo e importa os selecionados, com a mesma semântica do
`<grupo> import --all` de cada comando.

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
| `forms` | `forms/<Nome>/` | anexos + `events/*.js`; o vínculo pasta↔servidor fica em `.fluigcli/forms.json` |
| `datasets` | `datasets/<id>.js` | **só os customizados** (os nativos não têm código a versionar) |
| `workflows` | `workflow/scripts/<Processo>.<evento>.js` | **só os scripts de eventos** — o diagrama do processo fica no servidor |
| `events` | `events/<id>.js` | eventos globais |
| `mechanisms` | `mechanisms/<id>.js` | mecanismos de atribuição customizados |
| `widgets` | `wcm/widget/<code>/` | requer o [fluigcliHelper](./server#fluigcli-server-install-helper-name); widget SPA vem como o **bundle publicado** (sem o fonte TS/Vue) |

Fora do escopo (a CLI não gerencia): páginas e layouts, comunidades,
parâmetros da plataforma e documentos do GED (para documentos avulsos, use
[`document`](./document)).

## Seleção

- **Interativo** (terminal, sem flags): a tabela de inventário mostra cada
  tipo com a contagem; responda com números ou nomes (`1,3` ou
  `forms,widgets`) — Enter clona todos os disponíveis.
- **Não-interativo / CI / `--json`**: `--all` ou `--only <tipos>` é
  obrigatório (sem eles: exit 2). `--only` aceita os nomes no plural ou
  singular.

## Widgets e o fluigcliHelper

O download de widgets depende do componente auxiliar
[fluigcliHelper](./server#fluigcli-server-install-helper-name) instalado no servidor:

- Com `--all` e o helper ausente, as widgets são **puladas com aviso** e o
  resto segue normalmente.
- Com `--only widgets` e o helper ausente, o comando falha com exit 7 e a
  orientação do `server install-helper`.

## Re-execução e segurança

- Rodar de novo **sobrescreve os arquivos locais** com o estado do servidor
  (a mesma regra dos `import`) — commite antes de re-executar; o
  [`diff`](./diff) mostra o que difere sem tocar em nada.
- O clone **só lê** o servidor; nada é publicado (por isso funciona sem a
  trava de produção).
- Falha em itens individuais não interrompe o restante: o comando termina com
  exit 6 (sucesso parcial) e cada falha aparece em `data.results`.

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

Sem o fluigcliHelper, `data.unavailable.widgets` explica a ausência e
`available` não conta widgets.
