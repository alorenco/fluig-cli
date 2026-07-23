package com.fluigcli.helper.controller;

import java.util.LinkedHashMap;
import java.util.Map;

import javax.ws.rs.DELETE;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.Produces;
import javax.ws.rs.WebApplicationException;
import javax.ws.rs.core.MediaType;
import javax.ws.rs.core.Response;

import com.fluigcli.helper.repository.DatasetAdminRepository;

/**
 * Administração de dataset que a API oficial não expõe. Por ora: exclusão FÍSICA
 * (hard-delete) via o EJB DatasetService.deletePermanently. Herda o gate do
 * BaseController (só administrador do tenant). O tenant é o da sessão — o
 * chamador não escolhe empresa. Erros levam a mensagem no corpo (text/plain),
 * como o DbController (o helper não tem ExceptionMapper).
 */
@Path("/datasets")
public class DatasetController extends BaseController {

    @DELETE
    @Path("/{id}")
    @Produces(MediaType.APPLICATION_JSON)
    public Response delete(@PathParam("id") String id) {
        if (id == null || id.trim().isEmpty()) {
            throw error(Response.Status.BAD_REQUEST, "id do dataset ausente");
        }
        String datasetId = id.trim();
        try {
            long tenantId = securityService.getCurrentTenantId();
            new DatasetAdminRepository().deletePermanently(datasetId, tenantId);

            Map<String, Object> body = new LinkedHashMap<>();
            body.put("id", datasetId);
            body.put("deleted", true);
            return Response.ok(body).build();
        } catch (Exception e) {
            // Erro de negócio (dataset inexistente, dependência, recusa): devolve
            // a mensagem para o usuário/agente. Status 400 (não 404 — o 404 fica
            // reservado para "rota ausente", i.e. helper antigo sem esta rota).
            log.error("Erro ao excluir dataset " + datasetId, e);
            throw error(Response.Status.BAD_REQUEST, "Erro ao excluir dataset: " + e.getMessage());
        }
    }

    private WebApplicationException error(Response.Status status, String message) {
        return new WebApplicationException(
            Response.status(status).entity(message).type(MediaType.TEXT_PLAIN).build()
        );
    }
}
