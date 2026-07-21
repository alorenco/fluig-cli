package com.fluigcli.helper.dto;

public class HelperInfoDto {
    private String name;
    private String version;
    // Fuso da JVM do servidor (a mesma que escreve o server.log, cujo timestamp
    // não carrega offset). Permite ao painel de logs converter para o fuso do
    // navegador. offsetMinutes = deslocamento em relação ao UTC, "agora".
    private String zoneId;
    private Integer offsetMinutes;

    public HelperInfoDto() {}

    public HelperInfoDto(String name, String version) {
        this.name = name;
        this.version = version;
    }

    public HelperInfoDto(String name, String version, String zoneId, Integer offsetMinutes) {
        this.name = name;
        this.version = version;
        this.zoneId = zoneId;
        this.offsetMinutes = offsetMinutes;
    }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public String getVersion() { return version; }
    public void setVersion(String version) { this.version = version; }
    public String getZoneId() { return zoneId; }
    public void setZoneId(String zoneId) { this.zoneId = zoneId; }
    public Integer getOffsetMinutes() { return offsetMinutes; }
    public void setOffsetMinutes(Integer offsetMinutes) { this.offsetMinutes = offsetMinutes; }
}
