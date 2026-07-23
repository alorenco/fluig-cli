package com.fluigcli.helper.repository;

import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;

import javax.naming.InitialContext;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Exclusão FÍSICA de dataset via o EJB interno DatasetService do Fluig
 * (deletePermanently). Alcança o EJB por JNDI + reflexão, de propósito: compilar
 * contra a interface (ecm-ejb-api) arrastaria uma cascata de tipos internos do
 * EAR para o WAR. O lookup por JNDI é o mesmo mecanismo do @EJB do BaseController
 * e NÃO recai no problema de classloader/CDI de expor classes do motor.
 *
 * Nota: o @DELETE da REST legada (/ecm/api/rest/ecm/dataset/deleteDataset) só
 * DESATIVA (== disable, reversível). Só deletePermanently remove de fato.
 */
public class DatasetAdminRepository {
    protected final Logger log = LoggerFactory.getLogger(getClass());

    // JNDI global do EJB DatasetService (== DatasetService.JNDI_REMOTE_NAME).
    private static final String DATASET_SERVICE_JNDI = "java:global/fluig/ecm-impl/ecm/Dataset";

    /**
     * Remove um dataset fisicamente do tenant informado. Lança quando o dataset
     * não existe ou o servidor recusa (a causa real é desembrulhada da reflexão).
     */
    public void deletePermanently(String datasetId, long tenantId) throws Exception {
        InitialContext ic = null;
        try {
            ic = new InitialContext();
            Object service = ic.lookup(DATASET_SERVICE_JNDI);
            Method method = service.getClass().getMethod("deletePermanently", String.class, long.class);
            method.invoke(service, datasetId, tenantId);
        } catch (InvocationTargetException e) {
            // Desembrulha a exceção real do EJB (ex.: FDNException) para a mensagem.
            Throwable cause = e.getCause();
            if (cause instanceof Exception) {
                throw (Exception) cause;
            }
            throw e;
        } finally {
            if (ic != null) {
                try {
                    ic.close();
                } catch (Exception ignore) {
                    // nada
                }
            }
        }
    }
}
