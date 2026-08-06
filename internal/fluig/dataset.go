package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

const (
	soapDatasetPath = "/webdesk/ECMDatasetService"
	restDatasetBase = "/ecm/api/rest/ecm/dataset/"

	// datasetBuilder de datasets customizados.
	customDatasetBuilder = "com.datasul.technology.webdesk.dataset.CustomizedDatasetBuilder"
	datasetTypeCustom    = "CUSTOM"
)

// DatasetSummary é um dataset listado pela API REST v2 de datasets. Desde
// 2026-07-09 (troca do SOAP findAllFormulariesDatasets pela REST v2) não há
// mais o campo "version" — a API nova não o expõe; em compensação vieram
// description, active e draft.
type DatasetSummary struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // CUSTOM | BUILTIN | GENERATED
	Custom      bool   `json:"custom"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Draft       bool   `json:"draft"`
}

// ListDatasets retorna os datasets do servidor pela REST v2 (paginada). Se o
// módulo /dataset não existir no servidor (Fluig antigo → 404), cai para o
// SOAP findAllFormulariesDatasets (sem description/active/draft).
func (c *Client) ListDatasets(ctx context.Context) ([]DatasetSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []DatasetSummary
	for page := 1; ; page++ {
		endpoint := c.url("/dataset/api/v2/datasets") +
			"?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound && page == 1 {
			return c.listDatasetsSOAP(ctx)
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: "dataset/api/v2/datasets", Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items []struct {
				DatasetID          string `json:"datasetId"`
				DatasetDescription string `json:"datasetDescription"`
				Type               string `json:"type"`
				Custom             bool   `json:"custom"`
				Active             bool   `json:"active"`
				Draft              bool   `json:"draft"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de dataset/api/v2/datasets: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, DatasetSummary{
				ID:          it.DatasetID,
				Type:        it.Type,
				Custom:      it.Custom,
				Description: it.DatasetDescription,
				Active:      it.Active,
				Draft:       it.Draft,
			})
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// listDatasetsSOAP é o fallback para servidores sem a REST v2 de datasets.
func (c *Client) listDatasetsSOAP(ctx context.Context) ([]DatasetSummary, error) {
	reqBody, err := soap.BuildFindAllDatasets(int64(c.opts.CompanyID), c.opts.Username, c.opts.Password)
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapDatasetPath, "findAllFormulariesDatasets", reqBody)
	if err != nil {
		return nil, err
	}
	items, err := soap.ParseFindAllDatasets(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	out := make([]DatasetSummary, 0, len(items))
	for _, it := range items {
		out = append(out, DatasetSummary{
			ID:     it.DatasetID,
			Type:   it.Type,
			Custom: strings.EqualFold(it.Type, datasetTypeCustom),
			Active: true, // o SOAP não informa; lista só o que existe
		})
	}
	return out, nil
}

// Dataset é a estrutura de um dataset carregado. raw preserva o JSON completo
// para o round-trip no update (editDataset envia a estrutura carregada com o
// datasetImpl trocado).
type Dataset struct {
	CompanyID   int
	ID          string
	Description string
	Impl        string // datasetImpl: código JS
	Type        string

	raw map[string]json.RawMessage
}

// LoadDataset carrega estrutura + código de um dataset (REST loadDataset).
func (c *Client) LoadDataset(ctx context.Context, id string) (*Dataset, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url(restDatasetBase+"loadDataset") + "?datasetId=" + url.QueryEscape(id)
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// Quirk observado na homologação (Fluig 1.8.2, 2026-07-07): loadDataset de um
	// dataset inexistente responde HTTP 500 (não 404). Tratamos qualquer não-2xx
	// como "não encontrado" — a sessão já foi validada, então aqui a falha
	// significa que o dataset não existe/não é carregável.
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("%w: dataset %q (loadDataset HTTP %d)", ErrNotFound, id, status)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("resposta inesperada de loadDataset: %w", err)
	}
	// Um dataset inexistente pode voltar como objeto vazio/sem PK.
	if _, ok := raw["datasetPK"]; !ok {
		if _, ok := raw["datasetImpl"]; !ok {
			return nil, fmt.Errorf("%w: dataset %q", ErrNotFound, id)
		}
	}
	ds := &Dataset{raw: raw}
	ds.Description = jsonString(raw, "datasetDescription")
	ds.Impl = jsonString(raw, "datasetImpl")
	ds.Type = jsonString(raw, "type")
	if pk, ok := raw["datasetPK"]; ok {
		var p struct {
			CompanyID int    `json:"companyId"`
			DatasetID string `json:"datasetId"`
		}
		_ = json.Unmarshal(pk, &p)
		ds.CompanyID, ds.ID = p.CompanyID, p.DatasetID
	}
	return ds, nil
}

