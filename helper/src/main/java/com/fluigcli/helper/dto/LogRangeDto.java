package com.fluigcli.helper.dto;

import java.util.List;

public class LogRangeDto {
    private String file;
    private String from;
    private String to;
    private List<String> entries;
    private boolean truncated;

    public LogRangeDto() {}

    public LogRangeDto(String file, String from, String to, List<String> entries, boolean truncated) {
        this.file = file;
        this.from = from;
        this.to = to;
        this.entries = entries;
        this.truncated = truncated;
    }

    public String getFile() { return file; }
    public void setFile(String file) { this.file = file; }
    public String getFrom() { return from; }
    public void setFrom(String from) { this.from = from; }
    public String getTo() { return to; }
    public void setTo(String to) { this.to = to; }
    public List<String> getEntries() { return entries; }
    public void setEntries(List<String> entries) { this.entries = entries; }
    public boolean isTruncated() { return truncated; }
    public void setTruncated(boolean truncated) { this.truncated = truncated; }
}
