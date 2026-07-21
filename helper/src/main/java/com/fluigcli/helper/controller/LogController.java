package com.fluigcli.helper.controller;

import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.util.List;

import javax.ws.rs.DefaultValue;
import javax.ws.rs.GET;
import javax.ws.rs.InternalServerErrorException;
import javax.ws.rs.NotFoundException;
import javax.ws.rs.Path;
import javax.ws.rs.PathParam;
import javax.ws.rs.Produces;
import javax.ws.rs.QueryParam;
import javax.ws.rs.core.MediaType;

import com.fluigcli.helper.dto.LogChunkDto;
import com.fluigcli.helper.dto.LogFileDto;
import com.fluigcli.helper.dto.LogRangeDto;
import com.fluigcli.helper.dto.LogTailDto;
import com.fluigcli.helper.service.LogService;

@Path("/logs")
public class LogController extends BaseController {

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    public List<LogFileDto> list() {
        try {
            return new LogService().list();
        } catch (FileNotFoundException e) {
            log.error("Diretório de log não encontrado", e);
            throw new InternalServerErrorException("Diretório de log não encontrado no servidor.");
        }
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @Path("/{file: [a-zA-Z0-9_.\\-]+}/tail")
    public LogTailDto tail(
        @PathParam("file") String file,
        @QueryParam("lines") @DefaultValue("100") int lines,
        @QueryParam("skip") @DefaultValue("0") int skip,
        @QueryParam("level") String level,
        @QueryParam("grep") String grep
    ) {
        try {
            return new LogService().tail(file, lines, skip, level, grep);
        } catch (FileNotFoundException e) {
            throw new NotFoundException();
        } catch (Exception e) {
            log.error("Erro ao ler o log \"" + file + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @Path("/{file: [a-zA-Z0-9_.\\-]+}/range")
    public LogRangeDto range(
        @PathParam("file") String file,
        @QueryParam("from") String from,
        @QueryParam("to") String to,
        @QueryParam("level") String level,
        @QueryParam("grep") String grep
    ) {
        try {
            return new LogService().range(file, from, to, level, grep);
        } catch (FileNotFoundException e) {
            throw new NotFoundException();
        } catch (Exception e) {
            log.error("Erro ao ler o log \"" + file + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }

    @GET
    @Produces(MediaType.APPLICATION_JSON)
    @Path("/{file: [a-zA-Z0-9_.\\-]+}/read")
    public LogChunkDto read(
        @PathParam("file") String file,
        @QueryParam("from") @DefaultValue("0") long from
    ) {
        try {
            return new LogService().read(file, from);
        } catch (FileNotFoundException e) {
            throw new NotFoundException();
        } catch (Exception e) {
            log.error("Erro ao ler o log \"" + file + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }

    @GET
    @Produces(MediaType.APPLICATION_OCTET_STREAM)
    @Path("/{file: [a-zA-Z0-9_.\\-]+}/download")
    public FileInputStream download(@PathParam("file") String file) {
        try {
            FileInputStream in = new LogService().openForDownload(file);

            log.info(
                "Usuário \"{}\" efetuou download do log \"{}\"",
                userService.getCurrent().getLogin(),
                file
            );

            return in;
        } catch (FileNotFoundException e) {
            throw new NotFoundException();
        } catch (Exception e) {
            log.error("Erro ao efetuar download do log \"" + file + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }
}
