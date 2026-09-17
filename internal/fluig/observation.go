package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Observação em solicitação SEM movimentar, via fluigcliHelper (rotas
// /api/workflows/{id}/observations, existem a partir do helper 0.11.0).
//
// Por que passa pelo helper: a REST do Fluig não tem comentário sem move, e o
// SOAP setTasksComments exige a senha do usuário — um usuário de app OAuth não
// tem senha. O helper grava pelo EJB WorkflowAPIService do SDK, em nome do
// usuário autenticado.

// helperHasObservationAPI: as rotas de observação existem a partir do 0.11.0.
func helperHasObservationAPI(version string) bool { return helperAtLeast(version, 0, 11) }

// RequestObservation é uma observação registrada numa tarefa da solicitação.
type RequestObservation struct {
	ID                int64  `json:"id"`
	ProcessInstanceID int    `json:"processInstanceId"`
	StateSequence     int    `json:"stateSequence"`
	MovementSequence  int    `json:"movementSequence"`
	ThreadSequence    int    `json:"threadSequence"`
	ColleagueID       string `json:"colleagueId"`
	// ObservationDate vem em ISO-8601 com o fuso do servidor
	// ("2026-09-17T14:03:11.000-04:00"), formatado pelo helper.
	ObservationDate string `json:"observationDate"`
	Observation     string `json:"observation"`
}

// ObservationOptions são os parâmetros do registro de uma observação.
type ObservationOptions struct {
	Text string // obrigatório; HTML simples permitido; teto de 8.000 caracteres
	// StateSequence e MovementSequence identificam a tarefa. Zero nos DOIS
	// deixa o helper resolver pela tarefa corrente. Informe os dois juntos.
	StateSequence    int
	MovementSequence int
	ThreadSequence   int // 0 = fluxo principal
}

// RequestConflictError é o 409 do helper: a solicitação existe, mas o estado
// dela impede a operação (finalizada, cancelada, sem tarefa ativa ou com
// tarefas paralelas sem a etapa informada). Message é o texto do helper.
type RequestConflictError struct {
	ProcessInstanceID int
	Message           string
}

func (e *RequestConflictError) Error() string {
	return fmt.Sprintf("solicitação %d: %s", e.ProcessInstanceID, e.Message)
}

type observationRequest struct {
	Observation      string `json:"observation"`
	StateSequence    *int   `json:"stateSequence,omitempty"`
	MovementSequence *int   `json:"movementSequence,omitempty"`
	ThreadSequence   int    `json:"threadSequence"`
}

// CreateRequestObservation registra uma observação na solicitação sem
// movimentá-la, em nome do usuário autenticado. Requer o fluigcliHelper
// >= 0.11.0. Solicitação inexistente → ErrNotFound; estado incompatível →
// *RequestConflictError; texto recusado → erro de servidor com a mensagem.
func (c *Client) CreateRequestObservation(ctx context.Context, id int, o ObservationOptions) (*RequestObservation, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	payload := observationRequest{Observation: o.Text, ThreadSequence: o.ThreadSequence}
	if o.StateSequence != 0 || o.MovementSequence != 0 {
		st, mv := o.StateSequence, o.MovementSequence
		payload.StateSequence, payload.MovementSequence = &st, &mv
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodPost, c.observationsURL(id), raw)
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusCreated, http.StatusOK:
		var obs RequestObservation
		if err := json.Unmarshal(body, &obs); err != nil {
			return nil, fmt.Errorf("resposta inesperada do %s (observations): %w", HelperFluigcli, err)
		}
		return &obs, nil
	default:
		return nil, c.observationError(ctx, id, status, body)
	}
}

// ListRequestObservations lista as observações de uma etapa/thread da
// solicitação, ordenadas por data. stateSequence 0 = tarefa corrente (o helper
// resolve; tarefas paralelas → *RequestConflictError). Requer o helper >= 0.11.0.
func (c *Client) ListRequestObservations(ctx context.Context, id, stateSequence, threadSequence int) ([]RequestObservation, error) {
	if err := c.requireHelper(ctx); err != nil {
		return nil, err
	}
	endpoint := c.observationsURL(id) + "?threadSequence=" + strconv.Itoa(threadSequence)
	if stateSequence != 0 {
		endpoint += "&stateSequence=" + strconv.Itoa(stateSequence)
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, c.observationError(ctx, id, status, body)
	}
	var out []RequestObservation
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("resposta inesperada do %s (observations): %w", HelperFluigcli, err)
	}
	if out == nil {
		out = []RequestObservation{}
	}
	return out, nil
}

func (c *Client) observationsURL(id int) string {
	return c.url(helperWorkflowsBase) + strconv.Itoa(id) + "/observations"
}

// observationError traduz os códigos do helper. O 404 é ambíguo — solicitação
// inexistente OU helper sem a rota (< 0.11.0) — e a versão do helper decide,
// como nas rotas /db.
func (c *Client) observationError(ctx context.Context, id, status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	switch status {
	case http.StatusNotFound:
		if info, e := c.HelperStatus(ctx); e == nil && info.Installed && !helperHasObservationAPI(info.Version) {
			return ErrHelperOutdated
		}
		if msg == "" {
			msg = fmt.Sprintf("solicitação %d não encontrada", id)
		}
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case http.StatusConflict:
		if msg == "" {
			msg = "a solicitação não aceita a operação"
		}
		return &RequestConflictError{ProcessInstanceID: id, Message: msg}
	case http.StatusBadRequest:
		return fmt.Errorf("%w: %s", errServerRejected, msg)
	default:
		return &HTTPError{StatusCode: status, URL: HelperFluigcli + " observations", Body: truncate(msg, 512)}
	}
}
