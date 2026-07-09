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

// localPath é o overlay pessoal do projeto (git-ignorado). "" fora de projeto.
func (st *Store) localPath() string {
	if st.ProjectDir == "" {
		return ""
	}
	return filepath.Join(st.ProjectDir, ".fluigcli", "servers.local.json")
}

func (st *Store) readLocal() (*LocalFile, error) {
	p := st.localPath()
	if p == "" {
		return &LocalFile{Version: serversFileVersion}, nil
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &LocalFile{Version: serversFileVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var f LocalFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("arquivo %s inválido: %w", p, err)
	}
	return &f, nil
}

func (st *Store) writeLocal(f *LocalFile) error {
	p := st.localPath()
	if p == "" {
		return fmt.Errorf("identidade local exige um projeto (rode dentro da raiz do projeto ou use --global)")
	}
	if f.Version == "" {
		f.Version = serversFileVersion
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o600)
}

// applyIdentity preenche Username/UserCode de um servidor do projeto a partir do
// overlay local (só quando o próprio servidor ainda não os tem).
func applyIdentity(s *Server, local *LocalFile) {
	if local == nil {
		return
	}
	for _, id := range local.Identities {
		if id.Name != s.Name {
			continue
		}
		if s.Username == "" {
			s.Username = id.Username
		}
		if s.UserCode == "" {
			s.UserCode = id.UserCode
		}
		break
	}
	if s.UserCode == "" {
		s.UserCode = s.Username
	}
}

// SetIdentity grava (ou atualiza) a identidade local para um servidor do projeto.
func (st *Store) SetIdentity(name, username, userCode string) error {
	local, err := st.readLocal()
	if err != nil {
		return err
	}
	for i := range local.Identities {
		if local.Identities[i].Name == name {
			local.Identities[i].Username = username
			local.Identities[i].UserCode = userCode
			return st.writeLocal(local)
		}
	}
	local.Identities = append(local.Identities, Identity{Name: name, Username: username, UserCode: userCode})
	return st.writeLocal(local)
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
		local, err := st.readLocal()
		if err != nil {
			return nil, err
		}
		for _, s := range f.Servers {
			applyIdentity(&s, local)
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
	// Padrão pessoal do projeto (overlay local) vence tudo.
	local, err := st.readLocal()
	if err != nil {
		return "", err
	}
	if local.Default != "" {
		return local.Default, nil
	}
	// Legado v1: padrão gravado no próprio servers.json do projeto.
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

// SetDefault define o servidor padrão. No projeto (padrão) grava no overlay
// pessoal (servers.local.json) — assim seu padrão não vaza para o time no
// servers.json versionado; global=true grava o padrão no arquivo global. O
// servidor precisa existir em algum dos escopos.
func (st *Store) SetDefault(name string, global bool) (string, error) {
	if _, err := st.Get(name); err != nil {
		return "", err
	}
	if !global {
		if lp := st.localPath(); lp != "" {
			local, err := st.readLocal()
			if err != nil {
				return "", err
			}
			local.Default = name
			if err := st.writeLocal(local); err != nil {
				return "", err
			}
			return lp, nil
		}
	}
	gp, err := st.globalPath()
	if err != nil {
		return "", err
	}
	f, err := readServersFile(gp)
	if err != nil {
		return "", err
	}
	f.Default = name
	if err := writeServersFile(gp, f); err != nil {
		return "", err
	}
	return gp, nil
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

	projectShared := st.projectPath()
	for _, path := range paths {
		f, err := readServersFile(path)
		if err != nil {
			return nil, err
		}
		for i := range f.Servers {
			if f.Servers[i].Name != name {
				continue
			}
			// No arquivo do projeto, a identidade mora no overlay: aplica o
			// mutate sobre a identidade atual, mas grava só conexão no arquivo
			// versionado e roteia username/userCode para o servers.local.json.
			if path == projectShared {
				local, err := st.readLocal()
				if err != nil {
					return nil, err
				}
				applyIdentity(&f.Servers[i], local)
				mutate(&f.Servers[i])
				f.Servers[i].Name = name
				eff := f.Servers[i]
				f.Servers[i].Username, f.Servers[i].UserCode, f.Servers[i].ID = "", "", ""
				if err := writeServersFile(path, f); err != nil {
					return nil, err
				}
				if eff.Username != "" {
					if err := st.SetIdentity(name, eff.Username, eff.UserCode); err != nil {
						return nil, err
					}
				}
				return &eff, nil
			}
			mutate(&f.Servers[i])
			f.Servers[i].Name = name // o nome é a chave; não pode mudar aqui
			if err := writeServersFile(path, f); err != nil {
				return nil, err
			}
			return &f.Servers[i], nil
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

// Add grava o servidor. No projeto (padrão), os fatos de conexão vão para o
// servers.json versionável e a identidade (username/userCode) para o overlay
// pessoal servers.local.json — nunca ao mesmo arquivo. No global (ou fora de
// projeto) grava tudo junto, pois o global é pessoal.
func (st *Store) Add(s Server, global bool) error {
	if !global {
		if p := st.projectPath(); p != "" {
			return st.addProject(p, s)
		}
	}
	gp, err := st.globalPath()
	if err != nil {
		return err
	}
	return addToFile(gp, s)
}

func addToFile(path string, s Server) error {
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

// addProject grava só a conexão no servers.json (sem identidade nem id, seguros
// para versionar) e a identidade no overlay local.
func (st *Store) addProject(path string, s Server) error {
	conn := s
	conn.Username, conn.UserCode, conn.ID = "", "", "" // identidade sai do arquivo do time
	if err := addToFile(path, conn); err != nil {
		return err
	}
	if s.Username != "" {
		return st.SetIdentity(s.Name, s.Username, s.UserCode)
	}
	return nil
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

	// Overlay da identidade no servidor removido (o arquivo do time não a tem),
	// para o chamador conseguir apagar a senha certa do keyring.
	if local, err := st.readLocal(); err == nil {
		applyIdentity(removed, local)
	}

	// Se o nome sumiu de vez (não havia homônimo no outro escopo), o overlay
	// local (identidade + padrão) e um padrão nos arquivos ficariam órfãos.
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
		if local, err := st.readLocal(); err == nil && st.localPath() != "" {
			changed := false
			if local.Default == name {
				local.Default = ""
				changed = true
			}
			for i := range local.Identities {
				if local.Identities[i].Name == name {
					local.Identities = append(local.Identities[:i], local.Identities[i+1:]...)
					changed = true
					break
				}
			}
			if changed {
				_ = st.writeLocal(local)
			}
		}
	}
	return removed, nil
}