// CreateDataset cria um dataset customizado (REST createDataset).
func (c *Client) CreateDataset(ctx context.Context, id, description, impl string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	payload := map[string]any{
		"datasetPK":               map[string]any{"companyId": c.opts.CompanyID, "datasetId": id},
		"datasetDescription":      description,
		"datasetImpl":             impl,
		"datasetBuilder":          customDatasetBuilder,
		"serverOffline":           false,
		"mobileCache":             false,
		"mobileOffline":           false,
		"lastReset":               0,
		"lastRemoteSync":          0,
		"updateIntervalTimestamp": 0,
		"type":                    datasetTypeCustom,
	}
	return c.postDatasetWrite(ctx, "createDataset", payload)
}

// UpdateDataset atualiza um dataset existente: carrega a estrutura atual, troca
// só o datasetImpl e reenvia (editDataset?confirmnewstructure=false).
func (c *Client) UpdateDataset(ctx context.Context, loaded *Dataset, impl string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	raw := make(map[string]json.RawMessage, len(loaded.raw))
	for k, v := range loaded.raw {
		raw[k] = v
	}
	newImpl, err := json.Marshal(impl)
	if err != nil {
		return err
	}
	raw["datasetImpl"] = newImpl
	return c.postDatasetWrite(ctx, "editDataset?confirmnewstructure=false", raw)
}

// helperDatasetsPath é a rota de administração de dataset do fluigcliHelper
// (existe a partir do helper 0.7.0).
const helperDatasetsPath = "/" + HelperFluigcli + "/api/datasets"

// helperHasDatasetAdminAPI: a rota /datasets existe a partir do helper 0.7.0.
func helperHasDatasetAdminAPI(version string) bool { return helperAtLeast(version, 0, 7) }

// DeleteDatasetPermanently remove um dataset FISICAMENTE do servidor, via
// fluigcliHelper (DELETE /fluigcliHelper/api/datasets/{id} → EJB
// DatasetService.deletePermanently). Diferente do disable (reversível) e da REST
// legada deleteDataset (que só desativa), esta remoção é definitiva. Requer o
// helper >= 0.7.0. O tenant é o da sessão (resolvido no helper).
func (c *Client) DeleteDatasetPermanently(ctx context.Context, id string) error {
	if err := c.requireHelper(ctx); err != nil {
		return err
	}

	// Confirma que o dataset EXISTE antes de mandar apagar. O helper responde
	// 200 {"deleted":true} para id inexistente (medido na homologação em
	// 2026-07-27, ROADMAP §2.11-H), então sem esta checagem a CLI confirma uma
	// exclusão que nunca aconteceu. Mesmo padrão do form records delete
	// (§2.10-I): confirmar com uma leitura antes de destruir.
	if _, err := c.LoadDataset(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: dataset %q não existe no servidor (nada foi excluído)", ErrNotFound, id)
		}
		return err
	}

	endpoint := c.url(helperDatasetsPath + "/" + url.PathEscape(id))
	body, status, err := c.doJSON(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		// O controller nunca usa 404 para erro de negócio (usa 400). Então 404
		// aqui significa "rota ausente" — helper antigo, sem esta rota.
		if info, e := c.HelperStatus(ctx); e == nil && info.Installed && !helperHasDatasetAdminAPI(info.Version) {
			return ErrHelperOutdated
		}
		return &HTTPError{StatusCode: status, URL: HelperFluigcli + "/datasets", Body: truncate(string(body), 512)}
	default:
		// 400 e afins: erro de negócio do servidor com a mensagem do helper.
		return fmt.Errorf("%w: %s", errServerRejected, strings.TrimSpace(string(body)))
	}
}

// DatasetConstraint é um filtro do dataset-handle/search (equivalente ao
// constraint do getDataset SOAP).
type DatasetConstraint struct {
	Field   string
	Initial string
	Final   string
	Type    string // MUST | MUST_NOT | SHOULD (vazio = MUST)
	Like    bool
}

// DatasetQuery parametriza uma consulta de valores de dataset.
type DatasetQuery struct {
	Fields      []string
	Constraints []DatasetConstraint
	OrderBy     string // um único campo; sufixo _ASC/_DESC opcional
	Limit       int    // 0 = todas as linhas (pagina com offset por baixo)
	Offset      int
}

