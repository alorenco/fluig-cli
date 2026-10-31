package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/rhino"
	"github.com/alorenco/fluig-cli/internal/testrun"
)

// Teste de unidade dos scripts Fluig no Rhino local (ROADMAP §5.18, spec em
// docs/interno/SPEC-TEST-D.md).

// unitTimeoutDefault é o teto da execução sem --timeout explícito. O padrão
// global (30 s) é das chamadas HTTP; aqui a JVM sobe uma vez para tudo.
const unitTimeoutDefault = 60 * time.Second

// Pontos de troca para os testes: o motor real exige Java e o jar.
var (
	unitFindJava  = rhino.FindJava
	unitEnsureJar = rhino.EnsureJar
	unitExecutor  = func(java *rhino.Java, jar *rhino.Jar) testrun.UnitExecutor {
		return func(ctx context.Context, boot, plan string) error {
			_, stderr, err := rhino.Run(ctx, java, jar, boot, plan)
			if errors.Is(err, context.DeadlineExceeded) {
				return testrun.ErrUnitTimeout
			}
			if err != nil {
				msg := strings.TrimSpace(string(stderr))
				if len(msg) > 600 {
					msg = msg[len(msg)-600:]
				}
				return fmt.Errorf("%v: %s", err, msg)
			}
			return nil
		}
	}
)

