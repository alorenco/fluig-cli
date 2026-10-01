package project

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LayoutsDir é a pasta convencional dos layouts WCM: wcm/layout/<código>.
// A estrutura interna é a mesma do widget (src/main/resources, src/main/webapp),
// e o WAR sai pelo mesmo mapa de CollectWidgetWARFiles.
var LayoutsDir = filepath.Join("wcm", "layout")

// LayoutDir devolve o diretório de um layout.
func LayoutDir(root, code string) string {
	return filepath.Join(root, LayoutsDir, code)
}

// ApplicationInfoRel é o caminho do application.info dentro da pasta de um
// widget ou layout. É o arquivo que declara o tipo do artefato
// (application.type=widget|layout).
var ApplicationInfoRel = filepath.Join("src", "main", "resources", "application.info")

// ReadApplicationType lê o application.type do application.info de uma pasta
// de widget/layout. Devolve "" quando o arquivo existe mas não declara a
// chave; arquivo ausente → erro que satisfaz errors.Is(err, os.ErrNotExist).
//
// O arquivo segue o formato java.util.Properties simplificado que o Fluig
// usa: uma chave=valor por linha, comentários com # ou !. Só o "=" é aceito
// como separador — é o único que aparece nos artefatos gerados pelo Studio.
func ReadApplicationType(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, ApplicationInfoRel))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "application.type" {
			continue
		}
		return strings.TrimSpace(value), nil
	}
	return "", sc.Err()
}
