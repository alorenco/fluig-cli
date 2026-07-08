package project

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FormLink liga uma pasta local de formulário ao formulário no servidor.
// Resolve o problema de a pasta local ter nome diferente do documentDescription
type FormLink struct {
	Folder      string `json:"folder"`
	DocumentID  int    `json:"documentId"`
	Name        string `json:"name"`
	DatasetName string `json:"datasetName,omitempty"`
}

// FormMap é o mapeamento persistido em <root>/.fluigcli/forms.json.
type FormMap struct {
	path  string
	links []FormLink
}

type formMapFile struct {
	Version string     `json:"version"`
	Forms   []FormLink `json:"forms"`
}

const formMapVersion = "1.0.0"

// FormMapPath devolve o caminho do mapa de formulários do projeto.
func FormMapPath(root string) string {
	return filepath.Join(root, ".fluigcli", "forms.json")
}

// LoadFormMap carrega o mapa (vazio se o arquivo não existe).
func LoadFormMap(root string) (*FormMap, error) {
	path := FormMapPath(root)
	m := &FormMap{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	var f formMapFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	m.links = f.Forms
	return m, nil
}

// ByFolder busca o vínculo pelo nome da pasta.
func (m *FormMap) ByFolder(folder string) (FormLink, bool) {
	for _, l := range m.links {
		if l.Folder == folder {
			return l, true
		}
	}
	return FormLink{}, false
}

// ByDocumentID busca o vínculo pelo documentId.
func (m *FormMap) ByDocumentID(id int) (FormLink, bool) {
	for _, l := range m.links {
		if l.DocumentID == id {
			return l, true
		}
	}
	return FormLink{}, false
}

// ByName busca o vínculo pelo nome (documentDescription).
func (m *FormMap) ByName(name string) (FormLink, bool) {
	for _, l := range m.links {
		if l.Name == name {
			return l, true
		}
	}
	return FormLink{}, false
}

// Upsert insere ou atualiza o vínculo, chaveado pela pasta.
func (m *FormMap) Upsert(link FormLink) {
	for i := range m.links {
		if m.links[i].Folder == link.Folder {
			m.links[i] = link
			return
		}
	}
	m.links = append(m.links, link)
}

// Save grava o mapa em disco (cria .fluigcli/ se preciso).
func (m *FormMap) Save() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(formMapFile{Version: formMapVersion, Forms: m.links}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, append(data, '\n'), 0o644)
}