func newTestUnitCmd(app *App) *cobra.Command {
	var (
		filters   []string
		javaPath  string
		rhinoJar  string
		selfCheck bool
	)
	cmd := &cobra.Command{
		Use:   "unit [arquivo.unit.json|pasta]...",
		Short: "Teste de unidade dos scripts Fluig no Rhino local, sem servidor",
		Long: "Roda funções de script Fluig (eventos de formulário, scripts de processo,\n" +
			"datasets) na máquina local, sem servidor. O form, o hAPI, o\n" +
			"DatasetFactory, o getValue, o fluigAPI e o log são FALSOS declarados no\n" +
			"arquivo de teste. Cada caso leva milissegundos.\n\n" +
			"Alvo: um arquivo *.unit.json ou uma pasta (lê os *.unit.json da pasta e\n" +
			"das subpastas). Sem alvo, o comando lê tests/unit/ do projeto. Os\n" +
			"caminhos de \"scripts\" são relativos à raiz do projeto.\n\n" +
			"Formato (resumo; a referência completa está em docs/test.md):\n" +
			"  {\n" +
			"    \"scripts\": [\"forms/Compras/events/validateForm.js\"],\n" +
			"    \"defaults\": { \"wk\": { \"WKNumState\": \"4\" } },\n" +
			"    \"cases\": [\n" +
			"      { \"name\": \"sem fornecedor\", \"event\": \"validateForm\",\n" +
			"        \"form\": { \"fields\": { \"fornecedor\": \"\" } },\n" +
			"        \"expect\": { \"throws\": \"Fornecedor\" } }\n" +
			"    ]\n" +
			"  }\n\n" +
			"Motor: o Rhino 1.7.15.1 OFICIAL (a mesma versão do servidor), em modo ES6.\n" +
			"O jar vem do Maven Central na primeira execução, com o SHA-256 conferido,\n" +
			"e fica no cache local. Ele reproduz os defeitos do Rhino do Fluig (const\n" +
			"em laço, String Java vazia truthy). O --self-check confere o perfil.\n" +
			"Requer Java 11 ou mais novo (--java, $FLUIGCLI_JAVA, $JAVA_HOME ou PATH).\n" +
			"Sem Java ou sem o jar: exit 7.",
		Example: "  fluigcli test unit\n" +
			"  fluigcli test unit tests/unit/compras.unit.json --filter fornecedor\n" +
			"  fluigcli test unit --self-check\n" +
			"  fluigcli test unit --rhino-jar ~/jars/fluig-rhino-1.7.15.1.jar",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if selfCheck && (len(args) > 0 || len(filters) > 0) {
				return output.Usagef("--self-check não combina com arquivos nem com --filter")
			}
			var files []*testrun.UnitFile
			if selfCheck {
				files = []*testrun.UnitFile{testrun.SelfCheckFile()}
			} else {
				var err error
				if files, err = app.loadUnitFiles(args); err != nil {
					return err
				}
				if files, err = filterUnitCases(files, filters); err != nil {
					return err
				}
			}

			timeout := unitTimeoutDefault
			if app.timeoutExplicit {
				timeout = app.Timeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			java, err := unitFindJava(ctx, javaPath)
			if err != nil {
				return output.JavaNotFoundf("%v. Instale o Java %d ou mais novo, ou informe --java <caminho>", err, rhino.MinJava)
			}
			jar, err := unitEnsureJar(ctx, rhinoJar)
			if err != nil {
				return output.RhinoUnavailablef("%v. Sem rede, informe um jar local com --rhino-jar", err)
			}
			if jar.Source == "maven" {
				p.Infof("Rhino %s baixado do Maven Central (SHA-256 conferido): %s", rhino.Version, jar.Path)
			}

			dir, err := os.MkdirTemp("", "fluigcli-unit-")
			if err != nil {
				return output.LocalIOf("não consegui criar a pasta temporária: %v", err)
			}
			defer os.RemoveAll(dir)
			inicio := time.Now()
			rep, err := testrun.RunUnit(ctx, files, unitExecutor(java, jar), dir)
			if err != nil {
				if errors.Is(err, testrun.ErrMissing) {
					return output.NotFoundf("%v", err)
				}
				if errors.Is(err, testrun.ErrUnitEngine) {
					dica := ""
					if strings.Contains(err.Error(), "tools.shell.Main") {
						dica = ". O jar não tem o shell do Rhino (org.mozilla.javascript.tools.shell.Main); use o rhino-" + rhino.Version + ".jar oficial ou um jar que o inclua"
					}
					return output.RhinoUnavailablef("%v%s", err, dica)
				}
				return output.Genericf("falha no teste de unidade: %v", err)
			}
			rep.Summary.DurationMs = time.Since(inicio).Milliseconds()
			rep.Engine = testrun.UnitEngine{Rhino: rhino.Version, Source: jar.Source, Jar: jar.Path,
				Java: java.Version, LanguageVersion: rhino.LanguageVersion}
			if jar.Source == "flag" {
				rep.Engine.Rhino = "jar informado"
			}

			renderUnitReport(p, rep, app.Verbose, selfCheck)
			sm := rep.Summary
			if sm.Failed+sm.Errored == 0 {
				p.Done(rep)
				return nil
			}
			msg := fmt.Sprintf("%d de %d caso(s) não passaram (%d falharam, %d com erro)",
				sm.Failed+sm.Errored, sm.Cases, sm.Failed, sm.Errored)
			if selfCheck {
				msg = fmt.Sprintf("o motor diverge do Rhino do servidor em %d de %d sonda(s)", sm.Failed+sm.Errored, sm.Cases)
			}
			p.FailData(rep, output.CodeTestFailed, msg)
			return output.TestFailedf("%s", msg)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&filters, "filter", nil, "roda só os casos cujo nome contém o texto (repetível)")
	f.StringVar(&javaPath, "java", "", "executável do Java (default: $FLUIGCLI_JAVA, $JAVA_HOME/bin/java ou java no PATH)")
	f.StringVar(&rhinoJar, "rhino-jar", "", "jar do motor; dispensa o download e NÃO confere o hash")
	f.BoolVar(&selfCheck, "self-check", false, "confere se o motor local reproduz o Rhino do servidor (não roda casos)")
	return cmd
}

// loadUnitFiles resolve os alvos e carrega os arquivos de teste.
func (a *App) loadUnitFiles(args []string) ([]*testrun.UnitFile, error) {
	root, err := a.projectRootForFiles()
	if err != nil {
		return nil, err
	}
	var paths []string
	if len(args) == 0 {
		dir := filepath.Join(root, project.TestsDir, project.UnitTestsDir)
		if _, err := os.Stat(dir); err != nil {
			return nil, output.NotFoundf("pasta %s/%s/ não encontrada em %s; informe o arquivo ou a pasta dos testes de unidade",
				project.TestsDir, project.UnitTestsDir, root)
		}
		args = []string{dir}
	}
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, output.NotFoundf("arquivo de teste %q não encontrado", arg)
			}
			return nil, output.Usagef("não consegui ler %s: %v", arg, err)
		}
		if !info.IsDir() {
			paths = append(paths, arg)
			continue
		}
		found, err := testrun.UnitFilesInDir(arg)
		if err != nil {
			return nil, output.Usagef("não consegui ler %s: %v", arg, err)
		}
		if len(found) == 0 {
			return nil, output.NotFoundf("nenhum arquivo *%s em %s", testrun.UnitExt, arg)
		}
		paths = append(paths, found...)
	}
	var files []*testrun.UnitFile
	for _, p := range paths {
		f, err := testrun.LoadUnitFile(p, root)
		if err != nil {
			if errors.Is(err, testrun.ErrMissing) {
				return nil, output.NotFoundf("%v", err)
			}
			return nil, output.Usagef("%s", strings.TrimPrefix(err.Error(), testrun.ErrUsage.Error()+": "))
		}
		files = append(files, f)
	}
	return files, nil
}

