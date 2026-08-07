package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/audit"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newAuditCmd(app *App) *cobra.Command {
	var (
		syncCatalog bool
		failOn      string
		fix         bool
		processID   string
	)
	cmd := &cobra.Command{
		Use:   "audit [<path>...]",
		Short: "Audita o projeto: Style Guide 2.0 e APIs de script do Fluig (read-only)",
		Long: "Linter estático do projeto Fluig: varre forms/, wcm/widget/, datasets/,\n" +
			"events/, mechanisms/ e workflow/scripts/ (ou os caminhos informados) e\n" +
			"aponta o que briga com o tema fixo da plataforma (regras SG*) e as chamadas\n" +
			"de API que não existem (regras FL*, sobre a referência fluig.d.ts embutida).\n" +
			"Nada é alterado nem enviado ao servidor.\n\n" +
			"Regras:\n" +
			"  SG001 (aviso)  referência ao CSS legado do style guide (404 no 2.0)\n" +
			"  SG002 (erro)   recurso externo — CDN, Google Fonts etc.\n" +
			"  SG003 (erro)   cor fixa (hex/rgb) em CSS ou style= — sugere a variável do tema\n" +
			"  SG004 (aviso)  !important sobre classe do style guide\n" +
			"  SG005 (aviso)  estilo inline (style=)\n" +
			"  SG006 (aviso)  classe fs-* que não existe no catálogo do servidor\n" +
			"  SG007 (aviso)  alert/confirm/prompt nativos em vez do FLUIGC\n" +
			"  FL001 (aviso)  método hAPI.* que não existe (provável typo)\n" +
			"  FL002 (aviso)  variável WK* desconhecida em getValue() — devolve null em silêncio\n" +
			"  FL003 (aviso)  método form.* que não existe no FormController (eventos de form)\n" +
			"  FL004 (aviso)  membro inexistente em FLUIGC/DatasetFactory/docAPI/WCMAPI etc.\n" +
			"  FL005 (erro)   método do hAPI chamado como função global em script de\n" +
			"                   processo (getCardValue sem o hAPI.) — falha em runtime\n" +
			"  RHINO001 (aviso) === / !== entre retorno java.lang.String (getFieldName…) e\n" +
			"                   literal — no Rhino do Fluig é sempre false; use == ou String(...)\n" +
			"  RHINO002 (erro)  sintaxe ES6+ (class, import/export, async/await, parâmetro\n" +
			"                   default, spread, propriedade computada) — o Rhino do Fluig\n" +
			"                   (Voyager 2) não aceita; dá SyntaxError no deploy\n" +
			"  RHINO003 (erro)  const declarado no corpo de um laço (for/while/do) — o Rhino\n" +
			"                   congela o valor da 1ª iteração, sem erro; use let\n" +
			"  RHINO004 (aviso) dataset.values[i] acessado por NOME de coluna em JS\n" +
			"                   server-side — a linha é Object[] Java e quebra em runtime;\n" +
			"                   use getValue(i, \"coluna\") (índice numérico funciona)\n" +
			"  WF001 (erro)   [--process] seção activity-N do formulário sem etapa de\n" +
			"                   sequence N no processo — a seção nunca renderiza\n" +
			"  WF002 (aviso)  [--process] atividade humana sem seção activity-N no HTML\n\n" +
			"--process <id> liga as regras WF*: a CLI baixa o processo do servidor alvo\n" +
			"(read-only), acha o formulário vinculado a ele no forms.json e cruza as\n" +
			"classes activity-N do HTML com as sequences reais. activity-0 é sempre\n" +
			"válido (formulário de abertura, WKNumState = 0).\n\n" +
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
			if processID != "" {
				wf, err := runProcessCheck(app, p, root, processID, cfg)
				if err != nil {
					return err
				}
				res.Findings = append(res.Findings, wf...)
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
				p.Table(auditFindingsTable(res.Findings))
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
	cmd.Flags().StringVar(&processID, "process", "", "cruza o formulário do processo com as etapas reais dele (regras WF*; consulta o servidor, read-only)")
	cmd.Flags().StringVar(&failOn, "fail-on", "error", "reprova (exit 1) quando houver achados do nível: error, warning ou none")
	cmd.Flags().BoolVar(&fix, "fix", false, "aplica as correções determinísticas nos arquivos (CSS legado → flat; hex idêntico a variável → var(...))")
	return cmd
}

// runProcessCheck roda as regras WF* (audit --process): baixa o processo,
// resolve o formulário vinculado a ele no projeto e cruza as classes
// activity-N do HTML com as etapas reais. Só leitura no servidor.
func runProcessCheck(app *App, p *output.Printer, root, processID string, cfg audit.Config) ([]audit.Finding, error) {
	ctx := context.Background()
	server, client, err := app.connect(ctx, false)
	if err != nil {
		return nil, err
	}
	detail, err := client.ProcessDetail(ctx, processID, 0)
	if err != nil {
		return nil, mapFluigError(err)
	}
	if detail.FormID == 0 {
		p.Infof("o processo %q (versão %d) não tem formulário vinculado — regras WF* sem o que cruzar.", processID, detail.Version)
		return nil, nil
	}

	fmap, err := project.LoadFormMap(root, server.FormScopeKey())
	if err != nil {
		return nil, err
	}
	link, ok := fmap.ByDocumentID(detail.FormID)
	if !ok {
		return nil, output.NotFoundf(
			"o formulário %d (do processo %q) não está vinculado a nenhuma pasta local no forms.json; "+
				"baixe-o com: fluigcli form import %d — ou vincule uma pasta existente com: fluigcli form link",
			detail.FormID, processID, detail.FormID)
	}
	dir := project.FormDir(root, link.Folder)
	htmlPath, err := mainFormHTML(dir)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, err
	}
	rel, rerr := filepath.Rel(root, htmlPath)
	if rerr != nil {
		rel = htmlPath
	}

	states := make([]audit.ProcessActivity, 0, len(detail.States))
	for _, st := range detail.States {
		states = append(states, audit.ProcessActivity{Sequence: st.Sequence, Name: st.Name, Kind: st.Kind})
	}
	p.Infof("cruzando %s com o processo %q (versão %d, %d etapas).", filepath.ToSlash(rel), processID, detail.Version, len(states))
	findings := audit.CheckFormActivities(filepath.ToSlash(rel), content, processID, states)
	return audit.ApplySeverity(findings, cfg), nil
}

