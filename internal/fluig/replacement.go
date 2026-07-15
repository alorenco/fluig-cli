package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

const (
	restUserReplacements = "/process-management/api/v2/user-replacements"
	soapReplacementPath  = "/webdesk/ECMColleagueReplacementService"
	// replDateLayout formata a data SEM offset de fuso: as substituições são
	// dia-a-dia e o servidor interpreta a data no próprio fuso (enviar um
	// offset diferente do servidor deslocaria o dia). Validado na homologação.
	replDateLayout = "2006-01-02T15:04:05"
)

// Replacement é uma substituição de usuário (delegação de tarefas). User é o
// TITULAR (quem será substituído) e ReplacedBy o SUBSTITUTO. As flags de escopo
// (WorkflowTasks/GEDTasks) só vêm do SOAP (getReplacementsOfUser) — o `list`
// via REST não as expõe, por isso são ponteiros (nil = desconhecido). A
// justificativa só vem do REST.
type Replacement struct {
	User          *RequestUser `json:"user,omitempty"`
	ReplacedBy    *RequestUser `json:"replacedBy,omitempty"`
	StartDate     *time.Time   `json:"startDate,omitempty"`
	EndDate       *time.Time   `json:"endDate,omitempty"`
	Justification string       `json:"justification,omitempty"`
	WorkflowTasks *bool        `json:"workflowTasks,omitempty"`
	GEDTasks      *bool        `json:"gedTasks,omitempty"`
}

// ReplacementInput são os campos de uma nova substituição (create).
type ReplacementInput struct {
	Start         time.Time
	End           time.Time
	WorkflowTasks bool
	GEDTasks      bool
}

// ReplacementChanges são as alterações de um update (nil = não mexe no campo).
type ReplacementChanges struct {
	Start         *time.Time
	End           *time.Time
	WorkflowTasks *bool
	GEDTasks      *bool
}

// ReplacementFilter parametriza a listagem (REST v2). Os filtros são LOGINS
// (resolvidos para userCode internamente — a API compara pelo código; login
// inexistente vira ErrNotFound, não filtro silenciosamente ignorado).
type ReplacementFilter struct {
	UserLogin       string // titular
	ReplacedByLogin string // substituto
	Limit           int
}

// --- leitura ---

type replBPMUser struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Mail  string `json:"mail"`
	Login string `json:"login"`
}

func (u *replBPMUser) toRequestUser() *RequestUser {
	if u == nil {
		return nil
	}
	return &RequestUser{Name: u.Name, Login: u.Login, Code: u.Code}
}

type replRestItem struct {
	StartAt        string       `json:"startAt"`
	EndAt          string       `json:"endAt"`
	Justify        string       `json:"replacementJustify"`
	User           *replBPMUser `json:"user"`
	ReplacedByUser *replBPMUser `json:"replacedByUser"`
}

// ListReplacements lista as substituições de usuário (REST v2 user-replacements;
// paginado; expande titular e substituto para trazer nome/login). Os filtros são
// logins (resolvidos para userCode). Requer privilégio administrativo.
func (c *Client) ListReplacements(ctx context.Context, f ReplacementFilter) ([]Replacement, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	var userCode, replacedByCode string
	if f.UserLogin != "" {
		u, err := c.resolveReplacementUser(ctx, f.UserLogin)
		if err != nil {
			return nil, err
		}
		userCode = u.Code
	}
	if f.ReplacedByLogin != "" {
		u, err := c.resolveReplacementUser(ctx, f.ReplacedByLogin)
		if err != nil {
			return nil, err
		}
		replacedByCode = u.Code
	}
	return c.listReplacementsByCode(ctx, userCode, replacedByCode, f.Limit)
}

