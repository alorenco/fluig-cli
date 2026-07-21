package com.fluigcli.helper.service;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.List;

import org.junit.After;
import org.junit.Before;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

import com.fluigcli.helper.dto.LogChunkDto;
import com.fluigcli.helper.dto.LogFileDto;
import com.fluigcli.helper.dto.LogTailDto;

public class LogServiceTest {

    @Rule
    public TemporaryFolder tmp = new TemporaryFolder();

    private String oldLogDir;
    private final LogService service = new LogService();

    @Before
    public void setUp() {
        oldLogDir = System.getProperty("jboss.server.log.dir");
        System.setProperty("jboss.server.log.dir", tmp.getRoot().getAbsolutePath());
    }

    @After
    public void tearDown() {
        if (oldLogDir == null) {
            System.clearProperty("jboss.server.log.dir");
        } else {
            System.setProperty("jboss.server.log.dir", oldLogDir);
        }
    }

    private File write(String name, String content) throws IOException {
        File f = new File(tmp.getRoot(), name);
        Files.write(f.toPath(), content.getBytes(StandardCharsets.UTF_8));
        return f;
    }

    @Test
    public void listaArquivosDoDiretorio() throws Exception {
        write("server.log", "a\n");
        write("server.log.2026-07-17", "b\n");
        tmp.newFolder("subdir");
        List<LogFileDto> files = service.list();
        assertEquals(2, files.size());
        assertEquals("server.log", files.get(0).getName());
        assertEquals(2, files.get(0).getSize());
    }

    @Test
    public void tailUltimasEntradasNaOrdem() throws Exception {
        StringBuilder sb = new StringBuilder();
        for (int i = 1; i <= 10; i++) {
            sb.append("2026-07-18 09:00:0").append(i % 10).append(",000 INFO  [c] (t) linha ").append(i).append('\n');
        }
        write("server.log", sb.toString());
        LogTailDto tail = service.tail("server.log", 3, 0, null, null);
        assertEquals(3, tail.getEntries().size());
        assertTrue(tail.getEntries().get(0).endsWith("linha 8"));
        assertTrue(tail.getEntries().get(2).endsWith("linha 10"));
        assertEquals(sb.length(), tail.getSize());
        assertFalse(tail.isTruncated());
    }

    @Test
    public void tailAgrupaStackTraceComoUmaEntrada() throws Exception {
        String content = "2026-07-18 09:00:01,000 INFO  [c] (t) tudo bem\n"
            + "2026-07-18 09:00:02,000 ERROR [c] (t) quebrou\n"
            + "java.lang.RuntimeException: pum\n"
            + "\tat com.example.Foo.bar(Foo.java:1)\n"
            + "\tat com.example.Foo.baz(Foo.java:2)\n"
            + "2026-07-18 09:00:03,000 INFO  [c] (t) seguiu\n";
        write("server.log", content);
        LogTailDto tail = service.tail("server.log", 2, 0, null, null);
        assertEquals(2, tail.getEntries().size());
        String erro = tail.getEntries().get(0);
        assertTrue(erro.startsWith("2026-07-18 09:00:02,000 ERROR"));
        assertTrue(erro.contains("Foo.java:2"));
        assertEquals(4, erro.split("\n").length);
        assertTrue(tail.getEntries().get(1).endsWith("seguiu"));
    }

    @Test
    public void tailFiltraPorNivelMinimo() throws Exception {
        String content = "2026-07-18 09:00:01,000 DEBUG [c] (t) detalhe\n"
            + "2026-07-18 09:00:02,000 INFO  [c] (t) info\n"
            + "2026-07-18 09:00:03,000 WARN  [c] (t) atencao\n"
            + "2026-07-18 09:00:04,000 ERROR [c] (t) erro\n";
        write("server.log", content);
        LogTailDto tail = service.tail("server.log", 100, 0, "warn", null);
        assertEquals(2, tail.getEntries().size());
        assertTrue(tail.getEntries().get(0).endsWith("atencao"));
        assertTrue(tail.getEntries().get(1).endsWith("erro"));
    }

    @Test
    public void tailFiltraPorSubstringCaseInsensitive() throws Exception {
        String content = "2026-07-18 09:00:01,000 INFO  [c] (t) Deploy da Widget X\n"
            + "2026-07-18 09:00:02,000 INFO  [c] (t) outra coisa\n"
            + "2026-07-18 09:00:03,000 ERROR [c] (t) falhou\n"
            + "na widget Y\n";
        write("server.log", content);
        LogTailDto tail = service.tail("server.log", 100, 0, null, "WIDGET");
        assertEquals(2, tail.getEntries().size());
        assertTrue(tail.getEntries().get(0).contains("Widget X"));
        // O grep casa na continuação: a entrada inteira volta.
        assertTrue(tail.getEntries().get(1).contains("falhou"));
    }

    @Test
    public void tailComSkipPaginaParaTras() throws Exception {
        StringBuilder sb = new StringBuilder();
        for (int i = 1; i <= 5; i++) {
            sb.append("2026-07-18 09:00:00,000 INFO  [c] (t) linha ").append(i).append('\n');
        }
        write("server.log", sb.toString());
        LogTailDto tail = service.tail("server.log", 2, 2, null, null);
        assertEquals(2, tail.getEntries().size());
        assertTrue(tail.getEntries().get(0).endsWith("linha 2"));
        assertTrue(tail.getEntries().get(1).endsWith("linha 3"));
    }

