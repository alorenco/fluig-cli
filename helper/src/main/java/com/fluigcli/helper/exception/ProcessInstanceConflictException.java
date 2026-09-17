package com.fluigcli.helper.exception;

/**
 * A solicitação existe, mas o estado dela impede a operação: finalizada,
 * cancelada, sem tarefa ativa ou com tarefas paralelas sem a etapa informada.
 * Vira 409 com a mensagem no corpo.
 */
public class ProcessInstanceConflictException extends Exception {

    public ProcessInstanceConflictException(String message) {
        super(message);
    }
}
