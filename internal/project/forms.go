package project

import (
	"os"
	"path/filepath"
	"strings"
)

// FormsDirName é a pasta convencional dos formulários.
const FormsDirName = "forms"

// formEventsSubdir é a subpasta de eventos dentro de um formulário.
const formEventsSubdir = "events"

// FormDir devolve o diretório de um formulário: <root>/forms/<name>.
func FormDir(root, name string) string {
	return filepath.Join(root, FormsDirName, name)
}

// FormFolderContents são os caminhos dos arquivos de um formulário local.
type FormFolderContents struct {
	// Files são os anexos (arquivos no topo da pasta, exceto a subpasta events/).
	Files []string
	// EventFiles são os .js sob events/.
	EventFiles []string
}

// ReadFormFolder lê a estrutura de uma pasta de formulário: arquivos do topo
// viram anexos; os .js sob events/ viram eventos. Subpastas (além
// de events/) são ignoradas — anexos do Fluig têm nome plano.
func ReadFormFolder(dir string) (*FormFolderContents, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fc := &FormFolderContents{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fc.Files = append(fc.Files, filepath.Join(dir, e.Name()))
	}

	eventsDir := filepath.Join(dir, formEventsSubdir)
	eventEntries, err := os.ReadDir(eventsDir)
	if err == nil {
		for _, e := range eventEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
				continue
			}
			fc.EventFiles = append(fc.EventFiles, filepath.Join(eventsDir, e.Name()))
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return fc, nil
}

// FormEventsDir devolve a subpasta events/ de um formulário.
func FormEventsDir(formDir string) string {
	return filepath.Join(formDir, formEventsSubdir)
}
