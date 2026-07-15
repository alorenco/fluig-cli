package soap

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// NSFoundation é o targetNamespace do ECMColleagueReplacementService
// (substituição de usuário / colleague replacement). WSDL em
// testdata/ECMColleagueReplacementService.wsdl.
const NSFoundation = "http://ws.foundation.ecm.technology.totvs.com/"

// ColleagueReplacement é o DTO de substituição de usuário. ColleagueID é o
// TITULAR (quem será substituído) e ReplacementID o SUBSTITUTO — ambos userCode
// (não login; login devolve resultado nulo na homologação). StartDate/FinalDate
// são xs:dateTime já formatados (o cliente formata a partir da data informada).
// ViewWorkflowTasks/ViewGEDTasks definem o que o substituto enxerga/assume.
type ColleagueReplacement struct {
	ColleagueID       string
	ReplacementID     string
	CompanyID         int
	StartDate         string
	FinalDate         string
	ViewWorkflowTasks bool
	ViewGEDTasks      bool
}

// replDTOXML espelha a ordem do sequence do WSDL (colleagueReplacementDto). O
// nome do elemento é "colleagueReplacement" (o nome da part da mensagem RPC).
type replDTOXML struct {
	XMLName           xml.Name `xml:"colleagueReplacement"`
	ColleagueID       string   `xml:"colleagueId"`
	CompanyID         int      `xml:"companyId"`
	ReplacementID     string   `xml:"replacementId"`
	FinalDate         string   `xml:"validationFinalDate,omitempty"`
	StartDate         string   `xml:"validationStartDate,omitempty"`
	ViewGEDTasks      bool     `xml:"viewGEDTasks"`
	ViewWorkflowTasks bool     `xml:"viewWorkflowTasks"`
}

func (r ColleagueReplacement) toXML() replDTOXML {
	return replDTOXML{
		ColleagueID: r.ColleagueID, CompanyID: r.CompanyID, ReplacementID: r.ReplacementID,
		FinalDate: r.FinalDate, StartDate: r.StartDate,
		ViewGEDTasks: r.ViewGEDTasks, ViewWorkflowTasks: r.ViewWorkflowTasks,
	}
}

// --- create / update (mesma assinatura; só muda o nome do elemento raiz) ---

type replWriteReq struct {
	XMLName     xml.Name
	Username    string     `xml:"username"`
	Password    string     `xml:"password"`
	CompanyID   int        `xml:"companyId"`
	Replacement replDTOXML // XMLName colleagueReplacement
}

// BuildCreateReplacement monta o envelope de createColleagueReplacement.
func BuildCreateReplacement(username, password string, r ColleagueReplacement) ([]byte, error) {
	return buildReplWrite("ws:createColleagueReplacement", username, password, r)
}

// BuildUpdateReplacement monta o envelope de updateColleagueReplacement (o
// servidor substitui os campos do par colleagueId+replacementId pelos enviados).
func BuildUpdateReplacement(username, password string, r ColleagueReplacement) ([]byte, error) {
	return buildReplWrite("ws:updateColleagueReplacement", username, password, r)
}

func buildReplWrite(op, username, password string, r ColleagueReplacement) ([]byte, error) {
	req := replWriteReq{
		XMLName:     xml.Name{Local: op},
		Username:    username,
		Password:    password,
		CompanyID:   r.CompanyID,
		Replacement: r.toXML(),
	}
	return marshalEnvelope(NSFoundation, req)
}

// --- delete ---

type replDeleteReq struct {
	XMLName       xml.Name `xml:"ws:deleteColleagueReplacement"`
	Username      string   `xml:"username"`
	Password      string   `xml:"password"`
	CompanyID     int      `xml:"companyId"`
	ColleagueID   string   `xml:"colleagueId"`
	ReplacementID string   `xml:"replacementId"`
}

// BuildDeleteReplacement monta o envelope de deleteColleagueReplacement.
func BuildDeleteReplacement(username, password string, companyID int, colleagueID, replacementID string) ([]byte, error) {
	return marshalEnvelope(NSFoundation, replDeleteReq{
		XMLName: xml.Name{Local: "ws:deleteColleagueReplacement"},
		Username: username, Password: password, CompanyID: companyID,
		ColleagueID: colleagueID, ReplacementID: replacementID,
	})
}

