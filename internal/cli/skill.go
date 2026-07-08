package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
	skillassets "github.com/alorenco/fluig-cli/skills"
)

// Marcadores do bloco gerenciado no AGENTS.md do Codex. Reinstalar substitui o
// que estiver entre eles, sem tocar no resto do arquivo (idempotente).
const (
	skillBlockStart = "<!-- fluigcli:start (gerado por `fluigcli skill install`; não edite à mão) -->"
	skillBlockEnd   = "<!-- fluigcli:end -->"
)

// skillFileResult descreve o que aconteceu com um arquivo na instalação.
type skillFileResult struct {
	Path   string `json:"path"`
	Status string `json:"status"` // written | updated | unchanged | skipped
}

func newSkillCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Instala ou exibe a Skill de agente de IA do fluigcli",
		Long: "Gerencia a Skill que ensina agentes de IA (Claude Code, Codex) a usar o\n" +
			"fluigcli corretamente — saída --json, exit codes e mapa de comandos.\n\n" +
			"O conteúdo é embutido no binário; a instalação não acessa a rede.",
	}
	cmd.AddCommand(newSkillInstallCmd(app))
	cmd.AddCommand(newSkillShowCmd(app))
	return cmd
}

func newSkillInstallCmd(app *App) *cobra.Command {
	var target string
	var global, force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Instala a Skill para Claude Code e/ou Codex",
		Long: "Escreve os arquivos da Skill no lugar esperado por cada ferramenta.\n\n" +
			"Claude Code: .claude/skills/fluigcli/ (ou ~/.claude/skills/ com --global).\n" +
			"Codex: injeta um bloco gerenciado em AGENTS.md (ou ~/.codex/AGENTS.md com\n" +
			"--global), atualizado no lugar a cada reinstalação.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			switch target {
			case "claude", "codex", "all":
			default:
				return output.Usagef("alvo inválido %q (use claude, codex ou all)", target)
			}

			base, err := app.skillBaseDir(global)
			if err != nil {
				return err
			}

			var results []skillFileResult
			if target == "claude" || target == "all" {
				r, err := installClaudeSkill(base, force)
				if err != nil {
					return err
				}
				results = append(results, r...)
			}
			if target == "codex" || target == "all" {
				r, err := installCodexSkill(base)
				if err != nil {
					return err
				}
				results = append(results, r)
			}

			skipped := 0
			for _, r := range results {
				p.Successf("%s: %s", r.Status, r.Path)
				if r.Status == "skipped" {
					skipped++
				}
			}
			if skipped > 0 {
				p.Warnf("%d arquivo(s) foram preservados por diferirem do gerado; use --force para sobrescrever", skipped)
			}
			p.Done(map[string]any{"target": target, "files": results})
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "claude", "ferramenta alvo: claude, codex ou all")
	cmd.Flags().BoolVar(&global, "global", false, "instala no diretório do usuário em vez do projeto")
	cmd.Flags().BoolVar(&force, "force", false, "sobrescreve arquivos modificados localmente")
	return cmd
}

func newSkillShowCmd(app *App) *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Imprime o conteúdo da Skill no stdout",
		Long:  "Útil para inspecionar ou canalizar (pipe) o guia para outra ferramenta.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			var content string
			var err error
			switch target {
			case "claude":
				content, err = claudeSkillBundle()
			case "codex":
				content, err = codexSkillBlock()
			default:
				return output.Usagef("alvo inválido %q (use claude ou codex)", target)
			}
			if err != nil {
				return err
			}
			if app.JSON {
				p.Done(map[string]any{"target": target, "content": content})
				return nil
			}
			p.Successf("%s", content)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "claude", "conteúdo a exibir: claude ou codex")
	return cmd
}

// skillBaseDir devolve o diretório-base da instalação: o home do usuário quando
// --global, senão a raiz do projeto (ou o diretório atual, se fora de projeto).
func (a *App) skillBaseDir(global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", output.Genericf("não foi possível localizar o diretório do usuário: %s", err)
		}
		return home, nil
	}
	if root := a.ProjectRoot(); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", output.Genericf("não foi possível obter o diretório atual: %s", err)
	}
	return cwd, nil
}

