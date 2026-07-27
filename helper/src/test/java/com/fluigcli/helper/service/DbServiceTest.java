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

    /**
     * Os dois bypasses MEDIDOS ao vivo na homologação (SQL Server 2017) em
     * 2026-07-27, que a checagem de primeira palavra deixava passar. O
     * `setReadOnly(true)` da conexão não segura nenhum dos dois: é no-op no
     * driver da Microsoft (ROADMAP §2.11-C).
     */
    @Test
    public void recusaEscritaDepoisDeCte() {
        assertRejeitado(
            "WITH x AS (SELECT 1 AS n) UPDATE wcm_application SET DESCRIPTION = DESCRIPTION "
            + "OUTPUT deleted.APPLICATION_CODE WHERE 1 = 0"
        );
        assertRejeitado("WITH x AS (SELECT 1 AS n) SELECT n INTO #tmp FROM x");
    }

    @Test
    public void recusaEscritaEmQualquerPosicao() {
        assertRejeitado("SELECT 1 AS n INTO zz_nova_tabela");
        assertRejeitado("WITH x AS (SELECT 1 n) DELETE FROM t");
        assertRejeitado("SELECT * FROM t WHERE id IN (SELECT id FROM u) MERGE INTO v");
        assertRejeitado("select * from openquery(srv, 'delete from t') EXEC sp_who");
    }

    /**
     * Falsos positivos que NÃO podem acontecer: a varredura ignora literais,
     * comentários e identificadores entre colchetes ou aspas.
     */
    @Test
    public void aceitaPalavraDeEscritaForaDoCodigo() {
        // Dentro de literal.
        assertEquals(
            "SELECT * FROM t WHERE acao = 'update'",
            DbService.sanitizeReadOnly("SELECT * FROM t WHERE acao = 'update'")
        );
        // Aspa simples escapada dentro do literal.
        String comEscape = "SELECT * FROM t WHERE txt = 'não é delete'''";
        assertEquals(comEscape, DbService.sanitizeReadOnly(comEscape));
        // Coluna com nome reservado, entre colchetes (T-SQL) e aspas.
        assertEquals("SELECT [update] FROM t", DbService.sanitizeReadOnly("SELECT [update] FROM t"));
        assertEquals("SELECT \"delete\" FROM t", DbService.sanitizeReadOnly("SELECT \"delete\" FROM t"));
        // Comentário de linha e de bloco.
        assertEquals(
            "SELECT 1 -- update depois",
            DbService.sanitizeReadOnly("SELECT 1 -- update depois")
        );
        assertEquals(
            "SELECT 1 /* insert aqui */ FROM t",
            DbService.sanitizeReadOnly("SELECT 1 /* insert aqui */ FROM t")
        );
        // Identificadores que só CONTÊM a palavra não casam (fronteira).
        String colunas = "SELECT updated_at, deleted_by, created FROM t";
        assertEquals(colunas, DbService.sanitizeReadOnly(colunas));
    }

    @Test
    public void pontoEVirgulaDentroDeLiteralNaoSeparaInstrucao() {
        // Antes do §2.11-C isto era recusado por engano.
        String sql = "SELECT * FROM t WHERE txt = 'a;b'";
        assertEquals(sql, DbService.sanitizeReadOnly(sql));
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
