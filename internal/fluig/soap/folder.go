package soap

import (
	"encoding/xml"
	"fmt"
)

// Operações do ECMFolderService (mesmo namespace dm dos demais serviços —
// validado na homologação em 2026-07-11: getRootFolders/getSubFolders
// autenticam pela sessão, senha em branco). Usadas pelo navegador de pastas
// do GED no diálogo de publicação do `fluigcli dev`.

// FolderItem é uma pasta do GED (documentDto enxuto — só o que o seletor usa).
type FolderItem struct {
	DocumentID  int    `xml:"documentId"`
	Description string `xml:"documentDescription"`
}

type rootFoldersReq struct {
	XMLName     xml.Name `xml:"ws:getRootFolders"`
	Username    string   `xml:"username"`
	Password    string   `xml:"password"`
	CompanyID   int      `xml:"companyId"`
	ColleagueID string   `xml:"colleagueId"`
}

type subFoldersReq struct {
	XMLName     xml.Name `xml:"ws:getSubFolders"`
	Username    string   `xml:"username"`
	Password    string   `xml:"password"`
	CompanyID   int      `xml:"companyId"`
	DocumentID  int      `xml:"documentId"`
	ColleagueID string   `xml:"colleagueId"`
}

type foldersResp struct {
	XMLName xml.Name     `xml:"Envelope"`
	Root    []FolderItem `xml:"Body>getRootFoldersResponse>Document>item"`
	Sub     []FolderItem `xml:"Body>getSubFoldersResponse>Document>item"`
	Fault   *Fault       `xml:"Body>Fault"`
}

func BuildGetRootFolders(companyID int, username, password, colleagueID string) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, rootFoldersReq{
		Username: username, Password: password, CompanyID: companyID, ColleagueID: colleagueID,
	})
}

func BuildGetSubFolders(companyID int, username, password string, documentID int, colleagueID string) ([]byte, error) {
	return marshalEnvelope(NSCardIndex, subFoldersReq{
		Username: username, Password: password, CompanyID: companyID,
		DocumentID: documentID, ColleagueID: colleagueID,
	})
}

// ParseFolders lê a resposta de getRootFolders ou getSubFolders.
func ParseFolders(body []byte) ([]FolderItem, error) {
	var env foldersResp
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida do ECMFolderService: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	if len(env.Root) > 0 {
		return env.Root, nil
	}
	return env.Sub, nil
}
