package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/audit"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newAuditCmd(app *App) *cobra.Command {
	var (
		syncCatalog  bool
		failOn       string
		fix          bool
		processID    string
		saveBaseline bool
		noBaseline   bool
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
			"  FL006 (aviso)  getDataset(...).values encadeado sem guarda no client-side —\n" +
			"                   se a chamada falha, o formulário quebra com TypeError\n" +
			"  RHINO001 (aviso) === / !== entre um java.lang.String (getFieldName…,\n" +
			"                   getValue(\"WK...\") global) e literal — no Rhino do Fluig é\n" +
			"                   sempre false; use == ou String(...). Rastreia a variável\n" +
			"                   que recebe o retorno e o helper local (isEmpty) que compara\n" +
			"                   o parâmetro com === e recebe uma fonte java\n" +
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
			"  WF002 (aviso)  [--process] atividade humana sem seção activity-N no HTML\n" +
			"  WF003 (erro)   [--process] script do processo compara a etapa corrente com\n" +
			"                   um número que não é sequence de etapa — o ramo nunca roda\n\n" +
			"--process <id> liga as regras WF*: a CLI baixa o processo do servidor alvo\n" +
			"(read-only) e cruza as sequences reais dele com dois artefatos locais. No\n" +
			"formulário vinculado pelo forms.json, confere as classes activity-N do HTML\n" +
			"(WF001/WF002); activity-0 é sempre válido (formulário de abertura,\n" +
			"WKNumState = 0). Nos scripts workflow/scripts/<id>.*.js, confere os números\n" +
			"comparados com a etapa corrente (WF003) — o parâmetro de sequence do evento\n" +
			"e a variável que recebe getValue(\"WKNumState\").\n\n" +
			"--fix aplica as correções DETERMINÍSTICAS (CSS legado → flat; cor hex com\n" +
			"valor idêntico a uma variável do tema → var(...)); o restante fica no\n" +
			"relatório para correção manual.\n\n" +
			"O catálogo (classes e variáveis) vem embutido no binário; --sync o atualiza\n" +
			"do servidor alvo (o style guide é público, não requer login). Arquivos\n" +
			"minificados/vendorados e bundles gerados de widget SPA são ignorados;\n" +
			"em .fluigcli/audit.json ficam as exceções ({\"ignore\": [globs]}) e os\n" +
			"ajustes de nível ({\"severity\": {\"SG005\": \"off\"}}).\n\n" +
			"BASELINE (dívida antiga). --save-baseline grava os achados de hoje em\n" +
			".fluigcli/audit-baseline.json. Com esse arquivo no projeto, o audit e a\n" +
			"pré-checagem dos comandos de publish passam a reprovar só os achados NOVOS\n" +
			"— o código antigo aparece no relatório, marcado, mas não barra. Serve para\n" +
			"trocar o --no-audit (que desliga tudo, inclusive no código novo) por \"não\n" +
			"deixar piorar\". A identidade do achado é arquivo + regra + texto da linha,\n" +
			"não o número dela: reformatar o arquivo não invalida o baseline. Use\n" +
			"--no-baseline para conferir tudo, ignorando o arquivo.",
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

			if saveBaseline {
				b := audit.NewBaseline(root, res.Findings)
				if err := b.Save(root); err != nil {
					return output.Genericf("não consegui salvar o baseline: %v", err)
				}
				p.Successf("baseline gravado em .fluigcli/audit-baseline.json: %d achado(s) de %d arquivo(s). "+
					"Daqui em diante, só achados NOVOS reprovam.", len(res.Findings), res.Scanned)
				p.Done(map[string]any{"baseline": map[string]any{
					"path": ".fluigcli/audit-baseline.json", "entries": len(b.Entries), "findings": len(res.Findings)}})
				return nil
			}

			// O baseline não filtra o relatório: ele decide o que REPROVA. Um
			// achado antigo escondido viraria dívida invisível.
			var conhecidos, novos []audit.Finding
			resolvidos := 0
			if !noBaseline {
				base, err := audit.LoadBaseline(root)
				if err != nil {
					return output.Usagef("%v", err)
				}
				if base != nil {
					// Varredura completa cobre tudo o que o baseline referencia:
					// o que não apareceu foi corrigido ou o arquivo saiu do
					// projeto — dívida quitada nos dois casos. Com alvos
					// explícitos, só os arquivos desta rodada podem ser
					// declarados quitados.
					escopo := res.Files
					if len(args) == 0 {
						escopo = nil
					}
					conhecidos, novos, resolvidos = base.Partition(root, res.Findings, escopo)
					res.Findings = append(append([]audit.Finding{}, conhecidos...), novos...)
					sortFindings(res.Findings)
				}
			}
			usaBaseline := len(conhecidos) > 0 || resolvidos > 0

			errCount, warnCount := 0, 0
			errNovos, warnNovos := 0, 0
			for _, f := range res.Findings {
				novo := !f.Baseline
				if f.Severity == audit.SeverityError {
					errCount++
					if novo {
						errNovos++
					}
					continue
				}
				warnCount++
				if novo {
					warnNovos++
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
			if usaBaseline {
				p.Infof("%d achado(s) já estavam no baseline e não reprovam; %d novo(s).", len(conhecidos), len(novos))
				if resolvidos > 0 {
					p.Infof("%d achado(s) do baseline sumiram (dívida quitada) — regrave com: fluigcli audit --save-baseline", resolvidos)
				}
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
			if usaBaseline {
				data["baseline"] = map[string]int{"known": len(conhecidos), "new": len(novos), "resolved": resolvidos}
			}
			// Com baseline, só o que é NOVO reprova — é o ponto do recurso.
			errFail, warnFail := errCount, warnCount
			if usaBaseline {
				errFail, warnFail = errNovos, warnNovos
			}
			fail := (failOn == "error" && errFail > 0) || (failOn == "warning" && errFail+warnFail > 0)
			if fail {
				msg := fmt.Sprintf("auditoria reprovada: %d erro(s) e %d aviso(s) (limiar --fail-on %s)", errFail, warnFail, failOn)
				if usaBaseline {
					msg += fmt.Sprintf("; %d achado(s) do baseline não contam", len(conhecidos))
				}
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
	cmd.Flags().BoolVar(&saveBaseline, "save-baseline", false, "grava os achados de hoje em .fluigcli/audit-baseline.json (depois, só achados novos reprovam)")
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "ignora o .fluigcli/audit-baseline.json e reprova todos os achados")
	return cmd
}

// sortFindings devolve os achados na ordem do relatório (arquivo, linha) — a
// mesma do audit.Run. O baseline reordena a lista ao separar conhecidos de
// novos, e o relatório não pode mudar de ordem por causa disso.
func sortFindings(fs []audit.Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].File != fs[j].File {
			return fs[i].File < fs[j].File
		}
		return fs[i].Line < fs[j].Line
	})
}

