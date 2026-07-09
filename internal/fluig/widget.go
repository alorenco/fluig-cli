package fluig

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// WARFile é uma entrada de um WAR (nome dentro do zip + conteúdo).
type WARFile struct {
	Name    string
	Content []byte
}

// BuildWAR monta um WAR em memória com compressão STORE.
func BuildWAR(files []WARFile) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.Content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Widget é um widget listado pela fluiggersWidget (para import).
type Widget struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Filename    string `json:"filename"`
}

// ListWidgets lista os widgets do servidor via fluiggersWidget (import). Requer
// a widget auxiliar instalada; ausência → ErrHelperMissing (exit 7).
func (c *Client) ListWidgets(ctx context.Context) ([]Widget, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url("/fluiggersWidget/api/widgets"), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound || status == 0 {
		return nil, ErrHelperMissing
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "fluiggersWidget/widgets", Body: truncate(string(body), 512)}
	}
	var widgets []Widget
	if err := json.Unmarshal(body, &widgets); err != nil {
		return nil, fmt.Errorf("resposta inesperada de fluiggersWidget/widgets: %w", err)
	}
	return widgets, nil
}

// DownloadWidget baixa o .war/zip de um widget via fluiggersWidget.
func (c *Client) DownloadWidget(ctx context.Context, filename string) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url("/fluiggersWidget/api/widgets/") + url.PathEscape(filename)
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: widget %q", ErrNotFound, filename)
	}
	if status != http.StatusOK {
		return nil, &HTTPError{StatusCode: status, URL: "fluiggersWidget/widgets/" + filename, Body: truncate(string(body), 256)}
	}
	return body, nil
}

// ListWidgetsNative lista os widgets customizados pela API nativa de
// page-management (`GET /page-management/api/v2/applications?internal=false`),
// sem depender da fluiggersWidget. ⚠️ Limitações validadas na homologação
// (2026-07-09): a listagem nativa pode OMITIR widgets instaladas (3 de 28
// ficaram de fora, embora o GET por código as encontre) e não informa o nome
// do arquivo (.war), necessário ao download do import — por isso ela é o
// fallback do `widget list`, não a fonte primária.
func (c *Client) ListWidgetsNative(ctx context.Context) ([]Widget, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []Widget
	for page := 1; ; page++ {
		endpoint := c.url("/page-management/api/v2/applications") +
			"?internal=false&page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: "page-management/applications", Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items []struct {
				Code        string `json:"code"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de page-management/applications: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, Widget{Code: it.Code, Title: it.Title, Description: it.Description})
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// widgetUploadPath é o endpoint nativo de upload de widget/WAR.
const widgetUploadPath = "/portal/api/rest/wcmservice/rest/product/uploadfile"

const widgetUploadDescription = "WCM Eclipse Plugin Deploy Artifact"

// UploadWidgetWAR publica um WAR de widget via multipart. A
// instalação é assíncrona no servidor. Resposta com campo "message" = rejeição.
func (c *Client) UploadWidgetWAR(ctx context.Context, warName string, war []byte) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("fileName", warName); err != nil {
		return err
	}
	if err := mw.WriteField("fileDescription", widgetUploadDescription); err != nil {
		return err
	}
	part, err := mw.CreateFormFile("attachment", warName)
	if err != nil {
		return err
	}
	if _, err := part.Write(war); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(widgetUploadPath), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao publicar a widget em %s: %w", c.base.Host, err)
	}
	respBody, err := readBody(resp, 1<<20)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, URL: widgetUploadPath, Body: truncate(respBody, 512)}
	}
	// Sucesso = ausência de mensagem de erro no corpo.
	var parsed struct {
		Message json.RawMessage `json:"message"`
	}
	_ = json.Unmarshal([]byte(respBody), &parsed)
	if m := strings.TrimSpace(string(parsed.Message)); m != "" && m != "null" {
		return fmt.Errorf("%w: %s", errServerRejected, m)
	}
	return nil
}