// --- get (getReplacementsOfUser / getValidReplacementsOfUser) ---

type replGetOfUserReq struct {
	XMLName     xml.Name
	Username    string `xml:"username"`
	Password    string `xml:"password"`
	CompanyID   int    `xml:"companyId"`
	ColleagueID string `xml:"colleagueId"`
}

// BuildGetReplacementsOfUser lista TODAS as substituições de um usuário (inclui
// vigências expiradas). valid=true usa getValidReplacementsOfUser, que devolve
// só as vigentes na data atual.
func BuildGetReplacementsOfUser(username, password string, companyID int, colleagueID string, valid bool) ([]byte, error) {
	op := "ws:getReplacementsOfUser"
	if valid {
		op = "ws:getValidReplacementsOfUser"
	}
	return marshalEnvelope(NSFoundation, replGetOfUserReq{
		XMLName: xml.Name{Local: op},
		Username: username, Password: password, CompanyID: companyID, ColleagueID: colleagueID,
	})
}

// --- parsing ---

// replStatusResp captura o <result> textual de create/update/delete. O wrapper
// da resposta muda por operação (…Response), então usamos ",any" para casar
// qualquer elemento filho do Body que não seja o Fault.
type replStatusResp struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Fault *Fault `xml:"Fault"`
		Resp  struct {
			Result string `xml:"result"`
		} `xml:",any"`
	} `xml:"Body"`
}

// ParseReplacementStatus devolve o texto do <result> de create/update/delete. O
// serviço NÃO usa soap:Fault para erro de negócio: devolve "OK" em caso de
// sucesso ou "NOK <mensagem>" na recusa (ex.: substituição duplicada, par
// inexistente no delete). O chamador decide o tipo de erro pela mensagem.
func ParseReplacementStatus(body []byte) (string, error) {
	var env replStatusResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("resposta SOAP inválida de colleagueReplacement: %w", err)
	}
	if env.Body.Fault != nil {
		return "", env.Body.Fault
	}
	return strings.TrimSpace(env.Body.Resp.Result), nil
}

type replItemXML struct {
	ColleagueID       string `xml:"colleagueId"`
	CompanyID         int    `xml:"companyId"`
	ReplacementID     string `xml:"replacementId"`
	FinalDate         string `xml:"validationFinalDate"`
	StartDate         string `xml:"validationStartDate"`
	ViewGEDTasks      bool   `xml:"viewGEDTasks"`
	ViewWorkflowTasks bool   `xml:"viewWorkflowTasks"`
}

type replListResp struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		Fault *Fault `xml:"Fault"`
		Resp  struct {
			Items []replItemXML `xml:"result>item"`
		} `xml:",any"`
	} `xml:"Body"`
}

// ParseReplacementList interpreta getReplacementsOfUser/getValidReplacementsOfUser.
// ⚠️ Lista vazia NÃO vem como array vazio: o serviço retorna null e o JBoss
// responde a falha WS-I BP R2211 ("RPC/Literal parts cannot be null") — tratada
// aqui como zero substituições (não é erro), igual ao exportProcessInZipFormat.
func ParseReplacementList(body []byte) ([]ColleagueReplacement, error) {
	var env replListResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de getReplacementsOfUser: %w", err)
	}
	if env.Body.Fault != nil {
		if strings.Contains(env.Body.Fault.Error(), "RPC/Literal parts cannot be null") {
			return nil, nil
		}
		return nil, env.Body.Fault
	}
	out := make([]ColleagueReplacement, 0, len(env.Body.Resp.Items))
	for _, it := range env.Body.Resp.Items {
		out = append(out, ColleagueReplacement{
			ColleagueID:       it.ColleagueID,
			ReplacementID:     it.ReplacementID,
			CompanyID:         it.CompanyID,
			StartDate:         it.StartDate,
			FinalDate:         it.FinalDate,
			ViewWorkflowTasks: it.ViewWorkflowTasks,
			ViewGEDTasks:      it.ViewGEDTasks,
		})
	}
	return out, nil
}
