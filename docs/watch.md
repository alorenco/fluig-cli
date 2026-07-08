# fluigcli watch — publicar ao salvar

Observa as pastas do projeto e publica o artefato no servidor a cada
salvamento. O ciclo *editar → exportar → testar* vira só *editar → testar*:

```
$ fluigcli watch
Observando datasets/, forms/, workflow/scripts/ em "homolog" — Ctrl+C para parar.
✓ 14:32:01  dataset "ds_clientes" publicado
· 14:33:10  dataset "ds_clientes" sem mudança — nada a publicar
✓ 14:35:47  form "Solicitação de Compras" publicado (versão mantida)
✓ 14:38:02  workflow "Compras.beforeTaskSave" publicado
```

## Cobertura

| pasta | unidade de publicação | versão no servidor |
|---|---|---|
| `datasets/`, `events/`, `mechanisms/` | o arquivo `.js` salvo | atualização in-place, sem versão |
| `forms/<pasta>/` | a **pasta inteira** do formulário (salvar vários arquivos em rajada = 1 publicação) | **sempre mantida** (`--version keep`; para versionar de propósito, use `form export --version new`) |
| `workflow/scripts/` | o script `<Processo>.<evento>.js` salvo | atualização cirúrgica via fluiggersWidget, sem bump |

## Regras de segurança (por design)

- **Só roda em servidor `dev` ou `hml`.** Produção é recusada sem exceção (nem
  `--yes`); servidor sem ambiente marcado também — marque com
  `fluigcli server update <name> --env hml`. Deploy contínuo é ferramenta de
  desenvolvimento, não de produção.
- **Nunca cria artefato nem versão**: só atualiza o que já existe no servidor
  (arquivo/pasta novos geram um aviso com o comando de criação), e nenhuma
  atualização gera versão nova — validado na homologação para datasets e
  formulários (versão idêntica antes e depois).
- **Salvamento sem mudança não publica nada** — para datasets/eventos/
  mecanismos, o conteúdo é comparado com o do servidor (ignorando CRLF/LF);
  para formulários e scripts de processo, um hash local do último publish
  evita repetições.
- Erro de publicação vira aviso e o watch continua vivo; encerre com Ctrl+C.

## Detalhes

- `--debounce <dur>` (padrão `500ms`): espera após o salvamento antes de
  publicar, agrupando as rajadas de eventos que editores geram ao salvar —
  inclusive vários arquivos da mesma pasta de formulário.
- Salvar um evento de formulário (`forms/x/events/y.js`) republica o
  formulário inteiro — é como a API do Fluig funciona.
- Scripts de processo exigem a fluiggersWidget instalada
  (`fluigcli server install-helper`) e o processo já criado no Fluig Studio.
- `--json` não é suportado: watch é um modo interativo de longa duração; em
  automação/CI, use os comandos `export`.
