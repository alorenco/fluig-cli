package com.fluigcli.helper.repository;

import java.sql.Connection;
import java.sql.ResultSet;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;

import javax.naming.InitialContext;
import javax.sql.DataSource;

import com.fluigcli.helper.dto.WidgetDto;

public class WidgetRepository extends BaseRepository {

    public List<WidgetDto> findAll() throws Exception {
        var widgets = new ArrayList<WidgetDto>();
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            DataSource ds = (DataSource) ic.lookup(DB_DATASOURCE_NAME);

            try (
                Connection conn = ds.getConnection();
                Statement stmt = conn.createStatement();
                ResultSet rs = stmt.executeQuery(
                    "SELECT APPLICATION_CODE, APPLICATION_TITLE, DESCRIPTION, FILE_NAME "
                    + "FROM wcm_application "
                    + "WHERE INTERNAL = 0 AND APPLICATION_TYPE = 'widget' AND EAR_FILE_NAME IS NULL "
                    + "ORDER BY APPLICATION_TITLE"
                )
            ) {
                while (rs.next()) {
                    widgets.add(new WidgetDto(
                        rs.getString("APPLICATION_CODE"),
                        rs.getString("APPLICATION_TITLE"),
                        rs.getString("DESCRIPTION"),
                        rs.getString("FILE_NAME")
                    ));
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
