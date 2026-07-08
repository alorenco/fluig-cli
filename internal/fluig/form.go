package fluig

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

const soapCardIndexPath = "/webdesk/ECMCardIndexService"

// Persistence types de formulário.
const (
	PersistenceDB     = 1 // tabelas por formulário (recomendado)
	PersistenceSingle = 0 // tabela única
)

// Version options do update.
const (
	VersionKeep = "0" // mantém a versão atual
	VersionNew  = "2" // cria nova versão
)

// Form resume um formulário do servidor.
type Form struct {
	DocumentID      int    `json:"documentId"`
	Description     string `json:"description"`
	DatasetName     string `json:"datasetName"`
	Version         int    `json:"version"`
	CardDescription string `json:"cardDescription"`
}

// FormFile é um arquivo baixado de um formulário (conteúdo binário já decodificado).
type FormFile struct {
	Name    string
	Content []byte
}

// FormEvent é um evento de formulário (código JS).
type FormEvent struct {
	ID   string
	Code string
}

// ListForms lista os formulários do servidor (getCardIndexesWithoutApprover).
// colleagueID = userCode do servidor.
func (c *Client) ListForms(ctx context.Context, colleagueID string) ([]Form, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildListForms(c.opts.CompanyID, c.opts.Username, c.opts.Password, colleagueID)
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapCardIndexPath, "getCardIndexesWithoutApprover", reqBody)
	if err != nil {
		return nil, err
	}
	docs, err := soap.ParseListForms(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	out := make([]Form, 0, len(docs))
	for _, d := range docs {
		out = append(out, Form{
			DocumentID:      d.DocumentID,
			Description:     d.DocumentDescription,
			DatasetName:     d.DatasetName,
			Version:         d.Version,
			CardDescription: d.CardDescription,
		})
	}
	return out, nil
}

// FormAttachments lista os nomes dos arquivos de um formulário (getAttachmentsList).
func (c *Client) FormAttachments(ctx context.Context, documentID int) ([]string, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildAttachmentsList(c.opts.CompanyID, c.opts.Username, c.opts.Password, documentID)
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapCardIndexPath, "getAttachmentsList", reqBody)
	if err != nil {
		return nil, err
	}
	names, err := soap.ParseAttachmentsList(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	return names, nil
}

// DownloadFormFile baixa um arquivo do formulário e decodifica o base64.
func (c *Client) DownloadFormFile(ctx context.Context, documentID int, colleagueID string, version int, fileName string) (*FormFile, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildCardIndexContent(c.opts.CompanyID, c.opts.Username, c.opts.Password, documentID, colleagueID, version, fileName)
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapCardIndexPath, "getCardIndexContent", reqBody)
	if err != nil {
		return nil, err
	}
	b64, err := soap.ParseCardIndexContent(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	content, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("conteúdo base64 inválido de %q: %w", fileName, err)
	}
	return &FormFile{Name: fileName, Content: content}, nil
}

// FormEvents lista os eventos customizados de um formulário (getCustomizationEvents).
func (c *Client) FormEvents(ctx context.Context, documentID int) ([]FormEvent, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildCustomizationEvents(c.opts.CompanyID, c.opts.Username, c.opts.Password, documentID)
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapCardIndexPath, "getCustomizationEvents", reqBody)
	if err != nil {
		return nil, err
	}
	events, err := soap.ParseCustomizationEvents(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	out := make([]FormEvent, 0, len(events))
	for _, e := range events {
		out = append(out, FormEvent{ID: e.EventID, Code: e.EventDescription})
	}
	return out, nil
}

// FormUpload agrupa os anexos e eventos a subir num create/update.
// PrincipalFile é o nome do arquivo a marcar como principal (o HTML da página).
type FormUpload struct {
	Files         []FormFile
	Events        []FormEvent
	PrincipalFile string
}

