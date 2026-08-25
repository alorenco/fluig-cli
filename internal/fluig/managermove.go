package fluig

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/alorenco/fluig-cli/internal/fluig/soap"
)

// ManagerMoveOptions parametriza a movimentação em modo gestor.
type ManagerMoveOptions struct {
	TargetState    int    // choosedState: etapa de destino (obrigatório)
	Comment        string // comentário do movimento (histórico)
	ThreadSequence int    // ramo paralelo (0 = fluxo único)
}

// ManagerMoveResult é o resultado do saveAndSendTask em modo gestor.
type ManagerMoveResult struct {
	RequestID    int               `json:"requestId"`
	TargetState  int               `json:"targetState"`            // choosedState enviado
	NextState    int               `json:"nextState"`              // iTask: etapa onde a solicitação parou
	NextAssignee string            `json:"nextAssignee,omitempty"` // cDestino (ex.: "[Pool:Role:cabine_fiscal]")
	ProcessLink  int               `json:"processLink,omitempty"`  // linkSequence percorrido
	Raw          map[string]string `json:"raw,omitempty"`          // todos os pares devolvidos
}

var reLinkSequence = regexp.MustCompile(`linkSequence=(\d+)`)

// MoveRequestAsManager conclui a tarefa corrente de uma solicitação em MODO
// GESTOR (SOAP ECMWorkflowEngineService.saveAndSendTask com managerMode=true).
//
// É o único caminho conhecido para movimentar uma tarefa que não é sua —
// inclusive a de responsável `System:Auto`, quando a atividade automática já
// executou e a transição de saída falhou (ex.: erro no beforeStateEntry do
// destino). A REST v2 não tem equivalente: `/move` com `assignee` responde 500
// (validado em produção em 2026-08-25, solicitação 228691 do processo
// Compras). O motor avalia gateways normalmente a partir do destino informado
// e roda os eventos da etapa de entrada. ⚠️ Em modo gestor o "usuário
// corrente" dos eventos vem vazio (não é o solicitante).
//
// A sessão é a credencial (senha em branco, como no takeProcessTask).
func (c *Client) MoveRequestAsManager(ctx context.Context, id int, o ManagerMoveOptions) (*ManagerMoveResult, error) {
	if o.TargetState <= 0 {
		return nil, fmt.Errorf("modo gestor exige a etapa de destino (choosedState)")
	}
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	userCode, err := c.ResolveUserCode(ctx)
	if err != nil {
		return nil, err
	}
	reqBody, err := soap.BuildSaveAndSendTask(c.opts.CompanyID, c.opts.Username, "", userCode,
		id, o.TargetState, nil, o.Comment, true, true, o.ThreadSequence)
	if err != nil {
		return nil, err
	}
	respBody, err := c.postSOAP(ctx, soapWorkflowPath, "saveAndSendTask", reqBody)
	if err != nil {
		return nil, err
	}
	pairs, err := soap.ParseSaveAndSendTask(respBody)
	if err != nil {
		return nil, mapSOAPError(err)
	}
	if msg := strings.TrimSpace(pairs["ERROR"]); msg != "" {
		return nil, fmt.Errorf("%w: %s (solicitação %d)", errServerRejected, msg, id)
	}
	next, _ := strconv.Atoi(strings.TrimSpace(pairs["iTask"]))
	if next == 0 {
		return nil, fmt.Errorf("%w: saveAndSendTask não devolveu a etapa de destino (%v)", errServerRejected, pairs)
	}
	res := &ManagerMoveResult{
		RequestID: id, TargetState: o.TargetState, NextState: next,
		NextAssignee: strings.TrimSpace(pairs["cDestino"]), Raw: pairs,
	}
	if m := reLinkSequence.FindStringSubmatch(pairs["processLink"]); m != nil {
		res.ProcessLink, _ = strconv.Atoi(m[1])
	}
	return res, nil
}

// StateDetail devolve a etapa `sequence` de uma versão do processo, com as
// transições de saída (ProcessLink) — base para escolher o destino do modo
// gestor quando o usuário não informa --target-state.
func (c *Client) StateDetail(ctx context.Context, processID string, version, sequence int) (*ProcessStateDetail, error) {
	detail, err := c.ProcessDetail(ctx, processID, version)
	if err != nil {
		return nil, err
	}
	for i := range detail.States {
		if detail.States[i].Sequence == sequence {
			return &detail.States[i], nil
		}
	}
	return nil, fmt.Errorf("%w: etapa %d no processo %q v%d", ErrNotFound, sequence, processID, version)
}
