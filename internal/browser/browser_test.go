package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindPrecedencia(t *testing.T) {
	oldLook, oldHome := lookPath, homeDir
	t.Cleanup(func() { lookPath, homeDir = oldLook, oldHome })
	home := t.TempDir()
	homeDir = func() (string, error) { return home, nil }
	instalados := map[string]bool{}
	lookPath = func(p string) (string, error) {
		if instalados[p] || strings.HasPrefix(p, "/") {
			return p, nil
		}
		return "", errors.New("não achei")
	}

	t.Setenv("FLUIGCLI_BROWSER", "")
	if p, err := Find("/opt/meu-chrome"); err != nil || p != "/opt/meu-chrome" {
		t.Errorf("--browser: %q %v", p, err)
	}
	t.Setenv("FLUIGCLI_BROWSER", "/env/chrome")
	if p, _ := Find(""); p != "/env/chrome" {
		t.Errorf("$FLUIGCLI_BROWSER: %q", p)
	}
	t.Setenv("FLUIGCLI_BROWSER", "")
	instalados["chromium"] = true
	if p, _ := Find(""); p != "chromium" {
		t.Errorf("PATH: %q", p)
	}
	delete(instalados, "chromium")

	// Só o Chromium do Playwright: escolhe a revisão mais nova.
	for _, rev := range []string{"chromium-1228", "chromium-1234"} {
		d := filepath.Join(home, ".cache", "ms-playwright", rev, "chrome-linux")
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "chrome"), []byte("x"), 0o755)
	}
	if p := playwrightChromium(); !strings.Contains(p, "chromium-1234") {
		t.Errorf("Playwright: %q", p)
	}
}

func TestFindSemNavegador(t *testing.T) {
	oldLook, oldHome := lookPath, homeDir
	t.Cleanup(func() { lookPath, homeDir = oldLook, oldHome })
	lookPath = func(string) (string, error) { return "", errors.New("não achei") }
	homeDir = func() (string, error) { return t.TempDir(), nil }
	t.Setenv("FLUIGCLI_BROWSER", "")
	if osPathsExist() {
		t.Skip("esta máquina tem navegador num caminho padrão do SO")
	}
	_, err := Find("")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "Playwright") {
		t.Errorf("sem navegador: %v", err)
	}
	if _, err := Find("/nao/existe"); !errors.Is(err, ErrNotFound) {
		t.Errorf("--browser inexistente: %v", err)
	}
}

func osPathsExist() bool {
	for _, p := range osPaths() {
		if fileExists(p) {
			return true
		}
	}
	return false
}

func TestFormData(t *testing.T) {
	casos := []struct {
		body string
		want map[string]string
	}{
		{`{"processInstanceId": 1, "formData": {"controleEnvio": "123", "n": 5}}`, map[string]string{"controleEnvio": "123", "n": "5"}},
		{`{"formData": [{"name": "controleEnvio", "value": "9"}, {"name": "x", "value": null}]}`, map[string]string{"controleEnvio": "9", "x": ""}},
		{`{"formData": "{\"a\": \"1\"}"}`, map[string]string{"a": "1"}},
		{`formData=%7B%22a%22%3A%222%22%7D&outro=1`, map[string]string{"a": "2"}},
		{`{"semFormData": 1}`, nil},
		{``, nil},
	}
	for _, c := range casos {
		got := formData(c.body)
		if len(got) != len(c.want) {
			t.Errorf("%s: %v", c.body, got)
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %s=%q, quer %q", c.body, k, got[k], v)
			}
		}
	}
}
