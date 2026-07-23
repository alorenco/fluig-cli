# fluigcli db — consultas SQL de diagnóstico

O grupo `db` executa SQL de **leitura** contra um datasource do servidor de
aplicação do Fluig. Você faz isso do terminal, sem acesso direto ao banco. Use
o grupo para diagnóstico. Por exemplo, você confere as permissões do login do
datasource. Você valida um SQL antes de colar num dataset. Você checa se um
objeto ou uma coluna existe.

Estes comandos precisam do componente auxiliar **fluigcliHelper 0.6.0 ou
superior** no servidor. Instale ou atualize o helper com o comando
`fluigcli server install-helper <name> [--force]`.

Estes comandos precisam de um usuário administrador do tenant. O helper abre a
conexão em modo somente leitura. O helper aceita apenas consultas `SELECT` ou
`WITH`. O helper recusa qualquer instrução de escrita (`INSERT`, `UPDATE`,
`DELETE`, DDL). O helper também recusa mais de uma instrução por consulta.

O `db` é SQL cru de diagnóstico. Ele não é o mesmo que o
[`dataset query`](dataset.md), que executa um dataset cadastrado no Fluig.

## `fluigcli db datasources`

Este comando lista os datasources JNDI disponíveis no servidor.

```sh
fluigcli db datasources
fluigcli db datasources --json
```

O comando marca o datasource padrão (`/jdbc/AppDS`, o banco do Fluig) em verde.
Use um destes nomes na opção `--jndi` do `db query`.

O helper enumera os datasources pelo naming do servidor de aplicação. Alguns
ambientes não permitem esta enumeração. Neste caso, a lista vem vazia. Passe o
nome do datasource direto na opção `--jndi` do `db query`.

## `fluigcli db query`

Este comando executa uma consulta de leitura e mostra o resultado em tabela.

```sh
fluigcli db query "select suser_sname() as login, db_name() as db"
fluigcli db query "select has_perms_by_name(?, 'OBJECT', 'INSERT') as ok" --param dbo.MINHA_TABELA
fluigcli db query "select top 10 * from wcm_application" --jndi /jdbc/TotvsRM
fluigcli db query "select 1" --json
```

- `--jndi` — o datasource JNDI. O valor padrão é `/jdbc/AppDS` (o banco do
  Fluig). Use `db datasources` para ver os nomes disponíveis.
- `--param` — o valor de um `?` do SQL. A ordem dos `--param` segue a ordem dos
  `?`. Repita a opção para cada `?`. Use os `?` para não concatenar valores no
  texto do SQL.
- `--max-rows` — o teto de linhas. O valor padrão é 500. O valor máximo é
  10000.

No terminal, o comando mostra os nomes das colunas no cabeçalho. Ele mostra um
valor nulo do banco como `(null)`. Quando o resultado chega ao teto de linhas,
o comando avisa. Neste caso, aumente o valor de `--max-rows`.

Com `--json`, o envelope traz `{columns[], rows[], rowCount, truncated}`. Cada
item de `columns` tem `name` e `type` (o nome do tipo do driver). As linhas em
`rows` são **posicionais**. Cada linha é um vetor alinhado com `columns` na
ordem. Um valor nulo do banco vem como `null` no JSON.

Quando a consulta tem um erro de SQL, o servidor devolve a mensagem do banco. A
CLI mostra esta mensagem e termina com o código 5. Quando a consulta não é de
leitura, o servidor recusa com a mesma via.

## Exit codes

| código | quando |
|---|---|
| `0` | sucesso |
| `2` | uso incorreto (falta o SQL, flag inválida) |
| `4` | o datasource não existe |
| `5` | o servidor recusou a consulta (erro de SQL ou consulta que não é de leitura) |
| `7` | fluigcliHelper ausente ou **desatualizado** (< 0.6.0, sem as rotas de db). Atualize com `server install-helper <name> --force`. |
