package com.fluigcli.helper.service;

import java.io.BufferedReader;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.RandomAccessFile;
import java.nio.charset.StandardCharsets;
import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Comparator;
import java.util.Deque;
import java.util.List;
import java.util.Locale;
import java.util.regex.Pattern;

import com.fluigcli.helper.dto.LogChunkDto;
import com.fluigcli.helper.dto.LogFileDto;
import com.fluigcli.helper.dto.LogRangeDto;
import com.fluigcli.helper.dto.LogTailDto;

/**
 * Leitura dos logs do servidor de aplicação. O diretório vem de
 * jboss.server.log.dir (definido pelo WildFly tanto em standalone quanto por
 * servidor gerenciado no modo domain); o nome do arquivo passa por whitelist
 * de caracteres + checagem de containment do caminho canônico
 * (anti-traversal), no molde validado do logfluig2.0.
 */
public class LogService {

    private static final Pattern SAFE_NAME = Pattern.compile("^[a-zA-Z0-9_.\\-]+$");

    // Linha que ABRE uma entrada de log (formato padrão do WildFly:
    // "2026-07-18 09:33:12,345 INFO  [categoria] (thread) mensagem").
    // Linhas sem esse prefixo (stack traces) são continuação da entrada anterior.
    private static final Pattern ENTRY_START = Pattern.compile("^\\d{4}-\\d{2}-\\d{2}[ T]");

    public static final int MAX_ENTRIES = 5000;
    // Arquivo fora do formato degradaria numa entrada única gigante; acima
    // deste tanto de linhas sem cabeçalho, o bloco fecha como uma entrada.
    private static final int MAX_ENTRY_LINES = 300;
    private static final long MAX_TAIL_BYTES = 2L * 1024 * 1024;
    static final int MAX_CHUNK_BYTES = 512 * 1024;
    private static final int SCAN_CHUNK = 64 * 1024;

    public File logDir() throws FileNotFoundException {
        String dir = System.getProperty("jboss.server.log.dir");
        if (dir == null || dir.isEmpty()) {
            String home = System.getProperty("jboss.home.dir");
            if (home != null && !home.isEmpty()) {
                dir = home + File.separator + "standalone" + File.separator + "log";
            }
        }
        if (dir == null || dir.isEmpty()) {
            throw new FileNotFoundException("jboss.server.log.dir não definido");
        }
        File d = new File(dir);
        if (!d.isDirectory()) {
            throw new FileNotFoundException("diretório de log inexistente: " + dir);
        }
        return d;
    }

    public List<LogFileDto> list() throws FileNotFoundException {
        File[] files = logDir().listFiles(File::isFile);
        List<LogFileDto> out = new ArrayList<>();
        if (files != null) {
            for (File f : files) {
                out.add(new LogFileDto(f.getName(), f.length(), f.lastModified()));
            }
        }
        out.sort(Comparator.comparing(LogFileDto::getName));
        return out;
    }

    public File resolve(String name) throws IOException {
        if (name == null || !SAFE_NAME.matcher(name).matches()) {
            throw new FileNotFoundException(String.valueOf(name));
        }
        File dir = logDir();
        File f = new File(dir, name);
        if (!f.getCanonicalPath().startsWith(dir.getCanonicalPath() + File.separator) || !f.isFile()) {
            throw new FileNotFoundException(name);
        }
        return f;
    }

    public FileInputStream openForDownload(String name) throws IOException {
        return new FileInputStream(resolve(name));
    }

