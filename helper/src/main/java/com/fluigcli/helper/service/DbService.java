package com.fluigcli.helper.service;

import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import com.fluigcli.helper.dto.DbResultDto;
import com.fluigcli.helper.repository.DbRepository;

/**
 * Regras do db query: impõe read-only (só SELECT/WITH, uma instrução) e resolve
 * o teto de linhas. A validação é textual e pragmática — cobre o uso de
 * diagnóstico. A conexão ainda é aberta em read-only no repositório (defesa em
 * profundidade).
 */
public class DbService {
    public static final int DEFAULT_MAX_ROWS = 500;
    public static final int MAX_MAX_ROWS = 10_000;

    private static final Pattern FIRST_WORD = Pattern.compile("^\\s*([A-Za-z]+)");

    private final DbRepository repository;

    public DbService() {
        this(new DbRepository());
    }

    public DbService(DbRepository repository) {
        this.repository = repository;
    }

    /**
     * Valida a política read-only e devolve o SQL limpo (sem `;` final).
     * Lança IllegalArgumentException com mensagem pt-BR quando reprova.
     */
    public static String sanitizeReadOnly(String sql) {
        if (sql == null || sql.isBlank()) {
            throw new IllegalArgumentException("Necessário informar o SQL");
        }

        String cleaned = sql.trim();
        // Remove um único `;` final (e espaços) — permitido por conveniência.
        while (cleaned.endsWith(";")) {
            cleaned = cleaned.substring(0, cleaned.length() - 1).trim();
        }
        if (cleaned.contains(";")) {
            throw new IllegalArgumentException("Somente uma instrução por consulta (não use `;`)");
        }

        Matcher m = FIRST_WORD.matcher(cleaned);
        String verb = m.find() ? m.group(1).toUpperCase() : "";
        if (!verb.equals("SELECT") && !verb.equals("WITH")) {
            throw new IllegalArgumentException(
                "Somente consultas de leitura são permitidas (SELECT ou WITH)"
            );
        }

        return cleaned;
    }

    /** Resolve o teto de linhas: <=0 vira o default; acima do máximo, o máximo. */
    public static int resolveMaxRows(int requested) {
        if (requested <= 0) {
            return DEFAULT_MAX_ROWS;
        }
        return Math.min(requested, MAX_MAX_ROWS);
    }

    public DbResultDto query(String jndi, String sql, List<String> params, int maxRows) throws Exception {
        String cleaned = sanitizeReadOnly(sql);
        int limit = resolveMaxRows(maxRows);
        return repository.query(jndi, cleaned, params, limit);
    }

    public List<String> datasources() {
        return repository.listDatasources();
    }
}
