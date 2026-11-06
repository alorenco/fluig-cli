package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// pageStdin é a origem do JSON do `page set-preferences` (trocada nos testes).
var pageStdin io.Reader = os.Stdin

const defaultPageSlot = "SlotA"

func newPageCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "page",
		Short: "Páginas do portal: cria, põe widget, define permissão e publica",
		Long: "Gerencia as páginas do portal do Fluig.\n\n" +
			"Uma página tem uma versão publicada e, durante a edição, um rascunho.\n" +
			"A página nova nasce em rascunho. O add-widget e o permission gravam no\n" +
			"rascunho, e o publish torna as mudanças visíveis. A publicação troca o\n" +
			"instanceId das widgets.\n\n" +
			"Fluxo típico:\n" +
			"  fluigcli page create painel --title \"Painel\" --layout layoutdefault --view-group equipe\n" +
			"  fluigcli page add-widget painel minha_widget --slot SlotA\n" +
			"  fluigcli page publish painel\n\n" +
			"Para ver os layouts disponíveis, use: fluigcli layout list --all",
	}
	cmd.AddCommand(newPageListCmd(app))
	cmd.AddCommand(newPageShowCmd(app))
	cmd.AddCommand(newPageCreateCmd(app))
	cmd.AddCommand(newPageAddWidgetCmd(app))
	cmd.AddCommand(newPageRemoveWidgetCmd(app))
	cmd.AddCommand(newPageSetPreferencesCmd(app))
	cmd.AddCommand(newPagePermissionCmd(app))
	cmd.AddCommand(newPagePublishCmd(app))
	cmd.AddCommand(newPageDiscardCmd(app))
	cmd.AddCommand(newPageDeleteCmd(app))
	return cmd
}

// --- page list ---

func newPageListCmd(app *App) *cobra.Command {
	var (
		all           bool
		filter        string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista as páginas customizadas (com --all, também as nativas)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			pages, err := client.ListPages(ctx, all)
			if err != nil {
				return mapFluigError(err)
			}
			if filter != "" {
				f := strings.ToLower(filter)
				kept := pages[:0]
				for _, pg := range pages {
					if strings.Contains(strings.ToLower(pg.Code), f) || strings.Contains(strings.ToLower(pg.Title), f) {
						kept = append(kept, pg)
					}
				}
				pages = kept
			}
			if len(pages) == 0 {
				p.Infof("Nenhuma página encontrada. Crie uma com: fluigcli page create <código> --title <título> --layout <layout>")
			} else {
				rows := make([][]string, 0, len(pages))
				for _, pg := range pages {
					rows = append(rows, []string{pg.Code, pg.Title, pg.Layout, pg.Status, pg.Authorization})
				}
				// Padrão de listagem (ver CLAUDE.md). Rascunho em amarelo: falta publicar.
				p.Table(output.Table{
					Headers: []string{"Código", "Título", "Layout", "Status", "Autorização"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 3 && pages[row].Status == fluig.PageStatusDraft {
							return output.Yellow(padded)
						}
						return padded
					}),
				})
			}
			if pages == nil {
				pages = []fluig.Page{}
			}
			p.Done(map[string]any{"pages": pages})
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "inclui as páginas nativas do Fluig")
	cmd.Flags().StringVar(&filter, "filter", "", "filtra pelo trecho do código ou do título")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- page show ---

func newPageShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <página>",
		Short: "Mostra a página: layout, status, widgets publicadas (com preferências) e permissões",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			pg, err := client.GetPage(ctx, args[0])
			if err != nil {
				return mapFluigError(err)
			}
			widgets, err := client.PageWidgets(ctx, pg.Code)
			published := true
			if errors.Is(err, fluig.ErrPageNotPublished) {
				published, err = false, nil
			}
			if err != nil {
				return mapFluigError(err)
			}
			for i := range widgets {
				prefs, perr := client.WidgetPreferences(ctx, widgets[i].InstanceID)
				if perr != nil {
					return mapFluigError(perr)
				}
				widgets[i].Preferences = prefs
			}
			perms, err := client.PagePermissions(ctx, pg.Code)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("%s (%s)", pg.Title, pg.Code)
			p.Successf("Layout: %s · Status: %s · Versão: %d · Autorização: %s", pg.Layout, pg.Status, pg.Version, pg.Authorization)
			if !published {
				p.Successf("Widgets: a página ainda não tem versão publicada")
			} else if len(widgets) == 0 {
				p.Successf("Widgets: nenhuma na versão publicada")
			} else {
				rows := make([][]string, 0, len(widgets))
				for _, w := range widgets {
					rows = append(rows, []string{strconv.Itoa(w.InstanceID), w.Widget, w.Slot, formatPrefs(w.Preferences)})
				}
				p.Table(output.Table{Headers: []string{"Instância", "Widget", "Slot", "Preferências"}, Rows: rows, Style: output.BoldHeaderStyle(nil)})
			}
			if pg.Status == fluig.PageStatusDraft && published {
				p.Successf("Há um rascunho aberto. As widgets acima são da versão publicada. Publique com: fluigcli page publish %s", pg.Code)
			}
			p.Successf("Permissões: %s", formatPerms(perms))
			if widgets == nil {
				widgets = []fluig.PageWidget{}
			}
			p.Done(map[string]any{"page": pg, "published": published, "widgets": widgets, "permissions": perms})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

func formatPrefs(m map[string]string) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		if len(v) > 40 {
			v = v[:40] + "…"
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ", ")
}

func formatPerms(perms []fluig.PagePermission) string {
	byPerm := map[string][]string{}
	for _, pp := range perms {
		who := ""
		switch {
		case pp.Group != "":
			who = "grupo " + pp.Group
		case pp.Role != "":
			who = "papel " + pp.Role
		default:
			who = "usuário " + pp.User
		}
		byPerm[pp.Permission] = append(byPerm[pp.Permission], who)
	}
	var parts []string
	for _, k := range []string{fluig.PagePermView, fluig.PagePermUpdate, fluig.PagePermRemove} {
		if len(byPerm[k]) > 0 {
			parts = append(parts, k+": "+strings.Join(byPerm[k], ", "))
		}
	}
	if len(parts) == 0 {
		return "nenhuma"
	}
	return strings.Join(parts, " · ")
}

// viewFlags reúne as flags --view-* (quem pode ver a página).
type viewFlags struct {
	groups, roles, users []string
}

func (v *viewFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&v.groups, "view-group", nil, "grupo que pode ver a página (repita ou separe por vírgula)")
	cmd.Flags().StringSliceVar(&v.roles, "view-role", nil, "papel que pode ver a página (repita ou separe por vírgula)")
	cmd.Flags().StringSliceVar(&v.users, "view-user", nil, "login que pode ver a página (repita ou separe por vírgula)")
}

func (v *viewFlags) empty() bool {
	return len(v.groups) == 0 && len(v.roles) == 0 && len(v.users) == 0
}

func (v *viewFlags) permissions() []fluig.PagePermission {
	var out []fluig.PagePermission
	for _, g := range v.groups {
		out = append(out, fluig.PagePermission{Permission: fluig.PagePermView, Group: g})
	}
	for _, r := range v.roles {
		out = append(out, fluig.PagePermission{Permission: fluig.PagePermView, Role: r})
	}
	for _, u := range v.users {
		out = append(out, fluig.PagePermission{Permission: fluig.PagePermView, User: u})
	}
	return out
}

// validate confere que grupos, papéis e logins existem. O Fluig grava
// permissão para grupo inexistente sem erro (medido em 2026-11-06).
func (v *viewFlags) validate(ctx context.Context, client *fluig.Client) error {
	for _, g := range v.groups {
		if _, err := client.GetGroup(ctx, g); err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				return output.NotFoundf("grupo %q não existe no servidor", g)
			}
			return mapFluigError(err)
		}
	}
	for _, r := range v.roles {
		if _, err := client.GetRole(ctx, r); err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				return output.NotFoundf("papel %q não existe no servidor", r)
			}
			return mapFluigError(err)
		}
	}
	for _, u := range v.users {
		if _, err := client.GetAdminUser(ctx, u); err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				return output.NotFoundf("usuário %q não existe no servidor", u)
			}
			return mapFluigError(err)
		}
	}
	return nil
}

// --- page create ---

