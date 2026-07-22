# fluigcli watch — publicar ao salvar

Este comando observa as pastas do projeto. Ele publica o artefato no servidor a
cada salvamento. O ciclo *editar → exportar → testar* vira só
*editar → testar*.

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
| `workflow/scripts/` | o script `<Processo>.<evento>.js` salvo | atualização cirúrgica via componente auxiliar, sem bump |

## Regras de segurança (por design)

- **O comando só roda em servidor `dev` ou `hml`.** O comando recusa produção
  sem exceção. Nem `--yes` libera. O comando também recusa servidor sem
  ambiente marcado. Marque o ambiente com
  `fluigcli server update <name> --env hml`. O deploy contínuo é ferramenta de
  desenvolvimento. Ele não é ferramenta de produção.
- **O comando nunca cria artefato nem versão.** O comando só atualiza o que já
  existe no servidor. Arquivo ou pasta novos geram um aviso com o comando de
  criação. Nenhuma atualização gera versão nova. A homologação validou esta
  regra para datasets e formulários. A versão fica idêntica antes e depois.
- **O salvamento sem mudança não publica nada.** Para datasets, eventos e
  mecanismos, o comando compara o conteúdo com o do servidor. A comparação
  ignora CRLF e LF. Para formulários e scripts de processo, um hash local do
  último publish evita repetições.
- O erro de publicação vira aviso. O watch continua vivo. Encerre com Ctrl+C.

## Detalhes

- `--debounce <dur>` (padrão `500ms`): o comando espera este tempo após o
  salvamento antes de publicar. Assim, o comando agrupa as rajadas de eventos
  que editores geram ao salvar. Isso inclui vários arquivos da mesma pasta de
  formulário.
- Salvar um evento de formulário (`forms/x/events/y.js`) republica o
  formulário inteiro. A API do Fluig funciona assim.
- Os scripts de processo exigem o componente auxiliar instalado
  (`fluigcli server install-helper`). Eles também exigem o processo já criado
  no Fluig Studio.
- O comando não aceita `--json`. O watch é um modo interativo de longa
  duração. Em automação ou CI, use os comandos `export`.
