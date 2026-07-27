package com.fluigcli.helper.service;

import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import com.fluigcli.helper.dto.DbResultDto;
import com.fluigcli.helper.repository.DbRepository;

/**
 * Regras do db query: impõe read-only (só SELECT/WITH, uma instrução, nenhum
 * comando de escrita em qualquer posição) e resolve o teto de linhas.
 *
 * A validação é textual e pragmática — cobre o uso de diagnóstico. Ela é
 * **guarda-corpo, não fronteira de segurança**: a fronteira real é o grant do
 * usuário do datasource no banco. Quem precisa de garantia aponta o `--jndi`
 * para um datasource somente leitura.
 *
 * Não conte com o `setReadOnly(true)` da conexão: no driver da Microsoft ele é
 * no-op, e escrita passa. Isso foi medido na homologação em 2026-07-27, quando
 * um `WITH ... UPDATE ... OUTPUT` executou (ROADMAP §2.11-C). Por isso a
 * validação textual é a defesa que de fato atua em SQL Server.
 */
public class DbService {
    public static final int DEFAULT_MAX_ROWS = 500;
    public static final int MAX_MAX_ROWS = 10_000;

    private static final Pattern FIRST_WORD = Pattern.compile("^\\s*([A-Za-z]+)");

    /**
     * Comandos que escrevem, em qualquer posição da instrução. `INTO` entra
     * porque `SELECT ... INTO` cria tabela com a primeira palavra permitida.
     * A fronteira de palavra preserva identificadores como `updated_at` e
     * `deleted_by`, que não casam.
     */
    private static final Pattern ESCRITA = Pattern.compile(
        "(?i)\\b(INSERT|UPDATE|DELETE|MERGE|DROP|ALTER|CREATE|TRUNCATE"
        + "|GRANT|REVOKE|EXEC|EXECUTE|CALL|INTO)\\b"
    );

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
     *
     * A checagem olha a instrução INTEIRA, não só a primeira palavra. Medido ao
     * vivo na homologação (SQL Server 2017) em 2026-07-27: olhar só a primeira
     * palavra deixava passar escrita, porque o T-SQL aceita DML depois de um
     * CTE e `SELECT ... INTO` cria tabela (ROADMAP §2.11-C):
     *
     *   WITH x AS (SELECT 1 AS n) UPDATE t SET c = c OUTPUT deleted.id WHERE 1=0
     *   WITH x AS (SELECT 1 AS n) SELECT n INTO #tmp FROM x
     *
     * Literais, comentários e identificadores entre colchetes ou aspas saem
     * antes da varredura. Sem isso, `WHERE nome = 'update'` ou uma coluna
     * `[delete]` viraria falso positivo.
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

        // Só o código conta: `;` dentro de literal não separa instrução, e
        // palavra dentro de literal/comentário não é comando.
        String codigo = semLiteraisEComentarios(cleaned);

        if (codigo.contains(";")) {
            throw new IllegalArgumentException("Somente uma instrução por consulta (não use `;`)");
        }

        Matcher m = FIRST_WORD.matcher(cleaned);
        String verb = m.find() ? m.group(1).toUpperCase() : "";
        if (!verb.equals("SELECT") && !verb.equals("WITH")) {
            throw new IllegalArgumentException(
                "Somente consultas de leitura são permitidas (SELECT ou WITH)"
            );
        }

        Matcher escrita = ESCRITA.matcher(codigo);
        if (escrita.find()) {
            throw new IllegalArgumentException(
                "Instrução de escrita não é permitida: \"" + escrita.group(1).toUpperCase()
                + "\". Somente leitura (SELECT ou WITH)"
            );
        }

        return cleaned;
    }

    /**
     * Devolve só o "código" do SQL: troca literais `'...'`, comentários (`--` e
     * bloco) e identificadores `[...]`/`"..."` por espaço. O tamanho não é
     * preservado, e nem precisa — o resultado só alimenta as checagens de
     * política, nunca o banco (quem vai para o banco é o SQL original).
     */
    static String semLiteraisEComentarios(String sql) {
        StringBuilder out = new StringBuilder(sql.length());
        int i = 0;
        int n = sql.length();

        while (i < n) {
            char c = sql.charAt(i);

            // Literal de string: '' escapa a aspa simples dentro do literal.
            if (c == '\'') {
                i++;
                while (i < n) {
                    if (sql.charAt(i) == '\'') {
                        if (i + 1 < n && sql.charAt(i + 1) == '\'') {
                            i += 2;
                            continue;
                        }
                        i++;
                        break;
                    }
                    i++;
                }
                out.append(' ');
                continue;
            }

            // Identificador entre colchetes (T-SQL) ou aspas duplas (padrão).
            if (c == '[' || c == '"') {
                char fim = c == '[' ? ']' : '"';
                i++;
                while (i < n && sql.charAt(i) != fim) {
                    i++;
                }
                i++; // consome o fechamento
                out.append(' ');
                continue;
            }

            // Comentário de linha.
            if (c == '-' && i + 1 < n && sql.charAt(i + 1) == '-') {
                while (i < n && sql.charAt(i) != '\n') {
                    i++;
                }
                out.append(' ');
                continue;
            }

            // Comentário de bloco.
            if (c == '/' && i + 1 < n && sql.charAt(i + 1) == '*') {
                i += 2;
                while (i + 1 < n && !(sql.charAt(i) == '*' && sql.charAt(i + 1) == '/')) {
                    i++;
                }
                i += 2;
                out.append(' ');
                continue;
            }

            out.append(c);
            i++;
        }

        return out.toString();
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