// buildAttachments monta os anexos SOAP (base64), marcando o principal.
func buildAttachments(principalFile string, files []FormFile) []soap.Attachment {
	out := make([]soap.Attachment, 0, len(files))
	for _, f := range files {
		out = append(out, soap.Attachment{
			FileName:    f.Name,
			FileContent: base64.StdEncoding.EncodeToString(f.Content),
			Principal:   f.Name == principalFile,
		})
	}
	return out
}

// ChoosePrincipalFile decide qual arquivo é o principal (o HTML da página): se
// houver um único .htm/.html, é ele; com vários, o que casar com um dos
// candidatos (nome da pasta/do formulário); senão o primeiro HTML. "" se não há
// HTML.
func ChoosePrincipalFile(fileNames []string, candidates ...string) string {
	var htmls []string
	for _, n := range fileNames {
		l := strings.ToLower(n)
		if strings.HasSuffix(l, ".html") || strings.HasSuffix(l, ".htm") {
			htmls = append(htmls, n)
		}
	}
	switch len(htmls) {
	case 0:
		return ""
	case 1:
		return htmls[0]
	}
	for _, cand := range candidates {
		for _, h := range htmls {
			if baseNoExt(h) == cand {
				return h
			}
		}
	}
	return htmls[0]
}

func baseNoExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return name
}

func buildEvents(events []FormEvent) []soap.CardEvent {
	out := make([]soap.CardEvent, 0, len(events))
	for _, e := range events {
		out = append(out, soap.CardEvent{EventID: e.ID, EventDescription: e.Code, EventVersAnt: false})
	}
	return out
}

// CreateForm cria um formulário (createSimpleCardIndexWithDatasetPersisteType).
func (c *Client) CreateForm(ctx context.Context, publisherID, name, cardDescription, datasetName string, parentDocumentID, persistenceType int, up FormUpload) (*soap.WriteResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildCreateForm(soap.CreateFormParams{
		CompanyID: c.opts.CompanyID, Username: c.opts.Username, Password: c.opts.Password,
		PublisherID: publisherID, ParentDocumentID: parentDocumentID,
		DocumentDescription: name, CardDescription: cardDescription, DatasetName: datasetName,
		Attachments: buildAttachments(up.PrincipalFile, up.Files), CustomEvents: buildEvents(up.Events),
		PersistenceType: persistenceType,
	})
	if err != nil {
		return nil, err
	}
	return c.writeForm(ctx, "createSimpleCardIndexWithDatasetPersisteType", reqBody)
}

// UpdateForm atualiza um formulário (updateSimpleCardIndexWithDatasetAndGeneralInfo).
func (c *Client) UpdateForm(ctx context.Context, publisherID string, documentID int, name, cardDescription, datasetName, versionOption string, up FormUpload) (*soap.WriteResult, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildUpdateForm(soap.UpdateFormParams{
		CompanyID: c.opts.CompanyID, Username: c.opts.Username, Password: c.opts.Password,
		PublisherID: publisherID, DocumentID: documentID,
		CardDescription: cardDescription, DescriptionField: name, DatasetName: datasetName,
		Attachments: buildAttachments(up.PrincipalFile, up.Files), CustomEvents: buildEvents(up.Events),
		VersionOption: versionOption,
	})
	if err != nil {
		return nil, err
	}
	return c.writeForm(ctx, "updateSimpleCardIndexWithDatasetAndGeneralInfo", reqBody)
}

// writeForm faz o POST e checa o webServiceMessage == "ok".
func (c *Client) writeForm(ctx context.Context, action string, reqBody []byte) (*soap.WriteResult, error) {
	respBody, err := c.postSOAP(ctx, soapCardIndexPath, action, reqBody)
	if err != nil {
		return nil, err
	}
	res, err := soap.ParseWriteForm(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	if !strings.EqualFold(res.Message, "ok") {
		return res, fmt.Errorf("%w: %s", errServerRejected, res.Message)
	}
	return res, nil
}
