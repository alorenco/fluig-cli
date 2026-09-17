package com.fluigcli.helper.repository;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;

import javax.naming.InitialContext;
import javax.sql.DataSource;

/**
 * Leitura do cabeçalho da solicitação (PROCES_WORKFLOW) pelo datasource do
 * Fluig, no mesmo mecanismo do WorkflowRepository.
 *
 * Por que banco, e não só o SDK: o getActiveTasks não separa "não existe" de
 * "existe, mas está finalizada". Uma linha em PROCES_WORKFLOW prova a
 * existência, e o STATUS diz se ainda está em aberto. Valores medidos na
 * homologação em 2026-08-10 (docs/db.md): 0 = em aberto, 1 = cancelada,
 * 2 = concluída.
 */
public class ProcessInstanceRepository extends BaseRepository {

    public static final int STATUS_ABERTA = 0;
    public static final int STATUS_CANCELADA = 1;
    public static final int STATUS_CONCLUIDA = 2;

    /**
     * Devolve o STATUS da solicitação, ou null quando ela não existe no tenant.
     */
    public Integer findStatus(long tenantId, int processInstanceId) throws Exception {
        InitialContext ic = null;
        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                    "SELECT STATUS FROM PROCES_WORKFLOW WHERE COD_EMPRESA = ? AND NUM_PROCES = ?"
                )
            ) {
                stmt.setLong(1, tenantId);
                stmt.setInt(2, processInstanceId);

                try (ResultSet rs = stmt.executeQuery()) {
                    if (rs.next()) {
                        return rs.getInt("STATUS");
                    }
                }
            }
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }
        return null;
    }
}
