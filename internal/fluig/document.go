package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

const restCMDocs = "/content-management/api/v2/documents"

// GEDDocument é um item da listagem de uma pasta do GED (invdata do
// Datatable). documentType: "0" raiz, "1" pasta, "2" arquivo, "8" artigo.
type GEDDocument struct {
	ID          int64      `json:"id"`
	Type        string     `json:"type"` // folder | file | article | código cru
	Description string     `json:"description"`
	Version     int        `json:"version"`
	ParentID    int64      `json:"parentId"`
	Publisher   string     `json:"publisher,omitempty"`
	SizeMB      float64    `json:"sizeMB,omitempty"`
	MimeType    string     `json:"mimeType,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

// gedTypeName traduz o documentType numérico da API.
func gedTypeName(code string) string {
	switch code {
	case "0", "1":
		return "folder"
	case "2":
		return "file"
	case "8":
		return "article"
	default:
		return code
	}
}

// ListGEDDocuments lista o conteúdo de uma pasta do GED (subpastas,
// arquivos, artigos...). ⚠️ O parâmetro `order` é OBRIGATÓRIO na API — sem
// ele o Datatable responde `invdata` vazio EM SILÊNCIO (pegadinha descoberta
// em 2026-07-11 e só explicada em 2026-07-14; ver CLAUDE.md).
func (c *Client) ListGEDDocuments(ctx context.Context, folderID int) ([]GEDDocument, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []GEDDocument
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("order", "documentDescription")
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url("/content-management/api/v2/folders/"+strconv.Itoa(folderID)+"/documents") +
			"?" + params.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("%w: pasta %d", ErrNotFound, folderID)
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("v2/folders/{id}/documents", status, body)
		}
		var parsed struct {
			TotalPages int `json:"totalpages"`
			CurrPage   int `json:"currpage"`
			InvData    []struct {
				DocumentID          int64   `json:"documentId"`
				DocumentType        string  `json:"documentType"`
				DocumentDescription string  `json:"documentDescription"`
				Version             int     `json:"version"`
				ParentDocumentID    int64   `json:"parentDocumentId"`
				PublisherName       string  `json:"publisherName"`
				Size                float64 `json:"size"`
				MimeType            string  `json:"mimeType"`
				LastModifiedDate    int64   `json:"lastModifiedDate"`
			} `json:"invdata"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/folders/{id}/documents: %w", err)
		}
		for _, it := range parsed.InvData {
			doc := GEDDocument{
				ID:          it.DocumentID,
				Type:        gedTypeName(it.DocumentType),
				Description: it.DocumentDescription,
				Version:     it.Version,
				ParentID:    it.ParentDocumentID,
				Publisher:   it.PublisherName,
				SizeMB:      it.Size,
				MimeType:    it.MimeType,
			}
			if it.LastModifiedDate > 0 {
				t := time.UnixMilli(it.LastModifiedDate)
				doc.UpdatedAt = &t
			}
			out = append(out, doc)
		}
		if parsed.CurrPage >= parsed.TotalPages || len(parsed.InvData) == 0 {
			return out, nil
		}
	}
}

// GEDDocumentInfo são os metadados de um documento (GET /v2/documents/{id}).
type GEDDocumentInfo struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	Type        string `json:"type"` // FileDocument | Folder | ...
	ParentID    int64  `json:"parentId"`
}

// GetGEDDocument carrega os metadados de um documento do GED.
func (c *Client) GetGEDDocument(ctx context.Context, id int) (*GEDDocumentInfo, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restCMDocs+"/"+strconv.Itoa(id)), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: documento %d", ErrNotFound, id)
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("v2/documents/{id}", status, body)
	}
	var info GEDDocumentInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("resposta inesperada de v2/documents/{id}: %w", err)
	}
	return &info, nil
}

// DownloadGEDDocument baixa o conteúdo de um documento (round-trip byte a
// byte validado na homologação). ⚠️ O stream exige Accept != application/json
// (406 NotAcceptableException); documento cujo arquivo físico sumiu do volume
// responde 500 NoSuchFileException — vira mensagem clara.
func (c *Client) DownloadGEDDocument(ctx context.Context, id int) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.url(restCMDocs+"/"+strconv.Itoa(id)+"/stream"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao chamar %s: %w", c.base.Host, err)
	}
	body, err := readBody(resp, 256<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: documento %d", ErrNotFound, id)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if strings.Contains(body, "NoSuchFileException") {
			return nil, fmt.Errorf("%w: o arquivo físico do documento %d não existe no volume do servidor", errServerRejected, id)
		}
		return nil, restRequestError("v2/documents/{id}/stream", resp.StatusCode, []byte(body))
	}
	return []byte(body), nil
}

// UploadGEDDocument publica um arquivo numa pasta do GED em uma etapa
// (POST /v2/documents/upload/{fileName}/{parentId}/publish, multipart).
// Validado na homologação em 2026-07-14.
func (c *Client) UploadGEDDocument(ctx context.Context, folderID int, fileName string, content []byte) (*GEDDocumentInfo, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(content); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	endpoint := c.url(restCMDocs + "/upload/" + url.PathEscape(fileName) + "/" +
		strconv.Itoa(folderID) + "/publish")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao chamar %s: %w", c.base.Host, err)
	}
	body, err := readBody(resp, 1<<20)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: pasta %d", ErrNotFound, folderID)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, restRequestError("v2/documents/upload/{name}/{parent}/publish", resp.StatusCode, []byte(body))
	}
	var dto struct {
		DocumentID          int64  `json:"documentId"`
		Version             int    `json:"version"`
		DocumentDescription string `json:"documentDescription"`
		FolderID            int64  `json:"folderId"`
	}
	if err := json.Unmarshal([]byte(body), &dto); err != nil {
		return nil, fmt.Errorf("resposta inesperada do upload: %w", err)
	}
	return &GEDDocumentInfo{ID: dto.DocumentID, Description: dto.DocumentDescription,
		Version: dto.Version, ParentID: dto.FolderID}, nil
}

