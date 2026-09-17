package com.fluigcli.helper.exception;

/** A solicitação (instância de processo) não existe no tenant. Vira 404. */
public class ProcessInstanceNotFoundException extends Exception {

    public ProcessInstanceNotFoundException(int processInstanceId) {
        super("Solicitação " + processInstanceId + " não encontrada");
    }
}
