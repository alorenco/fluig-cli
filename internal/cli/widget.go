package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/scaffold"
)

func newWidgetCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "widget",
		Short: "Cria, lista, importa e exporta widgets (export = local → servidor; deploy nativo)",
	}
	cmd.AddCommand(newWidgetNewCmd(app))
	cmd.AddCommand(newWidgetListCmd(app))
	cmd.AddCommand(newWidgetImportCmd(app))
	cmd.AddCommand(newWidgetExportCmd(app))
	return cmd
}

// --- widget new (scaffold local, sem servidor) ---

func newWidgetNewCmd(app *App) *cobra.Command {
	var (
		title    string
		category string
		template string
		vuetify  bool
	)
	cmd := &cobra.Command{
		Use:   "new <code>",
		Short: "Cria um widget local a partir de um template (scaffold)",
		Long: "Gera em wcm/widget/<code> o esqueleto completo de um widget no padrão\n" +
			"oficial do Fluig (application.info, view/edit.ftl, i18n, JS SuperWidget,\n" +
			"CSS, ícone e README com o passo a passo). Nada é enviado ao servidor —\n" +
			"publique depois com `fluigcli widget export <code>`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			code := args[0]
			if err := scaffold.ValidateCode(code); err != nil {
				return output.Usagef("%s", err)
			}
			widgetDir := project.WidgetDir(root, code)
			files, err := scaffold.CreateWidget(widgetDir, scaffold.Options{
				Code:          code,
				Title:         title,
				Category:      category,
				Template:      template,
				Vuetify:       vuetify,
				DeveloperName: scaffoldDeveloperName(),
			})
			switch {
			case errors.Is(err, scaffold.ErrUnknownTemplate), errors.Is(err, scaffold.ErrDirExists),
				errors.Is(err, scaffold.ErrVuetifyTemplate):
				return output.Usagef("%s", err)
			case err != nil:
				return err
			}
			tplLabel := template
			if vuetify {
				tplLabel += " + Vuetify"
			}
			relDir := filepath.ToSlash(filepath.Join(project.WidgetsDir, code))
			p.Successf("widget %q criado em %s (template %s, %d arquivos)", code, relDir, tplLabel, len(files))
			p.Infof("Próximos passos: leia o %s/README.md; desenvolva com `fluigcli dev`; publique com `fluigcli widget export %s`.", relDir, code)
			p.Done(map[string]any{"widget": code, "template": template, "vuetify": vuetify, "dir": relDir, "files": files})
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "título do widget (padrão: o próprio código)")
	cmd.Flags().StringVar(&category, "category", "SYSTEM", "categoria no application.info")
	cmd.Flags().StringVar(&template, "template", "classic",
		"template do esqueleto (disponíveis: "+strings.Join(scaffold.Templates(), ", ")+")")
	cmd.Flags().BoolVar(&vuetify, "vuetify", false,
		"variante Vuetify 3 do template vue (UI kit via npm com tree-shaking; ícones @mdi/font — bom para converter widgets Vuetify antigas)")
	return cmd
}

// scaffoldDeveloperName resolve o developer.name do application.info: o usuário
// do SO, com fallback neutro.
func scaffoldDeveloperName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "fluigcli"
}

// --- widget list (componente auxiliar; fallback nativo) ---

func newWidgetListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os widgets do servidor",
		Long: "Lista os widgets customizados do servidor. Com o componente auxiliar\n" +
			"(fluigcliHelper ou fluiggersWidget) instalado usa a listagem dele (completa,\n" +
			"com o arquivo .war de cada widget); sem ele, cai para a API nativa de\n" +
			"page-management — que funciona, mas pode omitir widgets e não traz o\n" +
			"arquivo exigido pelo widget import.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			widgets, err := client.ListWidgets(ctx)
			// source = context-root do helper que respondeu (já cacheado).
			source, _ := client.ResolveHelper(ctx)
			if errors.Is(err, fluig.ErrHelperMissing) {
				// Fallback nativo: melhor uma listagem possivelmente incompleta
				// do que exit 7 num comando só de leitura.
				source = "native"
				p.Warnf("componente auxiliar não instalado — usando a listagem nativa, que pode omitir widgets e não traz o arquivo do widget import; para a listagem completa: fluigcli server install-helper %s", p.Server)
				widgets, err = client.ListWidgetsNative(ctx)
			}
			if err != nil {
				return mapFluigError(err)
			}
			rows := make([][]string, 0, len(widgets))
			for _, w := range widgets {
				rows = append(rows, []string{w.Code, w.Title})
			}
			if len(widgets) == 0 {
				p.Infof("Nenhum widget no servidor.")
			} else {
				// Padrão de listagem (ver CLAUDE.md).
				p.Table(output.Table{
					Headers: []string{"Código", "Título"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"widgets": widgets, "source": source})
			return nil
		},
	}
}

// --- widget import (servidor → local, via componente auxiliar) ---

func newWidgetImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <code>... | --all",
		Short: "Baixa widgets do servidor para o projeto local (servidor → local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais códigos de widget ou use --all")
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			widgets, err := client.ListWidgets(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byCode := make(map[string]fluig.Widget, len(widgets))
			for _, w := range widgets {
				byCode[w.Code] = w
			}

			codes := args
			if all {
				codes = codes[:0]
				for _, w := range widgets {
					codes = append(codes, w.Code)
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, code := range codes {
				w, ok := byCode[code]
				if !ok {
					failures++
					lastErr = output.NotFoundf("widget %q não encontrado no servidor", code)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("widget %q: não encontrado", code)
					continue
				}
				if err := app.importOneWidget(ctx, client, root, w); err != nil {
					failures++
					lastErr = mapFluigError(err)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("widget %q: %s", code, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: code, Action: "imported", Success: true})
				p.Successf("widget %q importado em wcm/widget/%s", code, code)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(codes))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os widgets do servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// importOneWidget baixa o .war do widget e desempacota no layout local.
func (a *App) importOneWidget(ctx context.Context, client *fluig.Client, root string, w fluig.Widget) error {
	war, err := client.DownloadWidget(ctx, w.Filename)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(war), int64(len(war)))
	if err != nil {
		return err
	}
	// O código do widget vem do servidor — confina a pasta em wcm/widget/.
	widgetDir, err := project.SafeJoin(filepath.Join(root, project.WidgetsDir), w.Code)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := project.MapWidgetEntryToLocal(f.Name)
		if rel == "" {
			continue // entrada fora do mapa (ignora)
		}
		dst, err := project.SafeJoin(widgetDir, rel) // defesa extra contra zip-slip
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// --- widget export (local → servidor, deploy nativo) ---

func newWidgetExportCmd(app *App) *cobra.Command {
	var (
		build         bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "export <NomeWidget>",
		Short: "Empacota e publica um widget no servidor (deploy nativo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			name := args[0]
			widgetDir := project.WidgetDir(root, name)
			if info, err := os.Stat(widgetDir); err != nil || !info.IsDir() {
				return output.NotFoundf("widget %q não encontrado em %s", name, project.WidgetsDir)
			}

			// Widget SPA (vue/react): --build compila antes de empacotar;
			// sem --build, bundle desatualizado vira aviso (o WAR levaria o
			// js velho e a mudança "não apareceria").
			switch {
			case build && !project.IsSPAWidgetDir(widgetDir):
				return output.Usagef("--build exige um widget com package.json (template vue/react); %q não tem", name)
			case build:
				if err := runNpmBuild(p, widgetDir); err != nil {
					return err
				}
			case project.IsSPAWidgetDir(widgetDir):
				if reason := project.StaleBundle(project.SPAWidget{Code: name, Dir: widgetDir}); reason != "" {
					p.Warnf("widget %q: %s — ou publique com widget export --build", name, reason)
				}
			}

			refs, err := project.CollectWidgetWARFiles(widgetDir)
			if err != nil {
				return err
			}
			if len(refs) == 0 {
				return output.Usagef("nada para empacotar em %s (esperado src/main/...)", widgetDir)
			}
			warFiles := make([]fluig.WARFile, 0, len(refs))
			for _, ref := range refs {
				content, err := os.ReadFile(ref.LocalPath)
				if err != nil {
					return err
				}
				warFiles = append(warFiles, fluig.WARFile{Name: ref.WARPath, Content: content})
			}
			war, err := fluig.BuildWAR(warFiles)
			if err != nil {
				return err
			}

			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar a widget")
			if err != nil {
				return err
			}
			if err := client.UploadWidgetWAR(ctx, name+".war", war); err != nil {
				return mapFluigError(err)
			}
			p.Successf("widget %q enviado (%d arquivos, %d KB). A instalação é assíncrona no servidor.", name, len(warFiles), len(war)/1024)
			p.Done(map[string]any{"widget": name, "files": len(warFiles)})
			return nil
		},
	}
	cmd.Flags().BoolVar(&build, "build", false, "roda `npm run build` no widget (template vue/react) antes de empacotar")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// npmBuildCommand monta o comando do build (variável para os testes trocarem).
var npmBuildCommand = func(dir string) *exec.Cmd {
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = dir
	return cmd
}

// runNpmBuild compila o bundle da widget SPA antes do empacotamento. A saída
// do npm vai para o stderr (o stdout é do envelope JSON); falha de build =
// exit 2, sem tocar o servidor.
func runNpmBuild(p *output.Printer, dir string) error {
	if _, err := exec.LookPath("npm"); err != nil {
		return output.Usagef("--build: npm não encontrado no PATH — instale o Node.js (ver .nvmrc da widget)")
	}
	p.Infof("compilando o bundle (npm run build)…")
	cmd := npmBuildCommand(dir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return output.Usagef("npm run build falhou (%v) — nada foi enviado ao servidor", err)
	}
	return nil
}
