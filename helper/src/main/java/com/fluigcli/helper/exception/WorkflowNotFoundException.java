package com.fluigcli.helper.exception;

public class WorkflowNotFoundException extends Exception {

    public WorkflowNotFoundException(String processId) {
        super("Processo \"" + processId + "\" não encontrado");
    }
}
