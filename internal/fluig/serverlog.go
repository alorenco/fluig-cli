package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Rotas de log do fluigcliHelper (existem a partir do helper 0.3.0): leem o
// diretório jboss.server.log.dir do servidor de aplicação, com whitelist de
// nome e anti-traversal no lado do helper.
const helperLogsPath = "/" + HelperFluigcli + "/api/logs"

// DefaultServerLog é o arquivo de log principal do WildFly.
const DefaultServerLog = "server.log"

// ServerLogFile é um arquivo do diretório de log do servidor.
type ServerLogFile struct {
	Name         string     `json:"name"`
	Size         int64      `json:"size"`
	LastModified *time.Time `json:"lastModified,omitempty"`
}

// ListServerLogs lista os arquivos do diretório de log (inclui os
// rotacionados, ex.: server.log.2026-07-17). Requer o fluigcliHelper ≥ 0.3.0.
func (c *Client) ListServerLogs(ctx context.Context) ([]ServerLogFile, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(helperLogsPath), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		// O helper responde ao ping mas não tem a rota: versão antiga.
		return nil, ErrHelperOutdated
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/logs", Body: truncate(string(body), 512)}
	}
	var items []struct {
		Name         string `json:"name"`
		Size         int64  `json:"size"`
		LastModified int64  `json:"lastModified"` // epoch millis (java.io.File)
	}
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s/logs: %w", HelperFluigcli, err)
	}
	out := make([]ServerLogFile, 0, len(items))
	for _, it := range items {
		f := ServerLogFile{Name: it.Name, Size: it.Size}
		if it.LastModified > 0 {
			t := time.UnixMilli(it.LastModified)
			f.LastModified = &t
		}
		out = append(out, f)
	}
	return out, nil
}

// ServerLogTailOptions são os parâmetros do tail server-side.
type ServerLogTailOptions struct {
	File  string // vazio = server.log
	Lines int    // entradas de log (stack trace inteiro conta como uma)
	Skip  int    // pula as N entradas mais recentes (paginação para trás)
	Level string // nível mínimo (trace|debug|info|warn|error|fatal)
	Grep  string // substring case-insensitive na entrada completa
}

// ServerLogTail é o resultado do tail: entradas da mais antiga para a mais
// nova; Size é o tamanho do arquivo no momento da leitura (offset para o
// acompanhamento via ReadServerLog).
type ServerLogTail struct {
	File      string   `json:"file"`
	Size      int64    `json:"size"`
	Entries   []string `json:"entries"`
	Truncated bool     `json:"truncated"`
}

// TailServerLog devolve as últimas entradas de um arquivo de log, com filtro
// opcional de nível/substring aplicado no servidor.
func (c *Client) TailServerLog(ctx context.Context, opts ServerLogTailOptions) (*ServerLogTail, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	file := opts.File
	if file == "" {
		file = DefaultServerLog
	}
	q := url.Values{}
	if opts.Lines > 0 {
		q.Set("lines", strconv.Itoa(opts.Lines))
	}
	if opts.Skip > 0 {
		q.Set("skip", strconv.Itoa(opts.Skip))
	}
	if opts.Level != "" {
		q.Set("level", opts.Level)
	}
	if opts.Grep != "" {
		q.Set("grep", opts.Grep)
	}
	endpoint := c.url(helperLogsPath+"/") + url.PathEscape(file) + "/tail"
	if enc := q.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, c.logNotFoundError(ctx, file)
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/logs/" + file + "/tail", Body: truncate(string(body), 512)}
	}
	var res ServerLogTail
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s/logs: %w", HelperFluigcli, err)
	}
	if res.Entries == nil {
		res.Entries = []string{}
	}
	return &res, nil
}

// ServerLogRangeOptions são os parâmetros da busca por intervalo de tempo.
// From/To no formato "yyyy-MM-ddTHH:mm:ss" (hora LOCAL do servidor — o log não
// tem offset; quem chama já converte). Vazio = sem limite naquele lado.
type ServerLogRangeOptions struct {
	File  string
	From  string
	To    string
	Level string
	Grep  string
}

