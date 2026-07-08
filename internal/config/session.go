package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// DiskSessionCache persiste os cookies de sessão do Fluig entre execuções.
// Os cookies são credenciais de sessão (equivalentes a um token):
// o arquivo fica no diretório de cache do usuário, com permissão 0600, e nunca
// no projeto. Chaveado por host|usuário.
type DiskSessionCache struct {
	path string
	mu   sync.Mutex
}

type sessionCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type sessionEntry struct {
	Cookies []sessionCookie `json:"cookies"`
}

// NewDiskSessionCache cria o cache em <cache do usuário>/fluigcli/sessions.json.
func NewDiskSessionCache() (*DiskSessionCache, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "fluigcli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &DiskSessionCache{path: filepath.Join(dir, "sessions.json")}, nil
}

func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func (c *DiskSessionCache) readAll() map[string]sessionEntry {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return map[string]sessionEntry{}
	}
	var m map[string]sessionEntry
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return map[string]sessionEntry{}
	}
	return m
}

// Load devolve os cookies de sessão gravados para a chave.
func (c *DiskSessionCache) Load(key string) []*http.Cookie {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.readAll()[keyHash(key)]
	if !ok {
		return nil
	}
	cookies := make([]*http.Cookie, 0, len(entry.Cookies))
	for _, ck := range entry.Cookies {
		cookies = append(cookies, &http.Cookie{Name: ck.Name, Value: ck.Value, Path: "/"})
	}
	return cookies
}

// Save grava os cookies de sessão para a chave (arquivo 0600).
func (c *DiskSessionCache) Save(key string, cookies []*http.Cookie) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.readAll()
	entry := sessionEntry{}
	for _, ck := range cookies {
		entry.Cookies = append(entry.Cookies, sessionCookie{Name: ck.Name, Value: ck.Value})
	}
	m[keyHash(key)] = entry
	return writeSecret(c.path, m)
}

// writeSecret grava o JSON com permissão 0600, reaplicando-a mesmo se o arquivo
// já existir (WriteFile não re-chmoda um arquivo pré-existente).
func writeSecret(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// Clear remove a sessão gravada para a chave. key vazia limpa tudo.
func (c *DiskSessionCache) Clear(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if key == "" {
		if err := os.Remove(c.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	m := c.readAll()
	delete(m, keyHash(key))
	return writeSecret(c.path, m)
}
