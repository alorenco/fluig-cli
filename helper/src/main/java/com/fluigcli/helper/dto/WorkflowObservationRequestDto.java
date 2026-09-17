package com.fluigcli.helper.dto;

/**
 * Corpo do POST /api/workflows/{processInstanceId}/observations.
 *
 * Só o texto é obrigatório. Etapa e movimento são opcionais: quando faltam, o
 * helper resolve os dois pela tarefa corrente da solicitação. A thread tem
 * default 0 (fluxo principal).
 */
public class WorkflowObservationRequestDto {
    private String observation;
    private Integer stateSequence;
    private Integer movementSequence;
    private Integer threadSequence;

    public WorkflowObservationRequestDto() {}

    public String getObservation() { return observation; }
    public void setObservation(String observation) { this.observation = observation; }
    public Integer getStateSequence() { return stateSequence; }
    public void setStateSequence(Integer stateSequence) { this.stateSequence = stateSequence; }
    public Integer getMovementSequence() { return movementSequence; }
    public void setMovementSequence(Integer movementSequence) { this.movementSequence = movementSequence; }
    public Integer getThreadSequence() { return threadSequence; }
    public void setThreadSequence(Integer threadSequence) { this.threadSequence = threadSequence; }
}
