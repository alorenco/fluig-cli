package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Import de layouts WCM via fluigcliHelper (rotas /api/layouts, a partir do
// helper 0.12.0).
//
// Por que passa pelo helper: a API nativa (`GET /page-management/api/v2/layouts`)
// não informa o arquivo .war, e ele nem sempre é `<código>.war` — medido na
// homologação em 2026-10-01: `kit_layout` mora em `wcm-layout-kit.war`
// (coluna FILE_NAME da wcm_application). O WAR em `appserver/apps` é a fonte
// enviada mais uma linha `application.tenant.code=` que o servidor acrescenta
// ao application.info.

const helperLayoutsPath = "/" + HelperFluigcli + "/api/layouts"

// helperHasLayoutAPI: as rotas /layouts existem a partir do 0.12.0.
func helperHasLayoutAPI(version string) bool { return helperAtLeast(version, 0, 12) }

// LayoutPackage é um layout listado pelo fluigcliHelper, com o arquivo .war
// necessário ao download. Difere de Layout (API nativa), que não traz o arquivo.
type LayoutPackage struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Filename    string `json:"filename"`
}

// ListLayoutsHelper lista os layouts customizados via fluigcliHelper. Helper
// ausente → ErrHelperMissing; helper anterior ao 0.12.0 (rota inexistente,
// 404) → ErrHelperOutdated.
func (c *Client) ListLayoutsHelper(ctx context.Context) ([]LayoutPackage, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(helperLayoutsPath), nil)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusNotFound:
		// O helper respondeu ao ping, logo está instalado: 404 aqui é a rota
		// que ainda não existe.
		return nil, ErrHelperOutdated
	case status != http.StatusOK:
		return nil, &HTTPError{StatusCode: status, URL: HelperFluigcli + "/layouts", Body: truncate(string(body), 512)}
	}
	var layouts []LayoutPackage
	if err := json.Unmarshal(body, &layouts); err != nil {
		return nil, fmt.Errorf("resposta inesperada de %s/layouts: %w", HelperFluigcli, err)
	}
	if layouts == nil {
		layouts = []LayoutPackage{}
	}
	return layouts, nil
}

// DownloadLayout baixa o .war de um layout via fluigcliHelper. O 404 é
// ambíguo — arquivo inexistente OU helper sem a rota (< 0.12.0) — e a versão
// do helper decide, como nas rotas /db.
func (c *Client) DownloadLayout(ctx context.Context, filename string) ([]byte, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	war, status, err := c.downloadWAR(ctx, c.url(helperLayoutsPath+"/")+url.PathEscape(filename), HelperFluigcli+"/layouts/"+filename)
	if status == http.StatusNotFound {
		if info, e := c.HelperStatus(ctx); e == nil && info.Installed && !helperHasLayoutAPI(info.Version) {
			return nil, ErrHelperOutdated
		}
		return nil, fmt.Errorf("%w: layout %q", ErrNotFound, filename)
	}
	return war, err
}

// downloadWAR faz o GET de um pacote binário do helper. Devolve o status para
// quem chama decidir o 404 (widget e layout tratam diferente).
//
// ⚠️ A rota produz octet-stream e o RESTEasy exige Accept compatível —
// Accept: application/json responde 406 (mesmo padrão do stream de
// documentos; visto ao vivo na homolog em 2026-07-18).
func (c *Client) downloadWAR(ctx context.Context, endpoint, label string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("falha ao baixar %s de %s: %w", label, c.base.Host, err)
	}
	body, err := readBody(resp, 256<<20)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &HTTPError{StatusCode: resp.StatusCode, URL: label, Body: truncate(body, 256)}
	}
	return []byte(body), resp.StatusCode, nil
}
