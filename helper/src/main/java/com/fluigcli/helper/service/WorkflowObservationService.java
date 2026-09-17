package com.fluigcli.helper.service;

import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.Date;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

import com.fluig.sdk.api.workflow.ProcessInstanceInfoVO;
import com.fluig.sdk.api.workflow.ProcessObservationVO;
import com.fluig.sdk.api.workflow.ProcessTaskInfoVO;
import com.fluigcli.helper.dto.WorkflowObservationDto;
import com.fluigcli.helper.dto.WorkflowObservationRequestDto;
import com.fluigcli.helper.exception.ProcessInstanceConflictException;
import com.fluigcli.helper.exception.ProcessInstanceNotFoundException;
import com.fluigcli.helper.repository.ProcessInstanceRepository;
import com.fluigcli.helper.repository.WorkflowObservationRepository;

/**
 * Observação em solicitação SEM movimentar. A regra de negócio fica aqui, em
 * métodos estáticos puros, para ter teste unitário sem servidor.
 */
public class WorkflowObservationService {

    /**
     * Teto do texto. Valor de contrato do helper; a coluna do Fluig é
     * conferida na homologação a cada mudança de versão da plataforma
     * (ver docs/request.md).
     */
    public static final int TETO_TEXTO = 8000;

    /** Thread do fluxo principal. É o default quando o chamador não informa. */
    public static final int THREAD_PRINCIPAL = 0;

    private static final DateTimeFormatter ISO_COM_FUSO =
        DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss.SSSXXX");

    /** Par (etapa, movimento) que identifica a tarefa da observação. */
    public static final class Tarefa {
        public final int stateSequence;
        public final int movementSequence;

        public Tarefa(int stateSequence, int movementSequence) {
            this.stateSequence = stateSequence;
            this.movementSequence = movementSequence;
        }

        @Override
        public boolean equals(Object o) {
            if (!(o instanceof Tarefa)) {
                return false;
            }
            Tarefa t = (Tarefa) o;
            return stateSequence == t.stateSequence && movementSequence == t.movementSequence;
        }

        @Override
        public int hashCode() {
            return 31 * stateSequence + movementSequence;
        }
    }

    private final ProcessInstanceRepository instancias;
    private final WorkflowObservationRepository observacoes;

    public WorkflowObservationService() {
        this(new ProcessInstanceRepository(), new WorkflowObservationRepository());
    }

    WorkflowObservationService(ProcessInstanceRepository instancias, WorkflowObservationRepository observacoes) {
        this.instancias = instancias;
        this.observacoes = observacoes;
    }

    /**
     * Cria a observação em nome de colleagueId. Resolve a tarefa corrente
     * quando o corpo não traz etapa E movimento.
     */
    public WorkflowObservationDto create(
        long tenantId,
        int processInstanceId,
        String colleagueId,
        WorkflowObservationRequestDto req
    ) throws Exception {
        exigirAberta(tenantId, processInstanceId);

        Tarefa tarefa;
        if (req.getStateSequence() != null && req.getMovementSequence() != null) {
            tarefa = new Tarefa(req.getStateSequence(), req.getMovementSequence());
        } else {
            tarefa = tarefaCorrente(observacoes.getActiveTasks(processInstanceId));
        }

        ProcessObservationVO vo = new ProcessObservationVO();
        vo.setProcessInstanceId(processInstanceId);
        vo.setColleagueId(colleagueId);
        vo.setObservationDate(new Date());
        vo.setStateSequence(tarefa.stateSequence);
        vo.setMovementSequence(tarefa.movementSequence);
        vo.setThreadSequence(threadOuDefault(req.getThreadSequence()));
        vo.setObservation(req.getObservation().trim());

        ProcessObservationVO criado = observacoes.create(vo);
        // Alguns EJBs do Fluig devolvem o próprio VO; outros, uma cópia sem
        // tudo preenchido. Completa com o que foi enviado.
        return paraDto(criado == null ? vo : preencher(criado, vo));
    }

    /**
     * Lista as observações de uma etapa/thread. Sem etapa, usa a tarefa
     * corrente (mesma resolução do POST).
     */
    public List<WorkflowObservationDto> list(
        long tenantId,
        int processInstanceId,
        Integer stateSequence,
        Integer threadSequence
    ) throws Exception {
        Integer status = instancias.findStatus(tenantId, processInstanceId);
        if (status == null) {
            throw new ProcessInstanceNotFoundException(processInstanceId);
        }

        int etapa;
        if (stateSequence != null) {
            etapa = stateSequence;
        } else {
            // Solicitação encerrada não tem tarefa corrente, e o getActiveTasks
            // do SDK responde NullPointerException nesse caso (medido na
            // homologação em 2026-09-17 com uma solicitação cancelada). O 409
            // diz o que fazer.
            String motivo = motivoSemEtapa(status);
            if (motivo != null) {
                throw new ProcessInstanceConflictException(motivo);
            }
            etapa = tarefaCorrente(observacoes.getActiveTasks(processInstanceId)).stateSequence;
        }

        List<ProcessObservationVO> lista = observacoes.find(processInstanceId, etapa, threadOuDefault(threadSequence));
        List<WorkflowObservationDto> out = new ArrayList<>();
        for (ProcessObservationVO vo : ordenar(lista)) {
            out.add(paraDto(vo));
        }
        return out;
    }

    // --- regras puras (testadas em WorkflowObservationServiceTest) ---

