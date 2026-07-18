package com.fluigcli.helper.dto;

public class LogChunkDto {
    private String file;
    private long from;
    private long to;
    private long size;
    private String content;

    public LogChunkDto() {}

    public LogChunkDto(String file, long from, long to, long size, String content) {
        this.file = file;
        this.from = from;
        this.to = to;
        this.size = size;
        this.content = content;
    }

    public String getFile() { return file; }
    public void setFile(String file) { this.file = file; }
    public long getFrom() { return from; }
    public void setFrom(long from) { this.from = from; }
    public long getTo() { return to; }
    public void setTo(long to) { this.to = to; }
    public long getSize() { return size; }
    public void setSize(long size) { this.size = size; }
    public String getContent() { return content; }
    public void setContent(String content) { this.content = content; }
}
