package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// serverVersionPath é o endpoint nativo do Fluig que devolve a versão do
// produto: {"value":"TOTVS Fluig Plataforma - Voyager 2.0.0-260707"}. Autentica
// pela sessão (o gate de /api/public/* responde 401 sem cookie) e não exige
// privilégio admin. Validado nas duas gerações (Fluig 1.8 e 2.0).
const serverVersionPath = "/api/public/wcm/version"

// ServerVersion identifica a versão do produto Fluig do servidor. Raw guarda a
// string bruta devolvida pelo servidor (ex.: "1.8.1-..."); Major/Minor são a
// forma comparável extraída dela (0 quando não foi possível interpretar).
type ServerVersion struct {
	Raw   string `json:"raw"`
	Major int    `json:"major"`
	Minor int    `json:"minor"`
}

// AtLeast informa se a versão é >= major.minor. Versão desconhecida (Major==0)
// é tratada como ANTIGA (retorna false para qualquer alvo >= 1) — o caminho de
// compatibilidade é sempre o mais defensivo.
func (v ServerVersion) AtLeast(major, minor int) bool {
	if v.Major == 0 {
		return major == 0 && minor == 0
	}
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// String devolve a forma legível (o raw, ou "desconhecida").
func (v ServerVersion) String() string {
	if v.Raw == "" {
		return "desconhecida"
	}
	return v.Raw
}

// versionNumberRe captura o "MAJOR.MINOR" inicial de uma string de versão
// (ex.: "1.8.1-20240101" → 1, 8).
var versionNumberRe = regexp.MustCompile(`(\d+)\.(\d+)`)

// parseServerVersion extrai a string de versão do corpo (JSON ou texto puro) e
// dela a forma comparável. O endpoint devolve formatos diferentes entre versões
// do Fluig, então tolera JSON com vários nomes de campo e também texto cru.
func parseServerVersion(body []byte) ServerVersion {
	raw := extractVersionString(body)
	v := ServerVersion{Raw: raw}
	if m := versionNumberRe.FindStringSubmatch(raw); m != nil {
		v.Major, _ = strconv.Atoi(m[1])
		v.Minor, _ = strconv.Atoi(m[2])
	}
	return v
}

// extractVersionString acha a string de versão no corpo, seja ele um JSON com
// campo de versão, uma string JSON solta ou texto puro.
func extractVersionString(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	// JSON objeto: procura um campo de versão por nome conhecido.
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) == nil {
		for _, key := range []string{"version", "serverVersion", "productVersion", "buildVersion", "value"} {
			for k, raw := range obj {
				if !strings.EqualFold(k, key) {
					continue
				}
				var s string
				if json.Unmarshal(raw, &s) == nil && s != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	// String JSON solta ("1.8.1").
	var s string
	if json.Unmarshal(body, &s) == nil && s != "" {
		return strings.TrimSpace(s)
	}
	// Texto puro (linha única).
	if !strings.ContainsAny(trimmed, "{}\n") {
		return trimmed
	}
	return ""
}

// ServerVersion consulta a versão do produto do servidor, cacheada no Client
// (paga uma vez por execução). Erro de rede/sessão é propagado; um corpo
// inesperado vira uma ServerVersion com Raw vazio (Major=0 = desconhecida), sem
// erro — quem chama decide o fallback.
func (c *Client) ServerVersion(ctx context.Context) (ServerVersion, error) {
	if c.serverVersion != nil {
		return *c.serverVersion, nil
	}
	if err := c.EnsureSession(ctx); err != nil {
		return ServerVersion{}, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(serverVersionPath), nil)
	if err != nil {
		return ServerVersion{}, err
	}
	if c.opts.Verbose && c.opts.LogWriter != nil {
		fmt.Fprintf(c.opts.LogWriter, "[version] %s → HTTP %d: %s\n", serverVersionPath, status, truncate(string(body), 256))
	}
	if status < 200 || status >= 300 {
		return ServerVersion{}, restRequestError("api/servers/local/version", status, body)
	}
	v := parseServerVersion(body)
	c.serverVersion = &v
	return v, nil
}
