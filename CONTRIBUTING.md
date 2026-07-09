# Contribuindo com o fluigcli

Obrigado pelo interesse! Este é um projeto **não oficial** e de código aberto,
sem vínculo com a TOTVS, criado e mantido por
[Alessandro Lorençone](https://github.com/alorenco) (@alorenco).
Contribuições são bem-vindas.

## Antes de começar

A documentação de cada grupo de comandos está em [docs/](docs/). O contrato de
interface (comandos, flags, envelope `--json` e exit codes) está descrito no
[README](README.md).

## Ambiente

- Go ≥ 1.26.
- Dependências mínimas: cada nova dependência precisa de justificativa no PR.

```sh
go build ./...
go test ./...
golangci-lint run ./...
go test -tags=integration ./internal/fluig/   # opcional; requer FLUIGCLI_TEST_*
```

## Convenções (inegociáveis)

1. **Idioma:** nomes de comandos e flags em **inglês**; toda mensagem para
   humanos, ajuda, log e comentário em **pt-BR**. Isso inclui o texto gerado pelo
   cobra (ver `internal/cli/help.go`).
2. **Fronteira de arquitetura:** `internal/fluig` **não importa cobra nem faz I/O
   de terminal** — é candidato a virar biblioteca pública. A CLI (`internal/cli`)
   orquestra; a tradução de erros para exit codes fica nela.
3. **Contrato de saída:** com `--json`, stdout recebe **exatamente um** envelope
   JSON. Exit codes (0–7) são estáveis e cobertos por teste — mudá-los é breaking
   change (só em major).
4. **Segurança:** senha nunca em arquivo nem em argumento de linha de comando; só
   keyring, env var ou stdin. `--verbose` mascara senha e cookies.
5. **Testes:** todo comportamento novo precisa de teste unitário (sem rede, com
   fixtures em `testdata/`). Integração contra a homologação valida cada fase.

## Fluxo de PR

- Um PR por mudança coesa; mensagens de commit no formato convencional
  (`feat:`, `fix:`, `docs:`…).
- Rode `go test ./...` e `golangci-lint run ./...` antes de abrir o PR.
- Atualize a doc do comando (`docs/`).

## Reportando problemas

Abra uma issue com o máximo de contexto: versão do fluigcli (`fluigcli version`),
versão do Fluig, comando executado (com `--verbose`, que mascara a senha) e a
saída/erro. **Nunca** cole senhas.