    /**
     * Últimas entradas do arquivo, varrendo de trás para frente. Uma entrada =
     * linha com timestamp + as continuações dela (stack trace inteiro conta
     * como UMA entrada). Filtros: nível mínimo e substring (case-insensitive)
     * aplicados à entrada completa.
     */
    public LogTailDto tail(String name, int wantEntries, int skip, String level, String grep) throws IOException {
        File f = resolve(name);
        if (wantEntries < 1) {
            wantEntries = 1;
        }
        if (wantEntries > MAX_ENTRIES) {
            wantEntries = MAX_ENTRIES;
        }
        TailCollector collector = new TailCollector(wantEntries, Math.max(0, skip), levelRank(level),
            grep == null || grep.isEmpty() ? null : grep.toLowerCase(Locale.ROOT));

        long size;
        try (RandomAccessFile raf = new RandomAccessFile(f, "r")) {
            size = raf.length();
            long pos = size;
            byte[] leftover = new byte[0];
            boolean atEnd = true;

            while (pos > 0 && !collector.done()) {
                int chunkLen = (int) Math.min(SCAN_CHUNK, pos);
                pos -= chunkLen;
                byte[] buf = new byte[chunkLen + leftover.length];
                raf.seek(pos);
                raf.readFully(buf, 0, chunkLen);
                System.arraycopy(leftover, 0, buf, chunkLen, leftover.length);

                int end = buf.length;
                for (int i = buf.length - 1; i >= 0 && !collector.done(); i--) {
                    if (buf[i] != '\n') {
                        continue;
                    }
                    String line = decodeLine(buf, i + 1, end);
                    end = i;
                    if (atEnd && line.isEmpty()) {
                        // Quebra de linha final do arquivo, não uma linha vazia real.
                        atEnd = false;
                        continue;
                    }
                    atEnd = false;
                    collector.line(line);
                }
                leftover = Arrays.copyOfRange(buf, 0, end);
            }
            if (!collector.done() && leftover.length > 0) {
                collector.line(decodeLine(leftover, 0, leftover.length));
            }
            collector.finish();
        }
        return new LogTailDto(name, size, new ArrayList<>(collector.entries), collector.truncated);
    }

    /**
     * Conteúdo bruto a partir de um offset (para acompanhar o arquivo em
     * polling). Corta na última quebra de linha para nunca repartir uma linha
     * (nem um caractere multi-byte) entre chamadas — a menos que o buffer
     * encha sem nenhuma quebra (linha gigante).
     */
    public LogChunkDto read(String name, long from) throws IOException {
        File f = resolve(name);
        try (RandomAccessFile raf = new RandomAccessFile(f, "r")) {
            long size = raf.length();
            long start = Math.max(0, Math.min(from, size));
            int len = (int) Math.min(MAX_CHUNK_BYTES, size - start);
            byte[] buf = new byte[len];
            if (len > 0) {
                raf.seek(start);
                raf.readFully(buf);
            }
            int cut = len;
            if (len > 0) {
                int lastNl = -1;
                for (int i = len - 1; i >= 0; i--) {
                    if (buf[i] == '\n') {
                        lastNl = i;
                        break;
                    }
                }
                if (lastNl >= 0) {
                    cut = lastNl + 1;
                } else if (len < MAX_CHUNK_BYTES) {
                    cut = 0; // linha ainda incompleta — espera a quebra chegar
                }
            }
            String content = new String(buf, 0, cut, StandardCharsets.UTF_8);
            return new LogChunkDto(name, start, start + cut, size, content);
        }
    }

    // Formato do timestamp do WildFly, até os segundos: "yyyy-MM-dd HH:mm:ss".
    // O log não carrega fuso — a comparação é toda em hora local do servidor,
    // e o cliente já converte os limites [from,to] para essa hora.
    private static final DateTimeFormatter TS_FMT =
        DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss");

    /**
     * Entradas cujo timestamp cai em [from, to], varrendo o arquivo para frente.
     * O log é (aproximadamente) ordenado no tempo, então a varredura PARA ao
     * passar de `to` — sem ler o arquivo inteiro à toa. from/to no formato
     * "yyyy-MM-dd HH:mm:ss" (ou com 'T'; segundos opcionais); vazios = sem
     * limite naquele lado. Mesmos filtros e tetos do tail (nível/substring,
     * MAX_ENTRIES/2 MB → truncated). Entradas sem timestamp reconhecível não
     * podem ser situadas no tempo e são ignoradas.
     */
    public LogRangeDto range(String name, String from, String to, String level, String grep) throws IOException {
        File f = resolve(name);
        LocalDateTime lo = parseBound(from, false);
        LocalDateTime hi = parseBound(to, true);
        RangeCollector c = new RangeCollector(lo, hi, levelRank(level),
            grep == null || grep.isEmpty() ? null : grep.toLowerCase(Locale.ROOT));

        try (BufferedReader br = new BufferedReader(
                new InputStreamReader(new FileInputStream(f), StandardCharsets.UTF_8))) {
            String line;
            while (!c.done() && (line = br.readLine()) != null) {
                c.line(line);
            }
            c.finish();
        }
        return new LogRangeDto(name, from, to, c.entries, c.truncated);
    }

