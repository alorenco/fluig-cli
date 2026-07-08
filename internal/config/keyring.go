package config

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// keyringService é o service name usado no keyring do SO.
const keyringService = "fluigcli"

// Keyring abstrai o keyring do SO para permitir mock em testes e ambientes
// headless (Linux sem Secret Service).
type Keyring interface {
	Get(serverID string) (string, error)
	Set(serverID, password string) error
	Delete(serverID string) error
	// Available informa se o backend de keyring está disponível neste ambiente
	// (falso em Linux headless sem Secret Service/kwallet, por exemplo).
	Available() bool
}

// ErrKeyringNotFound indica que não há senha gravada para o servidor.
var ErrKeyringNotFound = errors.New("senha não encontrada no keyring")

// systemKeyring implementa Keyring sobre zalando/go-keyring
// (Windows Credential Manager, macOS Keychain, Secret Service no Linux).
type systemKeyring struct {
	availOnce sync.Once
	avail     bool
}

func SystemKeyring() Keyring { return &systemKeyring{} }

// Available sonda o backend uma vez (cacheado): um Get de chave inexistente
// devolve "não encontrado" quando o keyring existe, ou erro de backend quando não.
func (k *systemKeyring) Available() bool {
	k.availOnce.Do(func() {
		_, err := keyring.Get(keyringService, "__fluigcli_probe__")
		k.avail = err == nil || errors.Is(err, keyring.ErrNotFound)
	})
	return k.avail
}

func (k *systemKeyring) Get(serverID string) (string, error) {
	pw, err := keyring.Get(keyringService, serverID)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrKeyringNotFound
	}
	return pw, err
}

func (k *systemKeyring) Set(serverID, password string) error {
	return keyring.Set(keyringService, serverID, password)
}

func (k *systemKeyring) Delete(serverID string) error {
	err := keyring.Delete(keyringService, serverID)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