// DatasetResult é o resultado da consulta: colunas + linhas (valor nil =
// campo ausente naquela linha).
type DatasetResult struct {
	Columns []string
	Rows    []map[string]*string
	// MissingFields são os campos pedidos em DatasetQuery.Fields que o dataset
	// não devolveu. Vazio quando a consulta não pediu campos.
	MissingFields []string
}

// OnlyEmptyRow informa que o resultado é UMA linha com todos os campos vazios.
//
// ⚠️ Validado ao vivo na homologação (2026-08-06): a REST v2
// `dataset-handle/search` **materializa uma linha vazia** quando o dataset não
// devolve linha nenhuma. Confirmado na resposta crua, sem passar pela CLI:
//   - dataset customizado com `return DatasetBuilder.newDataset()` (zero
//     addRow) → `values:[{"campo":"", …}]`;
//   - o dataset NATIVO `document` sem casamento → o mesmo, com as 74 colunas
//     vazias;
//   - mas o nativo `colleague` sem casamento → `values:[]`, correto.
//
// Ou seja, o defeito é da plataforma e não vale para todos os datasets. A
// linha fantasma só aparece SOZINHA: com resultado real ela não é acrescentada
// (conferido com 10 linhas de `document`).
//
// É heurística: um dataset pode legitimamente devolver uma linha toda vazia.
// Por isso quem chama AVISA, e nunca descarta a linha.
func (r *DatasetResult) OnlyEmptyRow() bool {
	if len(r.Rows) != 1 {
		return false
	}
	for _, v := range r.Rows[0] {
		if v != nil && *v != "" {
			return false
		}
	}
	return true
}

// datasetHandleMaxPage é o teto por requisição do dataset-handle/search — o
// servidor aplica 300 quando limit não é enviado, mas aceita valores maiores.
const datasetHandleMaxPage = 500

// QueryDataset consulta os valores de um dataset pela REST v2
// (dataset-handle/search — substituiu o SOAP getDataset em 2026-07-09, que
// respondia EOF na homologação). Com Limit=0, pagina com offset até o fim.
// ⚠️ Validado na homologação: dataset inexistente (ou consulta inválida)
// responde 200 com columns/values null → ErrNotFound; dataset vazio responde
// arrays vazios.
func (c *Client) QueryDataset(ctx context.Context, name string, q DatasetQuery) (*DatasetResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	res := &DatasetResult{}
	offset, remaining := q.Offset, q.Limit
paging:
	for {
		pageLimit := datasetHandleMaxPage
		if remaining > 0 && remaining < pageLimit {
			pageLimit = remaining
		}
		params := url.Values{}
		params.Set("datasetId", name)
		for _, f := range q.Fields {
			params.Add("field", f)
		}
		for _, cons := range q.Constraints {
			typ := cons.Type
			if typ == "" {
				typ = "MUST"
			}
			final := cons.Final
			if final == "" {
				final = cons.Initial
			}
			params.Add("constraintsField", cons.Field)
			params.Add("constraintsInitialValue", cons.Initial)
			params.Add("constraintsFinalValue", final)
			params.Add("constraintsType", typ)
			params.Add("constraintsLikeSearch", strconv.FormatBool(cons.Like))
		}
		if q.OrderBy != "" {
			params.Set("orderby", q.OrderBy)
		}
		params.Set("limit", strconv.Itoa(pageLimit))
		params.Set("offset", strconv.Itoa(offset))

		endpoint := c.url("/dataset/api/v2/dataset-handle/search") + "?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: "dataset-handle/search", Body: truncate(string(body), 512)}
		}
		// ⚠️ Os valores NÃO são só strings: datasets como `document` trazem
		// bool/número no mesmo campo (validado na homologação, 2026-07-15) —
		// por isso decodificamos como json.RawMessage e coagimos para *string
		// (null → nil; string → texto; bool/número → o literal JSON).
		var parsed struct {
			Columns []string                     `json:"columns"`
			Values  []map[string]json.RawMessage `json:"values"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de dataset-handle/search: %w", err)
		}
		if parsed.Columns == nil && parsed.Values == nil {
			return nil, fmt.Errorf("%w: dataset %q (ou consulta inválida — confira campos e ordenação)", ErrNotFound, name)
		}
		if res.Columns == nil {
			res.Columns = parsed.Columns
		}
		for _, raw := range parsed.Values {
			row := make(map[string]*string, len(raw))
			for k, v := range raw {
				row[k] = rawToStringPtr(v)
			}
			res.Rows = append(res.Rows, row)
		}
		pageCount := len(parsed.Values)
		if remaining > 0 {
			remaining -= pageCount
			if remaining <= 0 {
				break paging
			}
		}
		if pageCount < pageLimit {
			break paging
		}
		offset += pageCount
	}
	projectFields(res, q.Fields)
	return res, nil
}

// projectFields recorta o resultado para os campos pedidos, na ordem pedida.
//
// ⚠️ O recorte é NECESSÁRIO no cliente: o `field` do dataset-handle/search é
// repassado ao dataset, e **dataset customizado que monta as próprias colunas
// simplesmente ignora o argumento** (validado no uso real: 2 campos pedidos, 23
// colunas devolvidas). Datasets nativos honram o parâmetro e aqui o recorte não
// tem efeito. Por isso a CLI continua enviando os campos ao servidor: quem
// honra, filtra lá e trafega menos.
//
// Campo pedido que não existe no resultado entra em MissingFields. Quando NENHUM
// campo pedido existe, o resultado fica inteiro: devolver tabela vazia
// esconderia o dado de quem só errou o nome da coluna.
func projectFields(res *DatasetResult, fields []string) {
	if len(fields) == 0 || len(res.Columns) == 0 {
		return
	}
	// O nome da coluna vem do dataset e a caixa varia entre datasets do
	// produto (`numvenda` × `numVenda`), então o casamento tolera a caixa mas
	// preserva o nome real da coluna na saída.
	realName := make(map[string]string, len(res.Columns))
	for _, col := range res.Columns {
		realName[strings.ToLower(col)] = col
	}
	cols := make([]string, 0, len(fields))
	keep := make(map[string]bool, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		col, ok := realName[strings.ToLower(f)]
		if !ok {
			res.MissingFields = append(res.MissingFields, f)
			continue
		}
		if keep[col] {
			continue // campo repetido no pedido
		}
		keep[col] = true
		cols = append(cols, col)
	}
	if len(cols) == 0 {
		return
	}
	res.Columns = cols
	for _, row := range res.Rows {
		for k := range row {
			if !keep[k] {
				delete(row, k)
			}
		}
	}
}

// postDatasetWrite envia um payload JSON para create/editDataset e interpreta a
// resposta: sucesso = {"content":"OK"}; falha = {"message":{"message":"..."}}.
func (c *Client) postDatasetWrite(ctx context.Context, op string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := c.url(restDatasetBase + op)
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, data)
	if err != nil {
		return err
	}
	var parsed struct {
		Content string `json:"content"`
		Message *struct {
			Message string `json:"message"`
		} `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)
	if parsed.Message != nil && parsed.Message.Message != "" {
		return fmt.Errorf("%w: %s", errServerRejected, parsed.Message.Message)
	}
	if status != http.StatusOK || !strings.EqualFold(strings.TrimSpace(parsed.Content), "OK") {
		return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
	}
	return nil
}