// ServerLogRange é o resultado da busca por intervalo: entradas da mais antiga
// para a mais nova, dentro de [From, To].
type ServerLogRange struct {
	File      string   `json:"file"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Entries   []string `json:"entries"`
	Truncated bool     `json:"truncated"`
}

// RangeServerLog devolve as entradas de um arquivo de log cujo timestamp cai no
// intervalo [From, To], com filtro opcional de nível/substring. Requer o
// fluigcliHelper >= 0.5.0 (rota /range).
func (c *Client) RangeServerLog(ctx context.Context, opts ServerLogRangeOptions) (*ServerLogRange, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	file := opts.File
	if file == "" {
		file = DefaultServerLog
	}
	q := url.Values{}
	if opts.From != "" {
		q.Set("from", opts.From)
	}
	if opts.To != "" {
		q.Set("to", opts.To)
	}
	if opts.Level != "" {
		q.Set("level", opts.Level)
	}
	if opts.Grep != "" {
		q.Set("grep", opts.Grep)
	}
	endpoint := c.url(helperLogsPath+"/") + url.PathEscape(file) + "/range"
	if enc := q.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, c.rangeNotFoundError(ctx, file)
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/logs/" + file + "/range", Body: truncate(string(body), 512)}
	}
	var res ServerLogRange
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s/logs: %w", HelperFluigcli, err)
	}
	if res.Entries == nil {
		res.Entries = []string{}
	}
	return &res, nil
}

// ServerLogChunk é um trecho bruto do log a partir de um offset — a base do
// `log tail --follow` (polling por offset; o helper corta na última quebra de
// linha para nunca repartir uma linha entre chamadas).
type ServerLogChunk struct {
	File    string `json:"file"`
	From    int64  `json:"from"`
	To      int64  `json:"to"`
	Size    int64  `json:"size"`
	Content string `json:"content"`
}

// ReadServerLog lê o conteúdo do log a partir do offset. Size < from indica
// que o arquivo foi rotacionado/truncado desde a última leitura.
func (c *Client) ReadServerLog(ctx context.Context, file string, from int64) (*ServerLogChunk, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	if file == "" {
		file = DefaultServerLog
	}
	endpoint := c.url(helperLogsPath+"/") + url.PathEscape(file) + "/read?from=" + strconv.FormatInt(from, 10)
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, c.logNotFoundError(ctx, file)
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/logs/" + file + "/read", Body: truncate(string(body), 512)}
	}
	var res ServerLogChunk
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s/logs: %w", HelperFluigcli, err)
	}
	return &res, nil
}

// DownloadServerLog baixa o arquivo de log inteiro para w (streaming).
// ⚠️ A rota produz octet-stream — Accept: application/json responderia 406
// (mesmo padrão do download de widget/documento).
func (c *Client) DownloadServerLog(ctx context.Context, file string, w io.Writer) (int64, error) {
	if err := c.requireHelper(ctx); err != nil {
		return 0, err
	}
	if file == "" {
		file = DefaultServerLog
	}
	endpoint := c.url(helperLogsPath+"/") + url.PathEscape(file) + "/download"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("falha ao baixar o log de %s: %w", c.base.Host, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		_, _ = readBody(resp, 4096)
		return 0, c.logNotFoundError(ctx, file)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp, 4096)
		return 0, &HTTPError{StatusCode: resp.StatusCode, URL: HelperFluigcli + "/logs/" + file + "/download", Body: truncate(body, 256)}
	}
	defer resp.Body.Close()
	return io.Copy(w, resp.Body)
}

// logNotFoundError distingue "arquivo de log não existe" (exit 4) de "helper
// sem as rotas de log" (helper < 0.3.0, exit 7) — o 404 é o mesmo nos dois
// casos, então a versão do helper decide.
func (c *Client) logNotFoundError(ctx context.Context, file string) error {
	if info, err := c.HelperStatus(ctx); err == nil && info.Installed && !helperHasLogAPI(info.Version) {
		return ErrHelperOutdated
	}
	return fmt.Errorf("%w: arquivo de log %q", ErrNotFound, file)
}

// rangeNotFoundError: como o logNotFoundError, mas a rota /range só existe a
// partir do 0.5.0 (helper 0.3.0/0.4.0 responde 404 nela).
func (c *Client) rangeNotFoundError(ctx context.Context, file string) error {
	if info, err := c.HelperStatus(ctx); err == nil && info.Installed && !helperHasRangeAPI(info.Version) {
		return ErrHelperOutdated
	}
	return fmt.Errorf("%w: arquivo de log %q", ErrNotFound, file)
}

// helperHasLogAPI: as rotas de log existem a partir do fluigcliHelper 0.3.0.
func helperHasLogAPI(version string) bool { return helperAtLeast(version, 0, 3) }

// helperHasRangeAPI: a rota /range existe a partir do fluigcliHelper 0.5.0.
func helperHasRangeAPI(version string) bool { return helperAtLeast(version, 0, 5) }

// HelperSupportsLogRange informa se a versão do helper tem a busca por
// intervalo (/range, 0.5.0+) — usado pelo dev server para mostrar ou não o
// controle de intervalo no painel.
func HelperSupportsLogRange(version string) bool { return helperHasRangeAPI(version) }

// helperAtLeast compara "MAJOR.MINOR[.PATCH]" com o piso (maj, min).
// Versão irreconhecível conta como antiga (fallback defensivo).
func helperAtLeast(version string, maj, min int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > maj || (major == maj && minor >= min)
}
