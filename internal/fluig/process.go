package fluig

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// API REST v2 de process-management (validada na homologação em 2026-07-09).
// A resposta é paginada no envelope {items, hasNext}; o parâmetro fields não
// funciona neste endpoint (devolve itens vazios) e expand=versions multiplica
// o payload por ~25× — a listagem usa a resposta enxuta padrão.
const restProcessesPath = "/process-management/api/v2/processes"

// ProcessSummary é um processo listado pela API de process-management.
type ProcessSummary struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
	Active      bool   `json:"active"`
	Public      bool   `json:"public"`
}

// ListProcesses retorna todos os processos do servidor, percorrendo as páginas
// até hasNext=false. Processos sem categoria vêm sem a chave categoryId
// (observado na homologação: FLUIGADHOCPROCESS) — Category fica vazia.
func (c *Client) ListProcesses(ctx context.Context) ([]ProcessSummary, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []ProcessSummary
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url(restProcessesPath) + "?" + q.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: restProcessesPath, Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items []struct {
				ProcessID          string `json:"processId"`
				ProcessDescription string `json:"processDescription"`
				CategoryID         string `json:"categoryId"`
				Active             bool   `json:"active"`
				Public             bool   `json:"public"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de %s: %w", restProcessesPath, err)
		}
		for _, it := range parsed.Items {
			out = append(out, ProcessSummary{
				ID:          it.ProcessID,
				Description: it.ProcessDescription,
				Category:    it.CategoryID,
				Active:      it.Active,
				Public:      it.Public,
			})
		}
		// Página vazia encerra mesmo com hasNext=true — defesa contra loop.
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// --- publish nativo: export/import/release de versão de processo ---
//
// Semântica validada na homologação (2026-07-09):
//   - o export/xml devolve SÓ a última versão, em UTF-8, raiz <list> (diferente
//     do zip SOAP, que vem em ISO-8859-1);
//   - TODO import cria uma versão nova, em edição (as versões da PK no corpo
//     são renumeradas pelo servidor);
//   - release?=true no import NÃO é atômico: se a liberação falha, a versão
//     fica criada mesmo assim — por isso o publish libera pelo endpoint
//     dedicado, para distinguir "import falhou" de "liberação falhou";
//   - o release da última versão desativa a anterior; o withdraw reverte.

// ProcessVersion é uma versão de processo (REST v2 process-versions).
// FormID é o documentId do formulário (cardIndex) vinculado à versão — 0
// quando a versão não usa formulário.
type ProcessVersion struct {
	Version int  `json:"version"`
	Active  bool `json:"active"`
	Editing bool `json:"editing"`
	FormID  int  `json:"formId"`
}

// ProcessVersions lista as versões de um processo, da API REST v2.
func (c *Client) ProcessVersions(ctx context.Context, processID string) ([]ProcessVersion, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []ProcessVersion
	for page := 1; ; page++ {
		endpoint := c.url(processPath(processID, "/process-versions")) +
			"?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, processAPIError(status, body, processID, "process-versions")
		}
		var parsed struct {
			Items   []ProcessVersion `json:"items"`
			HasNext bool             `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de process-versions: %w", err)
		}
		out = append(out, parsed.Items...)
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// --- simulação de contexto do `fluigcli dev`: estados e vínculo com formulário ---
//
// Schemas conferidos no swagger real do process-management da homologação
// (2026-07-10). ⚠️ Validação viva pendente: o servidor estava fora do ar no
// dia — refazer o gate (fixtures reais + integração) quando voltar.

// ProcessState é uma etapa (estado) de uma versão de processo. O número que
// os eventos de formulário enxergam em WKNumState é o Sequence.
type ProcessState struct {
	Sequence    int    `json:"sequence"`
	Name        string `json:"stateName"`
	Description string `json:"stateDescription"`
	StateType   string `json:"stateType"`
	BpmnType    string `json:"bpmnType"`
}

// ProcessStates lista os estados de uma versão do processo, em ordem de
// sequence (a API não garante ordem entre páginas).
func (c *Client) ProcessStates(ctx context.Context, processID string, version int) ([]ProcessState, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	base := processPath(processID, "/process-versions/"+strconv.Itoa(version)+"/states")
	var out []ProcessState
	for page := 1; ; page++ {
		endpoint := c.url(base) + "?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, processAPIError(status, body, processID, "states")
		}
		var parsed struct {
			Items   []ProcessState `json:"items"`
			HasNext bool           `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de states: %w", err)
		}
		out = append(out, parsed.Items...)
		if !parsed.HasNext || len(parsed.Items) == 0 {
			sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
			return out, nil
		}
	}
}

// ProcessFormLink relaciona um processo ao formulário procurado.
type ProcessFormLink struct {
	ProcessID   string `json:"processId"`
	Description string `json:"description"`
	Version     int    `json:"version"` // maior versão do processo cujo formId casa
}

// FindProcessesByFormID varre os processos do servidor (expand=versions, uma
// requisição por página — payload grande, mas único) e devolve os que têm
// alguma versão vinculada ao formulário (documentId do cardIndex). É o
// caminho de auto-detecção do processo de um formulário local.
func (c *Client) FindProcessesByFormID(ctx context.Context, formID int) ([]ProcessFormLink, error) {
	if formID <= 0 {
		return nil, fmt.Errorf("formId inválido: %d", formID)
	}
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	const pageSize = 100
	var out []ProcessFormLink
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("expand", "versions")
		q.Set("page", strconv.Itoa(page))
		q.Set("pageSize", strconv.Itoa(pageSize))
		endpoint := c.url(restProcessesPath) + "?" + q.Encode()
		body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, &HTTPError{StatusCode: status, URL: restProcessesPath, Body: truncate(string(body), 512)}
		}
		var parsed struct {
			Items []struct {
				ProcessID          string           `json:"processId"`
				ProcessDescription string           `json:"processDescription"`
				Versions           []ProcessVersion `json:"versions"`
			} `json:"items"`
			HasNext bool `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de %s: %w", restProcessesPath, err)
		}
		for _, it := range parsed.Items {
			best := 0
			for _, v := range it.Versions {
				if v.FormID == formID && v.Version > best {
					best = v.Version
				}
			}
			if best > 0 {
				out = append(out, ProcessFormLink{ProcessID: it.ProcessID, Description: it.ProcessDescription, Version: best})
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// LatestProcessVersion devolve o maior número de versão (0 = sem versões).
func LatestProcessVersion(versions []ProcessVersion) int {
	max := 0
	for _, v := range versions {
		if v.Version > max {
			max = v.Version
		}
	}
	return max
}

// ExportProcessXML baixa o XML de configuração da última versão do processo.
func (c *Client) ExportProcessXML(ctx context.Context, processID string) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doRaw(ctx, http.MethodGet, c.url(processPath(processID, "/export/xml")), nil, "", "application/xml")
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, processAPIError(status, body, processID, "export/xml")
	}
	return body, nil
}

// ImportProcessXML importa o XML no processo, criando uma versão nova em edição.
func (c *Client) ImportProcessXML(ctx context.Context, processID string, xmlData []byte) error {
	return c.importProcessXML(ctx, processPath(processID, "/import/xml"), processID, xmlData)
}

// ImportNewProcessXML cria um processo novo a partir do XML.
func (c *Client) ImportNewProcessXML(ctx context.Context, processID string, xmlData []byte) error {
	path := restProcessesPath + "/import/xml?processId=" + url.QueryEscape(processID)
	return c.importProcessXML(ctx, path, processID, xmlData)
}

func (c *Client) importProcessXML(ctx context.Context, path, processID string, xmlData []byte) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	body, status, err := c.doRaw(ctx, http.MethodPost, c.url(path), xmlData, "application/xml", "application/json")
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return processAPIError(status, body, processID, "import/xml")
	}
	return nil
}