func filterUnitCases(files []*testrun.UnitFile, filters []string) ([]*testrun.UnitFile, error) {
	if len(filters) == 0 {
		return files, nil
	}
	var out []*testrun.UnitFile
	for _, f := range files {
		var keep []*testrun.UnitCase
		for _, c := range f.Cases {
			for _, flt := range filters {
				if strings.Contains(strings.ToLower(c.Name), strings.ToLower(flt)) {
					keep = append(keep, c)
					break
				}
			}
		}
		if len(keep) > 0 {
			nf := *f
			nf.Cases = keep
			out = append(out, &nf)
		}
	}
	if len(out) == 0 {
		return nil, output.Usagef("nenhum caso casa com --filter %s", strings.Join(filters, ", "))
	}
	return out, nil
}

func unitStatusLabel(s string) string {
	switch s {
	case testrun.UnitPassed:
		return "passou"
	case testrun.UnitFailed:
		return "FALHOU"
	}
	return "erro"
}

// fmtMs formata milissegundos curtos em pt-BR ("0,6 ms", "12 ms").
func fmtMs(ms float64) string {
	if ms < 10 {
		return strings.Replace(strconv.FormatFloat(ms, 'f', 1, 64), ".", ",", 1) + " ms"
	}
	return strconv.FormatFloat(ms, 'f', 0, 64) + " ms"
}

func renderUnitReport(p *output.Printer, r *testrun.UnitReport, verbose, selfCheck bool) {
	for _, f := range r.Files {
		p.Infof("%s", f.Path)
		if f.Description != "" {
			p.Infof("%s", f.Description)
		}
		rows := make([][]string, 0, len(f.Cases))
		status := make([]string, len(f.Cases))
		for i, c := range f.Cases {
			rows = append(rows, []string{strconv.Itoa(i + 1), c.Name, unitStatusLabel(c.Status), fmtMs(c.DurationMs)})
			status[i] = c.Status
		}
		p.Table(output.Table{
			Headers: []string{"#", "Caso", "Resultado", "Tempo"},
			Rows:    rows,
			Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
				if col != 2 || !output.ColorEnabled() || row < 0 || row >= len(status) {
					return padded
				}
				switch status[row] {
				case testrun.UnitPassed:
					return output.Green(padded)
				case testrun.UnitFailed:
					return output.Red(padded)
				}
				return output.Yellow(padded)
			}),
		})
		for i, c := range f.Cases {
			ok := c.Status == testrun.UnitPassed
			if ok && !verbose {
				continue
			}
			marca := "✓"
			if !ok {
				marca = "✗"
			}
			p.Infof("%s %d · %s", marca, i+1, c.Name)
			if c.Error != "" {
				p.Infof("    erro: %s", c.Error)
			}
			for _, a := range c.Assertions {
				if a.Passed && !verbose {
					continue
				}
				p.Infof("    %s", testrun.FormatAssertion(a))
			}
			if !ok || verbose {
				for _, l := range c.Logs {
					p.Infof("    log.%s: %s", l.Level, l.Message)
				}
			}
		}
		p.Infof("")
	}
	e := r.Engine
	origem := map[string]string{"maven": "oficial, baixado agora", "cache": "oficial, do cache", "flag": e.Jar}[e.Source]
	p.Infof("Motor: Rhino %s (%s), ES6, Java %s", e.Rhino, origem, e.Java)
	sm := r.Summary
	if selfCheck {
		p.Infof("Perfil do servidor: %d de %d sondas reproduzidas (%s)", sm.Passed, sm.Cases, fmtDuration(sm.DurationMs))
		return
	}
	p.Infof("Resumo: %d passaram · %d falharam · %d com erro (%s)", sm.Passed, sm.Failed, sm.Errored, fmtDuration(sm.DurationMs))
}
