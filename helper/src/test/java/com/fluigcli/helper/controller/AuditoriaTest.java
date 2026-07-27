package com.fluigcli.helper.controller;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;
import java.util.Collections;

import org.junit.Test;

import com.fluigcli.helper.dto.WorkflowEventDto;

/**
 * Formatação das linhas de auditoria (ROADMAP §2.11-D). Antes deste item, as
 * operações mais destrutivas — consulta SQL, exclusão física de dataset e
 * gravação de evento de processo — não deixavam nenhum rastro em caso de
 * sucesso.
 */
public class AuditoriaTest {

    @Test
    public void resumoDoSqlFicaNumaLinhaSo() {
        assertEquals(
            "SELECT 1 FROM t WHERE a = 1",
            DbController.resumo("SELECT 1\n  FROM t\n  WHERE a = 1")
        );
        assertEquals("", DbController.resumo(null));
        assertEquals("", DbController.resumo("   "));
    }

    @Test
    public void resumoDoSqlTemTeto() {
        String gigante = "SELECT " + repetir("x", 900);
        String resumo = DbController.resumo(gigante);

        assertTrue("deve marcar o corte", resumo.endsWith("… (truncado)"));
        assertTrue("não deve despejar a consulta inteira", resumo.length() < gigante.length());
        assertEquals(500 + "… (truncado)".length(), resumo.length());
    }

    @Test
    public void auditoriaDeEventosLevaOsNomesNaoOCodigo() {
        String nomes = WorkflowController.nomesDosEventos(Arrays.asList(
            evento("beforeTaskSave", "var senha = 'segredo';"),
            evento("afterProcessFinish", "log.info('fim');")
        ));

        assertEquals("beforeTaskSave, afterProcessFinish", nomes);
        assertFalse("o código do evento não pode ir para o log", nomes.contains("segredo"));
    }

    /**
     * O `log tail --follow` faz polling de segundo em segundo. A auditoria da
     * leitura precisa cobrir a sessão sem inundar o server.log com a leitura do
     * próprio server.log.
     */
    @Test
    public void leituraEmPollingRegistraUmaVezPorJanela() {
        Map<String, Long> estado = new HashMap<>();
        String chave = "jsilva|server.log";
        long janela = 60_000L;
        long t0 = 1_000_000L;

        assertTrue("a primeira leitura registra", LogController.deveRegistrar(estado, chave, t0, janela));

        // 59 chamadas de polling DENTRO da janela: nenhuma linha nova.
        for (int i = 1; i <= 59; i++) {
            assertFalse(
                "polling dentro da janela não pode registrar (chamada " + i + ")",
                LogController.deveRegistrar(estado, chave, t0 + i * 1000L, janela)
            );
        }

        // A borda conta como janela cumprida: em t0 + janela já registra.
        assertTrue("na borda da janela, registra de novo",
            LogController.deveRegistrar(estado, chave, t0 + janela, janela));
    }

    @Test
    public void janelaEhPorUsuarioEArquivo() {
        Map<String, Long> estado = new HashMap<>();
        long janela = 60_000L;
        long t = 1_000_000L;

        assertTrue(LogController.deveRegistrar(estado, "jsilva|server.log", t, janela));
        assertTrue("outro usuário tem janela própria",
            LogController.deveRegistrar(estado, "msouza|server.log", t, janela));
        assertTrue("outro arquivo tem janela própria",
            LogController.deveRegistrar(estado, "jsilva|audit.log", t, janela));
        assertFalse(LogController.deveRegistrar(estado, "jsilva|server.log", t + 1, janela));
    }

    @Test
    public void estadoDaAuditoriaNaoCresceSemLimite() {
        Map<String, Long> estado = new HashMap<>();
        long janela = 60_000L;

        // 600 chaves velhas, todas fora da janela quando a próxima chega.
        for (int i = 0; i < 600; i++) {
            LogController.deveRegistrar(estado, "u" + i + "|f.log", 1_000L, janela);
        }
        LogController.deveRegistrar(estado, "novo|f.log", 1_000L + janela, janela);

        assertTrue("as chaves vencidas têm de ser podadas, sobrando só a nova",
            estado.size() <= 1);
    }

    @Test
    public void auditoriaDeEventosComListaDeUmSo() {
        assertEquals("único", WorkflowController.nomesDosEventos(
            Collections.singletonList(evento("único", "//"))));
    }

    private WorkflowEventDto evento(String nome, String conteudo) {
        WorkflowEventDto e = new WorkflowEventDto();
        e.setName(nome);
        e.setContents(conteudo);
        return e;
    }

    private static String repetir(String s, int vezes) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < vezes; i++) {
            sb.append(s);
        }
        return sb.toString();
    }
}
