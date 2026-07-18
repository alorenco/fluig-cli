package com.fluigcli.helper.controller;

import java.util.List;

import javax.ws.rs.BadRequestException;
import javax.ws.rs.Consumes;
import javax.ws.rs.GET;
import javax.ws.rs.InternalServerErrorException;
import javax.ws.rs.NotFoundException;
import javax.ws.rs.PUT;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.Produces;
import javax.ws.rs.core.MediaType;

import com.fluigcli.helper.dto.WorkflowEventDto;
import com.fluigcli.helper.dto.WorkflowUpdatedEventsDto;
import com.fluigcli.helper.exception.WorkflowNotFoundException;
import com.fluigcli.helper.service.WorkflowService;

@Path("workflows")
public class WorkflowController extends BaseController {

    @GET
    @Path("/{processId}/version")
    @Produces(MediaType.APPLICATION_JSON)
    public int maxVersion(@PathParam("processId") String processId) {
        if (processId.isBlank()) {
            throw new BadRequestException("Necessário informar o processId");
        }

        try {
            return new WorkflowService().getMaxVersion(securityService.getCurrentTenantId(), processId);
        } catch (WorkflowNotFoundException e) {
            throw new NotFoundException(e.getMessage());
        } catch (Exception e) {
            log.error("Erro não identificado ao procurar última versão do processo \"" + processId + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }

    @PUT
    @Path("/{processId}/{version:[1-9][0-9]*}/events")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    public WorkflowUpdatedEventsDto updateWorkflowEvents(
        @PathParam("processId") String processId,
        @PathParam("version") int version,
        List<WorkflowEventDto> events
    ) {
        if (processId.isBlank()) {
            throw new BadRequestException("Necessário informar o processId");
        }

        if (events == null || events.isEmpty()) {
            throw new BadRequestException("Necessário informar ao menos um evento");
        }

        for (WorkflowEventDto event : events) {
            if (event.getName() == null || event.getName().isBlank()
                || event.getContents() == null || event.getContents().isBlank()) {
                throw new BadRequestException("Obrigatório que todos os eventos possuam `name` e `contents`");
            }

            event.setName(event.getName().trim());
        }

        var service = new WorkflowService();
        long tenantId;
        String userCode;
        int maxVersion;

        try {
            tenantId = securityService.getCurrentTenantId();
            userCode = userService.getCurrent().getCode();
            maxVersion = service.getMaxVersion(tenantId, processId);
        } catch (WorkflowNotFoundException e) {
            throw new NotFoundException(e.getMessage());
        } catch (Exception e) {
            log.error("Erro não identificado ao buscar metadados do processo \"" + processId + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }

        if (version > maxVersion) {
            throw new BadRequestException("A versão indicada deve ser menor ou igual a " + maxVersion);
        }

        try {
            return service.updateEvents(tenantId, processId, version, userCode, events);
        } catch (Exception e) {
            log.error("Erro não identificado ao atualizar eventos do processo \"" + processId + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }
}
