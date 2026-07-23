package com.fluigcli.helper.controller;

import java.sql.SQLException;
import java.util.List;

import javax.naming.NameNotFoundException;
import javax.ws.rs.Consumes;
import javax.ws.rs.GET;
import javax.ws.rs.POST;
import javax.ws.rs.Path;
import javax.ws.rs.Produces;
import javax.ws.rs.WebApplicationException;
import javax.ws.rs.core.MediaType;
import javax.ws.rs.core.Response;

import com.fluigcli.helper.dto.DbQueryRequestDto;
import com.fluigcli.helper.dto.DbResultDto;
import com.fluigcli.helper.service.DbService;

/**
 * SQL de diagnóstico contra um datasource JNDI do servidor. Read-only (a
 * política é imposta no DbService). Herda o gate do BaseController (só
 * administrador do tenant). Os erros levam a mensagem no corpo (text/plain)
 * para o agente/usuário ver o motivo — o helper não tem ExceptionMapper.
 */
@Path("/db")
public class DbController extends BaseController {

    private static final String DEFAULT_JNDI = "/jdbc/AppDS";

    @POST
    @Path("/query")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    public DbResultDto query(DbQueryRequestDto request) {
        if (request == null) {
            throw error(Response.Status.BAD_REQUEST, "Corpo da requisição ausente");
        }

        String jndi = request.getJndi() == null || request.getJndi().isBlank()
            ? DEFAULT_JNDI
            : request.getJndi().trim();

        if (!jndi.startsWith("/jdbc") && !jndi.startsWith("java:")) {
            throw error(Response.Status.BAD_REQUEST,
                "Nome de datasource inválido: use um JNDI como /jdbc/AppDS");
        }

        try {
            return new DbService().query(jndi, request.getSql(), request.getParams(), request.getMaxRows());
        } catch (IllegalArgumentException e) {
            throw error(Response.Status.BAD_REQUEST, e.getMessage());
        } catch (NameNotFoundException e) {
            throw error(Response.Status.NOT_FOUND, "Datasource não encontrado: " + jndi);
        } catch (ClassCastException e) {
            throw error(Response.Status.BAD_REQUEST, "O JNDI \"" + jndi + "\" não é um datasource");
        } catch (SQLException e) {
            // Erro de negócio do banco: devolve a mensagem para o usuário/agente.
            throw error(Response.Status.BAD_REQUEST, "Erro de SQL: " + e.getMessage());
        } catch (Exception e) {
            log.error("Erro não identificado no db query (jndi=" + jndi + ")", e);
            throw error(Response.Status.INTERNAL_SERVER_ERROR,
                "Consulte o log do Fluig para mais informações.");
        }
    }

    @GET
    @Path("/datasources")
    @Produces(MediaType.APPLICATION_JSON)
    public List<String> datasources() {
        try {
            return new DbService().datasources();
        } catch (Exception e) {
            log.error("Erro ao enumerar datasources", e);
            throw error(Response.Status.INTERNAL_SERVER_ERROR,
                "Consulte o log do Fluig para mais informações.");
        }
    }

    private WebApplicationException error(Response.Status status, String message) {
        return new WebApplicationException(
            Response.status(status).entity(message).type(MediaType.TEXT_PLAIN).build()
        );
    }
}
