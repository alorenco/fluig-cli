# fluigcli page — páginas do portal

O grupo `page` cria páginas do portal, põe widgets nelas, define quem pode ver
e publica. Use este grupo depois do `widget export`, para a widget aparecer
para os usuários sem abrir o editor de páginas.

## Rascunho e versão publicada

Uma página tem uma **versão publicada** e, durante a edição, um **rascunho**.
Os usuários veem só a versão publicada.

- A página nova nasce em rascunho.
- O `add-widget`, o `permission` e o `update` gravam no rascunho. O
  `add-widget` abre um rascunho quando a página não tem um.
- O `publish` torna o rascunho a nova versão publicada.
- O `discard` descarta o rascunho e mantém a versão publicada.
- Cada versão tem os próprios `instanceId`. O `page show` mostra as duas
  versões. O `move-widget` e o `remove-widget` agem na versão do id
  informado. Com rascunho aberto, use o id do rascunho. Sem rascunho, o id
  publicado age direto na versão publicada, sem `publish`.
- O `set-preferences` age direto na versão publicada. Por isso, ele exige que
  a página não tenha rascunho aberto.

### Rascunho de outro usuário

O rascunho pertence a quem o abriu. O `page show` mostra o dono do rascunho na
linha "Rascunho aberto por". Quando o rascunho é de outro usuário, o
`add-widget`, o `permission`, o `publish` e o `discard` param com exit **5** e
mostram o nome do dono. Para assumir o rascunho, repita o comando com
`--take-over`. As mudanças do outro usuário continuam no rascunho. A tela do
Fluig faz o mesmo quando você confirma a edição.

A publicação troca o `instanceId` de todas as widgets da página. As
preferências das widgets continuam iguais. Depois de publicar, consulte os ids
novos com `page show`.

## Fluxo típico

```sh
fluigcli layout list --all
fluigcli page create painel_vendas --title "Painel de Vendas" --layout layoutdefault \
  --view-group vendas,gestores
fluigcli page add-widget painel_vendas painel_vendas_widget --slot SlotA
fluigcli page publish painel_vendas
fluigcli page show painel_vendas
```

## `fluigcli page list [flags]`

Este comando lista as páginas customizadas numa tabela: código, título,
layout, status e autorização. O status `DRAFT` aparece em amarelo. Ele indica
um rascunho que falta publicar.

| Flag | Uso |
|---|---|
| `--all` | inclui as páginas nativas do Fluig |
| `--filter <trecho>` | filtra pelo trecho do código ou do título |

O Fluig lista só as páginas que o usuário pode ver.

## `fluigcli page show <página>`

Este comando mostra a página: layout, status, versão, autorização, URL
amigável e página-pai. Ele também mostra as widgets da versão publicada, com
o `instanceId`, o slot, a ordem e as preferências, e as permissões.

Com rascunho aberto, o comando mostra também as widgets do rascunho e o dono
do rascunho. O Fluig só deixa ler o rascunho slot a slot. Por isso, a CLI lê
os slots do layout e os slots já usados na versão publicada. No `--json`, o
rascunho vem em `draftWidgets`.

## `fluigcli page create <código> --title <título> --layout <layout> [flags]`

Este comando cria uma página em rascunho.

| Flag | Uso |
|---|---|
| `--title <texto>` | título da página (**obrigatório**) |
| `--layout <código>` | código do layout (**obrigatório**). Veja os códigos com `fluigcli layout list --all`. |
| `--description <texto>` | descrição da página (default: o título) |
| `--view-group <grupo>` | grupo que pode ver a página. Repita a flag ou separe por vírgula. |
| `--view-role <papel>` | papel que pode ver a página |
| `--view-user <login>` | usuário que pode ver a página |
| `--edit-group <grupo>` | grupo que pode editar e remover a página. Também existem `--edit-role` e `--edit-user`. |
| `--parent <página>` | cria a página como subpágina da página informada |
| `--public` | página pública, sem login. O default é só para usuários autenticados. |

Sem `--view-*`, a página segue o padrão do Fluig: todos os usuários
autenticados (papel `user`) veem. Com `--view-*`, só os grupos, papéis e
usuários informados veem. Sem `--edit-*`, o usuário que cria a página recebe
as permissões de editar e remover. Com `--edit-*`, os grupos, papéis e
usuários informados recebem essas permissões.

Com `--parent`, a página-pai precisa aceitar subpáginas. Ligue a opção com
`fluigcli page update <pai> --enable-children` e publique o pai. Sem isso, o
comando para com exit **2**.

