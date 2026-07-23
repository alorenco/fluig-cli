package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Rotas de banco do fluigcliHelper (existem a partir do helper 0.6.0): executam
// SQL de leitura contra um datasource JNDI do servidor de aplicação. A política
// read-only (só SELECT/WITH, uma instrução) é imposta no lado do helper.
const helperDbPath = "/" + HelperFluigcli + "/api/db"

// DefaultDatasource é o datasource do Fluig (usado quando o chamador não indica
// outro com --jndi). É o mesmo que os repositórios internos do helper usam.
const DefaultDatasource = "/jdbc/AppDS"

// helperHasDbAPI: as rotas /db existem a partir do fluigcliHelper 0.6.0.
func helperHasDbAPI(version string) bool { return helperAtLeast(version, 0, 6) }

// DbColumn é uma coluna do resultado: nome + tipo SQL (nome do tipo do driver).
type DbColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DbResult é o resultado de uma consulta: colunas + linhas posicionais (cada
// linha alinha com Columns na ordem). Os valores vêm como texto (null vira nil).
type DbResult struct {
	Columns   []DbColumn  `json:"columns"`
	Rows      [][]*string `json:"rows"`
	RowCount  int         `json:"rowCount"`
	Truncated bool        `json:"truncated"`
}

// DbQueryOptions são os parâmetros de uma consulta de diagnóstico.
type DbQueryOptions struct {
	JNDI    string   // vazio = DefaultDatasource (resolvido pelo helper)
	SQL     string   // SELECT ou WITH (o helper recusa o resto)
	Params  []string // valores dos `?` na ordem
	MaxRows int      // teto de linhas (0 = default do helper)
}

type dbQueryRequest struct {
	JNDI    string   `json:"jndi,omitempty"`
	SQL     string   `json:"sql"`
	Params  []string `json:"params,omitempty"`
	MaxRows int      `json:"maxRows,omitempty"`
}

// DbQuery executa uma consulta de leitura contra um datasource JNDI do servidor.
// Requer o fluigcliHelper >= 0.6.0. Erro de SQL ou violação da política
// read-only voltam como erro de servidor com a mensagem do helper.
func (c *Client) DbQuery(ctx context.Context, opts DbQueryOptions) (*DbResult, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(dbQueryRequest(opts))
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.url(helperDbPath+"/query"), payload)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		var res DbResult
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, fmt.Errorf("resposta inesperada de %s/db/query: %w", HelperFluigcli, err)
		}
		if res.Columns == nil {
			res.Columns = []DbColumn{}
		}
		if res.Rows == nil {
			res.Rows = [][]*string{}
		}
		return &res, nil
	case http.StatusBadRequest:
		// Corpo = mensagem (recusa read-only ou erro de SQL do banco).
		return nil, fmt.Errorf("%w: %s", errServerRejected, strings.TrimSpace(string(body)))
	case http.StatusNotFound:
		return nil, c.dbNotFoundError(ctx, strings.TrimSpace(string(body)))
	default:
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/db/query", Body: truncate(string(body), 512)}
	}
}

// ListDatasources enumera os datasources JNDI publicados no servidor
// (best-effort: alguns ambientes não permitem a enumeração e a lista vem
// vazia). Requer o fluigcliHelper >= 0.6.0.
func (c *Client) ListDatasources(ctx context.Context) ([]string, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(helperDbPath+"/datasources"), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		if info, e := c.HelperStatus(ctx); e == nil && info.Installed && !helperHasDbAPI(info.Version) {
			return nil, ErrHelperOutdated
		}
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/db/datasources", Body: truncate(string(body), 512)}
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/db/datasources", Body: truncate(string(body), 512)}
	}
	var out []string
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s/db/datasources: %w", HelperFluigcli, err)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// GrantPerms são as permissões de objeto que o preflight sabe checar. Os nomes
// entram no texto do SQL (3º argumento de HAS_PERMS_BY_NAME e alias de coluna),
// por isso a lista funciona como whitelist contra injeção.
var GrantPerms = []string{"SELECT", "INSERT", "UPDATE", "DELETE"}

// IsGrantPerm informa se p é uma permissão de objeto conhecida (maiúsculas).
func IsGrantPerm(p string) bool {
	for _, g := range GrantPerms {
		if g == p {
			return true
		}
	}
	return false
}

// GrantsOptions são os parâmetros do preflight de permissões de banco.
type GrantsOptions struct {
	JNDI   string   // vazio = DefaultDatasource (resolvido pelo helper)
	Tables []string // objetos a checar (ex.: dbo.MINHA_TABELA)
	Perms  []string // vazio = GrantPerms; cada item precisa passar por IsGrantPerm
}

// TableGrants é o resultado do preflight para um objeto. Grants mapeia cada
// permissão pedida para o veredicto: true = concedida, false = negada, nil =
// indeterminada (o objeto não existe ou o login não pode verificar). Missing
// lista as permissões não confirmadas (negadas ou indeterminadas), na ordem
// pedida — é o que o consumidor itera para saber o que falta.
type TableGrants struct {
	Table   string           `json:"table"`
	Exists  bool             `json:"exists"`
	Grants  map[string]*bool `json:"grants"`
	Missing []string         `json:"missing"`
}

// GrantsResult é o resultado completo do preflight: o login e o banco do
// datasource, as permissões checadas e o veredicto por tabela. OK é true só
// quando todo objeto existe e toda permissão pedida está concedida.
type GrantsResult struct {
	Login    string        `json:"login"`
	Database string        `json:"database"`
	Perms    []string      `json:"perms"`
	Tables   []TableGrants `json:"tables"`
	OK       bool          `json:"ok"`
}