// installClaudeSkill materializa SKILL.md e reference/ em <base>/.claude/skills/fluigcli/.
func installClaudeSkill(base string, force bool) ([]skillFileResult, error) {
	dest := filepath.Join(base, ".claude", "skills", "fluigcli")
	var results []skillFileResult
	err := fs.WalkDir(skillassets.FS, "fluigcli", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "fluigcli/")
		// O material do Codex não faz parte da skill do Claude Code.
		if strings.HasPrefix(rel, "codex/") {
			return nil
		}
		data, err := skillassets.FS.ReadFile(path)
		if err != nil {
			return err
		}
		outPath := filepath.Join(dest, filepath.FromSlash(rel))
		res, werr := writeSkillFile(outPath, data, force)
		if werr != nil {
			return werr
		}
		results = append(results, res)
		return nil
	})
	if err != nil {
		return nil, output.Genericf("falha ao instalar a skill do Claude Code: %s", err)
	}
	return results, nil
}

// writeSkillFile grava um arquivo, reportando o que fez e respeitando edições
// locais quando !force.
func writeSkillFile(path string, data []byte, force bool) (skillFileResult, error) {
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		if string(existing) == string(data) {
			return skillFileResult{Path: path, Status: "unchanged"}, nil
		}
		if !force {
			return skillFileResult{Path: path, Status: "skipped"}, nil
		}
	case !os.IsNotExist(err):
		return skillFileResult{}, output.Genericf("não foi possível ler %s: %s", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return skillFileResult{}, output.Genericf("não foi possível criar %s: %s", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return skillFileResult{}, output.Genericf("não foi possível escrever %s: %s", path, err)
	}
	status := "written"
	if len(existing) > 0 {
		status = "updated"
	}
	return skillFileResult{Path: path, Status: status}, nil
}

// installCodexSkill injeta/atualiza o bloco gerenciado no AGENTS.md do Codex.
func installCodexSkill(base string) (skillFileResult, error) {
	path := filepath.Join(base, "AGENTS.md")
	if isHome(base) {
		path = filepath.Join(base, ".codex", "AGENTS.md")
	}
	block, err := codexSkillBlock()
	if err != nil {
		return skillFileResult{}, err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return skillFileResult{}, output.Genericf("não foi possível ler %s: %s", path, err)
	}
	updated := spliceManagedBlock(string(existing), block)
	if updated == string(existing) {
		return skillFileResult{Path: path, Status: "unchanged"}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return skillFileResult{}, output.Genericf("não foi possível criar %s: %s", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return skillFileResult{}, output.Genericf("não foi possível escrever %s: %s", path, err)
	}
	status := "updated"
	if len(existing) == 0 {
		status = "written"
	}
	return skillFileResult{Path: path, Status: status}, nil
}

// spliceManagedBlock substitui o bloco gerenciado em content (se houver) ou o
// acrescenta ao final, preservando o restante do arquivo.
func spliceManagedBlock(content, blockBody string) string {
	block := skillBlockStart + "\n" + strings.TrimSpace(blockBody) + "\n" + skillBlockEnd
	i := strings.Index(content, skillBlockStart)
	j := strings.Index(content, skillBlockEnd)
	if i >= 0 && j > i {
		before := content[:i]
		after := content[j+len(skillBlockEnd):]
		return before + block + after
	}
	if strings.TrimSpace(content) == "" {
		return block + "\n"
	}
	return strings.TrimRight(content, "\n") + "\n\n" + block + "\n"
}

// claudeSkillBundle concatena SKILL.md e os arquivos de reference/ num guia único.
func claudeSkillBundle() (string, error) {
	var b strings.Builder
	files := []string{"fluigcli/SKILL.md", "fluigcli/reference/contract.md", "fluigcli/reference/commands.md"}
	for i, f := range files {
		data, err := skillassets.FS.ReadFile(f)
		if err != nil {
			return "", output.Genericf("falha ao ler a skill embutida %s: %s", f, err)
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.Write(data)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// codexSkillBlock devolve o conteúdo canônico para o AGENTS.md do Codex.
func codexSkillBlock() (string, error) {
	data, err := skillassets.FS.ReadFile("fluigcli/codex/AGENTS.md")
	if err != nil {
		return "", output.Genericf("falha ao ler o guia do Codex embutido: %s", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// isHome informa se dir é o diretório do usuário (define o layout do Codex).
func isHome(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	a, err1 := filepath.Abs(dir)
	b, err2 := filepath.Abs(home)
	return err1 == nil && err2 == nil && a == b
}