O Fluig grava permissão para grupo inexistente sem erro. Por isso, a CLI
confere antes se cada grupo, papel e usuário existe. Um nome inexistente
termina com exit **4**. Layout inexistente também termina com exit **4**.
Código repetido termina com exit **5**.

## `fluigcli page add-widget <página> <widget> [--slot <slot>]`

Este comando põe uma widget num slot do rascunho. O slot default é `SlotA`.

O Fluig não confere o slot e aceita qualquer nome. Por isso, a CLI confere o
slot pelo layout da página, nesta ordem:

1. A CLI abre uma página publicada com o mesmo layout e lê os slots de
   conteúdo dela (os slots marcados como editáveis).
2. Sem essa página, a CLI lê os slots do `layout.ftl` do projeto
   (`wcm/layout/<código>/src/main/resources/layout.ftl`): as chamadas
   `<@wcm.renderSlot id="..." editableSlot="true"/>`.
3. Sem nenhuma das duas fontes, a CLI avisa e segue.

Slot inexistente termina com exit **2**, e a mensagem lista os slots do
layout. Os slots do tema, como `SlotMenu` e `SlotUsePolicy`, não contam.

Widget inexistente termina com exit **4**. Publique a widget antes com
`fluigcli widget export`.

### Widgets de sistema (cabeçalho, menu e busca)

Não ponha as widgets de sistema na página. Por exemplo: `menu`,
`responsive_header`, `suggestsearch` e `alertpopover`. O tema mostra o
cabeçalho, o menu e a busca sozinho.

Medido no Fluig 2.0 (Voyager): uma página criada pelo `page create` com o
layout `portais` tem só a widget de conteúdo. Mesmo assim, ela mostra o
cabeçalho e o menu. Algumas páginas antigas têm essas widgets como instâncias,
em slots próprios (`SlotMenu`, `SlotHeaderResponsive`, `SlotGlobalAlert`...).
O `page show` lista essas instâncias, mas elas não são obrigatórias numa
página nova.

## `fluigcli page remove-widget <página> <instanceId>`

Este comando tira uma widget da página. Com o id do rascunho, a remoção fica
no rascunho até o `publish`. Com o id publicado e sem rascunho aberto, o
efeito é imediato. Um id publicado com rascunho aberto termina com exit
**2**. Um id que não está na página termina com exit **4**.

## `fluigcli page move-widget <página> <instanceId> --slot <slot> [--order N]`

Este comando move uma widget para outro slot ou outra posição no slot. A
regra das versões é a mesma do `remove-widget`. Sem `--order`, a widget vai
para o fim do slot de destino. A CLI confere o slot pelo layout, como no
`add-widget`.

```sh
fluigcli page move-widget painel_vendas 4816 --slot SlotB
fluigcli page move-widget painel_vendas 4816 --slot SlotA --order 1
```

## `fluigcli page set-preferences <página> <instanceId>`

Este comando grava as preferências de uma widget da versão publicada. O JSON
chega pelo stdin, como um objeto. Número e booleano viram texto. Objeto e lista
são recusados com exit **2**.

```sh
echo '{"titulo":"Painel de Vendas","limite":"20"}' | \
  fluigcli page set-preferences painel_vendas 4623
```

A gravação **substitui** todas as preferências da instância. Uma chave que
falta no JSON some. Para mudar uma chave, leia as atuais com `page show` e
envie o objeto completo.

## `fluigcli page permission <página> [--view-*] [--edit-*]`

Este comando troca a lista de quem pode ver a página pelos `--view-*`
informados. Ele também troca a lista de quem pode editar e remover pelos
`--edit-*` informados. A família que você não informa fica igual. O Fluig
grava a mudança num rascunho. Publique depois com `page publish`.

## `fluigcli page update <página> [flags]`

Este comando altera as informações da página. Só os campos das flags
informadas mudam. O Fluig grava a mudança num rascunho. Publique depois com
`page publish`.

| Flag | Uso |
|---|---|
| `--title <texto>` | título novo |
| `--description <texto>` | descrição nova |
| `--authorization <tipo>` | `public` (sem login) ou `authenticated` |
| `--friendly-url <trecho>` | URL amigável. Ela é relativa à raiz do servidor. Por exemplo, `painel` abre em `http://<servidor>/painel`. `""` remove a URL. |
| `--parent <página>` | página-pai nova. `""` tira a página de debaixo do pai. |
| `--enable-children` | aceita subpáginas. `--enable-children=false` desliga. |
| `--layout <código>` | layout novo |

