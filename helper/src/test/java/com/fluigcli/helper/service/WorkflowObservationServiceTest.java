package com.fluigcli.helper.service;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.time.ZoneId;
import java.util.Arrays;
import java.util.Date;
import java.util.List;

import org.junit.Test;

import com.fluig.sdk.api.workflow.ProcessInstanceInfoVO;
import com.fluig.sdk.api.workflow.ProcessObservationVO;
import com.fluig.sdk.api.workflow.ProcessTaskInfoVO;
import com.fluigcli.helper.exception.ProcessInstanceConflictException;

/**
 * Regras da observação sem movimentar (helper 0.11.0): validação do texto,
 * resolução da tarefa corrente, ordenação, formato da data e o 409 por
 * status da solicitação.
 */
public class WorkflowObservationServiceTest {

    @Test
    public void textoObrigatorioENaoVazioAposTrim() {
        assertTrue(WorkflowObservationService.validarTexto(null).contains("obrigatório"));
        assertTrue(WorkflowObservationService.validarTexto("").contains("obrigatório"));
        assertTrue(WorkflowObservationService.validarTexto("  \n\t ").contains("obrigatório"));
        assertNull(WorkflowObservationService.validarTexto("<b>teste helper</b> observação sem move"));
    }

    @Test
    public void textoTemTetoDe8000() {
        assertNull(WorkflowObservationService.validarTexto(repetir("x", 8000)));
        String erro = WorkflowObservationService.validarTexto(repetir("x", 8001));
        assertTrue("deve dizer o teto: " + erro, erro.contains("8000"));
        assertTrue("deve dizer o tamanho recebido: " + erro, erro.contains("8001"));
        // Espaço nas pontas não conta para o teto.
        assertNull(WorkflowObservationService.validarTexto("  " + repetir("x", 8000) + "  "));
    }

    @Test
    public void tarefaCorrenteComUmaTarefaAtiva() throws Exception {
        ProcessInstanceInfoVO info = info(
            tarefa(34, 5, true),
            tarefa(10, 2, false),   // já concluída: ignorada
            tarefa(3, 1, false)
        );
        WorkflowObservationService.Tarefa t = WorkflowObservationService.tarefaCorrente(info);
        assertEquals(34, t.stateSequence);
        assertEquals(5, t.movementSequence);
    }

    /**
     * Pool + usuário no MESMO movimento não é paralelismo (a mesma regra do
     * `request move`, que segue direto nesse caso).
     */
    @Test
    public void tarefasDoMesmoMovimentoContamComoUma() throws Exception {
        ProcessInstanceInfoVO info = info(tarefa(34, 5, true), tarefa(34, 5, true));
        WorkflowObservationService.Tarefa t = WorkflowObservationService.tarefaCorrente(info);
        assertEquals(34, t.stateSequence);
        assertEquals(5, t.movementSequence);
    }

    @Test
    public void tarefasParalelasExigemEtapaInformada() {
        ProcessInstanceInfoVO info = info(tarefa(34, 5, true), tarefa(36, 6, true));
        try {
            WorkflowObservationService.tarefaCorrente(info);
            fail("devia recusar por paralelismo");
        } catch (ProcessInstanceConflictException e) {
            assertEquals("solicitação tem tarefas paralelas; informe stateSequence e movementSequence", e.getMessage());
        }
    }

    @Test
    public void semTarefaAtivaEhConflito() {
        for (ProcessInstanceInfoVO info : Arrays.asList(null, info(), info(tarefa(10, 2, false)))) {
            try {
                WorkflowObservationService.tarefaCorrente(info);
                fail("devia recusar sem tarefa ativa");
            } catch (ProcessInstanceConflictException e) {
                assertEquals("solicitação sem tarefa ativa", e.getMessage());
            }
        }
    }

    @Test
    public void statusDaSolicitacaoDecideO409() {
        assertNull(WorkflowObservationService.motivoDeConflito(0));
        assertTrue(WorkflowObservationService.motivoDeConflito(1).startsWith("solicitação cancelada"));
        assertTrue(WorkflowObservationService.motivoDeConflito(2).startsWith("solicitação finalizada"));
        assertTrue(WorkflowObservationService.motivoDeConflito(7).contains("status 7"));
    }

