package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alorenco/fluig-cli/internal/output"
	skillassets "github.com/alorenco/fluig-cli/skills"
)

// skill install --target claude escreve SKILL.md + reference/, byte a byte
// iguais ao conteúdo embutido.
func TestSkillInstallClaude(t *testing.T) {
	proj := t.TempDir()
	code, stdout := runMain(t, "skill", "install", "--target", "claude", "--project", proj, "--json")
	if code != output.ExitOK {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
	for _, rel := range []string{"SKILL.md", "reference/contract.md", "reference/commands.md"} {
		got, err := os.ReadFile(filepath.Join(proj, ".claude", "skills", "fluigcli", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("arquivo %s não instalado: %v", rel, err)
		}
		want, err := skillassets.FS.ReadFile("fluigcli/" + rel)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s difere do conteúdo embutido", rel)
		}
	}
	// O material do Codex não deve entrar na skill do Claude Code.
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "fluigcli", "codex")); !os.IsNotExist(err) {
		t.Errorf("codex/ não deveria ser instalado no alvo claude")
	}
}

// O bloco do Codex é injetado sem apagar o conteúdo próprio do AGENTS.md e sem
// duplicar quando reinstalado.
func TestSkillInstallCodexManagedBlock(t *testing.T) {
	proj := t.TempDir()
	agents := filepath.Join(proj, "AGENTS.md")
	original := "# Projeto\n\nInstruções do time.\n"
	if err := os.WriteFile(agents, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, out := runMain(t, "skill", "install", "--target", "codex", "--project", proj, "--json"); code != output.ExitOK {
		t.Fatalf("exit=%d out=%s", code, out)
	}
	// Reinstala: não pode duplicar o bloco.
	if code, out := runMain(t, "skill", "install", "--target", "codex", "--project", proj, "--json"); code != output.ExitOK {
		t.Fatalf("reinstall exit=%d out=%s", code, out)
	}

	data, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "Instruções do time.") {
		t.Errorf("o conteúdo original do AGENTS.md foi perdido:\n%s", content)
	}
	if n := strings.Count(content, skillBlockStart); n != 1 {
		t.Errorf("esperava 1 bloco fluigcli, veio %d:\n%s", n, content)
	}
	if strings.Count(content, skillBlockEnd) != 1 {
		t.Errorf("marcador de fim duplicado/ausente:\n%s", content)
	}
}

// Arquivo modificado localmente é preservado sem --force e sobrescrito com --force.
func TestSkillInstallRespectsLocalEdits(t *testing.T) {
	proj := t.TempDir()
	skillMD := filepath.Join(proj, ".claude", "skills", "fluigcli", "SKILL.md")

	runMain(t, "skill", "install", "--target", "claude", "--project", proj, "--json")
	if err := os.WriteFile(skillMD, []byte("editado à mão\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	statusOf := func(stdout, needle string) string {
		var env output.Envelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("json inválido: %v\n%s", err, stdout)
		}
		data, _ := env.Data.(map[string]any)
		files, _ := data["files"].([]any)
		for _, f := range files {
			m, _ := f.(map[string]any)
			if p, _ := m["path"].(string); strings.Contains(p, needle) {
				s, _ := m["status"].(string)
				return s
			}
		}
		t.Fatalf("arquivo %q ausente no resultado: %s", needle, stdout)
		return ""
	}

	_, out := runMain(t, "skill", "install", "--target", "claude", "--project", proj, "--json")
	if s := statusOf(out, "SKILL.md"); s != "skipped" {
		t.Errorf("sem --force esperava skipped, veio %q", s)
	}
	if got, _ := os.ReadFile(skillMD); string(got) != "editado à mão\n" {
		t.Errorf("arquivo modificado foi sobrescrito sem --force")
	}

	_, out = runMain(t, "skill", "install", "--target", "claude", "--project", proj, "--force", "--json")
	if s := statusOf(out, "SKILL.md"); s != "updated" {
		t.Errorf("com --force esperava updated, veio %q", s)
	}
}

// skill show devolve o conteúdo no envelope JSON.
func TestSkillShow(t *testing.T) {
	for _, target := range []string{"claude", "codex"} {
		code, stdout := runMain(t, "skill", "show", "--target", target, "--json")
		if code != output.ExitOK {
			t.Fatalf("target=%s exit=%d", target, code)
		}
		var env output.Envelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("json inválido: %v", err)
		}
		data, _ := env.Data.(map[string]any)
		if c, _ := data["content"].(string); !strings.Contains(c, "fluigcli") {
			t.Errorf("target=%s: conteúdo inesperado", target)
		}
	}
}

// Alvo inválido é erro de uso (exit 2).
func TestSkillInvalidTarget(t *testing.T) {
	if code, _ := runMain(t, "skill", "install", "--target", "vscode", "--json"); code != output.ExitUsage {
		t.Errorf("alvo inválido deveria dar USAGE_ERROR (2), veio %d", code)
	}
}

// Guarda anti-drift: todo comando `fluigcli <cmd> [<sub>]` citado nos blocos de
// código da skill precisa existir na CLI. Impede que a skill referencie um
// comando renomeado/removido.
func TestSkillHasNoDrift(t *testing.T) {
	root := newRootCmd(&App{})
	code := extractCode(t)
	re := regexp.MustCompile(`fluigcli\s+([a-z][a-z-]+)(?:\s+([a-z][a-z-]+))?`)

	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(code, -1) {
		top, sub := m[1], m[2]
		if seen[top+" "+sub] {
			continue
		}
		seen[top+" "+sub] = true

		cmd, _, err := root.Find([]string{top})
		if err != nil || cmd == root || cmd.Name() != top {
			t.Errorf("skill cita comando inexistente: %q", top)
			continue
		}
		if sub != "" && cmd.HasSubCommands() {
			subCmd, _, err := root.Find([]string{top, sub})
			if err != nil || subCmd.Name() != sub {
				t.Errorf("skill cita subcomando inexistente: %q %q", top, sub)
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("nenhum comando extraído da skill — o extrator quebrou?")
	}
}

// extractCode concatena os blocos cercados (```) e trechos inline (`) de todos
// os arquivos markdown embutidos da skill.
func extractCode(t *testing.T) string {
	t.Helper()
	files := []string{
		"fluigcli/SKILL.md",
		"fluigcli/reference/contract.md",
		"fluigcli/reference/commands.md",
		"fluigcli/codex/AGENTS.md",
	}
	inline := regexp.MustCompile("`([^`]+)`")
	var b strings.Builder
	for _, f := range files {
		data, err := skillassets.FS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		md := string(data)
		inFence := false
		for _, line := range strings.Split(md, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
		for _, m := range inline.FindAllStringSubmatch(md, -1) {
			b.WriteString(m[1])
			b.WriteByte('\n')
		}
	}
	return b.String()
}
