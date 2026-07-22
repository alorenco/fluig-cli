# Instalação e quickstart

> ⚠️ Projeto **não oficial**, sem qualquer vínculo com a TOTVS.

## Instalação

**Linux e macOS:**

```sh
curl -fsSL https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/alorenco/fluig-cli/main/install.ps1 | iex
```

O script detecta o sistema. Ele baixa a última versão de
[Releases](https://github.com/alorenco/fluig-cli/releases). Ele confere o
checksum. Depois ele instala. Você também instala de forma manual. Baixe o
binário da sua plataforma em Releases. Coloque o binário no `PATH`. Ou compile
do código-fonte. Você precisa do Go 1.26 ou superior.

```sh
go install github.com/alorenco/fluig-cli/cmd/fluigcli@latest
```

A própria CLI faz a atualização depois. Veja [upgrade](./upgrade).

## Quickstart

```sh
# 1. Cadastre os servidores (a senha vai para o keyring do SO — nunca para arquivo).
#    O primeiro cadastrado vira o padrão; servidores "prod" ganham trava de escrita.
fluigcli server add --name homolog --host fluig-hml.empresa.com.br --username admin.deploy --env hml
fluigcli server add --name producao --host fluig.empresa.com.br --username admin.deploy --env prod

# 2. Teste o acesso (login + ping + dados do usuário + status da widget auxiliar)
fluigcli server test homolog

# 3. Traga os artefatos do servidor para o projeto (import = servidor → local).
#    Chegando num servidor já em uso com a pasta vazia? Clone tudo de uma vez:
fluigcli clone                # mostra o inventário e pergunta o que clonar (--all pula a pergunta)

#    ...ou pontualmente, por tipo:
fluigcli dataset import --all
fluigcli form import "Solicitação de Compras"
fluigcli event import --all
fluigcli workflow import Compras

# 4. Desenvolva com deploy automático: cada salvamento publica na homologação
#    (só roda em dev/hml; nunca cria artefato nem versão nova)
fluigcli watch

# 4b. Ou com feedback instantâneo, sem publicar nada: proxy local do portal
#     que serve o JS/CSS das widgets do disco e recarrega o navegador ao salvar
fluigcli dev

# 5. Ou no ritmo manual: confira o que mudaria e publique (export = local → servidor)
fluigcli diff
fluigcli dataset export datasets/ds_clientes.js
fluigcli workflow export workflow/scripts/Compras.beforeTaskSave.js

# 6. Na hora de ir para produção, a trava pede confirmação antes de escrever
fluigcli dataset export datasets/ds_clientes.js --server producao

# 7. Em scripts, CI e agentes de IA: --json + exit codes estáveis
fluigcli diff --json | jq '.data.counts'
```

## Estrutura de projeto Fluig

Os comandos trabalham sobre pastas convencionais na raiz do projeto. Os
scaffolds `new` e os comandos `import` criam essas pastas.

```
seu-projeto/
├── .fluigcli/            # servers.json (conexão, versionável) + overlays locais
├── datasets/<id>.js
├── events/<id>.js
├── mechanisms/<id>.js
├── forms/<Nome>/         # HTML principal + anexos + events/*.js
├── workflow/scripts/<Processo>.<evento>.js
└── wcm/widget/<NomeWidget>/src/main/...
```

## Uso por agentes de IA e CI/CD

A CLI é feita para agentes. Use sempre `--json` e `--non-interactive`. Decida
pelo exit code. Os exit codes vão de 0 a 7 e são estáveis. Passe a senha por
`FLUIGCLI_PASSWORD` ou `--password-stdin`. Nunca passe a senha em argumento.
Instale a [Skill](./skill) no projeto. Assim o Claude Code e o Codex descobrem
os comandos sozinhos.