// listReplacementsByCode é o loop REST propriamente dito (códigos já resolvidos).
func (c *Client) listReplacementsByCode(ctx context.Context, userCode, replacedByCode string, limit int) ([]Replacement, error) {
	const pageSize = 100
	var out []Replacement
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("pageSize", strconv.Itoa(pageSize))
		// ⚠️ expand só funciona REPETIDO (com vírgula o servidor ignora em silêncio).
		params.Add("expand", "user")
		params.Add("expand", "replacedByUser")
		if userCode != "" {
			params.Set("userCode", userCode)
		}
		if replacedByCode != "" {
			params.Set("replacedByUserCode", replacedByCode)
		}
		body, status, err := c.doJSON(ctx, http.MethodGet, c.url(restUserReplacements)+"?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, restRequestError("v2/user-replacements", status, body)
		}
		var parsed struct {
			Items   []replRestItem `json:"items"`
			HasNext bool           `json:"hasNext"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("resposta inesperada de v2/user-replacements: %w", err)
		}
		for _, it := range parsed.Items {
			out = append(out, Replacement{
				User:          it.User.toRequestUser(),
				ReplacedBy:    it.ReplacedByUser.toRequestUser(),
				StartDate:     requestTime(it.StartAt),
				EndDate:       requestTime(it.EndAt),
				Justification: it.Justify,
			})
			if limit > 0 && len(out) >= limit {
				return out[:limit], nil
			}
		}
		if !parsed.HasNext || len(parsed.Items) == 0 {
			return out, nil
		}
	}
}

// getUserReplacementsRaw devolve as substituições de um usuário direto do SOAP
// (com as flags de escopo e as datas originais). valid=true traz só as vigentes.
func (c *Client) getUserReplacementsRaw(ctx context.Context, colleagueCode string, valid bool) ([]soap.ColleagueReplacement, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildGetReplacementsOfUser(c.opts.Username, c.opts.Password, c.opts.CompanyID, colleagueCode, valid)
	if err != nil {
		return nil, err
	}
	action := "getReplacementsOfUser"
	if valid {
		action = "getValidReplacementsOfUser"
	}
	respBody, err := c.postSOAP(ctx, soapReplacementPath, action, reqBody)
	if err != nil {
		return nil, err
	}
	list, err := soap.ParseReplacementList(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	return list, nil
}

// GetUserReplacements devolve as substituições de um usuário (por login),
// incluindo as flags de escopo (SOAP). Enriquece o nome do substituto com o
// `list` REST quando disponível. valid=true traz só as vigentes hoje.
func (c *Client) GetUserReplacements(ctx context.Context, login string, valid bool) ([]Replacement, error) {
	titular, err := c.resolveReplacementUser(ctx, login)
	if err != nil {
		return nil, err
	}
	raw, err := c.getUserReplacementsRaw(ctx, titular.Code, valid)
	if err != nil {
		return nil, err
	}
	// Enriquecimento: o SOAP só devolve o userCode do substituto; o REST traz
	// nome/login. Falha do REST não é fatal (mostramos o código).
	nameByCode := map[string]*RequestUser{}
	if rest, rerr := c.listReplacementsByCode(ctx, titular.Code, "", 0); rerr == nil {
		for _, r := range rest {
			if r.ReplacedBy != nil {
				nameByCode[r.ReplacedBy.Code] = r.ReplacedBy
			}
		}
	}
	out := make([]Replacement, 0, len(raw))
	for _, d := range raw {
		sub := nameByCode[d.ReplacementID]
		if sub == nil {
			sub = &RequestUser{Code: d.ReplacementID}
		}
		wf, ged := d.ViewWorkflowTasks, d.ViewGEDTasks
		out = append(out, Replacement{
			User:          titular,
			ReplacedBy:    sub,
			StartDate:     requestTime(d.StartDate),
			EndDate:       requestTime(d.FinalDate),
			WorkflowTasks: &wf,
			GEDTasks:      &ged,
		})
	}
	return out, nil
}

// --- escrita ---

// CreateReplacement define um substituto para um usuário (SOAP
// createColleagueReplacement). titularLogin/subLogin são logins (resolvidos para
// userCode). Substituição duplicada (mesmo par e período) = errServerRejected.
func (c *Client) CreateReplacement(ctx context.Context, titularLogin, subLogin string, in ReplacementInput) (*Replacement, error) {
	titular, sub, err := c.resolveReplacementPair(ctx, titularLogin, subLogin)
	if err != nil {
		return nil, err
	}
	dto := soap.ColleagueReplacement{
		ColleagueID:       titular.Code,
		ReplacementID:     sub.Code,
		CompanyID:         c.opts.CompanyID,
		StartDate:         in.Start.Format(replDateLayout),
		FinalDate:         in.End.Format(replDateLayout),
		ViewWorkflowTasks: in.WorkflowTasks,
		ViewGEDTasks:      in.GEDTasks,
	}
	reqBody, err := soap.BuildCreateReplacement(c.opts.Username, c.opts.Password, dto)
	if err != nil {
		return nil, err
	}
	if err := c.doReplacementWrite(ctx, "createColleagueReplacement", reqBody); err != nil {
		return nil, err
	}
	start, end := in.Start, in.End
	wf, ged := in.WorkflowTasks, in.GEDTasks
	return &Replacement{User: titular, ReplacedBy: sub, StartDate: &start, EndDate: &end, WorkflowTasks: &wf, GEDTasks: &ged}, nil
}

// UpdateReplacement altera uma substituição existente (SOAP
// updateColleagueReplacement). Faz merge: busca o par atual e sobrescreve só os
// campos informados. Par inexistente = ErrNotFound.
func (c *Client) UpdateReplacement(ctx context.Context, titularLogin, subLogin string, ch ReplacementChanges) (*Replacement, error) {
	titular, sub, err := c.resolveReplacementPair(ctx, titularLogin, subLogin)
	if err != nil {
		return nil, err
	}
	raw, err := c.getUserReplacementsRaw(ctx, titular.Code, false)
	if err != nil {
		return nil, err
	}
	var cur *soap.ColleagueReplacement
	for i := range raw {
		if raw[i].ReplacementID == sub.Code {
			cur = &raw[i]
			break
		}
	}
	if cur == nil {
		return nil, fmt.Errorf("%w: substituição de %q por %q", ErrNotFound, titularLogin, subLogin)
	}
	dto := *cur
	dto.CompanyID = c.opts.CompanyID
	if ch.Start != nil {
		dto.StartDate = ch.Start.Format(replDateLayout)
	}
	if ch.End != nil {
		dto.FinalDate = ch.End.Format(replDateLayout)
	}
	if ch.WorkflowTasks != nil {
		dto.ViewWorkflowTasks = *ch.WorkflowTasks
	}
	if ch.GEDTasks != nil {
		dto.ViewGEDTasks = *ch.GEDTasks
	}
	reqBody, err := soap.BuildUpdateReplacement(c.opts.Username, c.opts.Password, dto)
	if err != nil {
		return nil, err
	}
	if err := c.doReplacementWrite(ctx, "updateColleagueReplacement", reqBody); err != nil {
		return nil, err
	}
	wf, ged := dto.ViewWorkflowTasks, dto.ViewGEDTasks
	return &Replacement{
		User: titular, ReplacedBy: sub,
		StartDate: requestTime(dto.StartDate), EndDate: requestTime(dto.FinalDate),
		WorkflowTasks: &wf, GEDTasks: &ged,
	}, nil
}

// DeleteReplacement remove uma substituição (SOAP deleteColleagueReplacement).
// Par inexistente responde "NOK Não foi encontrado..." → ErrNotFound.
func (c *Client) DeleteReplacement(ctx context.Context, titularLogin, subLogin string) error {
	titular, sub, err := c.resolveReplacementPair(ctx, titularLogin, subLogin)
	if err != nil {
		return err
	}
	reqBody, err := soap.BuildDeleteReplacement(c.opts.Username, c.opts.Password, c.opts.CompanyID, titular.Code, sub.Code)
	if err != nil {
		return err
	}
	return c.doReplacementWrite(ctx, "deleteColleagueReplacement", reqBody)
}

// doReplacementWrite faz o POST SOAP e traduz o result textual. O serviço não
// usa soap:Fault para erro de negócio — devolve "OK" ou "NOK <mensagem>".
func (c *Client) doReplacementWrite(ctx context.Context, action string, reqBody []byte) error {
	if err := c.EnsureSession(ctx); err != nil {
		return err
	}
	respBody, err := c.postSOAP(ctx, soapReplacementPath, action, reqBody)
	if err != nil {
		return err
	}
	res, err := soap.ParseReplacementStatus(respBody)
	if err != nil {
		return mapSOAPError(err)
	}
	if strings.EqualFold(strings.TrimSpace(res), "OK") {
		return nil
	}
	return replStatusError(res)
}

// replStatusError interpreta o "NOK <mensagem>" do serviço. "Não foi
// encontrado..." (delete de par inexistente) vira ErrNotFound; o resto vira
// rejeição de negócio (exit 5).
func replStatusError(res string) error {
	msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(res), "NOK"))
	if msg == "" {
		msg = res
	}
	low := strings.ToLower(msg)
	if strings.Contains(low, "não foi encontrado") || strings.Contains(low, "nao foi encontrado") {
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	}
	return fmt.Errorf("%w: %s", errServerRejected, msg)
}

// resolveReplacementUser resolve um login para RequestUser (com userCode). O
// colleagueId/replacementId do serviço é o userCode, NÃO o login (login devolve
// resultado nulo). Login inexistente = ErrNotFound.
func (c *Client) resolveReplacementUser(ctx context.Context, login string) (*RequestUser, error) {
	u, err := c.FindUserByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	if u.Code == "" {
		return nil, fmt.Errorf("%w: usuário %q", ErrNotFound, login)
	}
	return &RequestUser{Name: u.FullName, Login: login, Code: u.Code}, nil
}

func (c *Client) resolveReplacementPair(ctx context.Context, titularLogin, subLogin string) (*RequestUser, *RequestUser, error) {
	titular, err := c.resolveReplacementUser(ctx, titularLogin)
	if err != nil {
		return nil, nil, err
	}
	sub, err := c.resolveReplacementUser(ctx, subLogin)
	if err != nil {
		return nil, nil, err
	}
	return titular, sub, nil
}
