# fluigcli page — páginas do portal

O grupo `page` cria páginas do portal, põe widgets nelas, define quem pode ver
e publica. Use este grupo depois do `widget export`, para a widget aparecer
para os usuários sem abrir o editor de páginas.

## Rascunho e versão publicada

Uma página tem uma **versão publicada** e, durante a edição, um **rascunho**.
Os usuários veem só a versão publicada.

- A página nova nasce em rascunho.
- O `add-widget` e o `permission` gravam no rascunho. O `add-widget` abre um
  rascunho quando a página não tem um.
- O `publish` torna o rascunho a nova versão publicada.
- O `discard` descarta o rascunho e mantém a versão publicada.
- O `remove-widget` e o `set-preferences` agem direto na versão publicada, sem
  `publish`. Por isso, eles exigem que a página não tenha rascunho aberto.

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

Este comando mostra a página: layout, status, versão e autorização. Ele também
mostra as widgets da versão publicada, com o `instanceId`, o slot e as
preferências, e as permissões.

Com rascunho aberto, as widgets mostradas são as da versão publicada.

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
| `--public` | página pública, sem login. O default é só para usuários autenticados. |

Sem `--view-*`, a página segue o padrão do Fluig: todos os usuários
autenticados (papel `user`) veem. Com `--view-*`, só os grupos, papéis e
usuários informados veem. Nos dois casos, o usuário que cria a página recebe as
permissões de editar e remover.

O Fluig grava permissão para grupo inexistente sem erro. Por isso, a CLI
confere antes se cada grupo, papel e usuário existe. Um nome inexistente
termina com exit **4**. Layout inexistente também termina com exit **4**.
Código repetido termina com exit **5**.

## `fluigcli page add-widget <página> <widget> [--slot <slot>]`

Este comando põe uma widget num slot do rascunho. O slot default é `SlotA`.

O Fluig não confere o slot e aceita qualquer nome. Por isso, a CLI confere o
slot pelo layout da página. Para isso, ela abre uma página publicada com o
mesmo layout e lê os slots dela. Slot inexistente termina com exit **2**, e a
mensagem lista os slots do layout. Quando a CLI não acha uma página publicada
com o layout, ela avisa e segue.

Widget inexistente termina com exit **4**. Publique a widget antes com
`fluigcli widget export`.

## `fluigcli page remove-widget <página> <instanceId>`

Este comando tira uma widget da versão publicada. O efeito é imediato. A página
não pode ter rascunho aberto. Um id que não está na página termina com exit
**4**.

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

## `fluigcli page permission <página> --view-group <grupo>...`

Este comando troca a lista de quem pode ver a página pelos `--view-*`
informados. As permissões de editar e remover ficam iguais. O Fluig grava a
mudança num rascunho. Publique depois com `page publish`.

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

## Produção

Os comandos de escrita (`create`, `add-widget`, `remove-widget`,
`set-preferences`, `permission`, `publish`, `discard` e `delete`) pedem
confirmação num servidor de produção. Em modo não-interativo, confirme com
`--yes`.
