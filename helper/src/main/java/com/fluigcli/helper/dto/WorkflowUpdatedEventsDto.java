package com.fluigcli.helper.dto;

import java.util.List;

public class WorkflowUpdatedEventsDto {
    private String processId;
    private int version;
    private boolean hasError;
    private int totalProcessed;
    private List<String> errors;
    private List<String> successes;

    public WorkflowUpdatedEventsDto() {}

    public String getProcessId() { return processId; }
    public void setProcessId(String processId) { this.processId = processId; }
    public int getVersion() { return version; }
    public void setVersion(int version) { this.version = version; }
    public boolean isHasError() { return hasError; }
    public void setHasError(boolean hasError) { this.hasError = hasError; }
    public int getTotalProcessed() { return totalProcessed; }
    public void setTotalProcessed(int totalProcessed) { this.totalProcessed = totalProcessed; }
    public List<String> getErrors() { return errors; }
    public void setErrors(List<String> errors) { this.errors = errors; }
    public List<String> getSuccesses() { return successes; }
    public void setSuccesses(List<String> successes) { this.successes = successes; }
}