    /**
     * Valida o texto: obrigatório, não vazio após trim e no máximo TETO_TEXTO
     * caracteres. Devolve a mensagem do 400, ou null quando está válido.
     */
    public static String validarTexto(String observation) {
        if (observation == null || observation.trim().isEmpty()) {
            return "O campo observation é obrigatório e não pode ficar vazio";
        }
        if (observation.trim().length() > TETO_TEXTO) {
            return "O campo observation tem " + observation.trim().length()
                + " caracteres; o máximo é " + TETO_TEXTO;
        }
        return null;
    }

    /**
     * Resolve a tarefa corrente a partir das tarefas ativas. Tarefas do MESMO
     * movimento (ex.: pool + usuário) contam como uma só. Movimentos
     * distintos são paralelismo e exigem a etapa informada.
     */
    public static Tarefa tarefaCorrente(ProcessInstanceInfoVO info) throws ProcessInstanceConflictException {
        Set<Tarefa> ativas = new LinkedHashSet<>();
        if (info != null && info.getTasksInfo() != null) {
            for (ProcessTaskInfoVO t : info.getTasksInfo()) {
                if (t != null && t.isActive()) {
                    ativas.add(new Tarefa(t.getStateSequence(), t.getMovementSequence()));
                }
            }
        }
        if (ativas.isEmpty()) {
            throw new ProcessInstanceConflictException("solicitação sem tarefa ativa");
        }
        if (ativas.size() > 1) {
            throw new ProcessInstanceConflictException(
                "solicitação tem tarefas paralelas; informe stateSequence e movementSequence");
        }
        return ativas.iterator().next();
    }

    /** Ordena por observationDate (nulos por último) e depois por id. */
    public static List<ProcessObservationVO> ordenar(List<ProcessObservationVO> lista) {
        List<ProcessObservationVO> out = new ArrayList<>();
        if (lista != null) {
            for (ProcessObservationVO vo : lista) {
                if (vo != null) {
                    out.add(vo);
                }
            }
        }
        out.sort(
            Comparator.comparing(ProcessObservationVO::getObservationDate, Comparator.nullsLast(Comparator.naturalOrder()))
                .thenComparing(ProcessObservationVO::getId, Comparator.nullsLast(Comparator.naturalOrder()))
        );
        return out;
    }

    /** ISO-8601 com milissegundos e o offset do fuso da JVM; null vira null. */
    public static String formatarData(Date data, ZoneId zona) {
        if (data == null) {
            return null;
        }
        return ISO_COM_FUSO.withZone(zona).format(data.toInstant());
    }

    static int threadOuDefault(Integer threadSequence) {
        return threadSequence == null ? THREAD_PRINCIPAL : threadSequence;
    }

    static WorkflowObservationDto paraDto(ProcessObservationVO vo) {
        WorkflowObservationDto dto = new WorkflowObservationDto();
        dto.setId(vo.getId());
        dto.setProcessInstanceId(vo.getProcessInstanceId());
        dto.setStateSequence(vo.getStateSequence());
        dto.setMovementSequence(vo.getMovementSequence());
        dto.setThreadSequence(vo.getThreadSequence());
        dto.setColleagueId(vo.getColleagueId());
        dto.setObservationDate(formatarData(vo.getObservationDate(), ZoneId.systemDefault()));
        dto.setObservation(vo.getObservation());
        return dto;
    }

    /** Completa os campos nulos de `criado` com os de `enviado`. */
    static ProcessObservationVO preencher(ProcessObservationVO criado, ProcessObservationVO enviado) {
        if (criado.getProcessInstanceId() == null) criado.setProcessInstanceId(enviado.getProcessInstanceId());
        if (criado.getColleagueId() == null) criado.setColleagueId(enviado.getColleagueId());
        if (criado.getObservationDate() == null) criado.setObservationDate(enviado.getObservationDate());
        if (criado.getStateSequence() == null) criado.setStateSequence(enviado.getStateSequence());
        if (criado.getMovementSequence() == null) criado.setMovementSequence(enviado.getMovementSequence());
        if (criado.getThreadSequence() == null) criado.setThreadSequence(enviado.getThreadSequence());
        if (criado.getObservation() == null) criado.setObservation(enviado.getObservation());
        return criado;
    }

    // --- existência e estado (banco) ---

    private void exigirAberta(long tenantId, int processInstanceId) throws Exception {
        Integer status = instancias.findStatus(tenantId, processInstanceId);
        if (status == null) {
            throw new ProcessInstanceNotFoundException(processInstanceId);
        }
        String motivo = motivoDeConflito(status);
        if (motivo != null) {
            throw new ProcessInstanceConflictException(motivo);
        }
    }

    /**
     * Mensagem do 409 do GET sem stateSequence numa solicitação encerrada, ou
     * null se ela está aberta (aí a tarefa corrente resolve a etapa).
     */
    public static String motivoSemEtapa(int status) {
        switch (status) {
            case ProcessInstanceRepository.STATUS_ABERTA:
                return null;
            case ProcessInstanceRepository.STATUS_CANCELADA:
                return "solicitação cancelada; informe stateSequence";
            case ProcessInstanceRepository.STATUS_CONCLUIDA:
                return "solicitação finalizada; informe stateSequence";
            default:
                return "solicitação com status " + status + "; informe stateSequence";
        }
    }

    /** Mensagem do 409 para um STATUS de PROCES_WORKFLOW, ou null se aberta. */
    public static String motivoDeConflito(int status) {
        switch (status) {
            case ProcessInstanceRepository.STATUS_ABERTA:
                return null;
            case ProcessInstanceRepository.STATUS_CANCELADA:
                return "solicitação cancelada; não aceita observação";
            case ProcessInstanceRepository.STATUS_CONCLUIDA:
                return "solicitação finalizada; não aceita observação";
            default:
                return "solicitação com status " + status + "; não aceita observação";
        }
    }
}
