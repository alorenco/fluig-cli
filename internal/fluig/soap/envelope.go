// Package soap monta e interpreta os envelopes SOAP dos serviços do Fluig.
// É puro (encoding/xml, sem HTTP nem cobra); o cliente em internal/fluig faz o
// POST. Estilo RPC/literal, confirmado pelos WSDLs em testdata/.
package soap

import "encoding/xml"

// Namespaces (targetNamespace) dos serviços SOAP do Fluig.
const (
	NSDataset   = "http://ws.dataservice.ecm.technology.totvs.com/"
	NSCardIndex = "http://ws.dm.ecm.technology.totvs.com/"
)

const nsSoapEnv = "http://schemas.xmlsoap.org/soap/envelope/"

// envelope embrulha o corpo SOAP. Os prefixos são emitidos literalmente
// (padrão para clientes SOAP em Go); o servidor casa por prefixo + xmlns.
type envelope struct {
	XMLName      xml.Name `xml:"soapenv:Envelope"`
	XMLNSSoapenv string   `xml:"xmlns:soapenv,attr"`
	XMLNSWs      string   `xml:"xmlns:ws,attr"`
	Body         envBody
}

type envBody struct {
	XMLName xml.Name `xml:"soapenv:Body"`
	Inner   any
}

// marshalEnvelope serializa um corpo (com XMLName no namespace ws) num envelope
// SOAP completo, declarando o namespace ws indicado.
func marshalEnvelope(ns string, inner any) ([]byte, error) {
	env := envelope{XMLNSSoapenv: nsSoapEnv, XMLNSWs: ns, Body: envBody{Inner: inner}}
	out, err := xml.Marshal(env)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

// Fault é um erro SOAP (soap:Fault) devolvido pelo servidor.
type Fault struct {
	Code   string `xml:"faultcode"`
	String string `xml:"faultstring"`
}

func (f *Fault) Error() string {
	if f.String != "" {
		return f.String
	}
	return f.Code
}
