package fluig

import (
	"context"
	"encoding/xml"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Detalhamento de processo a partir do export XML nativo (validado em
// 2026-07-21). Uma única chamada (GET .../export/xml, a mesma do publish)
// traz TUDO que o Explorador de Processos do dev server precisa: etapas com
// atribuição e SLA, transições, regras de gateway, eventos com código e o
// SVG do diagrama renderizado pelo servidor. A raiz é <list> (XStream); as
// coleções vêm em <list> aninhados irmãos, então o path "list>Tipo" casa
// cada tipo em qualquer coleção. bpmnType é NUMÉRICO aqui (≠ da string da
// REST de states): 10 início, 43 evento de erro, 60 fim, 80 atividade
// humana, 82 service task, 120 gateway.
//
// Este arquivo é parse puro (sem cobra/terminal) — a agregação com nome do
// formulário e presença de scripts locais fica no devserver.

// ProcessDetail é a visão consolidada de uma versão de processo.
type ProcessDetail struct {
	ID          string               `json:"id"`
	Description string               `json:"description"`
	Version     int                  `json:"version"`
	Author      string               `json:"author,omitempty"`
	Active      bool                 `json:"active"`
	FormID      int                  `json:"formId"`
	Manager     *StateAssignment     `json:"manager,omitempty"` // gestor do processo (papel/grupo/usuário)
	States      []ProcessStateDetail `json:"states"`
	Events      []ProcessEventInfo   `json:"events"`
	DiagramSVG  string               `json:"diagramSvg,omitempty"`
}

// ProcessStateDetail é uma etapa do processo com o que o desenvolvedor
// costuma precisar ao escrever formulário/scripts.
type ProcessStateDetail struct {
	Sequence     int                `json:"sequence"` // = WKNumState
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Kind         string             `json:"kind"` // start|task|service|gateway|event|end|unknown
	BpmnType     int                `json:"bpmnType"`
	Assignment   *StateAssignment   `json:"assignment,omitempty"`   // só atividade humana
	Service      *StateService      `json:"service,omitempty"`      // só service task
	Transitions  []StateTransition  `json:"transitions,omitempty"`  // destinos (ProcessLink)
	Conditions   []GatewayCondition `json:"conditions,omitempty"`   // só gateway
	DeadlineTime int                `json:"deadlineTime,omitempty"` // prazo (segundos; 0 = sem prazo)
	Instruction  string             `json:"instruction,omitempty"`
}

// StateAssignment descreve quem atua numa etapa humana: o mecanismo
// (engineAllocationId, ex. "Pool Papel") e o alvo concreto extraído do
// AssignmentController (ex. papel "faturista", grupo "TI", campo do
// formulário "diretorAprovador").
type StateAssignment struct {
	Mechanism string `json:"mechanism"`       // engineAllocationId
	Kind      string `json:"kind,omitempty"`  // role|group|formField|baseActivity|user|colleague
	Value     string `json:"value,omitempty"` // código do papel/grupo/campo/etc.
	Name      string `json:"name,omitempty"`  // nome legível do alvo, quando resolvível (ex.: nome do usuário)
	Extra     string `json:"extra,omitempty"` // detalhe adicional (ex.: Returns=Last do executor)
}

// StateService é a configuração de uma service task (automação).
type StateService struct {
	Attempts       int    `json:"attempts,omitempty"`
	Frequency      int    `json:"frequency,omitempty"`
	FrequencyType  string `json:"frequencyType,omitempty"`
	SuccessMessage string `json:"successMessage,omitempty"`
}

// StateTransition é uma aresta de saída da etapa (ProcessLink).
type StateTransition struct {
	To      int    `json:"to"`                // sequence de destino
	Label   string `json:"label,omitempty"`   // actionLabel/name do link
	Default bool   `json:"default,omitempty"` // link default (defaultLink)
}

// GatewayCondition é uma alternativa de um gateway exclusivo: a ordem, o
// destino e as regras (campo/operador/valor) que a habilitam.
type GatewayCondition struct {
	Order int           `json:"order"` // expressionOrder
	To    int           `json:"to"`    // destinationSequenceId
	Rules []GatewayRule `json:"rules,omitempty"`
}

// GatewayRule é uma regra automática de um gateway (field <op> value).
type GatewayRule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // rótulo best-effort (ver operatorLabel)
	OpCode   int    `json:"opCode"`   // código bruto (Fase 2 mapeia o resto)
	Value    string `json:"value,omitempty"`
}

// ProcessEventInfo é um evento (script) do processo, com o tamanho do código
// no servidor. A presença/caminho local é anexada pelo devserver.
type ProcessEventInfo struct {
	Event     string `json:"event"`             // eventId (ex. beforeTaskSave, servicetask19)
	CodeChars int    `json:"codeChars"`         // tamanho do código no servidor
	Service   bool   `json:"service,omitempty"` // é script de service task (servicetask*)
}

// ExportProcessVersionXML baixa o XML de configuração de uma versão específica.
func (c *Client) ExportProcessVersionXML(ctx context.Context, processID string, version int) ([]byte, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	path := processPath(processID, "/process-versions/"+strconv.Itoa(version)+"/export/xml")
	body, status, err := c.doRaw(ctx, http.MethodGet, c.url(path), nil, "", "application/xml")
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, processAPIError(status, body, processID, "export/xml")
	}
	return body, nil
}

