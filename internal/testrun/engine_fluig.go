package testrun

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig"
)

// FluigEngine é a Engine real, sobre o cliente da plataforma.
type FluigEngine struct {
	Client *fluig.Client
	Login  string // login do ator "eu" (papéis e grupos no --dry-run)
}

var _ Engine = (*FluigEngine)(nil)

// wrap classifica o erro do cliente: infraestrutura (sessão, rede, tempo
// limite) vira "errored"; recusa do servidor vira "failed".
func wrap(err error) error {
	if err == nil {
		return nil
	}
	ee := &EngineError{Code: "SERVER_ERROR", Message: err.Error(), Err: err}
	var netErr net.Error
	var blocked *fluig.MoveBlockedError
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded),
		errors.As(err, &netErr) && netErr.Timeout():
		ee.Code, ee.Infra = "TIMEOUT", true
	case errors.Is(err, fluig.ErrAuthFailed):
		ee.Code, ee.Infra = "AUTH_FAILED", true
	case errors.As(err, &blocked):
		ee.Code = "NO_HUMAN_TASK"
		if blocked.Kind == fluig.MoveBlockedPool {
			ee.Code = "POOL_TASK_NOT_ASSIGNED"
		}
	case errors.Is(err, fluig.ErrNotFound):
		ee.Code = "NOT_FOUND"
	case errors.As(err, &netErr):
		ee.Infra = true
	}
	return ee
}

// Process lê o diagrama da versão ativa.
func (e *FluigEngine) Process(ctx context.Context, processID string) (*ProcessInfo, error) {
	d, err := e.Client.ProcessDetail(ctx, processID, 0)
	if err != nil {
		return nil, wrap(err)
	}
	p := &ProcessInfo{Version: d.Version, States: map[int]StateInfo{}}
	for _, s := range d.States {
		si := StateInfo{Name: s.Name, Kind: s.Kind}
		for _, t := range s.Transitions {
			si.Transitions = append(si.Transitions, t.To)
		}
		if s.Assignment != nil {
			si.Mechanism, si.AssignKind, si.AssignValue = s.Assignment.Mechanism, s.Assignment.Kind, s.Assignment.Value
		}
		p.States[s.Sequence] = si
		if s.Kind == "start" {
			p.Start = append(p.Start, s.Sequence)
		}
	}
	return p, nil
}

func toMoveOutput(r *fluig.MoveResult) *MoveOutput {
	out := &MoveOutput{RequestID: r.RequestID, CardID: int(r.CardID), NextState: r.NextState, NeedsAssignee: r.NeedsAssignee}
	for _, u := range r.PossibleAssignees {
		id := u.Login
		if id == "" {
			id = u.Code
		}
		out.PossibleAssignees = append(out.PossibleAssignees, id)
	}
	return out
}

// Start abre a solicitação (REST v2).
func (e *FluigEngine) Start(ctx context.Context, processID string, in MoveInput) (*MoveOutput, error) {
	r, err := e.Client.StartRequest(ctx, processID, fluig.RequestStartOptions{
		TargetState: in.Target, TargetAssignee: in.Assignee, Comment: in.Comment, FormFields: in.Fields,
	})
	if err != nil {
		return nil, wrap(err)
	}
	return toMoveOutput(r), nil
}

// Move conclui a tarefa corrente (REST v2). Sem movimento, o servidor resolve
// a tarefa aberta do usuário (medido no Gate 0).
func (e *FluigEngine) Move(ctx context.Context, requestID int, in MoveInput) (*MoveOutput, error) {
	r, err := e.Client.MoveRequestTo(ctx, requestID, fluig.RequestMoveOptions{
		MovementSequence: in.Movement, TargetState: in.Target, TargetAssignee: in.Assignee,
		Comment: in.Comment, FormFields: in.Fields,
	})
	if err != nil {
		return nil, wrap(err)
	}
	return toMoveOutput(r), nil
}

