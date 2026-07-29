# fluigcli deploy — release por manifesto

O comando executa um **plano de release** descrito em um arquivo JSON. O plano
lista os passos na ordem em que eles devem rodar. O arquivo fica versionado no
repositório, junto com o código que ele publica.

Este comando substitui o roteiro de deploy escrito à mão. Um documento com "rode
este SQL, publique estes datasets, depois a widget" vira algo executável e
auditável.

```sh
fluigcli deploy --plan release.json --dry-run    # valida tudo, sem escrever
fluigcli deploy --plan release.json --server homolog
fluigcli deploy --plan release.json --from 3     # retoma do passo 3
```

## O plano

O formato é **JSON**. O projeto não usa YAML, e YAML exigiria uma dependência
nova.

```json
{
  "server": "producao",
  "steps": [
    {"name": "diagnóstico de permissões", "db": "sql/001_check.sql"},
    {"dataset": "datasets/ds_jud_agenda.js", "new": true},
    {"dataset": "datasets/ds_jud_processos.js"},
    {"event": "events/displayCustomThemes.js"},
    {"mechanism": "mechanisms/mec_gestor_area.js"},
    {"widget": "processos_judiciais", "build": true}
  ]
}
```

- `server` — o servidor alvo do plano. A opção `--server` da linha de comando
  vence este valor.
- `steps` — a lista ordenada. Cada passo tem **exatamente uma** chave de tipo.

| Chave de tipo | Alvo | Opções |
|---|---|---|
| `dataset` | arquivo `.js` | `new` (cria o dataset), `description` |
| `event` | arquivo `.js` | — |
| `mechanism` | arquivo `.js` | `description` |
| `widget` | código do widget | `build` (roda `npm run build`), `force` |
| `db` | arquivo `.sql` | — (só leitura; o servidor recusa escrita) |

A chave `name` é um rótulo livre. Ela aparece no relatório e ajuda a ler o plano.

Os caminhos são relativos à raiz do projeto. Uma chave escrita errado (por
exemplo `datasets` no lugar de `dataset`) é **erro**, não silêncio.

O plano **nunca** contém senha. A autenticação segue a
[precedência normal](server.md).

## O que acontece na execução

1. A CLI lê e valida o plano inteiro. Um passo sem tipo, com dois tipos ou com
   chave desconhecida reprova o plano antes de qualquer conexão.
2. A CLI audita **todos** os scripts do plano com as regras do
   [`audit`](audit.md). Um achado de nível ERRO em qualquer script aborta o
   release, e **nada é publicado**. Use `--no-audit` para pular.
3. A CLI autentica. Em servidor marcado como produção, vale a trava de
   confirmação — **uma vez** para o plano todo, não por passo.
4. Os passos rodam na ordem. A CLI **para no primeiro erro**.

Os passos não tentados saem como `skipped` no relatório. Assim você vê onde o
release parou, e retoma com `--from N` depois de corrigir.

```
── [1] db sql/001_check.sql: 2 instrução(ões) de leitura executada(s)
── [2] dataset datasets/ds_jud_agenda.js: dataset ds_jud_agenda created
aviso: passo 3 (widget processos_judiciais): o código "processos_judiciais" já
existe no servidor como LAYOUT …
Plano interrompido no passo 3: 2 executado(s), 1 não tentado(s). Retome com --from 3.
```

## `--dry-run`

O `--dry-run` valida o plano **sem escrever nada**:

- todos os arquivos e pastas existem;
- a auditoria dos scripts passa;
- cada dataset é classificado como criação ou atualização (a CLI consulta o
  servidor);
- o código do widget não colide com um layout (a guarda de
  [`widget export`](widget.md#guarda-de-colisão-com-layout));
- cada script `.sql` tem instruções reconhecíveis, com a contagem.

Rode o `--dry-run` antes de qualquer release em produção. Ele responde "este
plano vai funcionar?" sem correr o risco.

## Saída `--json`

```json
{
  "ok": true,
  "command": "deploy",
  "server": "producao",
  "data": {
    "plan": "release.json",
    "dryRun": false,
    "steps": [
      {"index": 1, "name": "diagnóstico", "kind": "db", "target": "sql/001_check.sql",
       "status": "ok", "action": "2 instrução(ões) de leitura executada(s)"},
      {"index": 2, "kind": "dataset", "target": "datasets/ds_jud_agenda.js",
       "status": "failed", "error": "..."},
      {"index": 3, "kind": "widget", "target": "processos_judiciais", "status": "skipped"}
    ],
    "counts": {"ok": 1, "failed": 1, "skipped": 1}
  },
  "error": null
}
```

O `status` de cada passo é `ok`, `failed`, `skipped` (não tentado) ou
`validated` (no `--dry-run`).

## Exit codes

| código | quando |
|---|---|
| `0` | todos os passos concluíram |
| `1` | a auditoria reprovou um script do plano (`AUDIT_FAILED`) — nada rodou |
| `2` | plano inválido, `--from` fora do intervalo, ou `--dry-run` com problema |
| `4` | o arquivo do plano não existe |
| `6` | o plano rodou em parte e parou (veja `data.steps`) |

Quando o **primeiro** passo falha, o comando devolve o erro daquele passo, com o
código dele. Não existe release parcial quando nada foi publicado.

## Limitações

- Passos de **formulário** e de **processo** ainda não existem. Publique esses
  artefatos com [`form export`](form.md) e
  [`workflow publish`](workflow.md) enquanto isso.
- Não há rollback. O Fluig não tem transação entre artefatos. O plano para no
  erro e diz onde parou — a volta é sua, com o Git e o
  [`dataset history`](dataset.md).
- Os passos `db` são de **leitura**. Eles servem para diagnóstico e verificação
  dentro do release. A escrita continua recusada no servidor.