// ProcessDetail baixa e interpreta o export do processo. version <= 0 usa a
// última versão (endpoint /export/xml).
func (c *Client) ProcessDetail(ctx context.Context, processID string, version int) (*ProcessDetail, error) {
	var (
		data []byte
		err  error
	)
	if version > 0 {
		data, err = c.ExportProcessVersionXML(ctx, processID, version)
	} else {
		data, err = c.ExportProcessXML(ctx, processID)
	}
	if err != nil {
		return nil, err
	}
	return ParseProcessDetail(data)
}

// --- parse ---

type xmlProcessExport struct {
	XMLName    xml.Name `xml:"list"`
	Definition struct {
		ID          string `xml:"processDefinitionPK>processId"`
		Description string `xml:"processDescription"`
		Active      bool   `xml:"active"`
		// Gestor do processo: mesma estrutura de atribuição das etapas
		// (engineAllocationId + AssignmentController com Role/Group/User).
		ManagerAllocID     string `xml:"managerEngineAllocationId"`
		ManagerAllocConfig string `xml:"managerEngineAllocationConfiguration"`
	} `xml:"ProcessDefinition"`
	Version struct {
		Version int    `xml:"processDefinitionVersionPK>version"`
		FormID  int    `xml:"formId"`
		Author  string `xml:"authorName"`
		Diagram string `xml:"processDiagram"`
	} `xml:"ProcessDefinitionVersion"`
	// As coleções são <list> aninhados irmãos sob a raiz; "list>Tipo" casa
	// cada tipo onde quer que apareça.
	States     []xmlState     `xml:"list>ProcessState"`
	Links      []xmlLink      `xml:"list>ProcessLink"`
	Conditions []xmlCondition `xml:"list>ConditionProcessState"`
	Rules      []xmlRule      `xml:"list>ConditionProcessAutomaticRules"`
	Services   []xmlService   `xml:"list>ProcessStateService"`
	Events     []xmlEvent     `xml:"list>WorkflowProcessEvent"`
}

type xmlState struct {
	Sequence     int    `xml:"processStatePK>sequence"`
	Name         string `xml:"stateName"`
	Description  string `xml:"stateDescription"`
	Instruction  string `xml:"instruction"`
	DeadlineTime int    `xml:"deadlineTime"`
	AllocID      string `xml:"engineAllocationId"`
	AllocConfig  string `xml:"engineAllocationConfiguration"`
	BpmnType     int    `xml:"bpmnType"`
}

type xmlLink struct {
	Initial int    `xml:"initialStateSequence"`
	Final   int    `xml:"finalStateSequence"`
	Label   string `xml:"actionLabel"`
	Name    string `xml:"name"`
	Default bool   `xml:"defaultLink"`
}

