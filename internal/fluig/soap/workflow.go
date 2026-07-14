package soap

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
)

// NSWorkflow é o targetNamespace do ECMWorkflowEngineService.
const NSWorkflow = "http://ws.workflow.ecm.technology.totvs.com/"

// --- getWorkFlowProcessVersion ---

type wfVersionReq struct {
	XMLName   xml.Name `xml:"ws:getWorkFlowProcessVersion"`
	Username  string   `xml:"username"`
	Password  string   `xml:"password"`
	CompanyID int      `xml:"companyId"`
	ProcessID string   `xml:"processId"`
}

type wfVersionResp struct {
	XMLName xml.Name `xml:"Envelope"`
	Result  int      `xml:"Body>getWorkFlowProcessVersionResponse>result"`
	Fault   *Fault   `xml:"Body>Fault"`
}

func BuildWorkflowVersion(companyID int, username, password, processID string) ([]byte, error) {
	return marshalEnvelope(NSWorkflow, wfVersionReq{
		Username: username, Password: password, CompanyID: companyID, ProcessID: processID,
	})
}

// ParseWorkflowVersion devolve a versão do processo (0 = inexistente).
func ParseWorkflowVersion(body []byte) (int, error) {
	var env wfVersionResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("resposta SOAP inválida de getWorkFlowProcessVersion: %w", err)
	}
	if env.Fault != nil {
		return 0, env.Fault
	}
	return env.Result, nil
}

// --- exportProcessInZipFormat ---

type wfExportReq struct {
	XMLName   xml.Name `xml:"ws:exportProcessInZipFormat"`
	Username  string   `xml:"username"`
	Password  string   `xml:"password"`
	CompanyID int      `xml:"companyId"`
	ProcessID string   `xml:"processId"`
}

type wfExportResp struct {
	XMLName xml.Name `xml:"Envelope"`
	Result  string   `xml:"Body>exportProcessInZipFormatResponse>result"`
	Fault   *Fault   `xml:"Body>Fault"`
}

func BuildExportProcessZip(companyID int, username, password, processID string) ([]byte, error) {
	return marshalEnvelope(NSWorkflow, wfExportReq{
		Username: username, Password: password, CompanyID: companyID, ProcessID: processID,
	})
}

// ParseExportProcessZip devolve o zip do processo em base64 (bruto).
func ParseExportProcessZip(body []byte) (string, error) {
	var env wfExportResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("resposta SOAP inválida de exportProcessInZipFormat: %w", err)
	}
	if env.Fault != nil {
		return "", env.Fault
	}
	return env.Result, nil
}

// --- startProcess ---

// StartAttachment é um anexo enviado no startProcess.
type StartAttachment struct {
	FileName string
	Content  []byte
}

// wfStartReq segue a assinatura RPC/literal do startProcess (WSDL em
// testdata/ECMWorkflowEngineService.wsdl): arrays usam elementos <item>; o
// anexo é um processAttachmentDto com os arquivos em elementos <attachments>
// (tipo attachment, repetido).
type wfStartReq struct {
	XMLName      xml.Name          `xml:"ws:startProcess"`
	Username     string            `xml:"username"`
	Password     string            `xml:"password"`
	CompanyID    int               `xml:"companyId"`
	ProcessID    string            `xml:"processId"`
	ChoosedState int               `xml:"choosedState"`
	ColleagueIDs wfStringArray     `xml:"colleagueIds"`
	Comments     string            `xml:"comments"`
	UserID       string            `xml:"userId"`
	CompleteTask bool              `xml:"completeTask"`
	Attachments  wfAttachmentArray `xml:"attachments"`
	CardData     wfCardData        `xml:"cardData"`
	Appointment  struct{}          `xml:"appointment"`
	ManagerMode  bool              `xml:"managerMode"`
}

type wfStringArray struct {
	Items []string `xml:"item"`
}

type wfCardData struct {
	Items []wfStringArray `xml:"item"`
}

type wfAttachmentArray struct {
	Items []wfProcessAttachment `xml:"item"`
}

type wfProcessAttachment struct {
	AttachmentSequence int          `xml:"attachmentSequence"`
	Attachments        []wfFileItem `xml:"attachments"`
	CompanyID          int          `xml:"companyId"`
	Deleted            bool         `xml:"deleted"`
	Description        string       `xml:"description"`
	FileName           string       `xml:"fileName"`
	NewAttach          bool         `xml:"newAttach"`
	ProcessInstanceID  int          `xml:"processInstanceId"`
	Size               float64      `xml:"size"`
}

type wfFileItem struct {
	Attach      bool   `xml:"attach"`
	Editing     bool   `xml:"editing"`
	FileName    string `xml:"fileName"`
	FileSize    int64  `xml:"fileSize"`
	FileContent string `xml:"filecontent"` // base64 (o encoding/xml não converte []byte sozinho)
	Principal   bool   `xml:"principal"`
}

// BuildStartProcess monta o envelope do startProcess. userID/assigneeIDs são
// userCodes (colleagueId), não logins. cardData = pares campo/valor.
func BuildStartProcess(companyID int, username, password, processID string, choosedState int,
	assigneeIDs []string, comments, userID string, completeTask bool,
	attachments []StartAttachment, cardData map[string]string, cardOrder []string) ([]byte, error) {
	req := wfStartReq{
		Username: username, Password: password, CompanyID: companyID,
		ProcessID: processID, ChoosedState: choosedState,
		ColleagueIDs: wfStringArray{Items: assigneeIDs},
		Comments:     comments, UserID: userID, CompleteTask: completeTask,
		ManagerMode: false,
	}
	for i, a := range attachments {
		req.Attachments.Items = append(req.Attachments.Items, wfProcessAttachment{
			AttachmentSequence: i + 1,
			Attachments: []wfFileItem{{
				Attach: true, FileName: a.FileName,
				FileSize:    int64(len(a.Content)),
				FileContent: base64.StdEncoding.EncodeToString(a.Content),
			}},
			CompanyID: companyID, Description: a.FileName, FileName: a.FileName,
			NewAttach: true, Size: float64(len(a.Content)),
		})
	}
	for _, k := range cardOrder {
		req.CardData.Items = append(req.CardData.Items, wfStringArray{Items: []string{k, cardData[k]}})
	}
	return marshalEnvelope(NSWorkflow, req)
}

type wfStartResp struct {
	XMLName xml.Name        `xml:"Envelope"`
	Items   []wfStringArray `xml:"Body>startProcessResponse>result>item"`
	Fault   *Fault          `xml:"Body>Fault"`
}

// ParseStartProcess devolve os pares chave/valor do resultado (ex.:
// iProcess = número da solicitação criada).
func ParseStartProcess(body []byte) (map[string]string, error) {
	var env wfStartResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de startProcess: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	out := make(map[string]string, len(env.Items))
	for _, it := range env.Items {
		if len(it.Items) >= 2 {
			out[it.Items[0]] = it.Items[1]
		} else if len(it.Items) == 1 {
			out[it.Items[0]] = ""
		}
	}
	return out, nil
}
