package soap

import (
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
