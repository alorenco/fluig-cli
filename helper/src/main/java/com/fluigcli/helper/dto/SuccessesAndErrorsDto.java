package com.fluigcli.helper.dto;

import java.util.ArrayList;
import java.util.List;

public class SuccessesAndErrorsDto {
    private final List<String> successes = new ArrayList<>();
    private final List<String> errors = new ArrayList<>();

    public void addSuccess(String success) {
        successes.add(success);
    }

    public List<String> getSuccesses() {
        return successes;
    }

    public void addError(String error) {
        errors.add(error);
    }

    public List<String> getErrors() {
        return errors;
    }
}
