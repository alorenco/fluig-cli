package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/testrun"
)

// Esqueleto de suíte a partir do diagrama (ROADMAP §5.15, fase C.2; spec em
// docs/interno/SPEC-TEST-C.md §5).

type testNewFile struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"` // suite | fixture
	Scenarios int    `json:"scenarios,omitempty"`
	Fields    int    `json:"fields,omitempty"`
}

func newTestNewCmd(app *App) *cobra.Command {
	var (
		version       int
		fromRequest   int
		out           string
		splitService  string
		maxScenarios  int
		force         bool
		toStdout      bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "new <processId>",
		Short: "Gera o esqueleto de uma suíte a partir do diagrama do processo",
		Long: "Gera o ponto de partida de uma suíte de teste para um processo. O comando\n" +
			"lê o diagrama da versão ativa (só leitura) e escolhe caminhos que, juntos,\n" +
			"percorrem todas as transições do fluxo principal.\n\n" +
			"O esqueleto NÃO é uma suíte pronta. Os campos que fazem um gateway escolher\n" +
			"um caminho dependem da regra de negócio. Por isso cada passo traz uma\n" +
			"\"description\" com a orientação; as que começam com TODO pedem ação:\n" +
			"a condição do gateway, o campo que atribui a etapa e o mecanismo da etapa.\n\n" +
			"Um caminho que não precisa de service task para antes dela, numa etapa\n" +
			"humana. Os caminhos que passam por service task seguem até o fim e avisam\n" +
			"que podem gravar em sistema externo. Com --split-service <subpasta>, esses\n" +
			"cenários vão para um arquivo separado.\n\n" +
			"Com --from-request N, o comando grava também fixtures/card_base.json com o\n" +
			"card da solicitação N. A data de abertura vira {{today}} e o seu usuário vira\n" +
			"{{actors.eu.code}}. ⚠️ A fixture contém dados reais: revise antes de\n" +
			"versionar.\n\n" +
			"O mesmo diagrama gera sempre a mesma suíte. Arquivo existente sem --force =\n" +
			"recusa (exit 2), e nada é gravado.",
		Example: "  fluigcli test new Compras\n" +
			"  fluigcli test new Compras --split-service integracao --from-request 1201\n" +
			"  fluigcli test new Compras --stdout > rascunho.json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			pid := args[0]
			if toStdout && (fromRequest > 0 || splitService != "") {
				return output.Usagef("--stdout não combina com --from-request nem com --split-service (eles gravam mais de um arquivo)")
			}
			if toStdout && app.JSON {
				return output.Usagef("--stdout não combina com --json: o stdout recebe a suíte")
			}
			if maxScenarios <= 0 {
				return output.Usagef("--max-scenarios deve ser 1 ou mais")
			}
			if version < 0 {
				return output.Usagef("--version deve ser 1 ou mais")
			}
			if strings.ContainsAny(splitService, `/\`) || splitService == "." || splitService == ".." {
				return output.Usagef("--split-service espera o nome de uma subpasta (ex.: integracao)")
			}

			if out == "" && !toStdout {
				root, err := app.projectRootForFiles()
				if err != nil {
					return err
				}
				out = filepath.Join(root, project.TestsDir, pid)
			}
			mainPath := filepath.Join(out, pid+".json")
			svcPath := ""
			if splitService != "" {
				svcPath = filepath.Join(out, splitService, pid+".json")
			}
			fixPath := filepath.Join(out, project.TestFixturesDir, "card_base.json")
			if !toStdout && !force {
				alvos := []string{mainPath}
				if svcPath != "" {
					alvos = append(alvos, svcPath)
				}
				if fromRequest > 0 {
					alvos = append(alvos, fixPath)
				}
				for _, a := range alvos {
					if _, err := os.Stat(a); err == nil {
						return output.Usagef("%s já existe; use --force para sobrescrever (nada foi gravado)", a)
					}
				}
			}

			server, err := app.resolveServer("")
			if err != nil {
				return err
			}
			p.Server = server.Name
			ctx := context.Background()
			client, err := app.authenticate(ctx, server, passwordStdin)
			if err != nil {
				return err
			}
			eng := &testrun.FluigEngine{Client: client, Login: server.Username}
			pd, err := eng.Diagram(ctx, pid, version)
			if err != nil {
				subject := fmt.Sprintf("processo %q", pid)
				if version > 0 {
					subject = fmt.Sprintf("versão %d do processo %q", version, pid)
				}
				return testEngineErr(err, subject)
			}
			if len(pd.States) == 0 {
				return output.NotFoundf("o processo %q não tem versão ativa; informe --version", pid)
			}
			g := testrun.BuildGraph(pd)

			var warnings []string
			var fixture []byte
			fixFields := 0
			if fromRequest > 0 {
				fixture, fixFields, err = fixtureFromRequest(ctx, client, server, pid, fromRequest)
				if err != nil {
					return err
				}
				warnings = append(warnings, fmt.Sprintf(
					"a fixture contém dados reais da solicitação %d. Revise antes de versionar.", fromRequest))
			}
			card := ""
			if fixture != nil {
				card = "fixtures/card_base.json"
			} else if !toStdout {
				if _, err := os.Stat(fixPath); err == nil {
					card = "fixtures/card_base.json"
				}
			}
			res, err := testrun.Scaffold(g, testrun.ScaffoldOptions{
				MaxScenarios: maxScenarios, Card: card,
				SplitService: splitService != "", ServiceCard: "../" + card,
			})
			if err != nil {
				return output.Genericf("falha ao gerar a suíte: %v", err)
			}
			if len(res.Pending) > 0 {
				warnings = append(warnings, fmt.Sprintf("arestas do fluxo principal sem cenário (limite de %d cenários ou inalcançáveis): %s",
					maxScenarios, strings.Join(res.Pending, ", ")))
			}
			for _, w := range warnings {
				p.Warnf("%s", w)
			}

			if toStdout {
				data := res.Main
				if data == nil {
					data = res.Service
				}
				_, err := os.Stdout.Write(data)
				return err
			}

			var files []testNewFile
			write := func(path string, data []byte, f testNewFile) error {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return output.LocalIOf("não consegui criar %s: %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return output.LocalIOf("não consegui gravar %s: %v", path, err)
				}
				f.Path = path
				files = append(files, f)
				return nil
			}
			if res.Main != nil {
				if err := write(mainPath, res.Main, testNewFile{Kind: "suite", Scenarios: res.MainCount}); err != nil {
					return err
				}
			}
			if res.Service != nil {
				if err := write(svcPath, res.Service, testNewFile{Kind: "suite", Scenarios: res.ServiceCount}); err != nil {
					return err
				}
			}
			if fixture != nil {
				if err := write(fixPath, fixture, testNewFile{Kind: "fixture", Fields: fixFields}); err != nil {
					return err
				}
			}

			for _, f := range files {
				switch {
				case f.Kind == "fixture":
					p.Infof("Fixture:     %s (%d campos, da solicitação %d)", f.Path, f.Fields, fromRequest)
				case f.Path == svcPath:
					p.Infof("Integração:  %s (%d cenários)", f.Path, f.Scenarios)
				default:
					p.Infof("Suíte gerada: %s (%d cenários)", f.Path, f.Scenarios)
				}
			}
			p.Infof("")
			p.Infof("Arestas do fluxo principal cobertas pelos caminhos: %d de %d", res.Covered, res.Total)
			p.Infof("Itens TODO: %d (veja as descriptions)", res.Todos)
			p.Infof("")
			p.Infof("Próximo passo: fluigcli test run %s --dry-run", out)
			p.Done(map[string]any{
				"process": pid, "version": g.Version, "files": files,
				"coverage": map[string]int{"covered": res.Covered, "total": res.Total},
				"todos":    res.Todos, "scenarios": res.ScenarioNames, "warnings": nonNil(warnings),
			})
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&version, "version", 0, "versão do diagrama (default: a ativa)")
	f.IntVar(&fromRequest, "from-request", 0, "grava também fixtures/card_base.json com o card desta solicitação (dados reais)")
	f.StringVar(&out, "out", "", "pasta de saída (default: tests/<processId>/ na raiz do projeto)")
	f.StringVar(&splitService, "split-service", "", "grava os cenários que passam por service task em <out>/<subpasta>/")
	f.IntVar(&maxScenarios, "max-scenarios", 30, "limite de cenários gerados")
	f.BoolVar(&force, "force", false, "sobrescreve arquivos existentes")
	f.BoolVar(&toStdout, "stdout", false, "imprime a suíte no stdout em vez de gravar")
	f.BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// fixtureFromRequest monta a fixture plana a partir do card da solicitação
// (SPEC-TEST-C §5.4).
func fixtureFromRequest(ctx context.Context, client *fluig.Client, server *config.Server, pid string, id int) ([]byte, int, error) {
	req, err := client.GetRequest(ctx, id)
	if err != nil {
		return nil, 0, mapFluigError(err)
	}
	if req.ProcessID != pid {
		return nil, 0, output.Usagef("a solicitação %d é do processo %q, não de %q", id, req.ProcessID, pid)
	}
	if req.FormID == 0 || req.FormRecordID == 0 {
		return nil, 0, output.NotFoundf("a solicitação %d não tem card de formulário", id)
	}
	rec, err := client.GetFormRecord(ctx, int(req.FormID), int(req.FormRecordID), fluig.FormRecordOptions{})
	if err != nil {
		return nil, 0, mapFluigError(err)
	}
	code, err := client.ResolveUserCode(ctx)
	if err != nil {
		return nil, 0, mapFluigError(err)
	}
	abertura := ""
	if req.StartDate != nil {
		abertura = req.StartDate.Format("02/01/2006")
	}
	card := map[string]string{}
	for k, v := range testrun.FlattenCard(rec) {
		if k == "currentState" || k == "formMode" || strings.HasPrefix(k, "WK") {
			continue
		}
		switch {
		case abertura != "" && v == abertura:
			v = "{{today}}"
		case code != "" && v == code:
			v = "{{actors.eu.code}}"
		case server.Username != "" && v == server.Username:
			v = "{{actors.eu.login}}"
		case strings.Contains(v, "{{"):
			v = strings.ReplaceAll(v, "{{", "{{{{") // escape de texto literal
		}
		card[k] = v
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(card); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), len(card), nil
}
