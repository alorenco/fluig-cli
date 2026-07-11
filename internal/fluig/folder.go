package fluig

import (
	"context"
	"sort"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

// soapFolderPath é o serviço SOAP de pastas do GED (ECMFolderService).
// Validado na homologação em 2026-07-11: autentica pela sessão (cookie),
// como os demais serviços SOAP; getRootFolders lista as pastas-raiz e
// getSubFolders as subpastas de um documentId.
const soapFolderPath = "/webdesk/ECMFolderService"

// GEDFolder é uma pasta do GED para o navegador do diálogo de publicação.
type GEDFolder struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ListGEDFolders lista as pastas do GED: parentID 0 = raízes; senão as
// subpastas da pasta dada. Resultado ordenado por nome.
func (c *Client) ListGEDFolders(ctx context.Context, parentID int) ([]GEDFolder, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	colleague, err := c.ResolveUserCode(ctx)
	if err != nil {
		return nil, err
	}
	var (
		reqBody []byte
		action  string
	)
	if parentID <= 0 {
		action = "getRootFolders"
		reqBody, err = soap.BuildGetRootFolders(c.opts.CompanyID, c.opts.Username, c.opts.Password, colleague)
	} else {
		action = "getSubFolders"
		reqBody, err = soap.BuildGetSubFolders(c.opts.CompanyID, c.opts.Username, c.opts.Password, parentID, colleague)
	}
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapFolderPath, action, reqBody)
	if err != nil {
		return nil, err
	}
	items, err := soap.ParseFolders(respBody)
	if err != nil {
		return nil, err
	}
	out := make([]GEDFolder, 0, len(items))
	for _, it := range items {
		if it.DocumentID == 0 {
			continue
		}
		out = append(out, GEDFolder{ID: it.DocumentID, Name: it.Description})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