// errServerRejected marca uma rejeição de negócio do servidor (deploy recusado).
var errServerRejected = fmt.Errorf("operação rejeitada pelo servidor Fluig")

// postSOAP faz o POST de um envelope SOAP com os cookies de sessão do jar.
func (c *Client) postSOAP(ctx context.Context, path, soapAction string, envelope []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha na chamada SOAP %s em %s: %w", soapAction, c.base.Host, err)
	}
	body, err := readBody(resp, 8<<20)
	if err != nil {
		return nil, err
	}
	// SOAP Fault chega com HTTP 500 mas com corpo XML útil — deixa o parser tratar.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: path, Body: truncate(body, 512)}
	}
	return []byte(body), nil
}

// doJSON faz uma requisição REST com Accept/Content-Type JSON e cookies do jar.
func (c *Client) doJSON(ctx context.Context, method, endpoint string, body []byte) ([]byte, int, error) {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("falha ao chamar %s: %w", c.base.Host, err)
	}
	respBody, err := readBody(resp, 8<<20)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return []byte(respBody), resp.StatusCode, nil
}

// mapSOAPError converte um soap.Fault em erro de servidor do cliente.
func mapSOAPError(err error) error {
	var fault *soap.Fault
	if errors.As(err, &fault) {
		return fmt.Errorf("%w: %s", errServerRejected, fault.Error())
	}
	return err
}

// rawToStringPtr coage um valor JSON de célula de dataset para *string: null
// (ou ausente) → nil; string → o texto; bool/número/outros → o literal JSON
// (o dataset-handle/search mistura tipos no mesmo campo — ver QueryDataset).
func rawToStringPtr(raw json.RawMessage) *string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return &s
		}
	}
	s := string(trimmed)
	return &s
}

func jsonString(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}