// ManagerMove conclui a tarefa em modo gestor (SOAP saveAndSendTask).
func (e *FluigEngine) ManagerMove(ctx context.Context, requestID int, in MoveInput) (*MoveOutput, error) {
	r, err := e.Client.MoveRequestAsManager(ctx, requestID, fluig.ManagerMoveOptions{
		TargetState: in.Target, Comment: in.Comment, ThreadSequence: in.Thread,
	})
	if err != nil {
		return nil, wrap(err)
	}
	return &MoveOutput{RequestID: requestID, NextState: r.NextState}, nil
}

// Assume assume a tarefa de pool corrente.
func (e *FluigEngine) Assume(ctx context.Context, requestID, thread int) error {
	return wrap(e.Client.TakeProcessTask(ctx, requestID, thread))
}

// Request lê o estado da solicitação.
func (e *FluigEngine) Request(ctx context.Context, requestID int) (*RequestInfo, error) {
	r, err := e.Client.GetRequest(ctx, requestID)
	if err != nil {
		return nil, wrap(err)
	}
	info := &RequestInfo{Status: r.Status, FormID: int(r.FormID), CardID: int(r.FormRecordID)}
	for _, c := range r.CurrentSteps {
		info.Current = append(info.Current, StepState{Sequence: c.Sequence, Name: c.StateName, Movement: c.Movement})
	}
	return info, nil
}

// Tasks lê o histórico de tarefas.
func (e *FluigEngine) Tasks(ctx context.Context, requestID int) ([]TaskInfo, error) {
	ts, err := e.Client.RequestTasks(ctx, requestID)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]TaskInfo, 0, len(ts))
	for _, t := range ts {
		ti := TaskInfo{Sequence: t.Sequence, Status: t.Status}
		if t.Assignee != nil {
			ti.AssigneeCode, ti.AssigneeLogin = t.Assignee.Code, t.Assignee.Login
		}
		out = append(out, ti)
	}
	return out, nil
}

// metaCampo marca as colunas de controle que a API devolve no card e que não
// são campos do formulário. Elas ficam fora da leitura e da regravação.
func metaCampo(nome string) bool {
	switch strings.ToLower(nome) {
	case "cardid", "companyid", "documentid", "masterid", "tableid", "version",
		"anonymization_date", "anonymization_user_id":
		return true
	}
	return false
}

// ReadCard lê o card com as tabelas filhas, achatado: linha filha vira
// campo___<rowId>, a mesma convenção do --fields-file.
func (e *FluigEngine) ReadCard(ctx context.Context, formID, cardID int) (map[string]string, error) {
	rec, err := e.Client.GetFormRecord(ctx, formID, cardID, fluig.FormRecordOptions{})
	if err != nil {
		return nil, wrap(err)
	}
	return FlattenCard(rec), nil
}

// FlattenCard achata o registro (exportada para os testes com fixture real).
func FlattenCard(rec *fluig.FormRecord) map[string]string {
	out := map[string]string{}
	for k, v := range rec.Values {
		if !metaCampo(k) {
			out[k] = v
		}
	}
	for _, ch := range rec.Children {
		for k, v := range ch.Values {
			if !metaCampo(k) {
				out[k+"___"+ch.RowID] = v
			}
		}
	}
	return out
}

// WriteCard grava o card (PUT de cards; roda o validateForm — G0.1).
func (e *FluigEngine) WriteCard(ctx context.Context, formID, cardID int, values map[string]string) error {
	_, err := e.Client.UpdateFormRecord(ctx, formID, cardID, values)
	return wrap(err)
}

// Cancel cancela a solicitação (SOAP cancelInstance).
func (e *FluigEngine) Cancel(ctx context.Context, requestID int, comment string) error {
	return wrap(e.Client.CancelRequest(ctx, requestID, comment))
}

// SelfMembership lê os papéis e grupos do usuário da sessão.
func (e *FluigEngine) SelfMembership(ctx context.Context) ([]string, []string, error) {
	rs, err := e.Client.ListUserRoles(ctx, e.Login)
	if err != nil {
		return nil, nil, wrap(err)
	}
	gs, err := e.Client.ListUserGroups(ctx, e.Login)
	if err != nil {
		return nil, nil, wrap(err)
	}
	var roles, groups []string
	for _, r := range rs {
		roles = append(roles, r.Code)
	}
	for _, g := range gs {
		groups = append(groups, g.Code)
	}
	return roles, groups, nil
}