func newPageCreateCmd(app *App) *cobra.Command {
	var (
		title, description, layout string
		public, passwordStdin      bool
		view                       viewFlags
	)
	cmd := &cobra.Command{
		Use:   "create <código>",
		Short: "Cria uma página em rascunho",
		Long: "Cria uma página em rascunho. Depois, ponha as widgets com add-widget e\n" +
			"publique com publish.\n\n" +
			"Sem --view-*, a página segue o padrão do Fluig: todos os usuários\n" +
			"autenticados (papel user) veem. Com --view-*, só os grupos, papéis e\n" +
			"usuários informados veem. Nos dois casos, o usuário que cria a página\n" +
			"recebe as permissões de editar e remover.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			if title == "" || layout == "" {
				return output.Usagef("informe --title e --layout (veja os layouts com: fluigcli layout list --all)")
			}
			if description == "" {
				description = title
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar página")
			if err != nil {
				return err
			}
			if _, err := client.FindLayout(ctx, layout); err != nil {
				if errors.Is(err, fluig.ErrNotFound) {
					return output.NotFoundf("layout %q não existe no servidor; veja os layouts com: fluigcli layout list --all", layout)
				}
				return mapFluigError(err)
			}
			if err := view.validate(ctx, client); err != nil {
				return err
			}
			perms := client.DefaultPagePermissions()
			if !view.empty() {
				kept := perms[:0]
				for _, pp := range perms {
					if pp.Permission != fluig.PagePermView {
						kept = append(kept, pp)
					}
				}
				perms = append(view.permissions(), kept...)
			}
			err = client.CreatePage(ctx, fluig.PageCreateInput{
				Code: code, Title: title, Description: description, Layout: layout, Public: public, Permissions: perms,
			})
			if err != nil {
				if errors.Is(err, fluig.ErrPageExists) {
					return output.ServerErrorf("a página %q já existe no servidor", code).WithCause(err)
				}
				return mapFluigError(err)
			}
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("página %q criada em rascunho (layout %s)", pg.Code, pg.Layout)
			p.Infof("Próximo passo: fluigcli page add-widget %s <widget> --slot <slot>; depois fluigcli page publish %s", pg.Code, pg.Code)
			p.Done(map[string]any{"page": pg, "permissions": perms})
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "título da página (obrigatório)")
	cmd.Flags().StringVar(&description, "description", "", "descrição da página (default: o título)")
	cmd.Flags().StringVar(&layout, "layout", "", "código do layout (obrigatório; veja: fluigcli layout list --all)")
	cmd.Flags().BoolVar(&public, "public", false, "página pública (sem login); o default é só para usuários autenticados")
	view.register(cmd)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page add-widget ---

func newPageAddWidgetCmd(app *App) *cobra.Command {
	var (
		slot          string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "add-widget <página> <widget>",
		Short: "Põe uma widget num slot do rascunho da página",
		Long: "Põe uma widget num slot do rascunho da página. Sem rascunho aberto, o\n" +
			"comando abre um a partir da versão publicada. A mudança só aparece depois\n" +
			"do publish.\n\n" +
			"O Fluig não confere o slot. Por isso, a CLI confere o slot pelo layout da\n" +
			"página, numa página publicada com o mesmo layout. Quando não acha essa\n" +
			"página, a CLI avisa e segue.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code, widget := args[0], args[1]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar página")
			if err != nil {
				return err
			}
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			slots, err := client.LayoutSlots(ctx, pg.Layout)
			if err != nil {
				return mapFluigError(err)
			}
			if slots == nil {
				p.Warnf("não foi possível conferir o slot %q: nenhuma página publicada com o layout %s pôde ser renderizada", slot, pg.Layout)
			} else if !slices.Contains(slots, slot) {
				return output.Usagef("o layout %s não tem o slot %q; slots disponíveis: %s", pg.Layout, slot, strings.Join(slots, ", "))
			}
			draftOpened := false
			w, err := client.AddPageWidget(ctx, code, slot, widget)
			if errors.Is(err, fluig.ErrPageNoDraft) {
				if derr := client.CreatePageDraft(ctx, code); derr != nil {
					return mapFluigError(derr)
				}
				draftOpened = true
				w, err = client.AddPageWidget(ctx, code, slot, widget)
			}
			if err != nil {
				if errors.Is(err, fluig.ErrNotFound) {
					return output.NotFoundf("widget %q não existe no servidor (publique com: fluigcli widget export)", widget).WithCause(err)
				}
				return mapFluigError(err)
			}
			if draftOpened {
				p.Infof("rascunho aberto a partir da versão publicada")
			}
			p.Successf("widget %q posta no slot %s do rascunho de %q", w.Widget, w.Slot, code)
			p.Infof("Publique com: fluigcli page publish %s (a publicação troca o instanceId)", code)
			p.Done(map[string]any{"widget": w, "draftOpened": draftOpened, "slots": slots})
			return nil
		},
	}
	cmd.Flags().StringVar(&slot, "slot", defaultPageSlot, "slot do layout que recebe a widget")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// parseInstanceID lê o instanceId do argumento.
