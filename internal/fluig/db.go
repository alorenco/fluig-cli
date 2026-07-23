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
