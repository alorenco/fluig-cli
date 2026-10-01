package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// O grupo layout publica layouts WCM pelo mesmo deploy nativo do widget export
// (pedido da issue #2, 2026-10-01). A estrutura local é a do widget, em
// wcm/layout/<código>, e o application.info declara application.type=layout.

func newLayoutCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layout",
		Short: "Lista, importa e publica layouts WCM (export = local → servidor; deploy nativo)",
		Long: "Layouts WCM são as páginas-molde do portal (slots onde os widgets\n" +
			"entram). A pasta local é wcm/layout/<código>, com a mesma estrutura de um\n" +
			"widget: src/main/resources (application.info, layout.ftl, .properties) e\n" +
			"src/main/webapp (WEB-INF, resources).\n\n" +
			"O export publica pelo deploy nativo do WCM, o mesmo do widget export. O\n" +
			"import baixa o WAR pelo fluigcliHelper (0.12.0 ou mais novo). Não há\n" +
			"scaffold de layout nesta versão.",
	}
	cmd.AddCommand(newLayoutListCmd(app))
	cmd.AddCommand(newLayoutImportCmd(app))
	cmd.AddCommand(newLayoutExportCmd(app))
	return cmd
}

// --- layout list (API nativa de page-management) ---

func newLayoutListCmd(app *App) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista os layouts do servidor",
		Long: "Lista os layouts customizados do servidor pela API nativa de\n" +
			"page-management. Com --all, inclui também os layouts internos da\n" +
			"plataforma (Amplo, Público, Constante...), que não se publicam pela CLI.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			layouts, err := client.ListLayouts(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			if !all {
				custom := layouts[:0]
				for _, l := range layouts {
					if !l.Internal {
						custom = append(custom, l)
					}
				}
				layouts = custom
			}
			switch {
			case len(layouts) == 0 && all:
				p.Infof("Nenhum layout no servidor.")
			case len(layouts) == 0:
				p.Infof("Nenhum layout customizado no servidor. Use --all para ver os layouts internos da plataforma.")
			default:
				headers := []string{"Código", "Título"}
				if all {
					headers = append(headers, "Origem")
				}
				rows := make([][]string, 0, len(layouts))
				for _, l := range layouts {
					row := []string{l.Code, l.Title}
					if all {
						origem := "customizado"
						if l.Internal {
							origem = "plataforma"
						}
						row = append(row, origem)
					}
					rows = append(rows, row)
				}
				// Padrão de listagem (ver CLAUDE.md): cabeçalho em negrito; os
				// customizados em verde — são os layouts que a CLI publica.
				p.Table(output.Table{
					Headers: headers,
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 2 && !layouts[row].Internal {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			if layouts == nil {
				layouts = []fluig.Layout{}
			}
			p.Done(map[string]any{"layouts": layouts, "all": all})
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "inclui os layouts internos da plataforma")
	return cmd
}

// --- layout import (servidor → local, via componente auxiliar) ---

func newLayoutImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <code>... | --all",
		Short: "Baixa layouts do servidor para o projeto local (servidor → local)",
		Long: "Baixa o WAR de cada layout pelo fluigcliHelper e desempacota em\n" +
			"wcm/layout/<code>/, no mesmo mapa do widget import. Requer o helper\n" +
			"0.12.0 ou mais novo: a API nativa de layouts não informa o arquivo .war,\n" +
			"e ele nem sempre se chama <code>.war.\n\n" +
			"O servidor acrescenta a linha application.tenant.code ao application.info\n" +
			"na instalação. O import a preserva, como o widget import.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais códigos de layout ou use --all")
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
			layouts, err := client.ListLayoutsHelper(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byCode := make(map[string]fluig.LayoutPackage, len(layouts))
			for _, l := range layouts {
				byCode[l.Code] = l
			}

			codes := args
			if all {
				codes = codes[:0]
				for _, l := range layouts {
					codes = append(codes, l.Code)
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, code := range codes {
				l, ok := byCode[code]
				if !ok {
					failures++
					lastErr = output.NotFoundf("layout %q não encontrado no servidor", code)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("layout %q: não encontrado", code)
					continue
				}
				if err := app.importOneLayout(ctx, client, root, l); err != nil {
					failures++
					lastErr = mapFluigError(err)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("layout %q: %s", code, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: code, Action: "imported", Success: true})
				p.Successf("layout %q importado em wcm/layout/%s", code, code)
			}
			if all && len(codes) == 0 {
				p.Infof("Nenhum layout customizado no servidor.")
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(codes))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os layouts customizados do servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// importOneLayout baixa o .war do layout e desempacota em wcm/layout/<code>.
func (a *App) importOneLayout(ctx context.Context, client *fluig.Client, root string, l fluig.LayoutPackage) error {
	war, err := client.DownloadLayout(ctx, l.Filename)
	if err != nil {
		return err
	}
	// O código vem do servidor — confina a pasta em wcm/layout/.
	dir, err := project.SafeJoin(filepath.Join(root, project.LayoutsDir), l.Code)
	if err != nil {
		return err
	}
	return unpackWAR(war, dir)
}

// --- layout export (local → servidor, deploy nativo) ---

func newLayoutExportCmd(app *App) *cobra.Command {
	var (
		force         bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "export <code>",
		Short: "Empacota e publica um layout no servidor (deploy nativo)",
		Long: "Empacota a pasta local do layout (wcm/layout/<code>) em um WAR e publica\n" +
			"no servidor pelo deploy nativo do WCM, o mesmo caminho do widget export. A\n" +
			"instalação é assíncrona no servidor.\n\n" +
			"A pasta precisa ter src/main/resources/application.info com\n" +
			"application.type=layout. Para publicar um widget, use widget export.\n\n" +
			"Antes de publicar, a CLI checa se o código já existe no servidor como\n" +
			"WIDGET. O deploy nativo identifica o destino só pelo nome do arquivo\n" +
			"(<código>.war), por isso publicar um layout com o código de um widget\n" +
			"sobrescreve o WAR do widget. Neste caso o comando recusa a publicação.\n" +
			"Use --force para prosseguir de propósito. Republicar um layout que já\n" +
			"existe como layout é a atualização normal e não dispara a guarda.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			code := args[0]

			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar o layout")
			if err != nil {
				return err
			}
			if err := app.exportOneLayout(ctx, p, client, root, code, force); err != nil {
				return err
			}
			p.Successf("layout %q enviado. A instalação é assíncrona no servidor.", code)
			p.Done(map[string]any{"layout": code})
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "publica mesmo que o código já exista como widget no servidor (sobrescreve o WAR do widget)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// requireLayoutInfo confere que a pasta declara application.type=layout. A
// checagem é local e barata, e pega o erro mais provável: apontar o export
// para a pasta de um widget (ou para um layout sem application.info — o
// servidor aceitaria o WAR e falharia depois, em silêncio, na instalação).
func requireLayoutInfo(dir, code string) error {
	typ, err := project.ReadApplicationType(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return output.Usagef("layout %q sem %s — um layout precisa do application.info com application.type=layout",
			code, project.ApplicationInfoRel)
	case err != nil:
		return err
	case typ == "":
		return output.Usagef("o application.info de %q não declara application.type — um layout precisa de application.type=layout", code)
	case typ != "layout":
		return output.Usagef("o application.info de %q declara application.type=%s, não layout — para publicar um widget use widget export", code, typ)
	}
	return nil
}

// checkWidgetCollision recusa a publicação de um layout cujo código já existe
// no servidor como WIDGET. É o espelho de checkLayoutCollision: o deploy
// nativo do WCM identifica o destino só pelo nome do arquivo (`<código>.war`),
// então o upload SOBRESCREVERIA o WAR do widget.
//
// Mesma política de falha em aberto: servidor sem resposta → avisa e segue.
// E o código de um layout existente NÃO dispara (a coleção applications só
// enxerga widgets — medido na homologação em 2026-10-01), então republicar o
// próprio layout é a atualização normal.
func checkWidgetCollision(ctx context.Context, p *output.Printer, client *fluig.Client, code string, force bool) error {
	w, err := client.FindWidgetNative(ctx, code)
	switch {
	case errors.Is(err, fluig.ErrNotFound):
		return nil
	case err != nil:
		p.Warnf("não consegui checar se %q já existe como widget no servidor (%v). A publicação segue.", code, err)
		return nil
	}

	title := w.Title
	if title == "" {
		title = w.Code
	}
	if force {
		p.Warnf("o código %q já existe como widget (%q) — --force informado, o WAR do widget será sobrescrito.", code, title)
		return nil
	}
	return output.Usagef(
		"o código %q já existe no servidor como WIDGET (%q). Publicar o layout sobrescreveria o WAR do widget. "+
			"Renomeie o layout ou publique com --force.", code, title)
}

// exportOneLayout empacota e publica um layout: é o corpo do `layout export`,
// compartilhado com o passo `layout` do `deploy` — inclusive a guarda de
// colisão com widget, que nunca pode ficar de fora.
func (a *App) exportOneLayout(ctx context.Context, p *output.Printer, client *fluig.Client,
	root, code string, force bool) error {
	dir := project.LayoutDir(root, code)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return output.NotFoundf("layout %q não encontrado em %s", code, project.LayoutsDir)
	}
	if err := requireLayoutInfo(dir, code); err != nil {
		return err
	}
	war, err := packWAR(dir)
	if err != nil {
		return err
	}
	if err := checkWidgetCollision(ctx, p, client, code, force); err != nil {
		return err
	}
	if err := client.UploadWidgetWAR(ctx, code+".war", war); err != nil {
		return mapFluigError(err)
	}
	return nil
}
