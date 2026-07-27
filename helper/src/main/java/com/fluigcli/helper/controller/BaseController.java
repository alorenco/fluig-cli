package com.fluigcli.helper.controller;

import javax.annotation.PostConstruct;
import javax.ejb.EJB;
import javax.ws.rs.ForbiddenException;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.fluig.sdk.service.SecurityService;
import com.fluig.sdk.service.UserService;

/**
 * Base dos controllers: exige que o usuário da sessão seja administrador do
 * tenant. Modelo de segurança herdado do fluig-widget-helper (Fluiggers, MIT).
 *
 * Desde o helper 0.9.0 esta é a SEGUNDA camada. A primeira é o
 * {@link com.fluigcli.helper.AuthorizationFilter}, que aplica a mesma política
 * a toda requisição sob /api, antes de casar a rota. As duas ficam de pé de
 * propósito: se uma deixar de rodar, a outra nega.
 *
 * Não remova este @PostConstruct achando que o filtro basta. O motivo está no
 * ROADMAP §2.11-A: um callback de ciclo de vida não é contrato do JAX-RS, e um
 * provider pode deixar de ser registrado — nenhuma das duas camadas é garantida
 * sozinha.
 */
public abstract class BaseController {
    protected final Logger log = LoggerFactory.getLogger(getClass());

    @EJB(lookup = SecurityService.JNDI_REMOTE_NAME)
    protected SecurityService securityService;

    @EJB(lookup = UserService.JNDI_REMOTE_NAME)
    protected UserService userService;

    /**
     * Login do chamador, resolvido uma vez por requisição no gate. Os
     * controllers usam isto na auditoria em vez de chamar o EJB de novo — o
     * recurso é instanciado por requisição, então o valor é sempre do chamador
     * da vez. Use {@link #usuario()}, que trata o caso não resolvido.
     */
    private String loginAtual;

    /** Login do chamador para mensagem de log; nunca nulo. */
    protected String usuario() {
        return loginAtual == null ? "(não identificado)" : loginAtual;
    }

    @PostConstruct
    private void assertUserAccess() {
        try {
            String login = userService.getCurrent().getLogin();
            loginAtual = login;

            boolean isAdmin = securityService
                .listTenantAdmins(securityService.getCurrentTenantId())
                .stream()
                .anyMatch(user -> user.getLogin().equals(login));

            if (isAdmin) {
                return;
            }
        } catch (Exception e) {
            log.error("Erro não capturado ao validar usuário administrador", e);
        }

        throw new ForbiddenException();
    }
}