    @Test
    public void getSemEtapaEmSolicitacaoEncerradaPedeAEtapa() {
        assertNull(WorkflowObservationService.motivoSemEtapa(0));
        assertEquals("solicitação cancelada; informe stateSequence", WorkflowObservationService.motivoSemEtapa(1));
        assertEquals("solicitação finalizada; informe stateSequence", WorkflowObservationService.motivoSemEtapa(2));
        assertTrue(WorkflowObservationService.motivoSemEtapa(9).contains("status 9"));
    }

    @Test
    public void ordenaPorDataENulosPorUltimo() {
        ProcessObservationVO b = obs(2L, new Date(2_000L));
        ProcessObservationVO a = obs(1L, new Date(1_000L));
        ProcessObservationVO semData = obs(3L, null);
        List<ProcessObservationVO> out = WorkflowObservationService.ordenar(Arrays.asList(semData, b, null, a));
        assertEquals(3, out.size());
        assertEquals(Long.valueOf(1L), out.get(0).getId());
        assertEquals(Long.valueOf(2L), out.get(1).getId());
        assertEquals(Long.valueOf(3L), out.get(2).getId());
        assertTrue(WorkflowObservationService.ordenar(null).isEmpty());
    }

    @Test
    public void dataSaiEmIsoComMilissegundosEOffset() {
        // 2026-09-17T18:03:11.000Z em UTC → 14:03:11 em -04:00.
        Date d = new Date(1_789_668_191_000L);
        assertEquals("2026-09-17T14:03:11.000-04:00",
            WorkflowObservationService.formatarData(d, ZoneId.of("-04:00")));
        assertEquals("2026-09-17T18:03:11.000Z",
            WorkflowObservationService.formatarData(d, ZoneId.of("UTC")));
        assertNull(WorkflowObservationService.formatarData(null, ZoneId.of("UTC")));
    }

    @Test
    public void threadDefaultEhZero() {
        assertEquals(0, WorkflowObservationService.threadOuDefault(null));
        assertEquals(2, WorkflowObservationService.threadOuDefault(2));
    }

    @Test
    public void voDevolvidoIncompletoEhPreenchidoComOEnviado() {
        ProcessObservationVO enviado = obs(null, new Date(5_000L));
        enviado.setProcessInstanceId(235189);
        enviado.setColleagueId("sabia");
        enviado.setStateSequence(34);
        enviado.setMovementSequence(5);
        enviado.setThreadSequence(0);
        enviado.setObservation("laudo");

        ProcessObservationVO criado = new ProcessObservationVO();
        criado.setId(123456L);

        ProcessObservationVO cheio = WorkflowObservationService.preencher(criado, enviado);
        assertEquals(Long.valueOf(123456L), cheio.getId());
        assertEquals(Integer.valueOf(235189), cheio.getProcessInstanceId());
        assertEquals("sabia", cheio.getColleagueId());
        assertEquals(Integer.valueOf(34), cheio.getStateSequence());
        assertEquals(Integer.valueOf(5), cheio.getMovementSequence());
        assertEquals(Integer.valueOf(0), cheio.getThreadSequence());
        assertEquals("laudo", cheio.getObservation());
        assertEquals(new Date(5_000L), cheio.getObservationDate());
    }

    private static ProcessInstanceInfoVO info(ProcessTaskInfoVO... tarefas) {
        ProcessInstanceInfoVO info = new ProcessInstanceInfoVO();
        info.setTasksInfo(Arrays.asList(tarefas));
        return info;
    }

    private static ProcessTaskInfoVO tarefa(int state, int movement, boolean ativa) {
        ProcessTaskInfoVO t = new ProcessTaskInfoVO();
        t.setStateSequence(state);
        t.setMovementSequence(movement);
        t.setActive(ativa);
        return t;
    }

    private static ProcessObservationVO obs(Long id, Date data) {
        ProcessObservationVO vo = new ProcessObservationVO();
        vo.setId(id);
        vo.setObservationDate(data);
        return vo;
    }

    private static String repetir(String s, int vezes) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < vezes; i++) {
            sb.append(s);
        }
        return sb.toString();
    }
}
