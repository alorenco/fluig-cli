package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// WARUploadMode escolhe o envio do WAR de widget, layout e helper.
//
//   - WARUploadAuto (""): REST no Fluig 2.0 ou mais novo. Na versão 1.8.x ou
//     desconhecida, tenta a REST e, se a rota não existir, usa o servlet.
//   - WARUploadREST: só a REST (`.../product/uploadfile`).
//   - WARUploadServlet: só o servlet da Central de Componentes.
type WARUploadMode string

const (
	WARUploadAuto    WARUploadMode = ""
	WARUploadREST    WARUploadMode = "rest"
	WARUploadServlet WARUploadMode = "servlet"
)

// ParseWARUploadMode interpreta o valor da configuração ("", "auto", "rest",
// "servlet"). Valor desconhecido devolve erro.
func ParseWARUploadMode(v string) (WARUploadMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto":
		return WARUploadAuto, nil
	case "rest":
		return WARUploadREST, nil
	case "servlet":
		return WARUploadServlet, nil
	}
	return WARUploadAuto, fmt.Errorf("modo de envio de WAR inválido: %q (use auto, rest ou servlet)", v)
}

// widgetUploadPath é o endpoint REST de upload de WAR. Existe a partir do
// Fluig 1.8.2. No 1.8.1, a rota só existe na forma antiga, com usuário e senha
// no caminho (`.../uploadfile/{usuário}/{base64(senha)}`). A CLI não usa essa
// forma: a senha iria para a URL e para os logs de acesso (ROADMAP §5.24).
const widgetUploadPath = "/portal/api/rest/wcmservice/rest/product/uploadfile"

const widgetUploadDescription = "WCM Eclipse Plugin Deploy Artifact"

// applicationUploadPath é o servlet da Central de Componentes (página
// `applicationcenter`). Existe no 1.8.1 e no 2.0 e autentica pela sessão.
const applicationUploadPath = "/portal/p/api/servlet/ApplicationUpload"

// applicationUploadFileField é o nome do campo do arquivo no formulário da
// Central de Componentes.
const applicationUploadFileField = "com.datasul.technology.webdesk.upload.SaveFolder"

// currentUserPath devolve o usuário da sessão. O `content.id` é o `userId`
// numérico que o formulário da Central de Componentes envia.
const currentUserPath = "/api/public/2.0/users/getCurrent"

// UploadWidgetWAR publica um WAR (widget, layout ou helper). A instalação é
// assíncrona no servidor. O destino é identificado só pelo nome do arquivo.
// O modo de envio vem de Options.WARUpload (ver WARUploadMode).
func (c *Client) UploadWidgetWAR(ctx context.Context, warName string, war []byte) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	switch c.opts.WARUpload {
	case WARUploadREST:
		return c.uploadWARREST(ctx, warName, war)
	case WARUploadServlet:
		return c.uploadWARServlet(ctx, warName, war)
	}

	ver, verr := c.ServerVersion(ctx)
	if verr == nil && ver.AtLeast(2, 0) {
		return c.uploadWARREST(ctx, warName, war)
	}
	err := c.uploadWARREST(ctx, warName, war)
	if !restRouteMissing(err) {
		return err
	}
	c.logf("[war] a REST de upload respondeu %v (Fluig %s). Tentando o servlet ApplicationUpload da Central de Componentes.\n", err, ver)
	if serr := c.uploadWARServlet(ctx, warName, war); serr != nil {
		return fmt.Errorf("%w (o envio pela REST também falhou: %v)", serr, err)
	}
	return nil
}

// restRouteMissing informa se o erro da REST indica rota inexistente. No Fluig
// 1.8.1 o `FDNExceptionMapper` devolve a `NotFoundException` do RESTEasy como
// HTTP 500, não como 404. Por isso os dois status contam.
func restRouteMissing(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	return he.StatusCode == http.StatusNotFound || he.StatusCode == http.StatusInternalServerError
}

// logf escreve no log de requisições quando --verbose está ligado.
func (c *Client) logf(format string, args ...any) {
	if c.opts.Verbose && c.opts.LogWriter != nil {
		fmt.Fprintf(c.opts.LogWriter, format, args...)
	}
}

