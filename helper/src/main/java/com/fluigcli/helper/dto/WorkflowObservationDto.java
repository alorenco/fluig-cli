package com.fluigcli.helper.dto;

/**
 * Observação de solicitação como o helper a devolve.
 *
 * É um DTO próprio, e não o ProcessObservationVO do SDK, por causa da data: o
 * Jackson do RESTEasy serializa java.util.Date como epoch em milissegundos. A
 * CLI e os agentes querem ISO-8601 com o fuso do servidor
 * ("2026-09-17T14:03:11.000-04:00"), então a formatação é feita aqui.
 */
public class WorkflowObservationDto {
    private Long id;
    private Integer processInstanceId;
    private Integer stateSequence;
    private Integer movementSequence;
    private Integer threadSequence;
    private String colleagueId;
    private String observationDate;
    private String observation;

    public WorkflowObservationDto() {}

    public Long getId() { return id; }
    public void setId(Long id) { this.id = id; }
    public Integer getProcessInstanceId() { return processInstanceId; }
    public void setProcessInstanceId(Integer processInstanceId) { this.processInstanceId = processInstanceId; }
    public Integer getStateSequence() { return stateSequence; }
    public void setStateSequence(Integer stateSequence) { this.stateSequence = stateSequence; }
    public Integer getMovementSequence() { return movementSequence; }
    public void setMovementSequence(Integer movementSequence) { this.movementSequence = movementSequence; }
    public Integer getThreadSequence() { return threadSequence; }
    public void setThreadSequence(Integer threadSequence) { this.threadSequence = threadSequence; }
    public String getColleagueId() { return colleagueId; }
    public void setColleagueId(String colleagueId) { this.colleagueId = colleagueId; }
    public String getObservationDate() { return observationDate; }
    public void setObservationDate(String observationDate) { this.observationDate = observationDate; }
    public String getObservation() { return observation; }
    public void setObservation(String observation) { this.observation = observation; }
}
