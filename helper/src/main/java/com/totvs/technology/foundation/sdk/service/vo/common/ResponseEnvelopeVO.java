package com.totvs.technology.foundation.sdk.service.vo.common;

/**
 * SHIM de classloader — NÃO é a classe da TOTVS.
 *
 * A interface WorkflowAPIService do fluig-sdk-api 1.8.2 referencia este tipo
 * nos métodos de SLA (findRequestsSLA, findActivities, findRequests...). O
 * WildFly carrega a interface INTEIRA no classloader do WAR para criar o proxy
 * do EJB, e o lookup java:global/fluig/bpm-sdk/sdk/Workflow morria com
 * NoClassDefFoundError neste tipo (medido na homologação em 2026-09-17). O jar
 * que o define (foundation-sdk) não está vendorizado e o Nexus da TOTVS exige
 * autenticação.
 *
 * O helper nunca chama os métodos que usam este tipo. A classe existe só para
 * a resolução de assinaturas. Se um dia o helper precisar deles, troque este
 * shim pelo jar real.
 */
public class ResponseEnvelopeVO<T> implements java.io.Serializable {
    private static final long serialVersionUID = 1L;
}
