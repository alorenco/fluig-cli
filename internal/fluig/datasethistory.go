package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const restDatasetV2Base = "/dataset/api/v2/"

// DatasetVersion é uma entrada do histórico de versões de um dataset
// customizado (REST v2 dataset-history). O updateTime da API vem em epoch
// millis e é convertido para time.Time.
type DatasetVersion struct {
	DatasetID   string    `json:"datasetId"`
	Description string    `json:"description"`
	Version     int       `json:"version"`
	Status      string    `json:"status"` // PUBLISHED | DRAFT
	Author      string    `json:"author"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Impl        string    `json:"impl,omitempty"` // código JS da versão
}

// datasetHistoryItem é o item cru da API (schema DatasetHistory do swagger).
type datasetHistoryItem struct {
	UserName           string `json:"userName"`
	DatasetID          string `json:"datasetId"`
	DatasetDescription string `json:"datasetDescription"`
	DatasetImpl        string `json:"datasetImpl"`
	Version            int    `json:"version"`
	Status             string `json:"status"`
	UpdateTime         int64  `json:"updateTime"`
}

func (it datasetHistoryItem) toVersion() DatasetVersion {
	return DatasetVersion{
		DatasetID:   it.DatasetID,
		Description: it.DatasetDescription,
		Version:     it.Version,
		Status:      it.Status,
		Author:      it.UserName,
		UpdatedAt:   time.UnixMilli(it.UpdateTime),
		Impl:        it.DatasetImpl,
	}
}

// DatasetHistory devolve o histórico de versões de um dataset (paginado; a
// mais antiga primeiro, como o servidor devolve). Só datasets customizados
// têm histórico; dataset inexistente responde lista vazia — o chamador decide
// como tratar (a API não distingue).
func (c *Client) DatasetHistory(ctx context.Context, id string) ([]DatasetVersion, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []DatasetVersion
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("datasetId", id)
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url(restDatasetV2Base+"dataset-history") + "?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: "dataset-history", Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items   []datasetHistoryItem `json:"items"`
			HasNext bool                 `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de dataset-history: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, it.toVersion())
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// EnableDataset reativa um dataset desativado (POST /v2/datasets/enable/{id}).
func (c *Client) EnableDataset(ctx context.Context, id string) error {
	return c.setDatasetActive(ctx, "enable", id)
}

// DisableDataset desativa um dataset sem apagá-lo (POST /v2/datasets/disable/{id}).
func (c *Client) DisableDataset(ctx context.Context, id string) error {
	return c.setDatasetActive(ctx, "disable", id)
}

func (c *Client) setDatasetActive(ctx context.Context, op, id string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	endpoint := c.url(restDatasetV2Base + "datasets/" + op + "/" + url.PathEscape(id))
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: dataset %q", ErrNotFound, id)
	}
	if status < 200 || status >= 300 {
		return &HTTPError{StatusCode: status, URL: "datasets/" + op, Body: truncate(string(body), 512)}
	}
	return nil
}

// DatasetHasDraft informa se o dataset tem um rascunho não publicado — que um
// restore descartaria (GET /v2/dataset-history/restore/validation).
func (c *Client) DatasetHasDraft(ctx context.Context, id string) (bool, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return false, err
	}
	endpoint := c.url(restDatasetV2Base+"dataset-history/restore/validation") +
		"?datasetId=" + url.QueryEscape(id)
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	if status < 200 || status >= 300 {
		return false, &HTTPError{StatusCode: status, URL: "dataset-history/restore/validation", Body: truncate(string(body), 512)}
	}
	var parsed struct {
		Draft bool `json:"draft"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, fmt.Errorf("resposta inesperada de restore/validation: %w", err)
	}
	return parsed.Draft, nil
}

// RestoreDatasetVersion restaura um dataset para a versão dada do histórico
// (POST /v2/dataset-history/restore) e devolve a entrada resultante.
func (c *Client) RestoreDatasetVersion(ctx context.Context, id string, version int) (*DatasetVersion, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("datasetId", id)
	params.Set("version", strconv.Itoa(version))
	endpoint := c.url(restDatasetV2Base+"dataset-history/restore") + "?" + params.Encode()
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: dataset %q (ou versão %d inexistente)", ErrNotFound, id, version)
	}
	if status < 200 || status >= 300 {
		return nil, &HTTPError{StatusCode: status, URL: "dataset-history/restore", Body: truncate(string(body), 512)}
	}
	// O swagger promete um DatasetHistory no corpo, mas na homologação (Voyager
	// 2.0.0, validado 2026-07-13) o restore responde 2xx com corpo VAZIO — a
	// versão nova sai do histórico.
	if len(bytes.TrimSpace(body)) == 0 {
		versions, herr := c.DatasetHistory(ctx, id)
		if herr != nil || len(versions) == 0 {
			return nil, nil //nolint:nilnil // restore ok; sem detalhe da versão nova
		}
		latest := versions[len(versions)-1]
		return &latest, nil
	}
	var it datasetHistoryItem
	if err := json.Unmarshal(body, &it); err != nil {
		return nil, fmt.Errorf("resposta inesperada de dataset-history/restore: %w", err)
	}
	v := it.toVersion()
	return &v, nil
}
