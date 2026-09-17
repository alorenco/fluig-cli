package com.fluigcli.helper.controller;

import java.lang.reflect.InvocationTargetException;
import java.util.List;

import javax.ws.rs.BadRequestException;
import javax.ws.rs.Consumes;
import javax.ws.rs.GET;
import javax.ws.rs.InternalServerErrorException;
import javax.ws.rs.NotFoundException;
import javax.ws.rs.POST;
import javax.ws.rs.PUT;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.Produces;
import javax.ws.rs.QueryParam;
import javax.ws.rs.WebApplicationException;
import javax.ws.rs.core.MediaType;
import javax.ws.rs.core.Response;

import com.fluigcli.helper.dto.WorkflowEventDto;
import com.fluigcli.helper.dto.WorkflowObservationDto;
import com.fluigcli.helper.dto.WorkflowObservationRequestDto;
import com.fluigcli.helper.dto.WorkflowUpdatedEventsDto;
import com.fluigcli.helper.exception.ProcessInstanceConflictException;
import com.fluigcli.helper.exception.ProcessInstanceNotFoundException;
import com.fluigcli.helper.exception.WorkflowNotFoundException;
import com.fluigcli.helper.service.WorkflowObservationService;
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

        // Auditoria: esta rota grava CÓDIGO que o motor de processo executa no
        // servidor. É a operação mais poderosa do helper, e até o §2.11-D era a
        // que menos deixava rastro.
        log.info(
            "Usuário \"{}\" atualizou eventos do processo \"{}\" versão {}: {}",
            usuario(), processId, version, nomesDosEventos(events)
        );

        try {
            return service.updateEvents(tenantId, processId, version, userCode, events);
        } catch (Exception e) {
            log.error("Erro não identificado ao atualizar eventos do processo \"" + processId + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }

    // --- observações em solicitação (sem movimentar) — helper >= 0.11.0 ---

    /**
     * Registra uma observação na solicitação SEM movimentá-la, em nome do
     * usuário autenticado. Existe porque a REST do Fluig não tem comentário
     * sem move e o SOAP setTasksComments exige a senha do usuário — um usuário
     * de app OAuth não tem senha. O caminho é o EJB WorkflowAPIService do SDK.
     */
    @POST
    @Path("/{processInstanceId:[1-9][0-9]*}/observations")
    @Consumes(MediaType.APPLICATION_JSON)
    @Produces(MediaType.APPLICATION_JSON)
    public Response createObservation(
        @PathParam("processInstanceId") int processInstanceId,
        WorkflowObservationRequestDto req
    ) {
        if (req == null) {
            throw texto(Response.Status.BAD_REQUEST, "Corpo da requisição ausente");
        }
        String invalido = WorkflowObservationService.validarTexto(req.getObservation());
        if (invalido != null) {
            throw texto(Response.Status.BAD_REQUEST, invalido);
        }

        long tenantId;
        String userCode;
        try {
            tenantId = securityService.getCurrentTenantId();
            userCode = userService.getCurrent().getCode();
        } catch (Exception e) {
            log.error("Erro não identificado ao resolver o usuário corrente", e);
            throw texto(Response.Status.INTERNAL_SERVER_ERROR, "Consulte o log do Fluig para mais informações.");
        }

        WorkflowObservationDto criado;
        try {
            criado = new WorkflowObservationService().create(tenantId, processInstanceId, userCode, req);
        } catch (ProcessInstanceNotFoundException e) {
            throw texto(Response.Status.NOT_FOUND, e.getMessage());
        } catch (ProcessInstanceConflictException e) {
            throw texto(Response.Status.CONFLICT, e.getMessage());
        } catch (Exception e) {
            Throwable causa = causaReal(e);
            log.error("Erro ao registrar observação na solicitação " + processInstanceId, causa);
            throw texto(Response.Status.INTERNAL_SERVER_ERROR, mensagemDe(causa));
        }

        // Auditoria no padrão do updateWorkflowEvents: quem, onde e quanto. O
        // texto não entra no log — ele já fica na própria solicitação.
        log.info(
            "Usuário \"{}\" registrou observação na solicitação {} (etapa {}, movimento {}, thread {}): {} caracteres",
            usuario(), processInstanceId, criado.getStateSequence(), criado.getMovementSequence(),
            criado.getThreadSequence(), req.getObservation().trim().length()
        );

        return Response.status(Response.Status.CREATED).entity(criado).build();
    }

    /**
     * Lista as observações de uma etapa/thread da solicitação, ordenadas por
     * data. Sem stateSequence, usa a tarefa corrente (mesma regra do POST).
     */
    @GET
    @Path("/{processInstanceId:[1-9][0-9]*}/observations")
    @Produces(MediaType.APPLICATION_JSON)
    public List<WorkflowObservationDto> listObservations(
        @PathParam("processInstanceId") int processInstanceId,
        @QueryParam("stateSequence") Integer stateSequence,
        @QueryParam("threadSequence") Integer threadSequence
    ) {
        long tenantId;
        try {
            tenantId = securityService.getCurrentTenantId();
        } catch (Exception e) {
            log.error("Erro não identificado ao resolver o tenant corrente", e);
            throw texto(Response.Status.INTERNAL_SERVER_ERROR, "Consulte o log do Fluig para mais informações.");
        }

        try {
            return new WorkflowObservationService().list(tenantId, processInstanceId, stateSequence, threadSequence);
        } catch (ProcessInstanceNotFoundException e) {
            throw texto(Response.Status.NOT_FOUND, e.getMessage());
        } catch (ProcessInstanceConflictException e) {
            throw texto(Response.Status.CONFLICT, e.getMessage());
        } catch (Exception e) {
            Throwable causa = causaReal(e);
            log.error("Erro ao listar observações da solicitação " + processInstanceId, causa);
            throw texto(Response.Status.INTERNAL_SERVER_ERROR, mensagemDe(causa));
        }
    }

    /** Desembrulha InvocationTargetException até a exceção real do EJB. */
    static Throwable causaReal(Throwable e) {
        Throwable t = e;
        while (t instanceof InvocationTargetException && t.getCause() != null) {
            t = t.getCause();
        }
        return t;
    }

    /**
     * Mensagem do 500: a do EJB quando ela existe, senão a orientação padrão.
     * O stack trace fica só no log do Fluig.
     */
    static String mensagemDe(Throwable causa) {
        String m = causa == null ? null : causa.getMessage();
        if (m == null || m.trim().isEmpty()) {
            return "Consulte o log do Fluig para mais informações.";
        }
        return m.trim();
    }

    private static WebApplicationException texto(Response.Status status, String message) {
        return new WebApplicationException(
            Response.status(status).entity(message).type(MediaType.TEXT_PLAIN_TYPE.withCharset("UTF-8")).build()
        );
    }

    /** Nomes dos eventos para a linha de auditoria (o código não entra no log). */
    static String nomesDosEventos(List<WorkflowEventDto> events) {
        StringBuilder sb = new StringBuilder();
        for (WorkflowEventDto event : events) {
            if (sb.length() > 0) {
                sb.append(", ");
            }
            sb.append(event.getName());
        }
        return sb.toString();
    }
}
