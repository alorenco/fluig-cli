# [[.Title]]

Widget Fluig criada com `fluigcli widget new` (template **classic** — sem
toolchain: só HTML/FTL, JavaScript e CSS; nada para instalar ou compilar).

## Estrutura

```
[[.Code]]/
├── README.md                        ← este arquivo (não vai para o servidor)
└── src/main/
    ├── resources/                   ← lidos pelo Fluig de dentro do WAR
    │   ├── application.info         ← manifesto (código, título, recursos)
    │   ├── view.ftl                 ← template do modo visualização
    │   ├── edit.ftl                 ← template do modo edição da página
    │   └── [[.Code]]*.properties    ← textos/i18n (base, pt_BR, en_US, es)
    └── webapp/
        ├── WEB-INF/jboss-web.xml    ← context-root (identidade da widget)
        └── resources/
            ├── js/[[.Code]].js      ← lógica (SuperWidget)
            ├── css/[[.Code]].css    ← estilos (prefixe com o container!)
            └── images/icon.png      ← ícone na galeria de widgets
```

Só o conteúdo de `src/main/` entra no WAR — o `fluigcli widget export`
empacota exatamente essas três subárvores.

## Desenvolvimento

- **Ver no portal com live reload**: rode `fluigcli dev` na raiz do projeto e
  abra o portal pelo endereço local que ele mostrar. O JS/CSS desta widget é
  servido direto do disco e o navegador recarrega ao salvar.
- **Regras de ouro**:
  - A widget pode aparecer mais de uma vez na mesma página — todo id de DOM
    leva o sufixo `${instanceId}` (veja o `view.ftl`).
  - Visual: use as classes do Fluig Style Guide (`{host}/style-guide/`) —
    o portal já carrega o CSS e o `FLUIGC` em toda página, e o dark mode
    funciona sozinho.
  - Textos em `.properties` (chaves via `${i18n.getTranslation('chave')}` no
    FTL) — nada de texto fixo no HTML.
  - Chamadas às APIs do Fluig (`/api/public/...`) usam a sessão do usuário
    logado — nunca coloque credenciais no código.

## Deploy

```sh
fluigcli widget export [[.Code]]
```

Empacota o WAR e publica no servidor configurado (a instalação é assíncrona
no Fluig — acompanhe na Central de Componentes). Depois, adicione a widget a
uma página pelo editor do portal.
