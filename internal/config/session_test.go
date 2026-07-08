package config

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDiskSessionCacheRoundTrip(t *testing.T) {
	c := &DiskSessionCache{path: filepath.Join(t.TempDir(), "sessions.json")}
	key := "fluig.test:8080|admin"

	if got := c.Load(key); got != nil {
		t.Errorf("cache vazio deveria devolver nil, veio %v", got)
	}

	err := c.Save(key, []*http.Cookie{
		{Name: "JSESSIONIDSSO", Value: "abc"},
		{Name: "jwt.token", Value: "xyz"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got := c.Load(key)
	if len(got) != 2 || got[0].Name != "JSESSIONIDSSO" || got[0].Value != "abc" || got[0].Path != "/" {
		t.Errorf("cookies inesperados: %+v", got)
	}

	// Chave diferente não vê a sessão.
	if c.Load("outro|user") != nil {
		t.Error("chave diferente não deveria devolver cookies")
	}

	// Arquivo deve ser 0600 (credencial de sessão).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissão do arquivo = %o, quer 600", perm)
		}
	}

	// Clear remove a sessão da chave.
	if err := c.Clear(key); err != nil {
		t.Fatal(err)
	}
	if c.Load(key) != nil {
		t.Error("Clear deveria remover a sessão")
	}
}

func TestDiskSessionCacheClearAll(t *testing.T) {
	c := &DiskSessionCache{path: filepath.Join(t.TempDir(), "sessions.json")}
	c.Save("a|u", []*http.Cookie{{Name: "x", Value: "1"}})
	c.Save("b|u", []*http.Cookie{{Name: "y", Value: "2"}})
	if err := c.Clear(""); err != nil {
		t.Fatal(err)
	}
	if c.Load("a|u") != nil || c.Load("b|u") != nil {
		t.Error("Clear(\"\") deveria limpar tudo")
	}
}
