package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alorenco/fluig-cli/internal/output"
)

// Carimbo: grava/lê a versão; build dev/vazio não carimba.
func TestSkillVersionStamp(t *testing.T) {
	dir := t.TempDir()
	if s := readSkillVersionStamp(dir); s != "" {
		t.Errorf("sem carimbo deveria devolver vazio, veio %q", s)
	}
	if err := writeSkillVersionStamp(dir, "0.5.1"); err != nil {
		t.Fatal(err)
	}
	if s := readSkillVersionStamp(dir); s != "0.5.1" {
		t.Errorf("carimbo = %q, quer 0.5.1", s)
	}
	// dev/vazio não escrevem.
	dir2 := t.TempDir()
	_ = writeSkillVersionStamp(dir2, "dev")
	_ = writeSkillVersionStamp(dir2, "")
	if _, err := os.Stat(filepath.Join(dir2, skillVersionFile)); !os.IsNotExist(err) {
		t.Error("build dev/vazio não deveria carimbar")
	}
}

// Throttle: avisa 1×/dia por versão; reavisa ao mudar a versão ou passar 24h.
func TestSkillNoticeThrottle(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "skill-check.json")
	t0 := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	if !skillNoticeShouldShow(cache, "0.5.1", t0) {
		t.Fatal("primeira vez deveria avisar")
	}
	if skillNoticeShouldShow(cache, "0.5.1", t0.Add(time.Hour)) {
		t.Error("mesma versão dentro de 24h não deveria reavisar")
	}
	if !skillNoticeShouldShow(cache, "0.5.2", t0.Add(2*time.Hour)) {
		t.Error("versão nova deveria reavisar")
	}
	if !skillNoticeShouldShow(cache, "0.5.2", t0.Add(30*time.Hour)) {
		t.Error("após 24h deveria reavisar")
	}
}

// skill install grava o carimbo com a versão do binário (em teste = "test").
func TestSkillInstallWritesStamp(t *testing.T) {
	proj := t.TempDir()
	code, stdout := runMain(t, "skill", "install", "--target", "claude", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	got := readSkillVersionStamp(claudeSkillDir(proj))
	if got != "test" {
		t.Errorf("carimbo = %q, quer \"test\" (a versão do binário nos testes)", got)
	}
}

// projectSkillInstalled detecta a skill do Claude Code no projeto.
func TestProjectSkillInstalled(t *testing.T) {
	proj := t.TempDir()
	app := &App{Project: proj}
	if _, ok := app.projectSkillInstalled(); ok {
		t.Error("sem skill instalada não deveria detectar")
	}
	dir := claudeSkillDir(proj)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.projectSkillInstalled(); !ok {
		t.Error("com SKILL.md deveria detectar a skill instalada")
	}
}
