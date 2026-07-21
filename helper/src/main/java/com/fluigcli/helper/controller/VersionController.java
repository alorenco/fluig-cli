package com.fluigcli.helper.controller;

import java.io.InputStream;
import java.time.Instant;
import java.time.ZoneId;
import java.util.Properties;

import javax.ws.rs.GET;
import javax.ws.rs.Path;
import javax.ws.rs.Produces;
import javax.ws.rs.core.MediaType;

import com.fluigcli.helper.dto.HelperInfoDto;

@Path("/version")
public class VersionController extends BaseController {

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    public HelperInfoDto version() {
        // A versão vem do application.info (WEB-INF/classes), o manifesto do
        // widget — fonte única, atualizada a cada release do helper.
        String version = "";
        try (InputStream in = getClass().getResourceAsStream("/application.info")) {
            if (in != null) {
                Properties props = new Properties();
                props.load(in);
                version = props.getProperty("application.version", "");
            }
        } catch (Exception e) {
            log.error("Erro ao ler a versão do application.info", e);
        }

        // Fuso da JVM: é a mesma zona em que o server.log é escrito (o timestamp
        // do log não traz offset), então o painel converte para o navegador.
        String zoneId = "";
        Integer offsetMinutes = null;
        try {
            ZoneId zone = ZoneId.systemDefault();
            zoneId = zone.getId();
            offsetMinutes = zone.getRules().getOffset(Instant.now()).getTotalSeconds() / 60;
        } catch (Exception e) {
            log.error("Erro ao resolver o fuso da JVM", e);
        }

        return new HelperInfoDto("fluigcliHelper", version, zoneId, offsetMinutes);
    }
}
