package com.fluigcli.helper.dto;

import java.util.ArrayList;
import java.util.List;

/**
 * Resultado do db query. rows é posicional (cada linha alinha com columns na
 * ordem) — robusto a colunas de nome repetido. Os valores vêm como texto
 * (getString), com null preservado, para serialização previsível. truncated =
 * havia mais linhas que maxRows.
 */
public class DbResultDto {
    private List<DbColumnDto> columns = new ArrayList<>();
    private List<List<String>> rows = new ArrayList<>();
    private int rowCount;
    private boolean truncated;

    public DbResultDto() {}

    public List<DbColumnDto> getColumns() { return columns; }
    public void setColumns(List<DbColumnDto> columns) { this.columns = columns; }
    public List<List<String>> getRows() { return rows; }
    public void setRows(List<List<String>> rows) { this.rows = rows; }
    public int getRowCount() { return rowCount; }
    public void setRowCount(int rowCount) { this.rowCount = rowCount; }
    public boolean isTruncated() { return truncated; }
    public void setTruncated(boolean truncated) { this.truncated = truncated; }
}