Na troca de layout, o Fluig remaneja as widgets para os slots do layout novo.
Ele também acrescenta as widgets de sistema do layout, como o cabeçalho, a
busca e os alertas. Confira o resultado com `page show` antes do `publish`.
Se uma widget ficar num slot que o layout novo não tem, a CLI avisa.

## `fluigcli page publish <página> [--description <texto>]`

Este comando publica o rascunho. Página sem rascunho termina com exit **2**.
A saída traz as widgets publicadas, com os `instanceId` novos.

## `fluigcli page discard <página>`

Este comando descarta o rascunho e mantém a versão publicada. Para uma página
que nunca foi publicada, use `page delete`.

## `fluigcli page delete <página>... --yes`

Este comando exclui páginas. O Fluig só exclui página arquivada. Por isso, a
CLI arquiva a página antes. O comando pede confirmação. Em modo
não-interativo, confirme com `--yes`. Código inexistente termina com exit
**4**.

## Levar a página para outro ambiente

Use um arquivo de especificação versionado no Git. O fluxo é este:

1. Grave a especificação a partir do servidor de origem:

   ```sh
   fluigcli page export painel_vendas --server homolog
   ```

   O comando grava `wcm/page/painel_vendas.json`.
2. Revise o arquivo e faça o commit. Confira as preferências das widgets:
   elas podem conter URLs ou códigos do ambiente de origem.
3. Aplique no servidor de destino:

   ```sh
   fluigcli page apply wcm/page/painel_vendas.json --server producao --dry-run
   fluigcli page apply wcm/page/painel_vendas.json --server producao
   ```

Você também pode pôr o arquivo num plano de [`deploy`](deploy.md), com o passo
`page`.

### `fluigcli page export <página> [--output <arquivo>]`

Este comando grava a especificação da versão **publicada** da página em
`wcm/page/<código>.json`. Com `--output`, ele grava no arquivo informado.

```json
{
  "code": "painel_vendas",
  "title": "Painel de Vendas",
  "layout": "layoutdefault",
  "authorization": "AUTHENTICATED",
  "permissions": [
    {"permission": "pageView", "group": "vendas"},
    {"permission": "pageUpdate", "user": "jsilva"},
    {"permission": "pageRemove", "user": "jsilva"}
  ],
  "widgets": [
    {"widget": "painel_vendas_widget", "slot": "SlotA", "preferences": {"titulo": "Vendas"}}
  ]
}
```

O arquivo traz só as widgets de conteúdo, nos slots do layout. As widgets de
sistema ficam de fora, porque o layout as traz. Os campos opcionais são
`description`, `friendlyUrl`, `parent` e `enabledChildren`. A ordem das
widgets dentro de um slot é a ordem do arquivo. Uma página que nunca foi
publicada termina com exit **2**.

### `fluigcli page apply <código | arquivo.json> [--dry-run]`

Este comando cria a página quando ela não existe. Quando ela existe, o comando
alinha a página com a especificação: informações, permissões, layout, widgets
de conteúdo e preferências. Quando algo muda, o comando publica. Rodar de novo
não muda nada.

Informe o caminho do arquivo ou só o código. O código lê o arquivo
`wcm/page/<código>.json` do projeto. É o mesmo arquivo que o `export` grava.

Antes de gravar, a CLI confere o layout, a página-pai, as widgets, os grupos,
os papéis, os usuários e os slots. Um problema termina com exit **2** ou
**4**, e nada é gravado.

Regras:

- Uma página com rascunho aberto é recusada com exit **2**. O `apply` não
  publica a mudança de outra pessoa.
- Sem `permissions` no arquivo, a página nova recebe as permissões padrão, e a
  página existente mantém as dela.
- Quando as widgets diferem, o `apply` refaz as widgets de conteúdo no
  rascunho e publica. As widgets de sistema não mudam.
- As preferências são casadas pela posição (slot e ordem). Só as widgets com
  `preferences` no arquivo são alteradas.

Use `--dry-run` para ver as mudanças sem gravar.

## Produção

Os comandos de escrita (`create`, `update`, `add-widget`, `move-widget`,
`remove-widget`, `set-preferences`, `permission`, `publish`, `discard`,
`apply` e `delete`) pedem
confirmação num servidor de produção. Em modo não-interativo, confirme com
`--yes`.
