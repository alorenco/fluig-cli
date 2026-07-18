package com.fluigcli.helper.service;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.util.List;

import javax.servlet.ServletContext;

import com.fluigcli.helper.dto.WidgetDto;
import com.fluigcli.helper.repository.WidgetRepository;

public class WidgetService {

    public List<WidgetDto> findAll() throws Exception {
        return new WidgetRepository().findAll();
    }

    public FileInputStream getWidgetFileInputStream(
        ServletContext servletContext,
        String filename
    ) throws FileNotFoundException {
        File widgetFile = new File(getWidgetPath(servletContext, filename));

        if (!widgetFile.exists() || !widgetFile.isFile()) {
            throw new FileNotFoundException();
        }

        return new FileInputStream(widgetFile);
    }

    // Os WARs das widgets ficam em <instalação>/appserver/apps; o caminho do
    // appserver é derivado do diretório real deste próprio webapp.
    private String getWidgetPath(ServletContext servletContext, String filename) {
        return servletContext
            .getRealPath("/")
            .replaceAll("^(.+appserver).*", "$1")
            + File.separator
            + "apps"
            + File.separator
            + filename;
    }
}
