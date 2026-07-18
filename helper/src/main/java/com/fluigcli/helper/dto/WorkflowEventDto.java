package com.fluigcli.helper.dto;

public class WorkflowEventDto {
    private String name;
    private String contents;

    public WorkflowEventDto() {}

    public WorkflowEventDto(String name, String contents) {
        this.name = name;
        this.contents = contents;
    }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public String getContents() { return contents; }
    public void setContents(String contents) { this.contents = contents; }
}
