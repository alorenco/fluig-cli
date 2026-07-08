package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
)

// fakeReleases sobe um servidor que anuncia latest como última release e o
// aponta em updateBaseURL durante o teste.
func fakeReleases(t *testing.T, latest string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/releases/tag/v"+latest, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	old := updateBaseURL
	updateBaseURL = srv.URL + "/releases"
	t.Cleanup(func() { updateBaseURL = old })
}

func TestUpgradeCheckJSON(t *testing.T) {
	fakeReleases(t, "9.9.9")
	code, stdout := runMain(t, "upgrade", "--check", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit = %d, quer 0; stdout=%s", code, stdout)
	}
	var env output.Envelope
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout não é JSON: %v\n%s", err, stdout)
	}
	if !env.OK || env.Command != "upgrade" {
		t.Errorf("envelope inesperado: %+v", env)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data inesperado: %#v", env.Data)
	}
	if data["latest"] != "9.9.9" || data["updated"] != false {
		t.Errorf("data = %#v, quer latest=9.9.9 e updated=false", data)
	}
}

func TestUpgradeRecusaBuildDev(t *testing.T) {
	fakeReleases(t, "9.9.9")
	app := &App{Version: "dev"}
	root := newRootCmd(app)
	root.SetArgs([]string{"upgrade"})
	err := root.Execute()
	if err == nil {
		t.Fatal("upgrade deveria recusar build de desenvolvimento (dev)")
	}
	if output.ExitCodeFor(err) != output.ExitUsage {
		t.Errorf("exit = %d, quer %d (uso)", output.ExitCodeFor(err), output.ExitUsage)
	}
	if !strings.Contains(err.Error(), "go install") {
		t.Errorf("mensagem deveria orientar o go install: %q", err.Error())
	}
}

func TestUpgradeJaNaUltimaVersao(t *testing.T) {
	fakeReleases(t, "1.0.0")
	app := &App{Version: "1.0.0"}
	root := newRootCmd(app)
	root.SetArgs([]string{"upgrade"})
	if err := root.Execute(); err != nil {
		t.Fatalf("upgrade na última versão deveria ser sucesso sem ação: %v", err)
	}
}
