package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/alorenco/fluig-cli/internal/output"
)

// Store resolve servidores considerando a precedência: arquivo do projeto
// (<projeto>/.fluigcli/servers.json) sobrepõe o global
// (~/.config/fluigcli/servers.json; %APPDATA%\fluigcli no Windows).
type Store struct {
	ProjectDir string // raiz do projeto ("" se não descoberta)
	globalDir  string // sobreponível em testes
}

func NewStore(projectDir string) *Store {
	return &Store{ProjectDir: projectDir}
}

func (st *Store) projectPath() string {
	if st.ProjectDir == "" {
		return ""
	}
	return filepath.Join(st.ProjectDir, ".fluigcli", "servers.json")
}

func (st *Store) globalPath() (string, error) {
	if st.globalDir != "" {
		return filepath.Join(st.globalDir, "servers.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("diretório de configuração do usuário indisponível: %w", err)
	}
	return filepath.Join(dir, "fluigcli", "servers.json"), nil
}

func readServersFile(path string) (*ServersFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &ServersFile{Version: serversFileVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var f ServersFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("arquivo %s inválido: %w", path, err)
	}
	return &f, nil
}

func writeServersFile(path string, f *ServersFile) error {
	if f.Version == "" {
		f.Version = serversFileVersion
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// List retorna os servidores visíveis: os do projeto primeiro e, depois, os
// globais cujo nome não foi sobreposto pelo projeto.
func (st *Store) List() ([]Server, error) {
	var servers []Server
	seen := map[string]bool{}

	if p := st.projectPath(); p != "" {
		f, err := readServersFile(p)
		if err != nil {
			return nil, err
		}
		for _, s := range f.Servers {
			servers = append(servers, s)
			seen[s.Name] = true
		}
	}

	gp, err := st.globalPath()
	if err != nil {
		return nil, err
	}
	gf, err := readServersFile(gp)
	if err != nil {
		return nil, err
	}
	for _, s := range gf.Servers {
		if !seen[s.Name] {
			servers = append(servers, s)
		}
	}
	return servers, nil
}

// DefaultName devolve o nome do servidor padrão, com precedência projeto >
// global ("" se nenhum padrão foi definido). Não valida se o servidor ainda
// existe — quem resolve trata o caso com uma mensagem melhor.
func (st *Store) DefaultName() (string, error) {
	if p := st.projectPath(); p != "" {
		f, err := readServersFile(p)
		if err != nil {
			return "", err
		}
		if f.Default != "" {
			return f.Default, nil
		}
	}
	gp, err := st.globalPath()
	if err != nil {
		return "", err
	}
	gf, err := readServersFile(gp)
	if err != nil {
		return "", err
	}
	return gf.Default, nil
}

// SetDefault define o servidor padrão no arquivo do projeto (se houver raiz)
// ou no global; global=true força o arquivo global. O servidor precisa existir
// em algum dos escopos.
func (st *Store) SetDefault(name string, global bool) (string, error) {
	if _, err := st.Get(name); err != nil {
		return "", err
	}
	path, err := st.targetPath(global)
	if err != nil {
		return "", err
	}
	f, err := readServersFile(path)
	if err != nil {
		return "", err
	}
	f.Default = name
	if err := writeServersFile(path, f); err != nil {
		return "", err
	}
	return path, nil
}

// Update aplica mutate ao servidor (no arquivo em que ele estiver) e persiste.
// Retorna o servidor já atualizado.
func (st *Store) Update(name string, mutate func(*Server)) (*Server, error) {
	paths := []string{}
	if p := st.projectPath(); p != "" {
		paths = append(paths, p)
	}
	gp, err := st.globalPath()
	if err != nil {
		return nil, err
	}
	paths = append(paths, gp)

	for _, path := range paths {
		f, err := readServersFile(path)
		if err != nil {
			return nil, err
		}
		for i := range f.Servers {
			if f.Servers[i].Name == name {
				mutate(&f.Servers[i])
				f.Servers[i].Name = name // o nome é a chave; não pode mudar aqui
				if err := writeServersFile(path, f); err != nil {
					return nil, err
				}
				return &f.Servers[i], nil
			}
		}
	}
	return nil, output.NotFoundf("servidor %q não encontrado", name)
}

// Get busca um servidor pelo nome, respeitando a precedência projeto > global.
func (st *Store) Get(name string) (*Server, error) {
	servers, err := st.List()
	if err != nil {
		return nil, err
	}
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i], nil
		}
	}
	return nil, output.NotFoundf(
		"servidor %q não encontrado; cadastre com: fluigcli server add --name %s ...", name, name)
}

// Add grava o servidor no arquivo do projeto (se houver raiz) ou no global.
// global=true força o arquivo global.
func (st *Store) Add(s Server, global bool) error {
	path, err := st.targetPath(global)
	if err != nil {
		return err
	}
	f, err := readServersFile(path)
	if err != nil {
		return err
	}
	for _, existing := range f.Servers {
		if existing.Name == s.Name {
			return output.Usagef("já existe um servidor chamado %q em %s; remova-o antes com: fluigcli server remove %s", s.Name, path, s.Name)
		}
	}
	f.Servers = append(f.Servers, s)
	return writeServersFile(path, f)
}

// Remove exclui o servidor pelo nome do arquivo em que ele estiver (projeto
// primeiro) e retorna o servidor removido (para limpar o keyring).
func (st *Store) Remove(name string) (*Server, error) {
	paths := []string{}
	if p := st.projectPath(); p != "" {
		paths = append(paths, p)
	}
	gp, err := st.globalPath()
	if err != nil {
		return nil, err
	}
	paths = append(paths, gp)

	var removed *Server
	for _, path := range paths {
		f, err := readServersFile(path)
		if err != nil {
			return nil, err
		}
		for i, s := range f.Servers {
			if s.Name == name {
				f.Servers = append(f.Servers[:i], f.Servers[i+1:]...)
				if err := writeServersFile(path, f); err != nil {
					return nil, err
				}
				removed = &s
				break
			}
		}
		if removed != nil {
			break
		}
	}
	if removed == nil {
		return nil, output.NotFoundf("servidor %q não encontrado", name)
	}

	// Se o nome sumiu de vez (não havia homônimo no outro escopo), um padrão
	// apontando para ele ficaria órfão — limpa nos dois arquivos.
	if _, err := st.Get(name); err != nil {
		for _, path := range paths {
			f, err := readServersFile(path)
			if err != nil {
				continue
			}
			if f.Default == name {
				f.Default = ""
				_ = writeServersFile(path, f)
			}
		}
	}
	return removed, nil
}

func (st *Store) targetPath(global bool) (string, error) {
	if !global {
		if p := st.projectPath(); p != "" {
			return p, nil
		}
	}
	return st.globalPath()
}
