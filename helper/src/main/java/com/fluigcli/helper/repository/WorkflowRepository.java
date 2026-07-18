package com.fluigcli.helper.repository;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

import javax.naming.InitialContext;
import javax.sql.DataSource;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.fluigcli.helper.dto.SuccessesAndErrorsDto;
import com.fluigcli.helper.dto.WorkflowEventDto;
import com.fluigcli.helper.exception.WorkflowNotFoundException;

public class WorkflowRepository extends BaseRepository {
    protected final Logger log = LoggerFactory.getLogger(getClass());

    public int findMaxVersion(long tenantId, String processId) throws Exception {
        InitialContext ic = null;
        int version = 0;

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                    "SELECT MAX(NUM_VERS) AS MAX_VERSION "
                    + "FROM VERS_DEF_PROCES "
                    + "WHERE COD_EMPRESA = ? AND COD_DEF_PROCES = ?"
                )
            ) {
                stmt.setLong(1, tenantId);
                stmt.setString(2, processId);

                try (ResultSet rs = stmt.executeQuery()) {
                    if (rs.next()) {
                        version = rs.getInt("MAX_VERSION");
                    }
                }
            }
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        if (version == 0) {
            throw new WorkflowNotFoundException(processId);
        }

        return version;
    }

    public Set<String> getEventsFromWorkflow(long tenantId, String processId, int version) {
        Set<String> events = new HashSet<>();
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                    "SELECT COD_EVENT AS EVENT "
                    + "FROM event_proces "
                    + "WHERE COD_EMPRESA = ? AND COD_DEF_PROCES = ? AND NUM_VERS = ?"
                )
            ) {
                stmt.setLong(1, tenantId);
                stmt.setString(2, processId);
                stmt.setInt(3, version);

                try (ResultSet rs = stmt.executeQuery()) {
                    while (rs.next()) {
                        events.add(rs.getString("EVENT"));
                    }
                }
            }
        } catch (Exception e) {
            log.error("Erro não identificado ao buscar eventos do processo \"" + processId + "\"", e);
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        return events;
    }

    public SuccessesAndErrorsDto updateEvents(
        long tenantId,
        String processId,
        int version,
        List<WorkflowEventDto> events
    ) {
        var result = new SuccessesAndErrorsDto();
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                    "UPDATE event_proces "
                    + "SET DSL_EVENT = ? "
                    + "WHERE "
                        + "COD_EMPRESA = ? "
                        + "AND COD_DEF_PROCES = ? "
                        + "AND COD_EVENT = ? "
                        + "AND NUM_VERS = ? "
                )
            ) {
                conn.setAutoCommit(false);

                stmt.setLong(2, tenantId);
                stmt.setString(3, processId);
                stmt.setInt(5, version);

                for (WorkflowEventDto event : events) {
                    stmt.setString(1, event.getContents());
                    stmt.setString(4, event.getName());

                    stmt.addBatch();
                }

                int[] updatedRows = stmt.executeBatch();
                conn.commit();

                for (var i = 0; i < updatedRows.length; ++i) {
                    if (updatedRows[i] == 0) {
                        result.addError(events.get(i).getName() + " não foi atualizado.");
                        continue;
                    }

                    result.addSuccess(events.get(i).getName() + " foi atualizado.");
                }
            }
        } catch (Exception e) {
            log.error("Erro não identificado ao atualizar eventos do processo \"" + processId + "\"", e);

            for (WorkflowEventDto event : events) {
                result.addError(event.getName() + " não foi atualizado.");
            }
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        return result;
    }

    public SuccessesAndErrorsDto createEvents(
        long tenantId,
        String processId,
        int version,
        String userCode,
        List<WorkflowEventDto> events
    ) {
        var result = new SuccessesAndErrorsDto();
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                    "INSERT INTO event_proces "
                    + "(COD_EMPRESA, COD_DEF_PROCES, NUM_VERS, SCRIPT_VERSION, AUTHOR_ID, CREATE_DATE, COD_EVENT, DSL_EVENT) "
                    + "VALUES  (?, ?, ?, ?, ?, ?, ?, ?)"
                )
            ) {
                conn.setAutoCommit(false);

                stmt.setLong(1, tenantId);
                stmt.setString(2, processId);
                stmt.setInt(3, version);
                stmt.setInt(4, 0);
                stmt.setString(5, userCode);
                stmt.setTimestamp(6, Timestamp.from(Instant.now()));

                for (WorkflowEventDto event : events) {
                    stmt.setString(7, event.getName());
                    stmt.setString(8, event.getContents());

                    stmt.addBatch();
                }

                int[] createdRows = stmt.executeBatch();
                conn.commit();

                for (var i = 0; i < createdRows.length; ++i) {
                    if (createdRows[i] == 0) {
                        result.addError(events.get(i).getName() + " não foi criado.");
                        continue;
                    }

                    result.addSuccess(events.get(i).getName() + " foi criado.");
                }
            }
        } catch (Exception e) {
            log.error("Erro não identificado ao criar eventos do processo \"" + processId + "\"", e);

            for (WorkflowEventDto event : events) {
                result.addError(event.getName() + " não foi criado.");
            }
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        return result;
    }
}