// ReleaseLatestProcessVersion libera a última versão do processo para uso
// (desativa a versão anteriormente liberada).
func (c *Client) ReleaseLatestProcessVersion(ctx context.Context, processID string) error {
	return c.postProcessVersionOp(ctx, processID, "/process-versions/latest/release")
}

// WithdrawLatestProcessVersion retira a liberação da última versão.
func (c *Client) WithdrawLatestProcessVersion(ctx context.Context, processID string) error {
	return c.postProcessVersionOp(ctx, processID, "/process-versions/latest/withdraw")
}

// DeleteLatestProcessVersion remove a última versão do processo.
func (c *Client) DeleteLatestProcessVersion(ctx context.Context, processID string) error {
	return c.processOp(ctx, http.MethodDelete, processID, "/process-versions/latest")
}

// DeleteProcess remove o processo (precisa ter uma única versão, não liberada).
func (c *Client) DeleteProcess(ctx context.Context, processID string) error {
	return c.processOp(ctx, http.MethodDelete, processID, "")
}

func (c *Client) postProcessVersionOp(ctx context.Context, processID, op string) error {
	return c.processOp(ctx, http.MethodPost, processID, op)
}

func (c *Client) processOp(ctx context.Context, method, processID, op string) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	body, status, err := c.doJSON(ctx, method, c.url(processPath(processID, op)), nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return processAPIError(status, body, processID, strings.TrimPrefix(op, "/"))
	}
	return nil
}

