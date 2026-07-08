package soap

import (
	"encoding/xml"
	"fmt"
)

// Operações do ECMCardIndexService (RPC/literal, ns NSCardIndex). Ordem dos
// parâmetros conforme o WSDL em testdata/ECMCardIndexService.wsdl.

// Document é um formulário listado por getCardIndexesWithoutApprover.
type Document struct {
	DocumentID          int    `xml:"documentId"`
	DocumentDescription string `xml:"documentDescription"`
	DatasetName         string `xml:"datasetName"`
	Version             int    `xml:"version"`
	CardDescription     string `xml:"cardDescription"`
}

// Attachment é um anexo de formulário (filecontent em base64).
type Attachment struct {
	FileName    string `xml:"fileName"`
	FileContent string `xml:"filecontent"`
	Principal   bool   `xml:"principal"`
}

// CardEvent é um evento de formulário (eventDescription = código JS).
type CardEvent struct {
	EventID          string `xml:"eventId"`
	EventDescription string `xml:"eventDescription"`
	EventVersAnt     bool   `xml:"eventVersAnt"`
}

// --- getCardIndexesWithoutApprover ---

type listFormsReq struct {
	XMLName   xml.Name `xml:"ws:getCardIndexesWithoutApprover"`
	Username  string   `xml:"username"`
	Password  string   `xml:"password"`
	CompanyID int      `xml:"companyId"`
	Colleague string   `xml:"colleagueId"`
}

type listFormsResp struct {
	XMLName xml.Name   `xml:"Envelope"`
	Items   []Document `xml:"Body>getCardIndexesWithoutApproverResponse>result>item"`
	Fault   *Fault     `xml:"Body>Fault"`
}

func BuildListForms(companyID int, username, password, colleagueID string) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, listFormsReq{
		Username: username, Password: password, CompanyID: companyID, Colleague: colleagueID,
	})
}

func ParseListForms(body []byte) ([]Document, error) {
	var env listFormsResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de getCardIndexesWithoutApprover: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	return env.Items, nil
}

// --- getAttachmentsList ---

type attachListReq struct {
	XMLName    xml.Name `xml:"ws:getAttachmentsList"`
	Username   string   `xml:"username"`
	Password   string   `xml:"password"`
	CompanyID  int      `xml:"companyId"`
	DocumentID int      `xml:"documentId"`
}

type attachListResp struct {
	XMLName xml.Name `xml:"Envelope"`
	Items   []string `xml:"Body>getAttachmentsListResponse>result>item"`
	Fault   *Fault   `xml:"Body>Fault"`
}

func BuildAttachmentsList(companyID int, username, password string, documentID int) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, attachListReq{
		Username: username, Password: password, CompanyID: companyID, DocumentID: documentID,
	})
}

func ParseAttachmentsList(body []byte) ([]string, error) {
	var env attachListResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de getAttachmentsList: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	return env.Items, nil
}

// --- getCardIndexContent ---

type cardContentReq struct {
	XMLName    xml.Name `xml:"ws:getCardIndexContent"`
	Username   string   `xml:"username"`
	Password   string   `xml:"password"`
	CompanyID  int      `xml:"companyId"`
	DocumentID int      `xml:"documentId"`
	Colleague  string   `xml:"colleagueId"`
	Version    int      `xml:"version"`
	FileName   string   `xml:"nomeArquivo"`
}

type cardContentResp struct {
	XMLName xml.Name `xml:"Envelope"`
	Folder  string   `xml:"Body>getCardIndexContentResponse>folder"`
	Fault   *Fault   `xml:"Body>Fault"`
}

func BuildCardIndexContent(companyID int, username, password string, documentID int, colleagueID string, version int, fileName string) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, cardContentReq{
		Username: username, Password: password, CompanyID: companyID,
		DocumentID: documentID, Colleague: colleagueID, Version: version, FileName: fileName,
	})
}

// ParseCardIndexContent devolve o conteúdo do arquivo em base64 (bruto).
func ParseCardIndexContent(body []byte) (string, error) {
	var env cardContentResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("resposta SOAP inválida de getCardIndexContent: %w", err)
	}
	if env.Fault != nil {
		return "", env.Fault
	}
	return env.Folder, nil
}

// --- getCustomizationEvents ---

type custEventsReq struct {
	XMLName    xml.Name `xml:"ws:getCustomizationEvents"`
	Username   string   `xml:"username"`
	Password   string   `xml:"password"`
	CompanyID  int      `xml:"companyId"`
	DocumentID int      `xml:"documentId"`
}

type custEventsResp struct {
	XMLName xml.Name    `xml:"Envelope"`
	Items   []CardEvent `xml:"Body>getCustomizationEventsResponse>result>item"`
	Fault   *Fault      `xml:"Body>Fault"`
}