// uploadWARREST publica pelo endpoint REST. Resposta com campo "message" =
// rejeição.
func (c *Client) uploadWARREST(ctx context.Context, warName string, war []byte) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("fileName", warName); err != nil {
		return err
	}
	if err := mw.WriteField("fileDescription", widgetUploadDescription); err != nil {
		return err
	}
	part, err := mw.CreateFormFile("attachment", warName)
	if err != nil {
		return err
	}
	if _, err := part.Write(war); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(widgetUploadPath), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao publicar o WAR em %s: %w", c.base.Host, err)
	}
	respBody, err := readBody(resp, 1<<20)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{StatusCode: resp.StatusCode, URL: widgetUploadPath, Body: truncate(respBody, 512)}
	}
	// Sucesso = ausência de mensagem de erro no corpo.
	var parsed struct {
		Message json.RawMessage `json:"message"`
	}
	_ = json.Unmarshal([]byte(respBody), &parsed)
	if m := strings.TrimSpace(string(parsed.Message)); m != "" && m != "null" {
		return fmt.Errorf("%w: %s", errServerRejected, m)
	}
	return nil
}

// uploadWARServlet publica pelo servlet da Central de Componentes, com os
// mesmos campos do formulário da página `applicationcenter`. Medido na
// homologação Voyager 2.0 (2026-11-26, ROADMAP §5.24):
//   - sucesso: HTTP 200, text/html, `parent.WCMUploadComplete('<msg>','Info')`;
//   - arquivo que não é zip: HTTP 400 com JSON {"message":"<JavaError>..."};
//   - no 2.0 só o arquivo é obrigatório. A CLI manda todos os campos porque o
//     1.8.1 pode exigir os outros.
//
// O tipo `Question` pede confirmação para sobrescrever. A Central reenvia com
// forceDeploy=true, e a CLI faz o mesmo: as guardas de colisão já rodaram.
func (c *Client) uploadWARServlet(ctx context.Context, warName string, war []byte) error {
	fields := map[string]string{
		"tenantId": strconv.Itoa(c.opts.CompanyID),
	}
	if code := c.tenantCode(ctx); code != "" {
		fields["tenantCode"] = code
	}
	if id := c.currentUserID(ctx); id != "" {
		fields["userId"] = id
	}

	kind, msg, err := c.postApplicationUpload(ctx, warName, war, fields, false)
	if err != nil {
		return err
	}
	if kind == "Question" {
		c.logf("[war] o servidor pediu confirmação (%q). Reenviando com forceDeploy=true.\n", msg)
		kind, msg, err = c.postApplicationUpload(ctx, warName, war, fields, true)
		if err != nil {
			return err
		}
	}
	switch kind {
	case "Info":
		c.logf("[war] ApplicationUpload: %s\n", msg)
		return nil
	case "Question":
		return fmt.Errorf("%w: o servidor pediu confirmação mesmo com forceDeploy=true: %s", errServerRejected, msg)
	default:
		// Error, Warning ou tipo desconhecido. Warning também vira erro: sem
		// medição, não dá para afirmar que o deploy aconteceu.
		return fmt.Errorf("%w: %s (%s)", errServerRejected, msg, kind)
	}
}

// postApplicationUpload faz um envio ao servlet e devolve o tipo e a mensagem
// do `WCMUploadComplete`.
func (c *Client) postApplicationUpload(ctx context.Context, warName string, war []byte,
	fields map[string]string, force bool) (kind, msg string, err error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("forceDeploy", strconv.FormatBool(force)); err != nil {
		return "", "", err
	}
	part, err := mw.CreateFormFile(applicationUploadFileField, warName)
	if err != nil {
		return "", "", err
	}
	if _, err := part.Write(war); err != nil {
		return "", "", err
	}
	for _, k := range []string{"tenantId", "tenantCode", "userId"} {
		if v, ok := fields[k]; ok {
			if err := mw.WriteField(k, v); err != nil {
				return "", "", err
			}
		}
	}
	if err := mw.Close(); err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(applicationUploadPath), &body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("falha ao publicar o WAR em %s: %w", c.base.Host, err)
	}
	respBody, err := readBody(resp, 1<<20)
	if err != nil {
		return "", "", err
	}
	return parseApplicationUpload(resp.StatusCode, respBody)
}