type xmlCondition struct {
	Sequence    int `xml:"conditionProcessStatePK>sequence"`
	Order       int `xml:"conditionProcessStatePK>expressionOrder"`
	Destination int `xml:"destinationSequenceId"`
}

type xmlRule struct {
	Sequence  int    `xml:"sequence"`
	Order     int    `xml:"expressionOrder"`
	RuleOrder int    `xml:"ruleOrder"`
	Field     string `xml:"field"`
	Value     string `xml:"value"`
	Operator  int    `xml:"operator"`
}

type xmlService struct {
	Sequence       int    `xml:"sequence"`
	Attempts       int    `xml:"attempts"`
	Frequency      int    `xml:"frequency"`
	FrequencyType  string `xml:"frequencyType"`
	SuccessMessage string `xml:"sucessFullMessage"`
}

type xmlEvent struct {
	Event string `xml:"workflowProcessEventPK>eventId"`
	Code  string `xml:"eventDescription"`
}

// ParseProcessDetail interpreta o XML de export de um processo.
func ParseProcessDetail(data []byte) (*ProcessDetail, error) {
	var ex xmlProcessExport
	if err := xml.Unmarshal(data, &ex); err != nil {
		return nil, err
	}

	// Índices por sequence: transições, condições e serviços.
	transByFrom := map[int][]StateTransition{}
	for _, l := range ex.Links {
		label := strings.TrimSpace(l.Label)
		if label == "" {
			label = strings.TrimSpace(l.Name)
		}
		transByFrom[l.Initial] = append(transByFrom[l.Initial], StateTransition{
			To: l.Final, Label: label, Default: l.Default,
		})
	}
	// Regras agrupadas por (sequence, expressionOrder), em ordem de ruleOrder.
	rulesByCond := map[[2]int][]GatewayRule{}
	for _, r := range ex.Rules {
		key := [2]int{r.Sequence, r.Order}
		rulesByCond[key] = append(rulesByCond[key], GatewayRule{
			Field:    r.Field,
			Operator: operatorLabel(r.Operator),
			OpCode:   r.Operator,
			Value:    r.Value,
		})
	}
	condBySeq := map[int][]GatewayCondition{}
	for _, c := range ex.Conditions {
		cond := GatewayCondition{Order: c.Order, To: c.Destination}
		cond.Rules = rulesByCond[[2]int{c.Sequence, c.Order}]
		condBySeq[c.Sequence] = append(condBySeq[c.Sequence], cond)
	}
	svcBySeq := map[int]*StateService{}
	for _, s := range ex.Services {
		svcBySeq[s.Sequence] = &StateService{
			Attempts: s.Attempts, Frequency: s.Frequency,
			FrequencyType: s.FrequencyType, SuccessMessage: s.SuccessMessage,
		}
	}

	out := &ProcessDetail{
		ID:          strings.TrimSpace(ex.Definition.ID),
		Description: strings.TrimSpace(ex.Definition.Description),
		Version:     ex.Version.Version,
		Author:      strings.TrimSpace(ex.Version.Author),
		Active:      ex.Definition.Active,
		FormID:      ex.Version.FormID,
		Manager:     parseAssignment(ex.Definition.ManagerAllocID, ex.Definition.ManagerAllocConfig),
		DiagramSVG:  strings.TrimSpace(ex.Version.Diagram),
	}

	for _, st := range ex.States {
		d := ProcessStateDetail{
			Sequence:     st.Sequence,
			Name:         strings.TrimSpace(st.Name),
			Description:  strings.TrimSpace(st.Description),
			Kind:         bpmnKind(st.BpmnType),
			BpmnType:     st.BpmnType,
			DeadlineTime: st.DeadlineTime,
			Instruction:  strings.TrimSpace(st.Instruction),
		}
		// Ordena as transições por destino para saída estável.
		tr := transByFrom[st.Sequence]
		sort.SliceStable(tr, func(i, j int) bool { return tr[i].To < tr[j].To })
		d.Transitions = tr

		switch d.Kind {
		case "task":
			if a := parseAssignment(st.AllocID, st.AllocConfig); a != nil {
				d.Assignment = a
			}
		case "service":
			d.Service = svcBySeq[st.Sequence]
		case "gateway":
			conds := condBySeq[st.Sequence]
			sort.SliceStable(conds, func(i, j int) bool { return conds[i].Order < conds[j].Order })
			d.Conditions = conds
		}
		out.States = append(out.States, d)
	}
	sort.SliceStable(out.States, func(i, j int) bool { return out.States[i].Sequence < out.States[j].Sequence })

	for _, ev := range ex.Events {
		id := strings.TrimSpace(ev.Event)
		if id == "" {
			continue
		}
		out.Events = append(out.Events, ProcessEventInfo{
			Event:     id,
			CodeChars: len(ev.Code),
			Service:   strings.HasPrefix(id, "servicetask"),
		})
	}
	sort.SliceStable(out.Events, func(i, j int) bool { return out.Events[i].Event < out.Events[j].Event })

	return out, nil
}

