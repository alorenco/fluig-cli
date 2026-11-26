package cli

import (
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// FLUIGCLI_WAR_UPLOAD com valor desconhecido é erro de uso (exit 2) já na
// leitura das env vars. Antes, o erro passava pelo caminho de autenticação e
// saía como AUTH_FAILED.
func TestWARUploadEnvInvalido(t *testing.T) {
	t.Setenv(envWARUpload, "ftp")
	code, stdout := runMain(t, "server", "test", "--json", "--project", t.TempDir())
	if code != output.ExitUsage {
		t.Errorf("exit = %d, quer %d; stdout=%s", code, output.ExitUsage, stdout)
	}
	if !strings.Contains(stdout, envWARUpload) {
		t.Errorf("mensagem sem o nome da env var: %s", stdout)
	}
}