// DbGrants checa, para cada tabela, se o login do datasource tem as permissões
// pedidas (SQL Server, via HAS_PERMS_BY_NAME). Monta um único SELECT de leitura
// e o executa por DbQuery — não exige uma rota nova no helper (reusa a de db do
// helper >= 0.6.0). Erros de banco (ex.: motor não-SQL Server) voltam como erro
// de servidor com a mensagem do banco.
func (c *Client) DbGrants(ctx context.Context, opts GrantsOptions) (*GrantsResult, error) {
	if len(opts.Tables) == 0 {
		return nil, fmt.Errorf("nenhuma tabela informada")
	}
	perms := opts.Perms
	if len(perms) == 0 {
		perms = GrantPerms
	}
	norm := make([]string, len(perms))
	for i, p := range perms {
		up := strings.ToUpper(strings.TrimSpace(p))
		if !IsGrantPerm(up) {
			return nil, fmt.Errorf("permissão inválida %q", p)
		}
		norm[i] = up
	}
	sql, params := buildGrantsSQL(opts.Tables, norm)
	res, err := c.DbQuery(ctx, DbQueryOptions{JNDI: opts.JNDI, SQL: sql, Params: params})
	if err != nil {
		return nil, err
	}
	return parseGrants(res, opts.Tables, norm), nil
}

// buildGrantsSQL monta o SELECT do preflight. As tabelas viram parâmetros (`?`)
// para não concatenar entrada do usuário; as permissões já passaram pela
// whitelist, então podem entrar no texto como literal e alias. A coluna `ord`
// preserva a ordem de entrada no resultado.
//
// A coluna `__oid` (OBJECT_ID) decide a existência do objeto — é NULL quando o
// objeto não existe no banco daquele datasource. Não dá para usar o retorno do
// HAS_PERMS_BY_NAME para isso: ele devolve 0 (não NULL) para objeto inexistente
// (validado na homologação em 2026-07-23), o que confundiria "negado" com
// "inexistente".
func buildGrantsSQL(tables, perms []string) (string, []string) {
	var b strings.Builder
	b.WriteString("SELECT SUSER_SNAME() AS [login], DB_NAME() AS [db], q.tabela AS [tabela], OBJECT_ID(q.tabela) AS [__oid]")
	for _, p := range perms {
		fmt.Fprintf(&b, ", HAS_PERMS_BY_NAME(q.tabela, 'OBJECT', '%s') AS [%s]", p, p)
	}
	b.WriteString(" FROM (VALUES ")
	params := make([]string, len(tables))
	for i, t := range tables {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "(%d, ?)", i)
		params[i] = t
	}
	b.WriteString(") AS q(ord, tabela) ORDER BY q.ord")
	return b.String(), params
}

// parseGrants interpreta o resultado do SELECT do preflight. Mapeia por nome de
// coluna (robusto à ordem) e por nome de tabela (robusto à ordem das linhas).
func parseGrants(res *DbResult, tables, perms []string) *GrantsResult {
	idx := make(map[string]int, len(res.Columns))
	for i, c := range res.Columns {
		idx[strings.ToLower(c.Name)] = i
	}
	out := &GrantsResult{Perms: perms, OK: true, Tables: make([]TableGrants, 0, len(tables))}
	if len(res.Rows) > 0 {
		out.Login = cellAt(res.Rows[0], idx, "login")
		out.Database = cellAt(res.Rows[0], idx, "db")
	}
	byTable := make(map[string][]*string, len(res.Rows))
	if ti, ok := idx["tabela"]; ok {
		for _, row := range res.Rows {
			if ti < len(row) && row[ti] != nil {
				byTable[*row[ti]] = row
			}
		}
	}
	for _, t := range tables {
		tg := TableGrants{Table: t, Grants: make(map[string]*bool, len(perms))}
		row := byTable[t]
		// A existência vem do OBJECT_ID (NULL = objeto não existe naquele banco);
		// o retorno do HAS_PERMS_BY_NAME não serve para isso (ver buildGrantsSQL).
		tg.Exists = row != nil && cellAt(row, idx, "__oid") != ""
		for _, p := range perms {
			var allowed *bool
			// Só interpreta a permissão quando o objeto existe. Sem isso, o 0 do
			// HAS_PERMS_BY_NAME para objeto inexistente viraria "negado" (✗) em
			// vez de "indeterminado" (?).
			if tg.Exists {
				if ci, ok := idx[strings.ToLower(p)]; ok && ci < len(row) && row[ci] != nil {
					v := strings.TrimSpace(*row[ci]) == "1"
					allowed = &v
				}
			}
			tg.Grants[p] = allowed
			if allowed == nil || !*allowed {
				tg.Missing = append(tg.Missing, p)
			}
		}
		if !tg.Exists || len(tg.Missing) > 0 {
			out.OK = false
		}
		out.Tables = append(out.Tables, tg)
	}
	return out
}

// cellAt devolve o texto da célula da coluna name (vazio se ausente ou NULL).
func cellAt(row []*string, idx map[string]int, name string) string {
	if i, ok := idx[name]; ok && i < len(row) && row[i] != nil {
		return *row[i]
	}
	return ""
}

// dbNotFoundError distingue "datasource não existe" (exit 4) de "helper sem as
// rotas de db" (helper < 0.6.0, exit 7) — o 404 é o mesmo, então a versão do
// helper decide.
func (c *Client) dbNotFoundError(ctx context.Context, msg string) error {
	if info, err := c.HelperStatus(ctx); err == nil && info.Installed && !helperHasDbAPI(info.Version) {
		return ErrHelperOutdated
	}
	if msg == "" {
		msg = "datasource não encontrado"
	}
	return fmt.Errorf("%w: %s", ErrNotFound, msg)
}
