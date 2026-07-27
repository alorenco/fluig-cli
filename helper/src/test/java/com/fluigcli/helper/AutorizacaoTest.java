package com.fluigcli.helper;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertTrue;
import static org.junit.Assert.fail;

import java.io.IOException;
import java.net.URI;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.List;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import javax.ws.rs.container.ContainerRequestFilter;
import javax.ws.rs.container.PreMatching;
import javax.ws.rs.ext.Provider;

import org.junit.Test;

import com.fluigcli.helper.controller.BaseController;

/**
 * Protege o modelo de autorização do helper. As duas camadas têm de continuar
 * existindo: o filtro global (AuthorizationFilter) e o gate herdado do
 * BaseController.
 *
 * O motivo deste teste está no ROADMAP §2.11-A: o gate era um @PostConstruct
 * privado de superclasse, e um controller novo sem `extends BaseController`
 * ficaria aberto a qualquer usuário autenticado do portal, sem nada avisar.
 */
public class AutorizacaoTest {

    @Test
    public void filtroGlobalEstaRegistradoECobreTodaRequisicao() {
        Class<?> filtro = AuthorizationFilter.class;

        assertTrue(
            "AuthorizationFilter tem de implementar ContainerRequestFilter",
            ContainerRequestFilter.class.isAssignableFrom(filtro)
        );
        assertNotNull(
            "AuthorizationFilter tem de ser @Provider (senão o RESTEasy não o registra)",
            filtro.getAnnotation(Provider.class)
        );
        assertNotNull(
            "AuthorizationFilter tem de ser @PreMatching, para negar também rota inexistente",
            filtro.getAnnotation(PreMatching.class)
        );
    }

    @Test
    public void rotaDoLogSaiComUmaBarraSo() {
        // O RESTEasy devolve "/ping" em @PreMatching; sem normalizar, o log
        // saía como "GET //ping" (visto na homologação em 2026-07-27).
        assertEquals("ping", AuthorizationFilter.semBarraInicial("/ping"));
        assertEquals("ping", AuthorizationFilter.semBarraInicial("ping"));
        assertEquals("logs/server.log/tail", AuthorizationFilter.semBarraInicial("//logs/server.log/tail"));
        assertEquals("", AuthorizationFilter.semBarraInicial(null));
        assertEquals("", AuthorizationFilter.semBarraInicial("/"));
    }

    @Test
    public void requisicaoSemOriginPassa() {
        // Nenhum cliente legítimo do helper é navegador: a CLI em Go não manda
        // Origin, e o painel do `dev` fala pelo proxy Go. Recusar aqui quebraria
        // todo binário existente.
        URI base = URI.create("http://fluig.exemplo:8080/fluigcliHelper/api");
        assertTrue(AuthorizationFilter.origemPermitida(null, base));
        assertTrue(AuthorizationFilter.origemPermitida("", base));
        assertTrue(AuthorizationFilter.origemPermitida("   ", base));
    }

    @Test
    public void mesmaOrigemPassa() {
        URI base = URI.create("http://fluig.exemplo:8080/fluigcliHelper/api");
        assertTrue(AuthorizationFilter.origemPermitida("http://fluig.exemplo:8080", base));
        // Esquema e host não diferenciam maiúscula de minúscula.
        assertTrue(AuthorizationFilter.origemPermitida("HTTP://Fluig.Exemplo:8080", base));

        // Porta padrão implícita dos dois lados.
        URI https = URI.create("https://fluig.exemplo/fluigcliHelper/api");
        assertTrue(AuthorizationFilter.origemPermitida("https://fluig.exemplo", https));
        assertTrue(AuthorizationFilter.origemPermitida("https://fluig.exemplo:443", https));
    }

    @Test
    public void origemCruzadaEhRecusada() {
        URI base = URI.create("http://fluig.exemplo:8080/fluigcliHelper/api");

        assertFalse("host diferente",
            AuthorizationFilter.origemPermitida("http://evil.example", base));
        assertFalse("porta diferente",
            AuthorizationFilter.origemPermitida("http://fluig.exemplo:9090", base));
        assertFalse("esquema diferente",
            AuthorizationFilter.origemPermitida("https://fluig.exemplo:8080", base));
        assertFalse("host que só CONTÉM o nome do servidor",
            AuthorizationFilter.origemPermitida("http://fluig.exemplo.evil.example:8080", base));
        // Iframe sandbox manda o literal "null".
        assertFalse("origem opaca", AuthorizationFilter.origemPermitida("null", base));
        assertFalse("lixo", AuthorizationFilter.origemPermitida(":::", base));
    }

    @Test
    public void todoControllerHerdaOGateDoBaseController() throws Exception {
        List<Class<?>> recursos = classesAnotadasComPath();

        assertTrue(
            "nenhuma classe @Path encontrada em target/classes — o teste não estaria medindo nada",
            recursos.size() >= 7
        );

        List<String> semGate = recursos.stream()
            .filter(c -> !BaseController.class.isAssignableFrom(c))
            .map(Class::getName)
            .collect(Collectors.toList());

        if (!semGate.isEmpty()) {
            fail(
                "controller sem o gate do BaseController: " + semGate
                + " — todo recurso @Path do helper tem de herdar de BaseController"
            );
        }
    }

    /** Varre target/classes e devolve as classes anotadas com @Path. */
    private List<Class<?>> classesAnotadasComPath() throws IOException, ClassNotFoundException {
        Path raiz = Paths.get("target", "classes");
        assertTrue("rode o teste pelo Maven: target/classes não existe", Files.isDirectory(raiz));

        List<String> nomes = new ArrayList<>();
        try (Stream<Path> arquivos = Files.walk(raiz)) {
            for (Path p : arquivos.filter(f -> f.toString().endsWith(".class")).collect(Collectors.toList())) {
                String nome = raiz.relativize(p).toString()
                    .replace(java.io.File.separatorChar, '.')
                    .replaceAll("\\.class$", "");
                nomes.add(nome);
            }
        }

        List<Class<?>> comPath = new ArrayList<>();
        for (String nome : nomes) {
            // false = não inicializa a classe; só precisamos das anotações.
            Class<?> c = Class.forName(nome, false, getClass().getClassLoader());
            if (c.getAnnotation(javax.ws.rs.Path.class) != null) {
                comPath.add(c);
            }
        }
        return comPath;
    }
}