func parseInstanceID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, output.Usagef("instanceId inválido %q; veja os ids com: fluigcli page show <página>", s)
	}
	return id, nil
}

// publishedInstance confere que a instância está na versão publicada da
// página e que não há rascunho aberto (com rascunho, o id publicado não vale
// para o DELETE: medido 404 WCMWidgetNotFoundException).
func publishedInstance(ctx context.Context, client *fluig.Client, code string, id int) (*fluig.PageWidget, error) {
	pg, err := client.GetPage(ctx, code)
	if err != nil {
		return nil, mapFluigError(err)
	}
	widgets, err := client.PageWidgets(ctx, code)
	if errors.Is(err, fluig.ErrPageNotPublished) {
		return nil, output.Usagef("a página %q ainda não tem versão publicada; publique antes com: fluigcli page publish %s", code, code)
	}
	if err != nil {
		return nil, mapFluigError(err)
	}
	if pg.Status == fluig.PageStatusDraft {
		return nil, output.Usagef("a página %q tem um rascunho aberto; publique (fluigcli page publish %s) ou descarte (fluigcli page discard %s) antes", code, code, code)
	}
	for i := range widgets {
		if widgets[i].InstanceID == id {
			return &widgets[i], nil
		}
	}
	return nil, output.NotFoundf("a instância %d não está na versão publicada da página %q; veja os ids com: fluigcli page show %s", id, code, code)
}

// --- page remove-widget ---

func newPageRemoveWidgetCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "remove-widget <página> <instanceId>",
		Short: "Tira uma widget da versão publicada da página (efeito imediato)",
		Long: "Tira uma widget da versão publicada da página. O efeito é imediato, sem\n" +
			"publish. O comando exige que a página não tenha rascunho aberto.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			id, err := parseInstanceID(args[1])
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar página")
			if err != nil {
				return err
			}
			w, err := publishedInstance(ctx, client, code, id)
			if err != nil {
				return err
			}
			if err := client.RemovePageWidget(ctx, code, id); err != nil {
				return mapFluigError(err)
			}
			p.Successf("widget %q (instância %d, slot %s) removida da página %q", w.Widget, id, w.Slot, code)
			p.Done(map[string]any{"removed": w})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page set-preferences ---

func newPageSetPreferencesCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "set-preferences <página> <instanceId>",
		Short: "Grava as preferências de uma widget publicada (JSON pelo stdin; substitui todas)",
		Long: "Grava as preferências de uma widget da versão publicada. O JSON chega pelo\n" +
			"stdin, como um objeto de textos. Por exemplo: {\"titulo\":\"Painel\"}.\n\n" +
			"A gravação SUBSTITUI todas as preferências da instância. Uma chave que\n" +
			"falta no JSON some. O efeito é imediato, sem publish. O comando exige que\n" +
			"a página não tenha rascunho aberto.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			id, err := parseInstanceID(args[1])
			if err != nil {
				return err
			}
			if passwordStdin {
				return output.Usagef("set-preferences lê o JSON do stdin; use a sessão em cache ou FLUIGCLI_PASSWORD em vez de --password-stdin")
			}
			raw, err := io.ReadAll(io.LimitReader(pageStdin, 1<<20))
			if err != nil {
				return output.Usagef("falha ao ler o JSON do stdin: %v", err)
			}
			prefs, err := parsePreferences(raw)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, false, "alterar página")
			if err != nil {
				return err
			}
			w, err := publishedInstance(ctx, client, code, id)
			if err != nil {
				return err
			}
			if err := client.SetWidgetPreferences(ctx, id, prefs); err != nil {
				return mapFluigError(err)
			}
			p.Successf("preferências da widget %q (instância %d) gravadas: %d chave(s)", w.Widget, id, len(prefs))
			p.Done(map[string]any{"instanceId": id, "widget": w.Widget, "preferences": prefs})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "não suportado aqui: o stdin traz o JSON")
	return cmd
}