func BuildCustomizationEvents(companyID int, username, password string, documentID int) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, custEventsReq{
		Username: username, Password: password, CompanyID: companyID, DocumentID: documentID,
	})
}

func ParseCustomizationEvents(body []byte) ([]CardEvent, error) {
	var env custEventsResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de getCustomizationEvents: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	return env.Items, nil
}

// --- createSimpleCardIndexWithDatasetPersisteType ---

type createFormReq struct {
	XMLName             xml.Name     `xml:"ws:createSimpleCardIndexWithDatasetPersisteType"`
	Username            string       `xml:"username"`
	Password            string       `xml:"password"`
	CompanyID           int          `xml:"companyId"`
	ParentDocumentID    int          `xml:"parentDocumentId"`
	PublisherID         string       `xml:"publisherId"`
	DocumentDescription string       `xml:"documentDescription"`
	CardDescription     string       `xml:"cardDescription"`
	DatasetName         string       `xml:"datasetName"`
	Attachments         []Attachment `xml:"Attachments>item"`
	CustomEvents        []CardEvent  `xml:"customEvents>item"`
	PersistenceType     int          `xml:"persistenceType"`
}

// CreateFormParams agrupa os parâmetros de criação de formulário.
type CreateFormParams struct {
	CompanyID           int
	Username            string
	Password            string
	PublisherID         string
	ParentDocumentID    int
	DocumentDescription string
	CardDescription     string
	DatasetName         string
	Attachments         []Attachment
	CustomEvents        []CardEvent
	PersistenceType     int
}

func BuildCreateForm(p CreateFormParams) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, createFormReq{
		Username: p.Username, Password: p.Password, CompanyID: p.CompanyID,
		ParentDocumentID: p.ParentDocumentID, PublisherID: p.PublisherID,
		DocumentDescription: p.DocumentDescription, CardDescription: p.CardDescription,
		DatasetName: p.DatasetName, Attachments: p.Attachments, CustomEvents: p.CustomEvents,
		PersistenceType: p.PersistenceType,
	})
}

// --- updateSimpleCardIndexWithDatasetAndGeneralInfo ---

type updateFormReq struct {
	XMLName          xml.Name     `xml:"ws:updateSimpleCardIndexWithDatasetAndGeneralInfo"`
	Username         string       `xml:"username"`
	Password         string       `xml:"password"`
	CompanyID        int          `xml:"companyId"`
	DocumentID       int          `xml:"documentId"`
	PublisherID      string       `xml:"publisherId"`
	CardDescription  string       `xml:"cardDescription"`
	DescriptionField string       `xml:"descriptionField"`
	DatasetName      string       `xml:"datasetName"`
	Attachments      []Attachment `xml:"Attachments>item"`
	CustomEvents     []CardEvent  `xml:"customEvents>item"`
	GeneralInfo      generalInfo  `xml:"generalInfo"`
}

type generalInfo struct {
	VersionOption string `xml:"versionOption"`
}

// UpdateFormParams agrupa os parâmetros de atualização de formulário.
type UpdateFormParams struct {
	CompanyID        int
	Username         string
	Password         string
	PublisherID      string
	DocumentID       int
	CardDescription  string
	DescriptionField string
	DatasetName      string
	Attachments      []Attachment
	CustomEvents     []CardEvent
	VersionOption    string // "0" mantém a versão, "2" cria nova
}

func BuildUpdateForm(p UpdateFormParams) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, updateFormReq{
		Username: p.Username, Password: p.Password, CompanyID: p.CompanyID,
		DocumentID: p.DocumentID, PublisherID: p.PublisherID,
		CardDescription: p.CardDescription, DescriptionField: p.DescriptionField,
		DatasetName: p.DatasetName, Attachments: p.Attachments, CustomEvents: p.CustomEvents,
		GeneralInfo: generalInfo{VersionOption: p.VersionOption},
	})
}

// WriteResult é o resultado de create/update (webServiceMessage).
type WriteResult struct {
	Message    string `xml:"webServiceMessage"`
	DocumentID int    `xml:"documentId"`
}

type writeFormResp struct {
	XMLName   xml.Name      `xml:"Envelope"`
	CreateRes []WriteResult `xml:"Body>createSimpleCardIndexWithDatasetPersisteTypeResponse>result>item"`
	UpdateRes []WriteResult `xml:"Body>updateSimpleCardIndexWithDatasetAndGeneralInfoResponse>result>item"`
	Fault     *Fault        `xml:"Body>Fault"`
}

// ParseWriteForm interpreta a resposta de create/update: sucesso quando o
// webServiceMessage do primeiro item é "ok".
func ParseWriteForm(body []byte) (*WriteResult, error) {
	var env writeFormResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de create/update de formulário: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	results := env.CreateRes
	if len(results) == 0 {
		results = env.UpdateRes
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("resposta sem resultado")
	}
	return &results[0], nil
}
