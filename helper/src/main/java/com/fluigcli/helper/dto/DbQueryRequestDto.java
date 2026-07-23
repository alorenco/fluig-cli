package com.fluigcli.helper.dto;

import java.util.List;

/**
 * Corpo do POST /api/db/query. jndi = nome do datasource (default no lado da
 * CLI); sql = SELECT/WITH (a política read-only é imposta no DbService);
 * params = valores dos `?` na ordem; maxRows = teto de linhas (0 = default).
 */
public class DbQueryRequestDto {
    private String jndi;
    private String sql;
    private List<String> params;
    private int maxRows;

    public DbQueryRequestDto() {}

    public String getJndi() { return jndi; }
    public void setJndi(String jndi) { this.jndi = jndi; }
    public String getSql() { return sql; }
    public void setSql(String sql) { this.sql = sql; }
    public List<String> getParams() { return params; }
    public void setParams(List<String> params) { this.params = params; }
    public int getMaxRows() { return maxRows; }
    public void setMaxRows(int maxRows) { this.maxRows = maxRows; }
}