// CreateGEDFolder cria uma pasta no GED (POST /v2/folders/{parentId}).
func (c *Client) CreateGEDFolder(ctx context.Context, parentID int, name string) (*GEDDocumentInfo, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]string{"alias": name})
	if err != nil {
		return nil, err
	}
	endpoint := c.url("/content-management/api/v2/folders/" + strconv.Itoa(parentID))
	body, status, err := c.doJSON(ctx, http.MethodPost, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: pasta %d", ErrNotFound, parentID)
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("v2/folders/{parentId}", status, body)
	}
	var dto struct {
		DocumentID          int64  `json:"documentId"`
		DocumentDescription string `json:"documentDescription"`
		FolderID            int64  `json:"folderId"`
	}
	if err := json.Unmarshal(body, &dto); err != nil {
		return nil, fmt.Errorf("resposta inesperada da criação de pasta: %w", err)
	}
	return &GEDDocumentInfo{ID: dto.DocumentID, Description: dto.DocumentDescription, ParentID: dto.FolderID}, nil
}

// DeleteGEDDocument envia um documento ou pasta do GED para a lixeira.
func (c *Client) DeleteGEDDocument(ctx context.Context, id int) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, http.MethodDelete, c.url(restCMDocs+"/"+strconv.Itoa(id)), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: documento %d", ErrNotFound, id)
	}
	if status < 200 || status >= 300 {
		return restRequestError("v2/documents/{id}", status, body)
	}
	return nil
}

// GEDTreeEntry é um item da varredura recursiva: o documento + o caminho
// legível relativo à pasta inicial (ex.: "Contratos/2026/arquivo.pdf").
type GEDTreeEntry struct {
	GEDDocument
	Path  string `json:"path"`
	Depth int    `json:"depth"` // 1 = conteúdo direto da pasta inicial
}

// WalkGEDTree varre uma pasta do GED em largura (BFS), descendo até maxDepth
// níveis (1 = só o conteúdo direto). maxRequests limita o número de listagens
// (cada pasta visitada = 1 requisição); ao atingir o teto a varredura PARA e
// devolve truncated=true — quem chama avisa (sem teto silencioso). A ordem do
// resultado é a da travessia (nível a nível, na ordem do servidor).
func (c *Client) WalkGEDTree(ctx context.Context, folderID, maxDepth, maxRequests int) ([]GEDTreeEntry, bool, error) {
	type frame struct {
		id    int
		path  string
		depth int
	}
	queue := []frame{{id: folderID, path: "", depth: 1}}
	var out []GEDTreeEntry
	requests := 0
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		if requests >= maxRequests {
			return out, true, nil
		}
		requests++
		docs, err := c.ListGEDDocuments(ctx, f.id)
		if err != nil {
			// A raiz inacessível é erro de verdade; uma SUBpasta que falha
			// (ex.: sem permissão) vira só um buraco na árvore — segue o jogo.
			if f.depth == 1 {
				return nil, false, err
			}
			continue
		}
		for _, d := range docs {
			p := d.Description
			if f.path != "" {
				p = f.path + "/" + d.Description
			}
			out = append(out, GEDTreeEntry{GEDDocument: d, Path: p, Depth: f.depth})
			if d.Type == "folder" && f.depth < maxDepth {
				queue = append(queue, frame{id: int(d.ID), path: p, depth: f.depth + 1})
			}
		}
	}
	return out, false, nil
}

// soapDocServicePath é o serviço SOAP de documentos do GED (move — sem rota
// REST: o PATCH /v2/documents/{id} recusa parentId com PatchException,
// validado ao vivo em 2026-08-07).
const soapDocServicePath = "/webdesk/ECMDocumentService"

// MoveGEDDocuments move documentos/pastas para outra pasta do GED (SOAP
// ECMDocumentService.moveDocument; sessão como credencial). ⚠️ Validado ao
// vivo (2026-08-07): o result tem semântica própria — "[OK] - <id>" no
// sucesso e "[NOK] - <mensagem>" na recusa (ex.: pasta destino inexistente).
func (c *Client) MoveGEDDocuments(ctx context.Context, ids []int, destFolderID int) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	colleague, err := c.ResolveUserCode(ctx)
	if err != nil {
		return err
	}
	reqBody, err := soap.BuildMoveDocument(c.opts.CompanyID, c.opts.Username, "", colleague, ids, destFolderID)
	if err != nil {
		return err
	}
	respBody, err := c.postSOAP(ctx, soapDocServicePath, "moveDocument", reqBody)
	if err != nil {
		return err
	}
	result, err := soap.ParseStringResult(respBody, "moveDocument")
	if err != nil {
		return err
	}
	msg := strings.TrimSpace(result)
	if msg == "" || strings.HasPrefix(msg, "[OK]") {
		return nil
	}
	return fmt.Errorf("%w: %s", errServerRejected, strings.TrimSpace(strings.TrimPrefix(msg, "[NOK] -")))
}
