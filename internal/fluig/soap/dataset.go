package soap

import (
	"encoding/xml"
	"fmt"
)

// --- findAllFormulariesDatasets ---

type findAllReq struct {
	XMLName   xml.Name `xml:"ws:findAllFormulariesDatasets"`
	CompanyID int64    `xml:"companyId"`
	Username  string   `xml:"username"`
	Password  string   `xml:"password"`
}

// FormDataset é um item de findAllFormulariesDatasets. Type distingue CUSTOM.
type FormDataset struct {
	CompanyID  int64  `xml:"companyId"`
	DatasetID  string `xml:"datasetId"`
	DocumentID int    `xml:"documentId"`
	Type       string `xml:"type"`
	Version    int    `xml:"version"`
}

type findAllRespEnvelope struct {
	XMLName xml.Name      `xml:"Envelope"`
	Items   []FormDataset `xml:"Body>findAllFormulariesDatasetsResponse>return>item"`
	// Algumas versões nomeiam a part de retorno "dataset" em vez de "return".
	ItemsAlt []FormDataset `xml:"Body>findAllFormulariesDatasetsResponse>dataset>item"`
	Fault    *Fault        `xml:"Body>Fault"`
}

// BuildFindAllDatasets monta o envelope de findAllFormulariesDatasets.
func BuildFindAllDatasets(companyID int64, username, password string) ([]byte, error) {
	return marshalEnvelope(NSDataset, findAllReq{CompanyID: companyID, Username: username, Password: password})
}

// ParseFindAllDatasets interpreta a resposta, devolvendo os datasets ou o Fault.
func ParseFindAllDatasets(body []byte) ([]FormDataset, error) {
	var env findAllRespEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de findAllFormulariesDatasets: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	if len(env.Items) > 0 {
		return env.Items, nil
	}
	return env.ItemsAlt, nil
}

// O suporte a getDataset (consulta de valores) foi removido em 2026-07-09: a
// operação respondia EOF na homologação (nunca funcionou de fato — a fixture
// construída do WSDL jamais foi confirmada) e o `dataset query` migrou para a
// REST v2 `dataset-handle/search`. findAllFormulariesDatasets permanece como
// fallback do `dataset list` para servidores sem a REST v2.
