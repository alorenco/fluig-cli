package com.fluigcli.helper;

import java.net.URI;
import java.net.URISyntaxException;

import javax.naming.InitialContext;
import javax.ws.rs.container.ContainerRequestContext;
import javax.ws.rs.container.ContainerRequestFilter;
import javax.ws.rs.container.PreMatching;
import javax.ws.rs.core.MediaType;
import javax.ws.rs.core.Response;
import javax.ws.rs.ext.Provider;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.fluig.sdk.service.SecurityService;
import com.fluig.sdk.service.UserService;

/**
 * Autorização de TODA requisição sob /api: só administrador do tenant passa, e
 * só de mesma origem.
 *
 * O container exige apenas um usuário autenticado do portal (papel `user` no
 * web.xml). Quem separa usuário comum de administrador é este filtro.
 *
 * Por que um filtro, e não só o @PostConstruct do BaseController: o filtro roda
 * independente de herança, de escopo de bean e de a rota existir. O
 * @PostConstruct continua no lugar, como segunda camada — se um dos dois deixar
 * de rodar, o outro nega. Um controller novo que esqueça o `extends
 * BaseController` fica coberto por aqui.
 *
 * @PreMatching é deliberado: a checagem acontece antes de casar a rota, então
 * rota inexistente também é negada. Isso impede enumerar as rotas do helper com
 * uma conta comum.
 *
 * O EJB é alcançado por JNDI dentro do método, e não por injeção de campo. Um
 * @Provider é singleton; o lookup por requisição resolve o contexto de
 * segurança do chamador da vez, sem depender do ciclo de vida do provider. É o
 * mesmo mecanismo do DatasetAdminRepository.
 */
@Provider
@PreMatching
public class AuthorizationFilter implements ContainerRequestFilter {

    private static final Logger log = LoggerFactory.getLogger(AuthorizationFilter.class);

    @Override
    public void filter(ContainerRequestContext request) {
        // Origem primeiro: é barato e dispensa o lookup do EJB quando reprova.
        String origin = request.getHeaderString("Origin");
        if (!origemPermitida(origin, request.getUriInfo().getBaseUri())) {
            negarOrigem(request, origin);
            return;
        }

        String login = null;
        InitialContext ic = null;

        try {
            ic = new InitialContext();
            UserService userService = (UserService) ic.lookup(UserService.JNDI_REMOTE_NAME);
            SecurityService securityService = (SecurityService) ic.lookup(SecurityService.JNDI_REMOTE_NAME);

            final String current = userService.getCurrent().getLogin();
            login = current;

            boolean isAdmin = securityService
                .listTenantAdmins(securityService.getCurrentTenantId())
                .stream()
                .anyMatch(user -> user.getLogin().equals(current));

            if (isAdmin) {
                return;
            }
        } catch (Exception e) {
            // Falha ao decidir nega o acesso. Nunca libera por erro.
            log.error("Erro ao validar usuário administrador; acesso negado", e);
        } finally {
            if (ic != null) {
                try {
                    ic.close();
                } catch (Exception ignore) {
                    // nada
                }
            }
        }

        deny(request, login);
    }

    /**
     * Nega com 403 e registra a tentativa. O log é o ponto de auditoria: antes
     * desta versão a negativa saía como 500 e não deixava NENHUM rastro no
     * server.log (medido na homologação em 2026-07-27).
     */
    private void deny(ContainerRequestContext request, String login) {
        log.info(
            "Acesso NEGADO ao usuário \"{}\" em {} /{} — restrito aos administradores do tenant",
            login == null ? "(não identificado)" : login,
            request.getMethod(),
            semBarraInicial(request.getUriInfo().getPath())
        );

        request.abortWith(
            Response
                .status(Response.Status.FORBIDDEN)
                .entity("Acesso restrito aos administradores do tenant.")
                .type(MediaType.TEXT_PLAIN_TYPE.withCharset("UTF-8"))
                .build()
        );
    }

    /**
     * Recusa requisição vinda de outra origem (defesa contra CSRF).
     *
     * A API autentica só por cookie de sessão. Sem esta checagem, um
     * administrador logado que abrisse uma página hostil entregaria a API
     * inteira do helper, inclusive o PUT de eventos de processo — que grava
     * código executado pelo servidor.
     *
     * Hoje o navegador já barra essa requisição, mas por acidente: o Fluig
     * responde `Access-Control-Allow-Origin: *` JUNTO com
     * `Allow-Credentials: true`, combinação inválida pela spec do Fetch (medido
     * na homologação em 2026-07-27). Num ambiente que ecoe o Origin, a proteção
     * some. Por isso a checagem é nossa, e não da configuração da plataforma.
     *
     * Sem o header `Origin` a requisição passa: nenhum cliente legítimo do
     * helper é navegador. A CLI em Go nunca manda `Origin`, e o painel do `dev`
     * fala pelo proxy Go. Isso mantém a compatibilidade com binários antigos.
     *
     * Não olhamos o `Referer`: ele não acrescenta nada aqui (o navegador manda
     * `Origin` em toda requisição cross-origin que carrega efeito) e falha com
     * as políticas de privacidade que o omitem.
     *
     * Limite conhecido: GET disparado por `<img>` ou `<script>` numa página
     * hostil não carrega `Origin`. Ele passa, mas o atacante não lê a resposta
     * (mesma-origem do navegador) e nenhum GET do helper tem efeito colateral.
     */
    static boolean origemPermitida(String origin, URI base) {
        if (origin == null || origin.trim().isEmpty()) {
            return true;
        }
        if (base == null) {
            return false;
        }

        URI o;
        try {
            o = new URI(origin.trim());
        } catch (URISyntaxException e) {
            // Inclui o literal "null" que iframe sandbox manda.
            return false;
        }
        if (o.getHost() == null) {
            return false;
        }

        return equalsIgnoreCase(o.getScheme(), base.getScheme())
            && equalsIgnoreCase(o.getHost(), base.getHost())
            && porta(o) == porta(base);
    }

    /** Porta explícita, ou a padrão do esquema quando a URI não traz. */
    private static int porta(URI uri) {
        if (uri.getPort() != -1) {
            return uri.getPort();
        }
        if ("https".equalsIgnoreCase(uri.getScheme())) {
            return 443;
        }
        if ("http".equalsIgnoreCase(uri.getScheme())) {
            return 80;
        }
        return -1;
    }

    private static boolean equalsIgnoreCase(String a, String b) {
        return a == null ? b == null : a.equalsIgnoreCase(b);
    }

    private void negarOrigem(ContainerRequestContext request, String origin) {
        log.warn(
            "Requisição RECUSADA por origem cruzada: Origin \"{}\" em {} /{}",
            origin,
            request.getMethod(),
            semBarraInicial(request.getUriInfo().getPath())
        );

        request.abortWith(
            Response
                .status(Response.Status.FORBIDDEN)
                .entity("Requisição de outra origem não é aceita.")
                .type(MediaType.TEXT_PLAIN_TYPE.withCharset("UTF-8"))
                .build()
        );
    }

    /**
     * Normaliza a rota para o log. Em @PreMatching o RESTEasy devolve o caminho
     * COM barra inicial, e concatenar outra deixaria "//ping" no server.log.
     */
    static String semBarraInicial(String path) {
        if (path == null) {
            return "";
        }
        int i = 0;
        while (i < path.length() && path.charAt(i) == '/') {
            i++;
        }
        return path.substring(i);
    }
}
