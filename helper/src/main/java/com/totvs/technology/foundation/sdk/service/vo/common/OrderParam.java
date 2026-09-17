package com.totvs.technology.foundation.sdk.service.vo.common;

/**
 * SHIM de classloader — NÃO é a classe da TOTVS. Ver o comentário em
 * {@link ResponseEnvelopeVO}: existe só para a interface WorkflowAPIService
 * carregar no classloader do WAR. O helper não usa os métodos que o recebem.
 */
public class OrderParam implements java.io.Serializable {
    private static final long serialVersionUID = 1L;
}
