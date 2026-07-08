# fluigcli watch — publicar ao salvar

Observa as pastas `datasets/`, `events/` e `mechanisms/` do projeto e publica o
artefato no servidor a cada salvamento. O ciclo *editar → exportar → testar*
vira só *editar → testar*:

```
$ fluigcli watch
Observando datasets/, events/ em "homolog" — Ctrl+C para parar.
✓ 14:32:01  dataset "ds_clientes" publicado
· 14:33:10  dataset "ds_clientes" sem mudança — nada a publicar
✓ 14:35:47  event "displayCentralTasks" publicado
```

## Regras de segurança (por design)

- **Só roda em servidor `dev` ou `hml`.** Produção é recusada sem exceção (nem
  `--yes`); servidor sem ambiente marcado também — marque com
  `fluigcli server update <name> --env hml`. Deploy contínuo é ferramenta de
  desenvolvimento, não de produção.
- **Nunca cria artefato**: só atualiza o que já existe no servidor. Arquivo
  novo gera um aviso com o comando de criação (`dataset export --new`, etc.).
  Atualizações não geram novas versões (validado na homologação: a versão do
  dataset permanece a mesma após o export).
- **Salvamento sem mudança não publica nada** — o conteúdo é comparado com o
  do servidor (ignorando CRLF/LF) antes de gravar.
- Erro de publicação vira aviso e o watch continua vivo; encerre com Ctrl+C.

## Detalhes

- `--debounce <dur>` (padrão `500ms`): espera após o salvamento antes de
  publicar, agrupando as rajadas de eventos que editores geram ao salvar.
- Cobertura: datasets, eventos globais e mecanismos — os mesmos artefatos de
  arquivo único do [diff](diff.md). Formulários ficam de fora de propósito
  (o export deles **cria nova versão** no servidor a cada publicação).
- `--json` não é suportado: watch é um modo interativo de longa duração; em
  automação/CI, use os comandos `export`.