// parsePreferences aceita um objeto JSON de textos. Número e booleano viram
// texto; objeto e lista são recusados (o Fluig guarda só texto).
func parsePreferences(raw []byte) (map[string]string, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, output.Usagef("o stdin precisa trazer um objeto JSON, por exemplo {\"chave\":\"valor\"}")
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		switch t := v.(type) {
		case string:
			out[k] = t
		case float64, bool:
			out[k] = fmt.Sprint(t)
		case nil:
			out[k] = ""
		default:
			return nil, output.Usagef("a preferência %q precisa ser texto; serialize objetos e listas como texto JSON", k)
		}
	}
	return out, nil
}

// --- page permission ---

func newPagePermissionCmd(app *App) *cobra.Command {
	var (
		view          viewFlags
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "permission <página>",
		Short: "Troca quem pode ver a página (grava no rascunho; publique depois)",
		Long: "Troca a lista de quem pode ver a página pelos --view-* informados. As\n" +
			"permissões de editar e remover ficam iguais. O Fluig grava a mudança num\n" +
			"rascunho. Publique depois com: fluigcli page publish <página>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			if view.empty() {
				return output.Usagef("informe ao menos um --view-group, --view-role ou --view-user")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar página")
			if err != nil {
				return err
			}
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			if err := view.validate(ctx, client); err != nil {
				return err
			}
			current, err := client.PagePermissions(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			perms := view.permissions()
			for _, pp := range current {
				if pp.Permission != fluig.PagePermView {
					perms = append(perms, pp)
				}
			}
			if err := client.UpdatePagePermissions(ctx, pg, perms); err != nil {
				return mapFluigError(err)
			}
			p.Successf("permissões de %q gravadas no rascunho: %s", code, formatPerms(perms))
			p.Infof("Publique com: fluigcli page publish %s", code)
			p.Done(map[string]any{"page": code, "permissions": perms})
			return nil
		},
	}
	view.register(cmd)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page publish ---

func newPagePublishCmd(app *App) *cobra.Command {
	var (
		description   string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "publish <página>",
		Short: "Publica o rascunho da página",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar página")
			if err != nil {
				return err
			}
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			if pg.Status != fluig.PageStatusDraft {
				return output.Usagef("a página %q não tem rascunho para publicar (status %s)", code, pg.Status)
			}
			if err := client.PublishPage(ctx, code, description); err != nil {
				return mapFluigError(err)
			}
			widgets, err := client.PageWidgets(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("página %q publicada (%d widget(s))", code, len(widgets))
			if widgets == nil {
				widgets = []fluig.PageWidget{}
			}
			p.Done(map[string]any{"page": code, "widgets": widgets})
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "Publicado pelo fluigcli", "descrição da versão publicada")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page discard ---

func newPageDiscardCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "discard <página>",
		Short: "Descarta o rascunho aberto e volta à versão publicada",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "descartar rascunho")
			if err != nil {
				return err
			}
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			if pg.Status != fluig.PageStatusDraft {
				return output.Usagef("a página %q não tem rascunho aberto (status %s)", code, pg.Status)
			}
			if _, err := client.PageWidgets(ctx, code); errors.Is(err, fluig.ErrPageNotPublished) {
				return output.Usagef("a página %q nunca foi publicada; para apagar, use: fluigcli page delete %s", code, code)
			}
			if err := client.DiscardPageDraft(ctx, code); err != nil {
				return mapFluigError(err)
			}
			p.Successf("rascunho de %q descartado", code)
			p.Done(map[string]any{"page": code})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page delete ---

func newPageDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <página>...",
		Short: "Arquiva e exclui páginas",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais códigos de página a excluir")
			}
			if err := app.confirm(fmt.Sprintf("Excluir %d página(s) no servidor?", len(args))); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir páginas")
			if err != nil {
				return err
			}
			var results []itemResult
			var lastErr error
			failures := 0
			for _, code := range args {
				if derr := client.DeletePage(ctx, code); derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: code, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("página %q: %s", code, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: code, Action: "deleted", Success: true})
				p.Successf("página %q excluída", code)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}
