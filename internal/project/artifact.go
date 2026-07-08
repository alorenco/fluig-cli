package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SafeJoin junta base + segmentos garantindo que o resultado fique estritamente
// dentro de base. Rejeita traversal (`..`) — essencial ao gravar arquivos cujos
// nomes vêm do servidor (dataset id, nome de anexo, id de evento, código de
// widget etc.), evitando escrita fora do diretório do projeto.
func SafeJoin(base string, untrusted ...string) (string, error) {
	joined := filepath.Join(append([]string{base}, untrusted...)...)
	rel, err := filepath.Rel(base, joined)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("nome inseguro (fora do diretório permitido): %q", filepath.Join(untrusted...))
	}
	return joined, nil
}

// Pastas convencionais por tipo de artefato.
const (
	DatasetsDirName   = "datasets"
	EventsDirName     = "events"
	MechanismsDirName = "mechanisms"
)

// ArtifactName deriva o nome do artefato a partir do caminho: basename sem .js.
func ArtifactName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".js")
}

// FindArtifactFile procura, sob <root>/<subdir> (recursivo), um arquivo
// <name>.js. Retorna o primeiro caminho encontrado e todos os matches (para o
// chamador avisar em caso de ambiguidade). Vazio se não achar.
func FindArtifactFile(root, subdir, name string) (path string, matches []string, err error) {
	base := filepath.Join(root, subdir)
	target := name + ".js"
	walkErr := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if !d.IsDir() && d.Name() == target {
			matches = append(matches, p)
		}
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return "", nil, walkErr
	}
	if len(matches) > 0 {
		return matches[0], matches, nil
	}
	return "", nil, nil
}

// DefaultArtifactPath é onde um artefato novo é gravado: <root>/<subdir>/<name>.js.
// Confina em <root>/<subdir> — erro se `name` (vindo do servidor) tentar sair.
func DefaultArtifactPath(root, subdir, name string) (string, error) {
	return SafeJoin(filepath.Join(root, subdir), name+".js")
}
