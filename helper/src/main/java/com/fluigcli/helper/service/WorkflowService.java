package com.fluigcli.helper.service;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import com.fluigcli.helper.dto.SuccessesAndErrorsDto;
import com.fluigcli.helper.dto.WorkflowEventDto;
import com.fluigcli.helper.dto.WorkflowUpdatedEventsDto;
import com.fluigcli.helper.repository.WorkflowRepository;

public class WorkflowService {

    public int getMaxVersion(long tenantId, String processId) throws Exception {
        return new WorkflowRepository().findMaxVersion(tenantId, processId);
    }

    public WorkflowUpdatedEventsDto updateEvents(
        long tenantId,
        String processId,
        int version,
        String userCode,
        List<WorkflowEventDto> events
    ) {
        var repository = new WorkflowRepository();

        Set<String> workflowEvents = repository.getEventsFromWorkflow(tenantId, processId, version);

        List<WorkflowEventDto> eventsToUpdate = new ArrayList<>();
        List<WorkflowEventDto> eventsToCreate = new ArrayList<>();

        for (WorkflowEventDto event : events) {
            if (workflowEvents.contains(event.getName())) {
                eventsToUpdate.add(event);
                continue;
            }

            eventsToCreate.add(event);
        }

        SuccessesAndErrorsDto updatedEvents = repository.updateEvents(tenantId, processId, version, eventsToUpdate);
        SuccessesAndErrorsDto createdEvents = repository.createEvents(tenantId, processId, version, userCode, eventsToCreate);

        var result = new WorkflowUpdatedEventsDto();
        result.setProcessId(processId);
        result.setVersion(version);
        result.setHasError(!updatedEvents.getErrors().isEmpty() || !createdEvents.getErrors().isEmpty());
        result.setTotalProcessed(
            updatedEvents.getSuccesses().size()
            + updatedEvents.getErrors().size()
            + createdEvents.getSuccesses().size()
            + createdEvents.getErrors().size()
        );

        result.setErrors(
            Stream
                .concat(updatedEvents.getErrors().stream(), createdEvents.getErrors().stream())
                .collect(Collectors.toList())
        );

        result.setSuccesses(
            Stream
                .concat(updatedEvents.getSuccesses().stream(), createdEvents.getSuccesses().stream())
                .collect(Collectors.toList())
        );

        return result;
    }
}
