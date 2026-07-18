package com.fluigcli.helper.dto;

import java.util.List;

public class LogTailDto {
    private String file;
    private long size;
    private List<String> entries;
    private boolean truncated;

    public LogTailDto() {}

    public LogTailDto(String file, long size, List<String> entries, boolean truncated) {
        this.file = file;
        this.size = size;
        this.entries = entries;
        this.truncated = truncated;
    }

    public String getFile() { return file; }
    public void setFile(String file) { this.file = file; }
    public long getSize() { return size; }
    public void setSize(long size) { this.size = size; }
    public List<String> getEntries() { return entries; }
    public void setEntries(List<String> entries) { this.entries = entries; }
    public boolean isTruncated() { return truncated; }
    public void setTruncated(boolean truncated) { this.truncated = truncated; }
}
