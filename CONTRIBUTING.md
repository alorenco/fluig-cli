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
6. **Listagens em tabela:** todo comando que lista itens imprime, no modo
   humano, uma tabela com bordas (`output.Table`). O cabeçalho usa
   `output.BoldHeaderStyle`. As cores saem só com `output.ColorEnabled()`. Uma
   lista vazia imprime uma mensagem `Infof` com o próximo passo. A tabela não
   muda o contrato `--json`. Cada `list` tem um teste do modo humano
   (`Test*ListTabela`).
7. **Sem dados reais de pessoas:** a documentação, os exemplos, os comentários,
   as fixtures e os testes usam só valores fictícios. Por exemplo: login
   `jsilva`, nome "João Silva" e e-mail `joao@empresa.com`. Ao sanitizar uma
   fixture real, troque também os dados pessoais.

## Estilo da documentação

A documentação segue o **ASD-STE100 (Simplified Technical English, Issue 9)**,
adaptado ao pt-BR. A regra vale para `docs/`, README, `CONTRIBUTING`, help de
comando, textos das telas do `dev` e a Skill.

- Escreva frases curtas. Use uma ideia por frase.
- Use voz ativa e sujeito explícito.
- Escreva instruções no imperativo direto.
- Use uma palavra por conceito. Por exemplo, não alterne "default" e "padrão".
- Não use metáforas nem períodos longos com travessão ou parênteses. Use ponto
  final e um conector simples, como "Por isso", "Neste caso" ou "Por exemplo".
- Não exponha nomes de funções internas no texto para o usuário.

O estilo não muda o contrato. Nomes de comando e de flag seguem em inglês.
Blocos de código, links e tabelas de exit code ficam intactos. A página
[docs/log.md](docs/log.md) é uma referência do estilo aplicado.

## Documentação de comandos

Um comando, subcomando ou flag novo (ou renomeado) só está pronto quando
aparece em todas estas superfícies. Atualize todas no mesmo PR.

| Superfície | O que atualizar |
|---|---|
| `README.md` | A tabela `## Comandos`, com o grupo e os subcomandos |
| `docs/<grupo>.md` | A página do grupo, com a sintaxe, os exemplos e a tabela de flags |
| `docs/README.md` e `docs/index.md` | A descrição do grupo, quando o recurso novo muda o resumo das capacidades |
| `docs/.vitepress/config.mts` | O sidebar, quando o grupo é novo |
| `skills/fluigcli/reference/commands.md` | A tabela do grupo e o índice "Por objetivo" |
| `skills/fluigcli/codex/AGENTS.md` | A listagem compacta do grupo |
| Help do comando (`Short`, `Long` e `Example`) | O texto em pt-BR, coerente com a página do grupo |

Os testes `TestSkillCommandsDocCoversEveryGroup` e
`TestSkillCommandsDocCoversEverySubcommand` cobram o `commands.md`. As outras
superfícies não têm teste. Por isso, confira cada linha da tabela antes de
abrir o PR.

## Validação no servidor

Um comando que escreve no servidor Fluig precisa de validação ao vivo antes do
merge. Valide numa homologação, nunca em produção. Use sempre artefatos com o
prefixo `zz_fluigcli_test_` e apague todos no fim. Descreva no PR o que você
validou e o resultado.

Se você não tem acesso a um servidor Fluig, diga isso no PR. Neste caso, o
mantenedor faz a validação e informa o resultado na revisão.

## Fluxo de PR

- Um PR por mudança coesa; mensagens de commit no formato convencional
  (`feat:`, `fix:`, `docs:`…).
- Rode `go test ./...` e `golangci-lint run ./...` antes de abrir o PR.
- Atualize todas as superfícies de documentação (ver
  [Documentação de comandos](#documentação-de-comandos)).
- Para ajustes pedidos na revisão, faça commits novos no mesmo branch. Não abra
  um PR novo.

## Reportando problemas

Abra uma issue com o máximo de contexto: versão do fluigcli (`fluigcli version`),
versão do Fluig, comando executado (com `--verbose`, que mascara a senha) e a
saída/erro. **Nunca** cole senhas.
