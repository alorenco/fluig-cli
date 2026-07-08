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

// --- getDataset ---

// Constraint é um filtro de getDataset (searchConstraintDto).
type Constraint struct {
	FieldName    string `xml:"fieldName"`
	InitialValue string `xml:"initialValue"`
	FinalValue   string `xml:"finalValue"`
	LikeSearch   bool   `xml:"likeSearch"`
}

type getDatasetReq struct {
	XMLName     xml.Name     `xml:"ws:getDataset"`
	CompanyID   int64        `xml:"companyId"`
	Username    string       `xml:"username"`
	Password    string       `xml:"password"`
	Name        string       `xml:"name"`
	Fields      []string     `xml:"fields>item,omitempty"`
	Constraints []Constraint `xml:"constraints>item,omitempty"`
	Order       []string     `xml:"order>item,omitempty"`
}

type value struct {
	Nil     string `xml:"nil,attr"`
	Content string `xml:",chardata"`
}

type row struct {
	Values []value `xml:"value"`
}

type getDatasetRespEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Columns []string `xml:"Body>getDatasetResponse>return>columns"`
	Values  []row    `xml:"Body>getDatasetResponse>return>values"`
	// Variação de nome da part de retorno.
	ColumnsAlt []string `xml:"Body>getDatasetResponse>dataset>columns"`
	ValuesAlt  []row    `xml:"Body>getDatasetResponse>dataset>values"`
	Fault      *Fault   `xml:"Body>Fault"`
}

// DatasetResult é o resultado tabular de getDataset. Cada linha em Rows tem o
// mesmo comprimento de Columns; valores nil viram nil.
type DatasetResult struct {
	Columns []string
	Rows    [][]*string
}

// BuildGetDataset monta o envelope de getDataset.
func BuildGetDataset(companyID int64, username, password, name string, fields []string, constraints []Constraint, order []string) ([]byte, error) {
	return marshalEnvelope(NSDataset, getDatasetReq{
		CompanyID: companyID, Username: username, Password: password,
		Name: name, Fields: fields, Constraints: constraints, Order: order,
	})
}

// ParseGetDataset interpreta a resposta de getDataset.
func ParseGetDataset(body []byte) (*DatasetResult, error) {
	var env getDatasetRespEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("resposta SOAP inválida de getDataset: %w", err)
	}
	if env.Fault != nil {
		return nil, env.Fault
	}
	cols, rows := env.Columns, env.Values
	if len(cols) == 0 && len(env.ColumnsAlt) > 0 {
		cols, rows = env.ColumnsAlt, env.ValuesAlt
	}
	res := &DatasetResult{Columns: cols}
	for _, r := range rows {
		line := make([]*string, len(r.Values))
		for i, v := range r.Values {
			if v.Nil == "true" {
				line[i] = nil
				continue
			}
			s := v.Content
			line[i] = &s
		}
		res.Rows = append(res.Rows, line)
	}
	return res, nil
}