// uploadCompleteRe acha a chamada `WCMUploadComplete('<mensagem>','<tipo>')`.
var uploadCompleteRe = regexp.MustCompile(`WCMUploadComplete\(\s*'((?:[^'\\]|\\.)*)'\s*,\s*'(\w*)'\s*\)`)

// htmlTagRe remove marcação das mensagens de erro (ex.: <JavaError>).
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// parseApplicationUpload interpreta a resposta do servlet ApplicationUpload.
func parseApplicationUpload(status int, body string) (kind, msg string, err error) {
	if status < 200 || status >= 300 {
		var parsed struct {
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(body), &parsed) == nil && strings.TrimSpace(parsed.Message) != "" {
			text := strings.TrimSpace(htmlTagRe.ReplaceAllString(parsed.Message, ""))
			return "", "", fmt.Errorf("%w: %s", errServerRejected, truncate(text, 512))
		}
		return "", "", &HTTPError{StatusCode: status, URL: applicationUploadPath, Body: truncate(body, 512)}
	}
	m := uploadCompleteRe.FindStringSubmatch(body)
	if m == nil {
		// Sessão perdida devolve a página de login com 200. Sem o
		// WCMUploadComplete, nada garante que o deploy aconteceu.
		return "", "", fmt.Errorf("resposta inesperada de %s (sem WCMUploadComplete): %s",
			applicationUploadPath, truncate(strings.TrimSpace(body), 256))
	}
	return m[2], unescapeJSString(m[1]), nil
}

// unescapeJSString desfaz os escapes de uma string JavaScript entre aspas
// simples (\' \" \\ \/ \n \t \uXXXX).
func unescapeJSString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'u':
			if i+4 < len(s) {
				if r, err := strconv.ParseUint(s[i+1:i+5], 16, 32); err == nil {
					b.WriteRune(rune(r))
					i += 4
					continue
				}
			}
			b.WriteString(`\u`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// currentUserID devolve o id numérico do usuário da sessão. Falha vira "" e o
// campo sai do envio (no 2.0 ele é opcional).
func (c *Client) currentUserID(ctx context.Context) string {
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url(currentUserPath), nil)
	if err != nil || status < 200 || status >= 300 {
		c.logf("[war] não obtive o id do usuário em %s (HTTP %d, %v)\n", currentUserPath, status, err)
		return ""
	}
	var parsed struct {
		Content struct {
			ID json.Number `json:"id"`
		} `json:"content"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return parsed.Content.ID.String()
}

// tenantCode descobre o código da empresa (ex.: "01") pelo redirecionamento do
// portal: `/portal/p/{companyId}/home` responde 302 para `/portal/p/{código}/home`.
// Vale para qualquer usuário, ao contrário do getTenants da Central, que pode
// exigir perfil de administrador. Falha devolve o companyId como texto.
func (c *Client) tenantCode(ctx context.Context) string {
	fallback := strconv.Itoa(c.opts.CompanyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.url("/portal/p/"+fallback+"/home"), nil)
	if err != nil {
		return fallback
	}
	noFollow := *c.http
	noFollow.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noFollow.Do(req)
	if err != nil {
		return fallback
	}
	_, _ = readBody(resp, 64<<10)
	if code := tenantCodeFromLocation(resp.Header.Get("Location")); code != "" {
		return code
	}
	c.logf("[war] não achei o código da empresa no redirecionamento (HTTP %d); usando %s\n", resp.StatusCode, fallback)
	return fallback
}

// tenantCodeFromLocation extrai o código de `/portal/p/{código}/...`.
func tenantCodeFromLocation(loc string) string {
	if loc == "" {
		return ""
	}
	if u, err := url.Parse(loc); err == nil {
		loc = u.Path
	}
	parts := strings.Split(strings.Trim(loc, "/"), "/")
	if len(parts) >= 3 && parts[0] == "portal" && parts[1] == "p" && parts[2] != "" && parts[2] != "api" {
		return parts[2]
	}
	return ""
}
