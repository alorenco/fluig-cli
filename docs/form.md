# fluigcli form — formulários

Importa e exporta formulários (definição de card do Fluig). A estrutura local é
uma **pasta por formulário**:

```
forms/<NomeDoFormulario>/
├── <NomeDoFormulario>.html   # arquivo principal (principal=true no upload)
├── *.js, *.css, ...          # demais anexos
└── events/<evento>.js        # eventos do formulário
```

O **arquivo principal** (a página do form) é detectado assim: se há um único
`.html/.htm` na pasta, é ele; com vários, o que casar com o nome da pasta ou do
formulário. Os `.js` sob `events/` são os eventos.

- **import** = servidor → projeto local
- **export** = projeto local → servidor

### Nome da pasta ≠ nome no servidor

A pasta local pode ter um nome técnico (ex.: `frm_fin_pagamentos_diversos`)
diferente do nome do formulário no servidor (`Formulário de Pagamentos Diversos`).
A CLI grava esse vínculo em **`.fluigcli/forms.json`** (versionável no Git) no
import e no export, então depois do primeiro vínculo o `form export <pasta>`
reencontra o formulário sozinho.

Os vínculos são **separados por servidor** (chave `host:porta/companyId`):
o mesmo formulário tem `documentId` — e às vezes nome — diferente em cada
ambiente, então o vínculo criado na homologação nunca é usado num export para
a produção. O primeiro export para um servidor novo resolve pelo nome da
pasta (ou `--name`/`--document-id`) e grava o vínculo daquele servidor.

Para criar/inicializar o vínculo:

- **`form link`** (recomendado ao configurar um servidor): percorre as pastas
  de `forms/` sem vínculo e sugere o formulário correspondente — pelo nome já
  vinculado à pasta em outro servidor (o caso "acabei de cadastrar a
  produção"), pelo nome exato da pasta ou ignorando caixa. No modo
  interativo, Enter aceita a sugestão, um termo busca na lista do servidor,
  o número escolhe e `s` pula. `form link --auto` grava só as sugestões
  inequívocas, sem prompt (com `--json`, para scripts e agentes). O
  `server add`/`server test` lembram do comando quando o projeto tem
  formulários sem vínculo no servidor;
- no import: `--folder <pasta>` grava o formulário na pasta indicada;
- no export: `--name "<nome no servidor>"` ou `--document-id <id>` apontam o alvo
  (e o vínculo fica salvo para as próximas vezes).

## `fluigcli form list`

Lista os formulários do servidor (documentId, nome, dataset, versão).

## `fluigcli form import <documentId|nome>... | --all`

Baixa os anexos e eventos de cada formulário para `forms/<nome>/`. O alvo pode
ser o `documentId` (número) ou o nome exato do formulário.

```sh
fluigcli form import 42
fluigcli form import "Formulário de Contato"
fluigcli form import --all
```

## `fluigcli form export <pasta> [flags]`

Envia uma pasta de formulário. Se o formulário já existe (nome = nome da pasta),
atualiza; senão, cria (exige `--new`).

| Flag | Uso |
|---|---|
| `--name "..."` | nome do formulário no servidor (aponta o alvo / define o nome na criação) |
| `--document-id N` | documentId do formulário-alvo |
| `--new` | cria o formulário se ainda não existe |
| `--parent-id N` | id da pasta do GED onde criar (obrigatório na criação) |
| `--dataset-name X` | dataset do formulário (obrigatório na criação) |
| `--card-description` | campo descritor do card (default: o nome do formulário) |
| `--persistence-type db\|single` | `db` = tabelas por form (padrão); `single` = tabela única |
| `--version keep\|new` | no update: `keep` mantém a versão, `new` cria nova (padrão) |

```sh
# atualizar um formulário existente criando nova versão
fluigcli form export "forms/Formulário de Contato" --version new

# criar um formulário novo
fluigcli form export forms/NovoForm --new --parent-id 15 --dataset-name ds_novoform
```

## `fluigcli form records ...` — registros (dados) do formulário

CRUD dos **registros** (cards) de um formulário — os dados, não o layout.
Útil para consultar/testar formulários e datasets com dados reais, direto do
terminal ou por agentes de IA. O formulário pode ser indicado pelo
`documentId` ou pelo nome.

```sh
# listar (escolha as colunas; --json traz todos os campos)
fluigcli form records list "Cadastro de Clientes" --fields nome,email --limit 20

# filtrar (sintaxe $filter da API, estilo OData)
fluigcli form records list 12345 --filter "quantidade eq '99'"

# registro completo (com linhas filhas de tabela pai×filho)
fluigcli form records show 12345 67890

# criar e atualizar (mesmos --field/--fields-file do request start)
fluigcli form records create 12345 --field nome="Maria" --field quantidade=10
echo '{"nome":"Maria","quantidade":"10"}' | fluigcli form records create 12345 --fields-file -
fluigcli form records update 12345 67890 --field quantidade=99

# excluir (pede confirmação; --yes pula)
fluigcli form records delete 12345 67890 --yes
```

Semântica (validada na homologação):

- O **update mescla**: campos não enviados sobrevivem — e cada update cria
  uma **versão nova do registro** (1000 → 2000...).
- O servidor acrescenta campos de controle (`anonymization_date`,
  `anonymization_user_id`) automaticamente.
- Os **eventos do formulário não rodam** neste caminho (os dados entram como
  enviados — sem validateForm). Para testar as validações, use o processo
  (`request start`) ou o preview do `fluigcli dev`.

## Observações

- Nomes de pasta com acento e espaço são suportados (ex.: `Formulário de Troca`).
- Apenas arquivos no topo da pasta viram anexos (nomes planos); subpastas além
  de `events/` são ignoradas.
- O HTML principal precisa ter uma tag `<form>` — sem ela o servidor rejeita
  a criação ("Formulário não possui tag form").