    // Limite [from|to]: aceita "yyyy-MM-dd['T'| ]HH:mm[:ss]"; sem segundos, o
    // início vira :00 e o fim :59 (intervalo do minuto inclusivo). null = vazio.
    static LocalDateTime parseBound(String s, boolean end) {
        if (s == null || s.trim().isEmpty()) {
            return null;
        }
        String v = s.trim().replace('T', ' ');
        if (v.length() == 16) { // "yyyy-MM-dd HH:mm" sem segundos
            v += end ? ":59" : ":00";
        }
        try {
            return LocalDateTime.parse(v.substring(0, Math.min(19, v.length())), TS_FMT);
        } catch (Exception e) {
            return null;
        }
    }

    // Timestamp da primeira linha da entrada (os 19 primeiros caracteres), ou
    // null se a linha não começa com um timestamp reconhecível.
    static LocalDateTime parseEntryTime(String head) {
        if (head == null || head.length() < 19) {
            return null;
        }
        String s = head.substring(0, 19).replace('T', ' ');
        try {
            return LocalDateTime.parse(s, TS_FMT);
        } catch (Exception e) {
            return null;
        }
    }

    private static String decodeLine(byte[] buf, int start, int end) {
        if (end > start && buf[end - 1] == '\r') {
            end--;
        }
        return new String(buf, start, end - start, StandardCharsets.UTF_8);
    }

    // Recebe linhas da mais NOVA para a mais antiga e monta as entradas.
    static final class TailCollector {
        final Deque<String> entries = new ArrayDeque<>();
        boolean truncated;

        private final int want;
        private final Integer minLevel;
        private final String needle;
        private int toSkip;
        private long bytes;
        private boolean stopped;
        private final List<String> pending = new ArrayList<>();

        TailCollector(int want, int skip, Integer minLevel, String needle) {
            this.want = want;
            this.toSkip = skip;
            this.minLevel = minLevel;
            this.needle = needle;
        }

        boolean done() {
            return stopped;
        }

        void line(String line) {
            if (stopped) {
                return;
            }
            if (ENTRY_START.matcher(line).find()) {
                offer(join(line, pending));
                pending.clear();
            } else {
                pending.add(line);
                if (pending.size() >= MAX_ENTRY_LINES) {
                    flushPendingAsLines();
                }
            }
        }

        void finish() {
            flushPendingAsLines();
        }

        // Continuações que nunca acharam um cabeçalho (arquivo fora do formato
        // ou começo do arquivo): cada linha vira uma entrada própria.
        private void flushPendingAsLines() {
            for (int i = 0; i < pending.size() && !stopped; i++) {
                offer(pending.get(i));
            }
            pending.clear();
        }

        private void offer(String entry) {
            if (!matches(entry)) {
                return;
            }
            if (toSkip > 0) {
                toSkip--;
                return;
            }
            if (bytes + entry.length() > MAX_TAIL_BYTES && !entries.isEmpty()) {
                truncated = true;
                stopped = true;
                return;
            }
            entries.addFirst(entry);
            bytes += entry.length();
            if (entries.size() >= want) {
                stopped = true;
            }
        }

        private boolean matches(String entry) {
            if (needle != null && !entry.toLowerCase(Locale.ROOT).contains(needle)) {
                return false;
            }
            if (minLevel != null) {
                Integer rank = entryLevel(entry);
                return rank != null && rank >= minLevel;
            }
            return true;
        }

