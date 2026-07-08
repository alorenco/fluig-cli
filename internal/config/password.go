package config

import (
	"bufio"
	"io"
	"strings"

	"github.com/alorenco/fluig-cli/internal/output"
)

// EnvPassword é a variável de ambiente de senha (vale para o servidor selecionado).
const EnvPassword = "FLUIGCLI_PASSWORD"

// Source identifica de onde a senha resolvida veio.
type Source int

const (
	SourceStdin Source = iota
	SourceEnv
	SourceKeyring
	SourcePrompt
)

// PromptFunc pergunta a senha ao usuário (eco desligado). Retorna também se a
// senha deve ser salva no keyring.
type PromptFunc func(server *Server) (password string, save bool, err error)

// PasswordResult é a senha resolvida e sua origem. A gravação no keyring é
// deferida (SaveIfRequested) para acontecer só após a senha ser validada.
type PasswordResult struct {
	Password string
	Source   Source

	save     bool
	keyring  Keyring
	serverID string
}

// SaveIfRequested grava no keyring apenas quando o usuário pediu para salvar
// (senha vinda de prompt). Deve ser chamada só após autenticação bem-sucedida,
// para nunca persistir uma senha errada.
func (r *PasswordResult) SaveIfRequested() error {
	if r == nil || !r.save || r.keyring == nil || r.serverID == "" || !r.keyring.Available() {
		return nil
	}
	return r.keyring.Set(r.serverID, r.Password)
}

// PasswordSource resolve a senha de um servidor na ordem de precedência
// (stdin → env var → keyring → prompt). Campos nil/vazios pulam o respectivo degrau.
type PasswordSource struct {
	Stdin   io.Reader // não-nil quando --password-stdin foi passado
	Getenv  func(string) string
	Keyring Keyring
	Prompt  PromptFunc // nil em modo não-interativo
}

// Resolve retorna a senha do servidor ou erro AUTH_FAILED (exit 3) quando
// nenhuma fonte está disponível. NÃO grava no keyring — isso é responsabilidade
// de PasswordResult.SaveIfRequested, chamada após a validação.
func (ps PasswordSource) Resolve(server *Server) (*PasswordResult, error) {
	if ps.Stdin != nil {
		pw, err := readPasswordStdin(ps.Stdin)
		if err != nil {
			return nil, err
		}
		return &PasswordResult{Password: pw, Source: SourceStdin}, nil
	}

	if ps.Getenv != nil {
		if pw := ps.Getenv(EnvPassword); pw != "" {
			return &PasswordResult{Password: pw, Source: SourceEnv}, nil
		}
	}

	if ps.Keyring != nil && ps.Keyring.Available() && server.ID != "" {
		// Keyring indisponível (ex.: Linux headless) é ignorado em silêncio: a
		// resolução segue para o prompt/erro sem poluir a saída.
		if pw, err := ps.Keyring.Get(server.ID); err == nil && pw != "" {
			return &PasswordResult{Password: pw, Source: SourceKeyring}, nil
		}
	}

	if ps.Prompt != nil {
		pw, save, err := ps.Prompt(server)
		if err != nil {
			return nil, err
		}
		return &PasswordResult{
			Password: pw,
			Source:   SourcePrompt,
			save:     save,
			keyring:  ps.Keyring,
			serverID: server.ID,
		}, nil
	}

	return nil, output.AuthFailedf(
		"nenhuma senha disponível para o servidor %q; informe via --password-stdin, "+
			"variável %s ou grave no keyring com: fluigcli server add", server.Name, EnvPassword)
}

func readPasswordStdin(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", output.AuthFailedf("falha ao ler a senha do stdin: %v", err)
	}
	pw := strings.TrimRight(line, "\r\n")
	if pw == "" {
		return "", output.AuthFailedf("senha vazia recebida via --password-stdin")
	}
	return pw, nil
}
