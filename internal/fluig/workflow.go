package fluig

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

const soapWorkflowPath = "/webdesk/ECMWorkflowEngineService"

// WorkflowVersion devolve a última versão de um processo (0 = inexistente).
// Nativo do ECMWorkflowEngineService — não depende da fluiggersWidget.
func (c *Client) WorkflowVersion(ctx context.Context, processID string) (int, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return 0, err
	}
	reqBody, err := soap.BuildWorkflowVersion(c.opts.CompanyID, c.opts.Username, c.opts.Password, processID)
	if err != nil {
		return 0, err
	}
	respBody, err := c.postSOAP(ctx, soapWorkflowPath, "getWorkFlowProcessVersion", reqBody)
	if err != nil {
		return 0, err
	}
	v, err := soap.ParseWorkflowVersion(respBody)
	if err != nil {
		return 0, mapSOAPError(err)
	}
	return v, nil
}

// ExportProcessZip baixa o processo inteiro como zip (diagrama + scripts).
func (c *Client) ExportProcessZip(ctx context.Context, processID string) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildExportProcessZip(c.opts.CompanyID, c.opts.Username, c.opts.Password, processID)
	if err != nil {
		return nil, err
	}
	// Quirk do WSDL: exportProcessInZipFormat compartilha soapAction "exportProcess"
	// com exportProcess; o servidor despacha pelo elemento do corpo. O header
	// SOAPAction precisa ser "exportProcess" (não o nome da operação).
	respBody, err := c.postSOAP(ctx, soapWorkflowPath, "exportProcess", reqBody)
	if err != nil {
		return nil, err
	}
	b64, err := soap.ParseExportProcessZip(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return nil, fmt.Errorf("%w: processo %q (export vazio)", ErrNotFound, processID)
	}
	zip, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("zip base64 inválido do processo %q: %w", processID, err)
	}
	return zip, nil
}

// ProcessEventScripts devolve os scripts de eventos de um processo como mapa
// evento → código, extraídos do export nativo (§5.7 da SPEC: o zip traz um
// único XML <ProcessDefinition>, com os scripts em <WorkflowProcessEvent>).
// Leitura pura — não requer a fluiggersWidget. Quando o XML traz eventos de
// mais de uma versão do processo, prevalece a versão mais alta.
func (c *Client) ProcessEventScripts(ctx context.Context, processID string) (map[string]string, error) {
	zipData, err := c.ExportProcessZip(ctx, processID)
	if err != nil {
		return nil, err
	}
	events, err := parseProcessEventScripts(zipData)
	if err != nil {
		return nil, fmt.Errorf("processo %q: %w", processID, err)
	}
	return events, nil
}

// parseProcessEventScripts abre o zip do export e extrai os eventos do XML de
// definição (o primeiro .xml do pacote).
func parseProcessEventScripts(zipData []byte) (map[string]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("zip de processo inválido: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		return parseProcessDefinitionEvents(data)
	}
	return nil, errors.New("zip de processo sem o XML de definição")
}

// parseProcessDefinitionEvents varre o XML <ProcessDefinition> atrás dos
// elementos <WorkflowProcessEvent> (eventId na PK + eventDescription = código).
// A varredura é tolerante a namespace e caixa — o XML é gerado pelo servidor e
// não temos um schema publicado.
func parseProcessDefinitionEvents(data []byte) (map[string]string, error) {
	// O servidor exporta o XML em ISO-8859-1 (validado na homologação: bytes
	// fora de UTF-8 quebravam o decoder). Normaliza para UTF-8 antes do parse;
	// como Latin-1 mapeia byte a byte para os mesmos code points, a conversão
	// é sem perdas. Depois disso o CharsetReader só precisa aceitar o rótulo
	// declarado (ex.: encoding="ISO-8859-1") sem reconverter nada.
	if !utf8.Valid(data) {
		data = latin1ToUTF8(data)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	byVersion := map[int]map[string]string{}
	maxVersion := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML de definição inválido: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || !strings.EqualFold(se.Name.Local, "workflowProcessEvent") {
			continue
		}
		id, code, version, err := decodeProcessEvent(dec)
		if err != nil {
			return nil, fmt.Errorf("evento de processo inválido no XML: %w", err)
		}
		if id == "" {
			continue
		}
		if byVersion[version] == nil {
			byVersion[version] = map[string]string{}
		}
		byVersion[version][id] = code
		if version > maxVersion {
			maxVersion = version
		}
	}
	events := byVersion[maxVersion]
	if events == nil {
		events = map[string]string{}
	}
	return events, nil
}

// latin1ToUTF8 reencoda ISO-8859-1 → UTF-8 (cada byte é o próprio code point).
func latin1ToUTF8(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/8)
	for _, c := range b {
		out.WriteRune(rune(c))
	}
	return out.Bytes()
}

// decodeProcessEvent consome o subárvore de um <WorkflowProcessEvent> já
// aberto, coletando eventId, eventDescription e version (da PK) por nome de
// elemento, sem depender da ordem nem da caixa exata.
func decodeProcessEvent(dec *xml.Decoder) (id, code string, version int, err error) {
	var (
		stack      []string
		idB, codeB strings.Builder
		verSet     bool
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", "", 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) == 0 { // fechou o próprio WorkflowProcessEvent
				return strings.TrimSpace(idB.String()), codeB.String(), version, nil
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			switch leaf := stack[len(stack)-1]; {
			case strings.EqualFold(leaf, "eventId"):
				idB.Write(t)
			case strings.EqualFold(leaf, "eventDescription"):
				codeB.Write(t)
			case strings.EqualFold(leaf, "version") && !verSet:
				if v, convErr := strconv.Atoi(strings.TrimSpace(string(t))); convErr == nil {
					version, verSet = v, true
				}
			}
		}
	}
}
