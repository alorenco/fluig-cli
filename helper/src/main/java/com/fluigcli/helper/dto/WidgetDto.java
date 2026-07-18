package com.fluigcli.helper.dto;

public class WidgetDto {
    private String code;
    private String title;
    private String description;
    private String filename;

    public WidgetDto() {}

    public WidgetDto(String code, String title, String description, String filename) {
        this.code = code;
        this.title = title;
        this.description = description;
        this.filename = filename;
    }

    public String getCode() { return code; }
    public void setCode(String code) { this.code = code; }
    public String getTitle() { return title; }
    public void setTitle(String title) { this.title = title; }
    public String getDescription() { return description; }
    public void setDescription(String description) { this.description = description; }
    public String getFilename() { return filename; }
    public void setFilename(String filename) { this.filename = filename; }
}
