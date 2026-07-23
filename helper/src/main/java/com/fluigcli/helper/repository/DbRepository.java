package com.fluigcli.helper.repository;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.ResultSetMetaData;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

import javax.naming.InitialContext;
import javax.naming.NameClassPair;
import javax.naming.NamingEnumeration;
import javax.sql.DataSource;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.fluigcli.helper.dto.DbColumnDto;
import com.fluigcli.helper.dto.DbResultDto;

/**
 * Acesso JDBC genérico para o db query. Diferente dos outros repositórios, o
 * datasource JNDI é PARÂMETRO (não o /jdbc/AppDS fixo). A conexão é aberta em
 * modo read-only (defesa extra à validação de SQL do DbService).
 */
public class DbRepository {
    protected final Logger log = LoggerFactory.getLogger(getClass());

    /**
     * Raízes de naming onde os datasources costumam estar publicados, em ordem
     * de preferência da forma exibida (o mesmo datasource aparece em mais de uma
     * raiz — ex.: /jdbc/AppDS e java:/jdbc/AppDS; a dedup mantém a 1ª por folha).
     */
    private static final String[] DATASOURCE_ROOTS = {
        "/jdbc", "java:/jdbc", "java:jboss/datasources", "java:comp/env/jdbc"
    };

    public DbResultDto query(String jndi, String sql, List<String> params, int maxRows) throws Exception {
        InitialContext ic = null;
        DbResultDto result = new DbResultDto();

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(jndi);

            try (Connection conn = ds.getConnection()) {
                conn.setReadOnly(true);

                try (PreparedStatement stmt = conn.prepareStatement(sql)) {
                    // maxRows+1 para detectar truncamento sem custo de contar tudo.
                    stmt.setMaxRows(maxRows + 1);

                    if (params != null) {
                        for (int i = 0; i < params.size(); ++i) {
                            stmt.setString(i + 1, params.get(i));
                        }
                    }

                    try (ResultSet rs = stmt.executeQuery()) {
                        ResultSetMetaData meta = rs.getMetaData();
                        int cols = meta.getColumnCount();

                        for (int c = 1; c <= cols; ++c) {
                            result.getColumns().add(new DbColumnDto(
                                meta.getColumnLabel(c),
                                meta.getColumnTypeName(c)
                            ));
                        }

                        while (rs.next()) {
                            if (result.getRows().size() >= maxRows) {
                                result.setTruncated(true);
                                break;
                            }
                            List<String> row = new ArrayList<>(cols);
                            for (int c = 1; c <= cols; ++c) {
                                row.add(rs.getString(c)); // null preservado
                            }
                            result.getRows().add(row);
                        }
                    }
                }
            }
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        result.setRowCount(result.getRows().size());
        return result;
    }

    /**
     * Enumera os datasources publicados no naming (best-effort). Alguns
     * ambientes WildFly não permitem list() em todos os contextos — nesse caso
     * a lista pode vir vazia, e a CLI orienta a passar --jndi direto.
     */
    public List<String> listDatasources() {
        // Dedup por nome-folha (o mesmo datasource aparece em várias raízes);
        // mantém a 1ª forma vista, na ordem de preferência de DATASOURCE_ROOTS.
        Set<String> seenLeaf = new LinkedHashSet<>();
        List<String> out = new ArrayList<>();
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            for (String root : DATASOURCE_ROOTS) {
                try {
                    NamingEnumeration<NameClassPair> en = ic.list(root);
                    while (en.hasMore()) {
                        NameClassPair pair = en.next();
                        if (seenLeaf.add(pair.getName())) {
                            String prefix = root.endsWith("/") ? root : root + "/";
                            out.add(prefix + pair.getName());
                        }
                    }
                    en.close();
                } catch (Exception ignore) {
                    // contexto inexistente ou sem suporte a list — tenta o próximo.
                }
            }
        } catch (Exception e) {
            log.error("Erro ao enumerar datasources", e);
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        return out;
    }
}
