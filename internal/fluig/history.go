package fluig

import (
	"context"
	"fmt"
	"strconv"
)

// HistoryMove é um movimento da solicitação na tabela HISTOR_PROCES.
//
// Medido na homologação (Gate C do §5.15, 2026-10-31): gateways, service
// tasks, o fim e o modo gestor aparecem como movimento; o cancelamento NÃO
// cria movimento; o erro de service task também não cria movimento no evento
// de erro (a solicitação vai direto para a tarefa de correção). Cada movimento
// grava a versão do processo. A aresta percorrida é (etapa do movimento
// PreviousMovement, etapa do movimento).
type HistoryMove struct {
	Movement         int    `json:"movement"`         // NUM_SEQ_MOVTO
	PreviousMovement int    `json:"previousMovement"` // NUM_SEQ_MOVTO_ANT (0 = entrada pelo início)
	State            int    `json:"state"`            // NUM_SEQ_ESTADO
	Thread           int    `json:"thread"`           // NUM_SEQ_THREAD
	Version          int    `json:"version"`          // PROCESS_DEFINITION_VERSION
	ProcessID        string `json:"processId"`        // PROCES_WORKFLOW.COD_DEF_PROCES
}

// historySQL é SQL padrão de propósito (sem "with (nolock)"): o Fluig roda em
// SQL Server, Oracle, PostgreSQL e MySQL.
const historySQL = `select h.NUM_SEQ_MOVTO, h.NUM_SEQ_MOVTO_ANT, h.NUM_SEQ_ESTADO, h.NUM_SEQ_THREAD,
       h.PROCESS_DEFINITION_VERSION, p.COD_DEF_PROCES
  from HISTOR_PROCES h
  join PROCES_WORKFLOW p on p.NUM_PROCES = h.NUM_PROCES and p.COD_EMPRESA = h.COD_EMPRESA
 where h.NUM_PROCES = ? and h.COD_EMPRESA = ?
 order by h.NUM_SEQ_MOVTO`

// RequestHistory lê os movimentos da solicitação pela API de banco do
// fluigcliHelper (datasource padrão). Sem helper: ErrHelperMissing/Outdated.
// Solicitação sem movimento (inexistente): ErrNotFound.
//
// A REST /v2/requests/{id}/tasks NÃO serve para isso: numa tarefa concluída
// ela devolve a etapa ESCOLHIDA, não a etapa onde a tarefa aconteceu (Gate 0,
// ROADMAP §5.16).
func (c *Client) RequestHistory(ctx context.Context, requestID int) ([]HistoryMove, error) {
	res, err := c.DbQuery(ctx, DbQueryOptions{
		SQL:    historySQL,
		Params: []string{strconv.Itoa(requestID), strconv.Itoa(c.opts.CompanyID)},
	})
	if err != nil {
		return nil, err
	}
	return ParseHistoryRows(requestID, res)
}

// ParseHistoryRows converte as linhas do DbResult (ordem das colunas do
// historySQL). Exportada para os testes com fixture real.
func ParseHistoryRows(requestID int, res *DbResult) ([]HistoryMove, error) {
	if len(res.Rows) == 0 {
		return nil, fmt.Errorf("%w: solicitação %d", ErrNotFound, requestID)
	}
	num := func(v *string) int {
		if v == nil {
			return 0
		}
		n, _ := strconv.Atoi(*v)
		return n
	}
	out := make([]HistoryMove, 0, len(res.Rows))
	for _, r := range res.Rows {
		if len(r) < 6 {
			return nil, fmt.Errorf("resposta inesperada do histórico: %d colunas", len(r))
		}
		m := HistoryMove{
			Movement: num(r[0]), PreviousMovement: num(r[1]), State: num(r[2]),
			Thread: num(r[3]), Version: num(r[4]),
		}
		if r[5] != nil {
			m.ProcessID = *r[5]
		}
		out = append(out, m)
	}
	return out, nil
}
