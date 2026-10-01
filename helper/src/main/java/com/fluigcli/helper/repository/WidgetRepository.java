package com.fluigcli.helper.repository;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.PreparedStatement;
import java.util.ArrayList;
import java.util.List;

import javax.naming.InitialContext;
import javax.sql.DataSource;

import com.fluigcli.helper.dto.WidgetDto;

public class WidgetRepository extends BaseRepository {

    /** Widgets customizados (APPLICATION_TYPE = 'widget'). */
    public List<WidgetDto> findAll() throws Exception {
        return findAll("widget");
    }

    /**
     * Layouts customizados (APPLICATION_TYPE = 'layout'). Mesma tabela e mesmo
     * DTO do widget: só o tipo muda. Rota /layouts, helper >= 0.12.0.
     */
    public List<WidgetDto> findAllLayouts() throws Exception {
        return findAll("layout");
    }

    // EAR_FILE_NAME IS NULL exclui os artefatos internos da plataforma, que
    // moram dentro do fluig-server-components.ear e não têm WAR em apps/.
    private List<WidgetDto> findAll(String applicationType) throws Exception {
        var widgets = new ArrayList<WidgetDto>();
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                PreparedStatement stmt = conn.prepareStatement(
                    "SELECT APPLICATION_CODE, APPLICATION_TITLE, DESCRIPTION, FILE_NAME "
                    + "FROM wcm_application "
                    + "WHERE INTERNAL = 0 AND APPLICATION_TYPE = ? AND EAR_FILE_NAME IS NULL "
                    + "ORDER BY APPLICATION_TITLE"
                )
            ) {
                stmt.setString(1, applicationType);
                try (ResultSet rs = stmt.executeQuery()) {
                    while (rs.next()) {
                        widgets.add(new WidgetDto(
                            rs.getString("APPLICATION_CODE"),
                            rs.getString("APPLICATION_TITLE"),
                            rs.getString("DESCRIPTION"),
                            rs.getString("FILE_NAME")
                        ));
                    }
                }
            }
        } finally {
            if (ic != null) {
                try { ic.close(); } catch (Exception ignore) {}
            }
        }

        return widgets;
    }
}