        // pending guarda as continuações da mais nova para a mais antiga.
        private static String join(String head, List<String> pending) {
            StringBuilder sb = new StringBuilder();
            if (head != null) {
                sb.append(head);
            }
            for (int i = pending.size() - 1; i >= 0; i--) {
                if (sb.length() > 0) {
                    sb.append('\n');
                }
                sb.append(pending.get(i));
            }
            return sb.toString();
        }
    }

    // Varre para FRENTE: agrupa cada entrada (cabeçalho + continuações na
    // ordem) e coleta as que caem no intervalo. Para ao passar do fim.
    static final class RangeCollector {
        final List<String> entries = new ArrayList<>();
        boolean truncated;

        private final LocalDateTime lo;
        private final LocalDateTime hi;
        private final Integer minLevel;
        private final String needle;
        private long bytes;
        private boolean stopped;
        private String head;
        private final List<String> cont = new ArrayList<>();

        RangeCollector(LocalDateTime lo, LocalDateTime hi, Integer minLevel, String needle) {
            this.lo = lo;
            this.hi = hi;
            this.minLevel = minLevel;
            this.needle = needle;
        }

        boolean done() {
            return stopped;
        }

        void line(String line) {
            if (stopped) {
                return;
            }
            if (ENTRY_START.matcher(line).find()) {
                flush();
                head = line;
                cont.clear();
            } else if (head != null && cont.size() < MAX_ENTRY_LINES) {
                cont.add(line);
            }
            // Continuação antes de qualquer cabeçalho: sem timestamp, ignorada.
        }

        void finish() {
            flush();
        }

        private void flush() {
            if (head == null) {
                return;
            }
            LocalDateTime ts = parseEntryTime(head);
            String h = head;
            head = null;
            if (ts == null) {
                return; // sem timestamp: não dá para situar no tempo
            }
            if (lo != null && ts.isBefore(lo)) {
                return; // antes do intervalo: segue varrendo
            }
            if (hi != null && ts.isAfter(hi)) {
                stopped = true; // passou do fim: o log é ordenado, encerra
                return;
            }
            String entry = join(h, cont);
            if (!matches(entry)) {
                return;
            }
            if (bytes + entry.length() > MAX_TAIL_BYTES && !entries.isEmpty()) {
                truncated = true;
                stopped = true;
                return;
            }
            entries.add(entry);
            bytes += entry.length();
            if (entries.size() >= MAX_ENTRIES) {
                truncated = true;
                stopped = true;
            }
        }

        private boolean matches(String entry) {
            if (needle != null && !entry.toLowerCase(Locale.ROOT).contains(needle)) {
                return false;
            }
            if (minLevel != null) {
                Integer rank = entryLevel(entry);
                return rank != null && rank >= minLevel;
            }
            return true;
        }

        // Continuações na ordem natural (varredura para frente).
        private static String join(String head, List<String> cont) {
            StringBuilder sb = new StringBuilder(head);
            for (String line : cont) {
                sb.append('\n').append(line);
            }
            return sb.toString();
        }
    }

    static Integer levelRank(String token) {
        if (token == null) {
            return null;
        }
        switch (token.toUpperCase(Locale.ROOT)) {
            case "TRACE": case "FINEST": case "FINER":
                return 0;
            case "DEBUG": case "FINE":
                return 1;
            case "INFO": case "CONFIG":
                return 2;
            case "WARN": case "WARNING":
                return 3;
            case "ERROR": case "SEVERE":
                return 4;
            case "FATAL":
                return 5;
            default:
                return null;
        }
    }

    // Nível da entrada: primeiro token reconhecível entre os 4 primeiros da
    // primeira linha ("data hora NÍVEL [categoria] ..." no formato padrão).
    static Integer entryLevel(String entry) {
        int eol = entry.indexOf('\n');
        String first = eol < 0 ? entry : entry.substring(0, eol);
        String[] tokens = first.trim().split("\\s+", 5);
        for (int i = 0; i < tokens.length && i < 4; i++) {
            Integer rank = levelRank(tokens[i]);
            if (rank != null) {
                return rank;
            }
        }
        return null;
    }
}
