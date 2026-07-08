// Package config gerencia os arquivos servers.json (sem segredos), o keyring
// do SO e a resolução de senha por precedência.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Server descreve um servidor Fluig registrado. Nunca contém senha.
type Server struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	SSL       bool   `json:"ssl"`
	Username  string `json:"username"`
	UserCode  string `json:"userCode,omitempty"`
	CompanyID int    `json:"companyId"`
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

// ServersFile é o formato do servers.json (versionável em Git, sem segredos).
type ServersFile struct {
	Version string   `json:"version"`
	Servers []Server `json:"servers"`
}

const serversFileVersion = "1.0.0"

// NewServerID gera um id curto aleatório para chavear a senha no keyring.
func NewServerID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand não falha em plataformas suportadas
	}
	return hex.EncodeToString(b)
}