// mainFormHTML acha o HTML principal do formulário: o único .html no topo da
// pasta (a mesma regra do form export).
func mainFormHTML(dir string) (string, error) {
	fc, err := project.ReadFormFolder(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", output.NotFoundf("pasta do formulário %q não existe no projeto", dir)
		}
		return "", err
	}
	var htmls []string
	for _, f := range fc.Files {
		if ext := strings.ToLower(filepath.Ext(f)); ext == ".html" || ext == ".htm" {
			htmls = append(htmls, f)
		}
	}
	switch len(htmls) {
	case 1:
		return htmls[0], nil
	case 0:
		return "", output.NotFoundf("a pasta %q não tem arquivo .html", dir)
	default:
		return "", output.Usagef("a pasta %q tem %d arquivos .html — o formulário deve ter um único HTML principal", dir, len(htmls))
	}
}

// auditFindingsTable monta a tabela de achados no modo humano. Compartilhada
// pelo comando `audit` e pela pré-checagem do `dataset export`.
func auditFindingsTable(findings []audit.Finding) output.Table {
	rows := make([][]string, 0, len(findings))
	for _, f := range findings {
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
	return output.Table{
		Headers: []string{"Sev", "Regra", "Local", "Problema"},
		Rows:    rows,
		Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
			if col != 0 {
				return padded
			}
			if findings[row].Severity == audit.SeverityError {
				return output.Red(padded)
			}
			return output.Yellow(padded)
		}),
	}
}

// loadAuditConfig lê as exceções do projeto (problema no arquivo = erro de uso).
func loadAuditConfig(root string) (audit.Config, error) {
	cfg, err := audit.LoadConfig(root)
	if err != nil {
		return cfg, output.Usagef("%s", err)
	}
	return cfg, nil
}