// runProcessCheck roda as regras WF* (audit --process): baixa o processo e
// cruza as etapas reais dele com dois artefatos locais — o HTML do formulário
// vinculado (WF001/WF002) e os scripts de evento do processo (WF003). Só
// leitura no servidor.
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

	states := make([]audit.ProcessActivity, 0, len(detail.States))
	for _, st := range detail.States {
		states = append(states, audit.ProcessActivity{Sequence: st.Sequence, Name: st.Name, Kind: st.Kind})
	}

	var findings []audit.Finding

	// WF001/WF002 — dependem do formulário vinculado ao processo.
	if detail.FormID == 0 {
		p.Infof("o processo %q (versão %d) não tem formulário vinculado — as regras WF001/WF002 ficam de fora.", processID, detail.Version)
	} else {
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
		htmlPath, err := mainFormHTML(project.FormDir(root, link.Folder))
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(htmlPath)
		if err != nil {
			return nil, err
		}
		rel := relOuAbsoluto(root, htmlPath)
		p.Infof("cruzando %s com o processo %q (versão %d, %d etapas).", rel, processID, detail.Version, len(states))
		findings = append(findings, audit.CheckFormActivities(rel, content, processID, states)...)
	}

	// WF003 — scripts de evento do processo. Não dependem do formulário: um
	// processo sem formulário vinculado ainda compara etapas nos scripts.
	scripts, err := project.FindProcessScripts(root, processID)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		// O prefixo do arquivo local pode diferir do processId do servidor
		// (ROADMAP §1.7-A) — dizer isso evita concluir "está tudo certo".
		p.Infof("nenhum script local com o prefixo %q em %s — a regra WF003 fica de fora.",
			processID, project.WorkflowScriptsDir)
	}
	for _, sc := range scripts {
		content, err := os.ReadFile(sc.Path)
		if err != nil {
			return nil, err
		}
		rel := relOuAbsoluto(root, sc.Path)
		findings = append(findings, audit.CheckProcessScriptStates(rel, content, processID, states)...)
	}
	if len(scripts) > 0 {
		p.Infof("cruzando %d script(s) de %q com as %d etapas do processo.", len(scripts), processID, len(states))
	}

	return audit.ApplySeverity(findings, cfg), nil
}

// relOuAbsoluto devolve o caminho relativo à raiz do projeto, em barras, ou o
// absoluto quando a relativização falha.
func relOuAbsoluto(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
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