// bpmnKind classifica a etapa pelo bpmnType numérico do export.
func bpmnKind(bpmn int) string {
	switch bpmn {
	case 10:
		return "start"
	case 60:
		return "end"
	case 80:
		return "task"
	case 82:
		return "service"
	case 120:
		return "gateway"
	case 43, 40, 41, 42, 44, 45, 46, 47, 48, 49:
		return "event" // eventos intermediários (43 = erro; faixa 40s)
	default:
		return "unknown"
	}
}

// parseAssignment extrai o alvo do AssignmentController embutido na config.
// Ex.: <AssignmentController><Role>faturista</Role></AssignmentController>.
// Variantes vistas na homologação: Role, Group, FormField,
// BaseActivity+Returns (Executor de Atividade), Colleague/User.
func parseAssignment(allocID, config string) *StateAssignment {
	allocID = strings.TrimSpace(allocID)
	a := &StateAssignment{Mechanism: allocID}
	config = strings.TrimSpace(config)
	if config == "" {
		if allocID == "" {
			return nil
		}
		return a
	}
	var ctrl struct {
		Role         string `xml:"Role"`
		Group        string `xml:"Group"`
		FormField    string `xml:"FormField"`
		BaseActivity string `xml:"BaseActivity"`
		Returns      string `xml:"Returns"`
		Colleague    string `xml:"Colleague"`
		User         string `xml:"User"`
	}
	if err := xml.Unmarshal([]byte(config), &ctrl); err != nil {
		return a // mecanismo conhecido, alvo não parseável
	}
	switch {
	case ctrl.Role != "":
		a.Kind, a.Value = "role", ctrl.Role
	case ctrl.Group != "":
		a.Kind, a.Value = "group", ctrl.Group
	case ctrl.FormField != "":
		a.Kind, a.Value = "formField", ctrl.FormField
	case ctrl.BaseActivity != "":
		a.Kind, a.Value = "baseActivity", ctrl.BaseActivity
		if ctrl.Returns != "" {
			a.Extra = "returns=" + ctrl.Returns
		}
	case ctrl.Colleague != "":
		a.Kind, a.Value = "colleague", ctrl.Colleague
	case ctrl.User != "":
		a.Kind, a.Value = "user", ctrl.User
	}
	return a
}

// operatorLabel devolve um rótulo do operador de regra de gateway. A
// enumeração é a padrão do Fluig (confirmada contra o export real do
// compras_entrada_documento em 2026-07-21: os pares vazio/preenchido (0/9) e
// ==/!= (1/2) e o "contém" (7) batem com as condições observadas). Códigos
// fora da tabela saem como "op N" com o bruto em OpCode.
func operatorLabel(op int) string {
	switch op {
	case 0:
		return "vazio"
	case 1:
		return "=="
	case 2:
		return "!="
	case 3:
		return ">"
	case 4:
		return "<"
	case 5:
		return ">="
	case 6:
		return "<="
	case 7:
		return "contém"
	case 8:
		return "não contém"
	case 9:
		return "preenchido"
	default:
		return "op" + strconv.Itoa(op)
	}
}
