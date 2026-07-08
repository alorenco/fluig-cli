package fluig

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

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
