package project

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// FormLink liga uma pasta local de formulário ao formulário no servidor.
// Resolve o problema de a pasta local ter nome diferente do documentDescription.
type FormLink struct {
	Folder      string `json:"folder"`
	DocumentID  int    `json:"documentId"`
	Name        string `json:"name"`
	DatasetName string `json:"datasetName,omitempty"`
}

// FormMap é o mapeamento persistido em <root>/.fluigcli/forms.json, com os
// vínculos agrupados POR SERVIDOR (chave host:porta/companyId): documentId e
// nome do formulário variam por ambiente — o mesmo form é um id na homologação
// e outro na produção. As consultas e o Upsert operam no servidor ativo; o
// Save preserva os buckets dos demais servidores.
//
// Schema v2 (2026-07-09). O v1 (lista única, sem servidor) foi abandonado sem
// retrocompatibilidade por decisão do mantenedor — arquivo v1 é tratado como
// vazio e reescrito no primeiro Save.
type FormMap struct {
	path string
	key  string // servidor ativo (host:porta/companyId)
	file formMapFile
}

type formMapFile struct {
	Version string                `json:"version"`
	Servers map[string][]FormLink `json:"servers"`
}

const formMapVersion = "2.0.0"

// FormMapPath devolve o caminho do mapa de formulários do projeto.
func FormMapPath(root string) string {
	return filepath.Join(root, ".fluigcli", "forms.json")
}

// LoadFormMap carrega o mapa do projeto já posicionado no servidor dado
// (serverKey = host:porta/companyId). Arquivo ausente ou em schema antigo
// resulta em mapa vazio.
func LoadFormMap(root, serverKey string) (*FormMap, error) {
	path := FormMapPath(root)
	m := &FormMap{path: path, key: serverKey, file: formMapFile{
		Version: formMapVersion,
		Servers: map[string][]FormLink{},
	}}
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
	if f.Version == formMapVersion && f.Servers != nil {
		m.file = f
	}
	return m, nil
}

// links devolve o bucket do servidor ativo.
func (m *FormMap) links() []FormLink {
	return m.file.Servers[m.key]
}

// ByFolder busca o vínculo pelo nome da pasta (no servidor ativo).
func (m *FormMap) ByFolder(folder string) (FormLink, bool) {
	for _, l := range m.links() {
		if l.Folder == folder {
			return l, true
		}
	}
	return FormLink{}, false
}

// ByDocumentID busca o vínculo pelo documentId (no servidor ativo).
func (m *FormMap) ByDocumentID(id int) (FormLink, bool) {
	for _, l := range m.links() {
		if l.DocumentID == id {
			return l, true
		}
	}
	return FormLink{}, false
}

// ByName busca o vínculo pelo nome do formulário (no servidor ativo).
func (m *FormMap) ByName(name string) (FormLink, bool) {
	for _, l := range m.links() {
		if l.Name == name {
			return l, true
		}
	}
	return FormLink{}, false
}

// FolderNameHint procura nos buckets dos OUTROS servidores o nome de
// formulário já vinculado a esta pasta — a melhor pista para o mapeamento
// inicial num servidor novo: o nome tende a ser igual entre ambientes, só o
// documentId muda. Devolve o primeiro achado, em ordem estável de chave.
func (m *FormMap) FolderNameHint(folder string) (name, serverKey string, ok bool) {
	keys := make([]string, 0, len(m.file.Servers))
	for k := range m.file.Servers {
		if k != m.key {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, l := range m.file.Servers[k] {
			if l.Folder == folder && l.Name != "" {
				return l.Name, k, true
			}
		}
	}
	return "", "", false
}

// Upsert insere ou atualiza o vínculo no servidor ativo, chaveado pela pasta.
func (m *FormMap) Upsert(link FormLink) {
	bucket := m.links()
	for i := range bucket {
		if bucket[i].Folder == link.Folder {
			bucket[i] = link
			return
		}
	}
	m.file.Servers[m.key] = append(bucket, link)
}

// Save grava o mapa em disco (cria .fluigcli/ se preciso), preservando os
// vínculos dos demais servidores.
func (m *FormMap) Save() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, append(data, '\n'), 0o644)
}
