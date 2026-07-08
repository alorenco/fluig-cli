---
name: fluigcli
description: >-
  Use ao desenvolver ou implantar em servidores TOTVS Fluig com a CLI `fluigcli`:
  datasets, formulários, eventos globais, mecanismos de atribuição, scripts de
  eventos de processo e widgets. A CLI é não-interativa e feita para agentes
  (saída `--json` com envelope estável e exit codes 0–7). Ative quando o projeto
  tiver pasta `.fluigcli/`, `datasets/`, `forms/`, `events/`, `mechanisms/`,
  `workflow/` ou `wcm/`, ou quando o pedido envolver Fluig / fluigcli.
---

# fluigcli para agentes

`fluigcli` é uma CLI **não oficial** e de código aberto para desenvolvimento
TOTVS Fluig. Ela foi desenhada para uso não-interativo: você a dirige por flags,
lê um envelope JSON e decide pelo **exit code** — não pelo texto.

## Regras de ouro

1. **Sempre** passe `--json` e `--non-interactive`. Nunca dependa de prompt.
2. **Decida pelo exit code**, não pela mensagem (que é humana e em pt-BR):

   | código | significado | o que fazer |
   |---|---|---|
   | 0 | sucesso | seguir |
   | 2 | uso incorreto | corrigir o comando/flags |
   | 3 | autenticação/sessão | conferir credenciais (ver Autenticação) |
   | 4 | recurso não encontrado | conferir o id/nome |
   | 5 | erro do servidor Fluig | ler `error.message`; pode ser transitório |
   | 6 | falha parcial (lote) | inspecionar `data` item a item |
   | 7 | dependência ausente no servidor | rodar `fluigcli server install-helper` |

   Tabela completa e formato do envelope: [`reference/contract.md`](reference/contract.md).
3. **Nunca** ponha senha em argumento de linha de comando (vaza em `ps`/histórico).
   Use `FLUIGCLI_PASSWORD` ou `--password-stdin`. Ver Autenticação.
4. **Direção dos verbos** (importante, é o contrário de "git"):
   `import` = servidor → local · `export` = local → servidor.

## Autenticação (não-interativa)

A CLI cadastra servidores (metadados, **sem senha**) e resolve a senha nesta
ordem: `--password-stdin` → `FLUIGCLI_PASSWORD` → keyring do SO → prompt. Em
agente/CI use uma das duas primeiras. A **sessão é reaproveitada entre execuções**
(cache em disco), então normalmente a senha só é usada no primeiro comando.

```sh
# 1) cadastrar o servidor uma vez (metadados; senha vai para o keyring se houver)
echo "$SENHA" | fluigcli server add --name homolog \
  --host fluig.empresa.com.br --port 443 --ssl \
  --username deploy --company-id 1 --password-stdin --json

# 2) validar o acesso
echo "$SENHA" | fluigcli server test homolog --password-stdin --json

# 3) nos demais comandos, aponte o servidor e forneça a senha por env var
export FLUIGCLI_SERVER=homolog FLUIGCLI_PASSWORD="$SENHA"
fluigcli dataset list --json
```

## Descobrindo comandos

O mapa de comandos e receitas prontas (subir um dataset, exportar um script de
processo, baixar um formulário) está em [`reference/commands.md`](reference/commands.md).

Cada comando tem ajuda detalhada em pt-BR — **prefira consultá-la** a assumir
flags:

```sh
fluigcli --help
fluigcli dataset --help
fluigcli dataset export --help
```

## Fluxo típico de deploy

1. `fluigcli server test <name> --json` → confirmar acesso (exit 0).
2. Editar os artefatos nas pastas convencionais do projeto (`datasets/`, `forms/`…).
3. `fluigcli <recurso> export <arquivo|pasta> --json` → publicar (local → servidor).
4. Conferir `ok`/exit code; em lote, tratar exit 6 (parcial) olhando `data`.

## Limites

- Scripts de evento de processo (`workflow export`) e `widget list|import`
  exigem a widget auxiliar **fluiggersWidget** no servidor. Exit 7 → rode
  `fluigcli server install-helper <name>` (uma vez por servidor).
- HTTP (`--ssl=false`) trafega senha e cookies em texto claro; prefira HTTPS.