// processPath monta o caminho REST de um processo (o id pode ter espaço/acento).
func processPath(processID, suffix string) string {
	return restProcessesPath + "/" + url.PathEscape(processID) + suffix
}

// processAPIError interpreta o erro da API de process-management: negócio vem
// como {"code":"BPM...Exception","message":"..."} (HTTP 400); 404 = inexistente.
func processAPIError(status int, body []byte, processID, op string) error {
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: processo %q", ErrNotFound, processID)
	}
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Message != "" {
		return fmt.Errorf("%w: %s", errServerRejected, parsed.Message)
	}
	return &HTTPError{StatusCode: status, URL: op, Body: truncate(string(body), 512)}
}

// doRaw faz uma requisição com Content-Type/Accept arbitrários e cookies do jar.
func (c *Client) doRaw(ctx context.Context, method, endpoint string, body []byte, ctype, accept string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("falha ao chamar %s: %w", c.base.Host, err)
	}
	respBody, err := readBody(resp, 32<<20)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return []byte(respBody), resp.StatusCode, nil
}

// ApplyProcessEventScripts substitui, no XML exportado do processo, o código
// (eventDescription) dos eventos presentes em scripts (evento → código novo).
// Devolve o XML atualizado, os eventos efetivamente atualizados e os que não
// existem no processo — o publish NÃO cria eventos (isso é papel do Fluig
// Studio), então quem chama decide falhar antes de importar.
func ApplyProcessEventScripts(xmlData []byte, scripts map[string]string) (out []byte, updated []string, missing []string) {
	const (
		openEvent = "<WorkflowProcessEvent>"
		openID    = "<eventId>"
		closeID   = "</eventId>"
		openDesc  = "<eventDescription>"
		closeDesc = "</eventDescription>"
		emptyDesc = "<eventDescription/>"
	)
	data := string(xmlData)
	var buf strings.Builder
	buf.Grow(len(data))
	seen := map[string]bool{}
	rest := data
	for {
		blockStart := strings.Index(rest, openEvent)
		if blockStart < 0 {
			buf.WriteString(rest)
			break
		}
		blockEnd := strings.Index(rest[blockStart:], "</WorkflowProcessEvent>")
		if blockEnd < 0 {
			buf.WriteString(rest)
			break
		}
		blockEnd += blockStart
		block := rest[blockStart:blockEnd]

		id := ""
		if i := strings.Index(block, openID); i >= 0 {
			if j := strings.Index(block[i:], closeID); j >= 0 {
				id = strings.TrimSpace(block[i+len(openID) : i+j])
			}
		}
		if code, ok := scripts[id]; ok && id != "" {
			seen[id] = true
			var esc bytes.Buffer
			_ = xml.EscapeText(&esc, []byte(code))
			switch {
			case strings.Contains(block, openDesc):
				i := strings.Index(block, openDesc)
				j := strings.Index(block[i:], closeDesc)
				if j >= 0 {
					block = block[:i+len(openDesc)] + esc.String() + block[i+j:]
				}
			case strings.Contains(block, emptyDesc):
				block = strings.Replace(block, emptyDesc, openDesc+esc.String()+closeDesc, 1)
			}
		}
		buf.WriteString(rest[:blockStart])
		buf.WriteString(block)
		rest = rest[blockEnd:]
	}
	for id := range scripts {
		if seen[id] {
			updated = append(updated, id)
		} else {
			missing = append(missing, id)
		}
	}
	sort.Strings(updated)
	sort.Strings(missing)
	return []byte(buf.String()), updated, missing
}