    @Test
    public void tailVarreArquivoMaiorQueUmChunk() throws Exception {
        // > 64 KB para forçar mais de um chunk da varredura reversa.
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < 2000; i++) {
            sb.append("2026-07-18 09:00:00,000 INFO  [categoria.comprida.pra.encher] (thread-1) entrada numero ")
                .append(i).append(" com recheio para ocupar espaço\n");
        }
        write("server.log", sb.toString());
        LogTailDto tail = service.tail("server.log", 2, 0, null, null);
        assertEquals(2, tail.getEntries().size());
        assertTrue(tail.getEntries().get(1).contains("entrada numero 1999"));
    }

    @Test
    public void tailArquivoForaDoFormatoTrataLinhaComoEntrada() throws Exception {
        write("console.log", "primeira\nsegunda\nterceira\n");
        LogTailDto tail = service.tail("console.log", 2, 0, null, null);
        assertEquals(2, tail.getEntries().size());
        assertEquals("segunda", tail.getEntries().get(0));
        assertEquals("terceira", tail.getEntries().get(1));
    }

    @Test
    public void readDevolveAPartirDoOffsetECortaNaQuebra() throws Exception {
        write("server.log", "abc\ndef\nparcial");
        LogChunkDto chunk = service.read("server.log", 0);
        assertEquals("abc\ndef\n", chunk.getContent());
        assertEquals(0, chunk.getFrom());
        assertEquals(8, chunk.getTo());
        assertEquals(15, chunk.getSize());

        // A linha incompleta espera a quebra chegar.
        LogChunkDto resto = service.read("server.log", chunk.getTo());
        assertEquals("", resto.getContent());
        assertEquals(8, resto.getTo());

        write("server.log", "abc\ndef\nparcial agora completa\n");
        LogChunkDto completo = service.read("server.log", chunk.getTo());
        assertEquals("parcial agora completa\n", completo.getContent());
    }

    @Test
    public void resolveRejeitaTraversalENomesInvalidos() throws Exception {
        write("server.log", "x\n");
        for (String name : new String[]{"../server.log", "..", "a/b", "a\\b", "", "server log"}) {
            try {
                service.resolve(name);
                fail("aceitou nome inválido: " + name);
            } catch (FileNotFoundException expected) {
                // ok
            }
        }
        assertEquals("server.log", service.resolve("server.log").getName());
    }

    @Test
    public void tailArquivoInexistenteFalha() throws Exception {
        try {
            service.tail("nao-existe.log", 10, 0, null, null);
            fail("deveria falhar");
        } catch (FileNotFoundException expected) {
            // ok
        }
    }

    @Test
    public void niveisReconhecidos() {
        assertEquals(Integer.valueOf(3), LogService.levelRank("warn"));
        assertEquals(Integer.valueOf(3), LogService.levelRank("WARNING"));
        assertEquals(Integer.valueOf(4), LogService.levelRank("SEVERE"));
        assertEquals(null, LogService.levelRank("qualquer"));
        assertEquals(Integer.valueOf(4),
            LogService.entryLevel("2026-07-18 09:00:02,000 ERROR [c] (t) quebrou\ndetalhe"));
        assertEquals(null, LogService.entryLevel("linha sem nivel nenhum aqui"));
    }

    @Test
    public void rangeSoAsEntradasDoIntervalo() throws Exception {
        StringBuilder sb = new StringBuilder();
        for (int h = 8; h <= 12; h++) {
            sb.append("2026-07-18 ").append(String.format("%02d", h))
              .append(":00:00,000 INFO  [c] (t) hora ").append(h).append('\n');
        }
        write("server.log", sb.toString());
        // [09:00, 11:00] inclusivo => horas 9, 10, 11
        List<String> e = service.range("server.log",
            "2026-07-18T09:00:00", "2026-07-18T11:00:00", null, null).getEntries();
        assertEquals(3, e.size());
        assertTrue(e.get(0).contains("hora 9"));
        assertTrue(e.get(2).contains("hora 11"));
    }

    @Test
    public void rangeAgrupaContinuacoesEParaNoFim() throws Exception {
        String content =
            "2026-07-18 09:00:00,000 ERROR [c] (t) falhou\n" +
            "\tat com.exemplo.Foo.bar(Foo.java:1)\n" +
            "\tat com.exemplo.Baz.qux(Baz.java:2)\n" +
            "2026-07-18 10:00:00,000 INFO  [c] (t) depois do intervalo\n";
        write("server.log", content);
        List<String> e = service.range("server.log",
            "2026-07-18T08:00:00", "2026-07-18T09:30:00", null, null).getEntries();
        assertEquals(1, e.size());
        // stack trace inteiro numa entrada só
        assertTrue(e.get(0).contains("falhou"));
        assertTrue(e.get(0).contains("Foo.java:1"));
        assertTrue(e.get(0).contains("Baz.java:2"));
    }

    @Test
    public void rangeFiltraNivelELimitesVazios() throws Exception {
        String content =
            "2026-07-18 09:00:00,000 INFO  [c] (t) um\n" +
            "2026-07-18 09:00:01,000 ERROR [c] (t) dois\n" +
            "2026-07-18 09:00:02,000 WARN  [c] (t) tres\n";
        write("server.log", content);
        // sem limites (from/to vazios) + nível mínimo WARN => ERROR e WARN
        List<String> e = service.range("server.log", "", "", "warn", null).getEntries();
        assertEquals(2, e.size());
        assertTrue(e.get(0).contains("dois"));
        assertTrue(e.get(1).contains("tres"));
    }

    @Test
    public void parseBoundPreencheSegundos() {
        // toString() omite os segundos quando são :00
        assertEquals("2026-07-18T09:30", LogService.parseBound("2026-07-18T09:30", false).toString());
        assertEquals("2026-07-18T09:30:59", LogService.parseBound("2026-07-18 09:30", true).toString());
        assertEquals(null, LogService.parseBound("", false));
        assertEquals(null, LogService.parseBound(null, true));
    }
}
