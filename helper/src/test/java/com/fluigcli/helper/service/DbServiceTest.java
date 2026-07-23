package com.fluigcli.helper.service;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.fail;

import org.junit.Test;

public class DbServiceTest {

    @Test
    public void aceitaSelect() {
        assertEquals("select 1", DbService.sanitizeReadOnly("select 1"));
    }

    @Test
    public void aceitaWithCte() {
        String sql = "WITH x AS (SELECT 1 AS n) SELECT n FROM x";
        assertEquals(sql, DbService.sanitizeReadOnly(sql));
    }

    @Test
    public void removePontoEVirgulaFinal() {
        assertEquals("select suser_sname()", DbService.sanitizeReadOnly("  select suser_sname();  "));
    }

    @Test
    public void recusaMultiplasInstrucoes() {
        assertRejeitado("select 1; drop table t");
    }

    @Test
    public void recusaEscrita() {
        assertRejeitado("update t set a = 1");
        assertRejeitado("delete from t");
        assertRejeitado("insert into t values (1)");
        assertRejeitado("drop table t");
    }

    @Test
    public void recusaVazio() {
        assertRejeitado("   ");
        assertRejeitado(null);
    }

    @Test
    public void resolveMaxRows() {
        assertEquals(DbService.DEFAULT_MAX_ROWS, DbService.resolveMaxRows(0));
        assertEquals(DbService.DEFAULT_MAX_ROWS, DbService.resolveMaxRows(-5));
        assertEquals(10, DbService.resolveMaxRows(10));
        assertEquals(DbService.MAX_MAX_ROWS, DbService.resolveMaxRows(999_999));
    }

    private void assertRejeitado(String sql) {
        try {
            DbService.sanitizeReadOnly(sql);
            fail("deveria rejeitar: " + sql);
        } catch (IllegalArgumentException expected) {
            // ok
        }
    }
}
