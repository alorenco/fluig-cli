// Package config gerencia os arquivos servers.json (sem segredos), o keyring
// do SO e a resolução de senha por precedência.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Server descreve um servidor Fluig registrado. Nunca contém senha.
//
// A partir do schema v2, no arquivo do projeto (versionado, compartilhado com o
// time) só ficam os campos de conexão — Username/UserCode/ID vêm do overlay
// pessoal (servers.local.json, git-ignorado) ou do global. Por isso esses
// campos são omitempty: o servers.json compartilhado sai sem identidade.
type Server struct {
	ID        string `json:"id,omitempty"` // legado v1; não é mais gerado (keyring usa KeyringKey)
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	SSL       bool   `json:"ssl"`
	Username  string `json:"username,omitempty"`
	UserCode  string `json:"userCode,omitempty"`
	CompanyID int    `json:"companyId"`
	Env       string `json:"env,omitempty"` // dev | hml | prod ("" = não informado)
}

// KeyringKey é a chave da senha no keyring do SO: derivada de baseURL+usuário,
// estável entre execuções e independente de um id gravado em arquivo (o id v1
// morava no servers.json e vazava ao versionar). Vazia sem usuário resolvido.
func (s *Server) KeyringKey() string {
	if s.Username == "" {
		return ""
	}
	return s.BaseURL() + "|" + s.Username
}

// FormScopeKey é a chave do bucket do servidor no .fluigcli/forms.json
// (host:porta/companyId): documentId e nome de formulário variam por ambiente
// — e por empresa, no multi-tenant — então o vínculo pasta↔form só vale
// dentro deste escopo. Independe do nome dado ao servidor (que é local de
// cada máquina) e de SSL.
func (s *Server) FormScopeKey() string {
	return fmt.Sprintf("%s:%d/%d", s.Host, s.Port, s.CompanyID)
}

// Ambientes canônicos de um servidor.
const (
	EnvDev  = "dev"
	EnvHml  = "hml"
	EnvProd = "prod"
)

// NormalizeEnv aceita apelidos comuns e devolve o ambiente canônico
// (dev/hml/prod). Vazio é válido (ambiente não informado).
func NormalizeEnv(env string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "":
		return "", nil
	case EnvDev, "desenvolvimento", "development":
		return EnvDev, nil
	case EnvHml, "homolog", "homologacao", "homologação", "staging":
		return EnvHml, nil
	case EnvProd, "producao", "produção", "production", "prd":
		return EnvProd, nil
	default:
		return "", fmt.Errorf("ambiente inválido %q (use dev, hml ou prod)", env)
	}
}

// BaseURL monta a URL base do servidor (ex.: https://fluig.empresa.com.br:8443).
func (s *Server) BaseURL() string {
	scheme := "http"
	if s.SSL {
		scheme = "https"
	}
	defaultPort := (s.SSL && s.Port == 443) || (!s.SSL && s.Port == 80)
	if defaultPort {
		return fmt.Sprintf("%s://%s", scheme, s.Host)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, s.Host, s.Port)
}

// ServersFile é o formato do servers.json. No escopo do projeto ele guarda só
// fatos de conexão do time e é seguro versionar em Git; no global carrega também
// identidade (é pessoal). Default é legado v1 no projeto — hoje o padrão do
// projeto mora no LocalFile (pessoal); no global segue aqui.
type ServersFile struct {
	Version string   `json:"version"`
	Default string   `json:"defaultServer,omitempty"`
	Servers []Server `json:"servers"`
}

// LocalFile é o overlay pessoal do projeto (.fluigcli/servers.local.json,
// git-ignorado): identidade por servidor e o padrão pessoal. Nunca versionado —
// é o que separa "qual servidor" (time) de "quem sou eu nele" (você).
type LocalFile struct {
	Version    string     `json:"version"`
	Default    string     `json:"defaultServer,omitempty"`
	Identities []Identity `json:"identities,omitempty"`
}

// Identity liga o usuário local a um servidor compartilhado pelo nome.
type Identity struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	UserCode string `json:"userCode,omitempty"`
}

const serversFileVersion = "2.0.0"

// NewServerID gera um id curto aleatório. Mantido só para compatibilidade de
// leitura de arquivos v1; a CLI não gera mais ids (o keyring usa KeyringKey).
func NewServerID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand não falha em plataformas suportadas
	}
	return hex.EncodeToString(b)
}
