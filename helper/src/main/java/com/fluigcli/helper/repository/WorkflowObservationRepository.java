package com.fluigcli.helper.repository;

import java.util.List;

import javax.naming.InitialContext;

import com.fluig.sdk.api.workflow.ProcessInstanceInfoVO;
import com.fluig.sdk.api.workflow.ProcessObservationVO;
import com.fluig.sdk.service.WorkflowAPIService;

/**
 * Observações de solicitação via o EJB WorkflowAPIService do SDK
 * (java:global/fluig/bpm-sdk/sdk/Workflow).
 *
 * O EJB é alcançado por JNDI dentro do método, com cast para a interface do
 * SDK, como o AuthorizationFilter faz com UserService. Não é @EJB no
 * controller de propósito: uma injeção que falhe no deploy derruba o
 * WorkflowController inteiro, inclusive o PUT de eventos que já está em uso.
 * Com o lookup aqui, uma falha fica restrita às rotas de observação e sai
 * como 500 com a causa no log.
 *
 * Por que o SDK, e não REST nem SOAP: a REST do Fluig não tem comentário sem
 * movimentação, e o SOAP setTasksComments exige a senha do usuário, que um
 * usuário de app OAuth não tem.
 */
public class WorkflowObservationRepository {

    /** Tarefas ativas da solicitação (a fonte da "tarefa corrente"). */
    public ProcessInstanceInfoVO getActiveTasks(int processInstanceId) throws Exception {
        InitialContext ic = null;
        try {
            ic = new InitialContext();
            WorkflowAPIService service = (WorkflowAPIService) ic.lookup(WorkflowAPIService.JNDI_REMOTE_NAME);
            return service.getActiveTasks(processInstanceId);
        } finally {
            fechar(ic);
        }
    }

    /** Grava a observação e devolve o VO como o motor o persistiu (com id). */
    public ProcessObservationVO create(ProcessObservationVO vo) throws Exception {
        InitialContext ic = null;
        try {
            ic = new InitialContext();
            WorkflowAPIService service = (WorkflowAPIService) ic.lookup(WorkflowAPIService.JNDI_REMOTE_NAME);
            return service.createProcessObservation(vo);
        } finally {
            fechar(ic);
        }
    }

    /** Observações de uma etapa/thread da solicitação. */
    public List<ProcessObservationVO> find(int processInstanceId, int stateSequence, int threadSequence) throws Exception {
        InitialContext ic = null;
        try {
            ic = new InitialContext();
            WorkflowAPIService service = (WorkflowAPIService) ic.lookup(WorkflowAPIService.JNDI_REMOTE_NAME);
            return service.findObservations(processInstanceId, stateSequence, threadSequence);
        } finally {
            fechar(ic);
        }
    }

    private static void fechar(InitialContext ic) {
        if (ic != null) {
            try {
                ic.close();
            } catch (Exception ignore) {
                // nada
            }
        }
    }
}
