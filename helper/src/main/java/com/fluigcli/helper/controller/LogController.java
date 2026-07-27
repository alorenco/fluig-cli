package com.fluigcli.helper.controller;

import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

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

    /** Uma linha de auditoria por usuário+arquivo a cada minuto, no polling. */
    static final long JANELA_AUDITORIA_MS = 60_000L;

    private static final Map<String, Long> ULTIMA_LEITURA = new ConcurrentHashMap<>();

    /**
     * Diz se a leitura merece uma linha nova de auditoria, e já marca o
     * horário. Estado e relógio entram por parâmetro para o teste não depender
     * de espera real.
     *
     * A checagem não é atômica de propósito: duas requisições simultâneas podem
     * gerar duas linhas. Para auditoria, linha repetida é inofensiva — perder
     * linha é que não pode.
     */
    static boolean deveRegistrar(Map<String, Long> estado, String chave, long agora, long janela) {
        Long anterior = estado.get(chave);
        if (anterior != null && agora - anterior < janela) {
            return false;
        }
        estado.put(chave, agora);

        // Poda barata: o mapa só cresce com usuário × arquivo, mas não deve
        // virar vazamento num servidor que fica meses de pé.
        if (estado.size() > 500) {
            estado.entrySet().removeIf(e -> agora - e.getValue() >= janela);
        }
        return true;
    }

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
        // grep repetido = OU (a entrada passa se casar com qualquer um).
        @QueryParam("grep") List<String> grep
    ) {
        log.info("Usuário \"{}\" leu o log \"{}\" (tail, {} entradas)", usuario(), file, lines);

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
        // grep repetido = OU (idem tail).
        @QueryParam("grep") List<String> grep
    ) {
        log.info(
            "Usuário \"{}\" leu o log \"{}\" (intervalo de \"{}\" a \"{}\")",
            usuario(), file, from == null ? "" : from, to == null ? "" : to
        );

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
        // Esta rota serve o `log tail --follow`, que faz polling de segundo em
        // segundo. Registrar toda chamada encheria o server.log com a própria
        // leitura do server.log.
        //
        // A primeira tentativa foi registrar só `from == 0`, e ela NÃO
        // funcionava: o follow começa no fim do arquivo (offset = tamanho), e o
        // zero nunca chega. Medido ao vivo — nenhuma linha saía (§2.11-D).
        //
        // Agora vale uma janela por usuário+arquivo: uma linha por minuto, que
        // cobre a sessão de acompanhamento sem inundar o log.
        if (deveRegistrar(ULTIMA_LEITURA, usuario() + "|" + file,
                System.currentTimeMillis(), JANELA_AUDITORIA_MS)) {
            log.info("Usuário \"{}\" está acompanhando o log \"{}\"", usuario(), file);
        }

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

            log.info("Usuário \"{}\" efetuou download do log \"{}\"", usuario(), file);

            return in;
        } catch (FileNotFoundException e) {
            throw new NotFoundException();
        } catch (Exception e) {
            log.error("Erro ao efetuar download do log \"" + file + "\"", e);
            throw new InternalServerErrorException("Consulte o log do Fluig para mais informações.");
        }
    }
}
