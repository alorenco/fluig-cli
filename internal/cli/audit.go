package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/audit"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newAuditCmd(app *App) *cobra.Command {
	var (
		syncCatalog bool
		failOn      string
		fix         bool
	)
	cmd := &cobra.Command{
		Use:   "audit [<path>...]",
		Short: "Audita formulários e widgets contra o Fluig Style Guide 2.0 (read-only)",
		Long: "Linter estático de conformidade com o Fluig Style Guide 2.0: varre forms/ e\n" +
			"wcm/widget/ (ou os caminhos informados) e aponta o que briga com o tema fixo\n" +
			"da plataforma. Nada é alterado nem enviado ao servidor.\n\n" +
			"Regras:\n" +
			"  SG001 (aviso)  referência ao CSS legado do style guide (404 no 2.0)\n" +
			"  SG002 (erro)   recurso externo — CDN, Google Fonts etc.\n" +
			"  SG003 (erro)   cor fixa (hex/rgb) em CSS ou style= — sugere a variável do tema\n" +
			"  SG004 (aviso)  !important sobre classe do style guide\n" +
			"  SG005 (aviso)  estilo inline (style=)\n" +
			"  SG006 (aviso)  classe fs-* que não existe no catálogo do servidor\n" +
			"  SG007 (aviso)  alert/confirm/prompt nativos em vez do FLUIGC\n\n" +
			"--fix aplica as correções DETERMINÍSTICAS (CSS legado → flat; cor hex com\n" +
			"valor idêntico a uma variável do tema → var(...)); o restante fica no\n" +
			"relatório para correção manual.\n\n" +
			"O catálogo (classes e variáveis) vem embutido no binário; --sync o atualiza\n" +
			"do servidor alvo (o style guide é público, não requer login). Arquivos\n" +
			"minificados/vendorados e bundles gerados de widget SPA são ignorados;\n" +
			"em .fluigcli/audit.json ficam as exceções ({\"ignore\": [globs]}) e os\n" +
			"ajustes de nível ({\"severity\": {\"SG005\": \"off\"}}).",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			switch failOn {
			case "error", "warning", "none":
			default:
				return output.Usagef("--fail-on inválido: %q (use error, warning ou none)", failOn)
			}
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			cat, err := audit.Embedded()
			if err != nil {
				return err
			}
			catalogSource := "embutido (" + cat.Server + ")"
			if syncCatalog {
				server, err := app.resolveServer("")
				if err != nil {
					return err
				}
				p.Server = server.Name
				synced, err := audit.FetchFromServer(context.Background(), server.BaseURL(), app.Timeout)
				if err != nil {
					p.Warnf("--sync falhou (%s) — usando o catálogo embutido", err)
				} else {
					cat = synced
					catalogSource = "servidor " + server.Name
				}
			}
			cfg, err := loadAuditConfig(root)
			if err != nil {
				return err
			}
			res, err := audit.Run(root, args, cat, cfg)
			if err != nil {
				return err
			}
			fixed := 0
			if fix {
				fixed, err = audit.ApplyFixes(root, res.Findings)
				if err != nil {
					return err
				}
				if fixed > 0 {
					p.Successf("%d correção(ões) determinística(s) aplicada(s) — confira com git diff.", fixed)
					// Reaudita: o relatório final reflete o que sobrou.
					if res, err = audit.Run(root, args, cat, cfg); err != nil {
						return err
					}
				}
			}

			errCount, warnCount := 0, 0
			for _, f := range res.Findings {
				if f.Severity == audit.SeverityError {
					errCount++
				} else {
					warnCount++
				}
			}

			if len(res.Findings) == 0 {
				p.Successf("nenhuma pendência de style guide (%d arquivos auditados, catálogo %s).", res.Scanned, catalogSource)
			} else {
				rows := make([][]string, 0, len(res.Findings))
				for _, f := range res.Findings {
					sev := "AVISO"
					if f.Severity == audit.SeverityError {
						sev = "ERRO"
					}
					msg := f.Message
					if f.Suggestion != "" {
						msg += " → " + f.Suggestion
					}
					rows = append(rows, []string{sev, f.Rule, fmt.Sprintf("%s:%d", f.File, f.Line), msg})
				}
				// Padrão de listagem (ver CLAUDE.md): erro em vermelho, aviso em amarelo.
				p.Table(output.Table{
					Headers: []string{"Sev", "Regra", "Local", "Problema"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col != 0 {
							return padded
						}
						if res.Findings[row].Severity == audit.SeverityError {
							return output.Red(padded)
						}
						return output.Yellow(padded)
					}),
				})
				p.Infof("%d erro(s) e %d aviso(s) em %d arquivo(s) auditado(s) (catálogo %s).",
					errCount, warnCount, res.Scanned, catalogSource)
			}
			if len(res.Ignored) > 0 {
				p.Infof("%d arquivo(s) fora da auditoria (minificado/vendorado, bundle de SPA ou audit.json) — detalhes no --json.", len(res.Ignored))
			}

			findings := res.Findings
			if findings == nil {
				findings = []audit.Finding{}
			}
			data := map[string]any{
				"findings": findings,
				"counts":   map[string]int{"error": errCount, "warning": warnCount},
				"fixed":    fixed,
				"scanned":  res.Scanned,
				"ignored":  res.Ignored,
				"catalog":  catalogSource,
			}
			fail := (failOn == "error" && errCount > 0) || (failOn == "warning" && errCount+warnCount > 0)
			if fail {
				msg := fmt.Sprintf("auditoria reprovada: %d erro(s) e %d aviso(s) (limiar --fail-on %s)", errCount, warnCount, failOn)
				p.FailData(data, output.CodeAuditFailed, msg)
				return output.AuditFailedf("%s", msg)
			}
			p.Done(data)
			return nil
		},
	}
	cmd.Flags().BoolVar(&syncCatalog, "sync", false, "atualiza o catálogo (classes/variáveis) do style guide do servidor alvo antes de auditar")
	cmd.Flags().StringVar(&failOn, "fail-on", "error", "reprova (exit 1) quando houver achados do nível: error, warning ou none")
	cmd.Flags().BoolVar(&fix, "fix", false, "aplica as correções determinísticas nos arquivos (CSS legado → flat; hex idêntico a variável → var(...))")
	return cmd
}

// loadAuditConfig lê as exceções do projeto em .fluigcli/audit.json (ausente = vazio).
func loadAuditConfig(root string) (audit.Config, error) {
	var cfg audit.Config
	raw, err := os.ReadFile(filepath.Join(root, ".fluigcli", "audit.json"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, output.Usagef(".fluigcli/audit.json inválido: %s", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, output.Usagef(".fluigcli/audit.json: %s", err)
	}
	return cfg, nil
}
