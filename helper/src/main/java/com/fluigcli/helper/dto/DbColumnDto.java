package com.fluigcli.helper.dto;

/** Coluna do resultado: nome + tipo SQL (nome do tipo devolvido pelo driver). */
public class DbColumnDto {
    private String name;
    private String type;

    public DbColumnDto() {}

    public DbColumnDto(String name, String type) {
        this.name = name;
        this.type = type;
    }

    public String getName() { return name; }
    public void setName(String name) { this.name = name; }
    public String getType() { return type; }
    public void setType(String type) { this.type = type; }
}
