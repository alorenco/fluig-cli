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
	cmd.AddCommand(newPageMoveWidgetCmd(app))
	cmd.AddCommand(newPageUpdateCmd(app))
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
			v, err := loadPageVersions(ctx, client, pg)
			if err != nil {
				return err
			}
			widgets, published := v.published, v.hasPub
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
			if pg.FriendlyURL != "" {
				p.Successf("URL amigável: %s", pg.FriendlyURL)
			}
			if pg.ParentCode != "" {
				p.Successf("Página-pai: %s", pg.ParentCode)
			}
			switch {
			case !published:
				p.Successf("Versão publicada: nenhuma (a página nunca foi publicada)")
			case len(widgets) == 0:
				p.Successf("Versão publicada: sem widgets")
			default:
				p.Successf("Versão publicada:")
				p.Table(output.Table{Headers: []string{"Instância", "Widget", "Slot", "Ordem", "Preferências"}, Rows: widgetRows(widgets, true), Style: output.BoldHeaderStyle(nil)})
			}
			if v.hasDraft {
				if pg.EditorLogin != "" {
					p.Successf("Rascunho aberto por: %s (%s)", pg.EditorName, pg.EditorLogin)
				}
				if len(v.draft) == 0 {
					p.Successf("Rascunho: sem widgets nos slots conhecidos")
				} else {
					p.Successf("Rascunho (ids valem até o publish):")
					p.Table(output.Table{Headers: []string{"Instância", "Widget", "Slot", "Ordem"}, Rows: widgetRows(v.draft, false), Style: output.BoldHeaderStyle(nil)})
				}
				p.Successf("Publique com: fluigcli page publish %s", pg.Code)
			}
			p.Successf("Permissões: %s", formatPerms(perms))
			if widgets == nil {
				widgets = []fluig.PageWidget{}
			}
			data := map[string]any{"page": pg, "published": published, "widgets": widgets, "permissions": perms}
			if v.hasDraft {
				draft := v.draft
				if draft == nil {
					draft = []fluig.PageWidget{}
				}
				data["draftWidgets"] = draft
			}
			p.Done(data)
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

// principals reúne grupos, papéis e logins de uma família de flags
// (--view-* = quem vê; --edit-* = quem edita e remove).
type principals struct {
	groups, roles, users []string
}

func (v *principals) register(cmd *cobra.Command, prefix, what string) {
	cmd.Flags().StringSliceVar(&v.groups, prefix+"-group", nil, "grupo que pode "+what+" a página (repita ou separe por vírgula)")
	cmd.Flags().StringSliceVar(&v.roles, prefix+"-role", nil, "papel que pode "+what+" a página (repita ou separe por vírgula)")
	cmd.Flags().StringSliceVar(&v.users, prefix+"-user", nil, "login que pode "+what+" a página (repita ou separe por vírgula)")
}

func (v *principals) empty() bool {
	return len(v.groups) == 0 && len(v.roles) == 0 && len(v.users) == 0
}

// permissions gera uma permissão de cada tipo informado para cada
// grupo/papel/login.
func (v *principals) permissions(kinds ...string) []fluig.PagePermission {
	var out []fluig.PagePermission
	for _, k := range kinds {
		for _, g := range v.groups {
			out = append(out, fluig.PagePermission{Permission: k, Group: g})
		}
		for _, r := range v.roles {
			out = append(out, fluig.PagePermission{Permission: k, Role: r})
		}
		for _, u := range v.users {
			out = append(out, fluig.PagePermission{Permission: k, User: u})
		}
	}
	return out
}

// validate confere que grupos, papéis e logins existem. O Fluig grava
// permissão para grupo inexistente sem erro (medido em 2026-11-06).
func (v *principals) validate(ctx context.Context, client *fluig.Client) error {
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

// mergePermissions troca, em current, as permissões de ver (se view não for
// vazio) e as de editar/remover (se edit não for vazio).
func mergePermissions(current []fluig.PagePermission, view, edit *principals) []fluig.PagePermission {
	var out []fluig.PagePermission
	if !view.empty() {
		out = append(out, view.permissions(fluig.PagePermView)...)
	}
	if !edit.empty() {
		out = append(out, edit.permissions(fluig.PagePermUpdate, fluig.PagePermRemove)...)
	}
	for _, pp := range current {
		isView := pp.Permission == fluig.PagePermView
		if (isView && !view.empty()) || (!isView && !edit.empty()) {
			continue
		}
		out = append(out, pp)
	}
	return out
}

// --- page create ---

func newPageCreateCmd(app *App) *cobra.Command {
	var (
		title, description, layout, parent string
		public, passwordStdin              bool
		view, edit                         principals
	)
	cmd := &cobra.Command{
		Use:   "create <código>",
		Short: "Cria uma página em rascunho",
		Long: "Cria uma página em rascunho. Depois, ponha as widgets com add-widget e\n" +
			"publique com publish.\n\n" +
			"Sem --view-*, a página segue o padrão do Fluig: todos os usuários\n" +
			"autenticados (papel user) veem. Com --view-*, só os grupos, papéis e\n" +
			"usuários informados veem. Sem --edit-*, o usuário que cria a página\n" +
			"recebe as permissões de editar e remover. Com --edit-*, recebem os\n" +
			"grupos, papéis e usuários informados.\n\n" +
			"Com --parent, a página nasce como subpágina. A página-pai precisa aceitar\n" +
			"subpáginas (fluigcli page update <pai> --enable-children).",
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
			if err := edit.validate(ctx, client); err != nil {
				return err
			}
			var parentPage *fluig.Page
			if parent != "" {
				parentPage, err = client.GetPage(ctx, parent)
				if err != nil {
					if errors.Is(err, fluig.ErrNotFound) {
						return output.NotFoundf("página-pai %q não existe no servidor", parent)
					}
					return mapFluigError(err)
				}
				if !parentPage.EnabledChildren {
					return output.Usagef("a página %q não aceita subpáginas; ligue com: fluigcli page update %s --enable-children (e publique)", parent, parent)
				}
			}
			perms := mergePermissions(client.DefaultPagePermissions(), &view, &edit)
			err = client.CreatePage(ctx, fluig.PageCreateInput{
				Code: code, Title: title, Description: description, Layout: layout, Public: public, Permissions: perms, Parent: parentPage,
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
	cmd.Flags().StringVar(&parent, "parent", "", "código da página-pai (cria uma subpágina)")
	view.register(cmd, "view", "ver")
	edit.register(cmd, "edit", "editar e remover")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page add-widget ---

func newPageAddWidgetCmd(app *App) *cobra.Command {
	var (
		slot          string
		passwordStdin bool
		takeOver      bool
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
			if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
				return err
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
				if errors.Is(err, fluig.ErrPageConcurrentEdit) {
					return concurrentEditError(code, err)
				}
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
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
	cmd.Flags().StringVar(&slot, "slot", defaultPageSlot, "slot do layout que recebe a widget")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// guardDraftOwner recusa mexer num rascunho aberto por outro usuário, a
// menos que --take-over seja informado. Com --take-over, assume o rascunho
// (PUT /change-edit-user), como o "Confirmar" da tela do Fluig.
func guardDraftOwner(ctx context.Context, p *output.Printer, client *fluig.Client, pg *fluig.Page, takeOver bool) error {
	if !client.DraftHeldByOther(pg) {
		return nil
	}
	who := pg.EditorLogin
	if pg.EditorName != "" {
		who = pg.EditorName + " (" + pg.EditorLogin + ")"
	}
	if !takeOver {
		return output.ServerErrorf("o rascunho da página %q está com %s; para assumir o rascunho, repita com --take-over (as mudanças dele continuam no rascunho)", pg.Code, who)
	}
	if err := client.TakeOverPageDraft(ctx, pg.Code); err != nil {
		return mapFluigError(err)
	}
	p.Warnf("rascunho de %q assumido de %s", pg.Code, who)
	return nil
}

// concurrentEditError traduz o ConcurrentPageEditException que escapar da
// checagem prévia (corrida entre a leitura e a escrita).
func concurrentEditError(code string, err error) error {
	if errors.Is(err, fluig.ErrPageConcurrentEdit) {
		return output.ServerErrorf("outro usuário passou a editar o rascunho da página %q; confira com fluigcli page show %s e repita com --take-over", code, code).WithCause(err)
	}
	return mapFluigError(err)
}

// parseInstanceID lê o instanceId do argumento.
func parseInstanceID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, output.Usagef("instanceId inválido %q; veja os ids com: fluigcli page show <página>", s)
	}
	return id, nil
}

// pageVersions reúne as widgets da versão publicada e, com rascunho aberto,
// as do rascunho. O rascunho só pode ser lido slot a slot (API legada); os
// slots vêm do layout (página renderizada) mais os slots já usados na
// versão publicada.
type pageVersions struct {
	page      *fluig.Page
	published []fluig.PageWidget
	draft     []fluig.PageWidget
	hasPub    bool
	hasDraft  bool
	slots     []string // slots do layout (nil = desconhecidos)
}

func loadPageVersions(ctx context.Context, client *fluig.Client, pg *fluig.Page) (*pageVersions, error) {
	v := &pageVersions{page: pg, hasDraft: pg.Status == fluig.PageStatusDraft}
	pub, err := client.PageWidgets(ctx, pg.Code)
	switch {
	case errors.Is(err, fluig.ErrPageNotPublished):
	case err != nil:
		return nil, mapFluigError(err)
	default:
		v.published, v.hasPub = pub, true
	}
	slots, err := client.LayoutSlots(ctx, pg.Layout)
	if err != nil {
		return nil, mapFluigError(err)
	}
	v.slots = slots
	if v.hasDraft {
		set := map[string]bool{defaultPageSlot: true}
		for _, s := range slots {
			set[s] = true
		}
		for _, w := range v.published {
			set[w.Slot] = true
		}
		all := make([]string, 0, len(set))
		for s := range set {
			all = append(all, s)
		}
		sort.Strings(all)
		draft, err := client.DraftWidgets(ctx, pg.ID, all)
		if err != nil {
			return nil, mapFluigError(err)
		}
		v.draft = draft
	}
	return v, nil
}

// find acha a instância e diz se ela é do rascunho. Um id publicado com
// rascunho aberto é recusado: o rascunho tem cópias com ids próprios (o
// DELETE do id publicado dá 404 nesse caso; medido).
func (v *pageVersions) find(id int) (*fluig.PageWidget, bool, error) {
	code := v.page.Code
	for i := range v.draft {
		if v.draft[i].InstanceID == id {
			return &v.draft[i], true, nil
		}
	}
	for i := range v.published {
		if v.published[i].InstanceID == id {
			if v.hasDraft {
				return nil, false, output.Usagef("a instância %d é da versão publicada, mas a página %q tem rascunho aberto; use o id do rascunho (fluigcli page show %s) ou descarte o rascunho", id, code, code)
			}
			return &v.published[i], false, nil
		}
	}
	return nil, false, output.NotFoundf("a instância %d não está na página %q; veja os ids com: fluigcli page show %s", id, code, code)
}

// checkSlot confere o slot contra o layout. Sem slots conhecidos, avisa.
func (v *pageVersions) checkSlot(p *output.Printer, slot string) error {
	if v.slots == nil {
		p.Warnf("não foi possível conferir o slot %q: nenhuma página publicada com o layout %s pôde ser renderizada", slot, v.page.Layout)
		return nil
	}
	if !slices.Contains(v.slots, slot) {
		return output.Usagef("o layout %s não tem o slot %q; slots disponíveis: %s", v.page.Layout, slot, strings.Join(v.slots, ", "))
	}
	return nil
}

func widgetRows(ws []fluig.PageWidget, withPrefs bool) [][]string {
	rows := make([][]string, 0, len(ws))
	for _, w := range ws {
		row := []string{strconv.Itoa(w.InstanceID), w.Widget, w.Slot, strconv.Itoa(w.Order)}
		if withPrefs {
			row = append(row, formatPrefs(w.Preferences))
		}
		rows = append(rows, row)
	}
	return rows
}

// --- page remove-widget ---

func newPageRemoveWidgetCmd(app *App) *cobra.Command {
	var passwordStdin, takeOver bool
	cmd := &cobra.Command{
		Use:   "remove-widget <página> <instanceId>",
		Short: "Tira uma widget da página (do rascunho ou, sem rascunho, da versão publicada)",
		Long: "Tira uma widget da página. Com rascunho aberto, use o id do rascunho\n" +
			"(page show): a remoção fica no rascunho até o publish. Sem rascunho, use o\n" +
			"id publicado: o efeito é imediato.",
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
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			v, err := loadPageVersions(ctx, client, pg)
			if err != nil {
				return err
			}
			w, inDraft, err := v.find(id)
			if err != nil {
				return err
			}
			if inDraft {
				if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
					return err
				}
			}
			if err := client.RemovePageWidget(ctx, code, id); err != nil {
				return concurrentEditError(code, err)
			}
			if inDraft {
				p.Successf("widget %q (instância %d, slot %s) removida do rascunho de %q", w.Widget, id, w.Slot, code)
				p.Infof("Publique com: fluigcli page publish %s", code)
			} else {
				p.Successf("widget %q (instância %d, slot %s) removida da página %q", w.Widget, id, w.Slot, code)
			}
			p.Done(map[string]any{"removed": w, "draft": inDraft})
			return nil
		},
	}
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page move-widget ---

func newPageMoveWidgetCmd(app *App) *cobra.Command {
	var (
		slot                    string
		order                   int
		passwordStdin, takeOver bool
	)
	cmd := &cobra.Command{
		Use:   "move-widget <página> <instanceId> --slot <slot>",
		Short: "Move uma widget para outro slot ou outra posição",
		Long: "Move uma widget para outro slot ou outra posição no slot. Com rascunho\n" +
			"aberto, use o id do rascunho (page show): a mudança fica no rascunho até o\n" +
			"publish. Sem rascunho, use o id publicado: o efeito é imediato.\n\n" +
			"Sem --order, a widget vai para o fim do slot de destino.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			id, err := parseInstanceID(args[1])
			if err != nil {
				return err
			}
			if slot == "" {
				return output.Usagef("informe o slot de destino com --slot")
			}
			if cmd.Flags().Changed("order") && order < 1 {
				return output.Usagef("--order começa em 1")
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
			v, err := loadPageVersions(ctx, client, pg)
			if err != nil {
				return err
			}
			w, inDraft, err := v.find(id)
			if err != nil {
				return err
			}
			if err := v.checkSlot(p, slot); err != nil {
				return err
			}
			if inDraft {
				if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
					return err
				}
			}
			if !cmd.Flags().Changed("order") {
				same := v.published
				if inDraft {
					same = v.draft
				}
				order = 1
				for _, o := range same {
					if o.Slot == slot && o.InstanceID != id {
						order++
					}
				}
			}
			if err := client.MovePageWidget(ctx, code, id, slot, order); err != nil {
				return concurrentEditError(code, err)
			}
			where := "da página"
			if inDraft {
				where = "do rascunho de"
			}
			p.Successf("widget %q (instância %d) movida para %s, posição %d, %s %q", w.Widget, id, slot, order, where, code)
			if inDraft {
				p.Infof("Publique com: fluigcli page publish %s", code)
			}
			p.Done(map[string]any{"instanceId": id, "widget": w.Widget, "slot": slot, "order": order, "draft": inDraft})
			return nil
		},
	}
	cmd.Flags().StringVar(&slot, "slot", "", "slot de destino (obrigatório)")
	cmd.Flags().IntVar(&order, "order", 0, "posição no slot, a partir de 1 (default: fim do slot)")
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page update ---

func newPageUpdateCmd(app *App) *cobra.Command {
	var (
		title, description, authorization, friendly, parent, layout string
		enableChildren, passwordStdin, takeOver                     bool
	)
	cmd := &cobra.Command{
		Use:   "update <página>",
		Short: "Altera título, descrição, autorização, URL amigável, página-pai e layout (grava no rascunho)",
		Long: "Altera as informações da página. Só os campos das flags informadas mudam.\n" +
			"O Fluig grava a mudança num rascunho. Publique depois com:\n" +
			"fluigcli page publish <página>\n\n" +
			"A URL amigável é relativa à raiz do servidor. Por exemplo, --friendly-url\n" +
			"painel abre a página em http://<servidor>/painel. --friendly-url \"\" remove.\n" +
			"--parent \"\" tira a página de debaixo da página-pai.\n\n" +
			"Na troca de layout, o Fluig remaneja as widgets para os slots do layout\n" +
			"novo e acrescenta as widgets de sistema do layout (cabeçalho, busca,\n" +
			"alertas). Confira o resultado com page show antes do publish. Se uma\n" +
			"widget ficar num slot que o layout novo não tem, a CLI avisa.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			changed := cmd.Flags().Changed
			var u fluig.PageUpdate
			hasChange := false
			if changed("title") {
				if title == "" {
					return output.Usagef("--title não pode ficar vazio")
				}
				u.Title, hasChange = &title, true
			}
			if changed("description") {
				u.Description, hasChange = &description, true
			}
			if changed("authorization") {
				var pub bool
				switch strings.ToLower(authorization) {
				case "public":
					pub = true
				case "authenticated":
				default:
					return output.Usagef("--authorization aceita public ou authenticated (veio %q)", authorization)
				}
				u.Public, hasChange = &pub, true
			}
			if changed("friendly-url") {
				f := strings.TrimPrefix(strings.TrimSpace(friendly), "/")
				if strings.ContainsAny(f, " /?#") {
					return output.Usagef("--friendly-url aceita um único trecho de caminho, sem espaço, barra, ? ou # (veio %q)", friendly)
				}
				u.FriendlyURL, hasChange = &f, true
			}
			if changed("enable-children") {
				u.EnabledChildren, hasChange = &enableChildren, true
			}
			if !hasChange && !changed("parent") && !changed("layout") {
				return output.Usagef("informe ao menos uma alteração (--title, --description, --authorization, --friendly-url, --parent, --enable-children ou --layout)")
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
			if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
				return err
			}
			if changed("parent") {
				id := 0
				if parent != "" {
					if parent == code {
						return output.Usagef("a página não pode ser pai dela mesma")
					}
					pp, err := client.GetPage(ctx, parent)
					if err != nil {
						if errors.Is(err, fluig.ErrNotFound) {
							return output.NotFoundf("página-pai %q não existe no servidor", parent)
						}
						return mapFluigError(err)
					}
					if !pp.EnabledChildren {
						return output.Usagef("a página %q não aceita subpáginas; ligue com: fluigcli page update %s --enable-children (e publique)", parent, parent)
					}
					id = pp.ID
				}
				u.ParentID, hasChange = &id, true
			}
			if layout != "" {
				if _, err := client.FindLayout(ctx, layout); err != nil {
					if errors.Is(err, fluig.ErrNotFound) {
						return output.NotFoundf("layout %q não existe no servidor; veja os layouts com: fluigcli layout list --all", layout)
					}
					return mapFluigError(err)
				}
			}
			if hasChange {
				perms, err := client.PagePermissions(ctx, code)
				if err != nil {
					return mapFluigError(err)
				}
				if err := client.UpdatePage(ctx, pg, u, perms); err != nil {
					return concurrentEditError(code, err)
				}
			}
			if layout != "" && layout != pg.Layout {
				if err := client.ChangePageLayout(ctx, code, layout); err != nil {
					return concurrentEditError(code, err)
				}
				pg2, err := client.GetPage(ctx, code)
				if err != nil {
					return mapFluigError(err)
				}
				if v, err := loadPageVersions(ctx, client, pg2); err == nil && v.slots != nil {
					for _, w := range v.draft {
						if !slices.Contains(v.slots, w.Slot) {
							p.Warnf("a widget %q (instância %d) está no slot %s, que o layout %s não tem; mova com: fluigcli page move-widget %s %d --slot <slot>", w.Widget, w.InstanceID, w.Slot, layout, code, w.InstanceID)
						}
					}
				}
			}
			after, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("página %q alterada no rascunho", code)
			p.Infof("Publique com: fluigcli page publish %s", code)
			p.Done(map[string]any{"page": after})
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "título novo")
	cmd.Flags().StringVar(&description, "description", "", "descrição nova")
	cmd.Flags().StringVar(&authorization, "authorization", "", "public (sem login) ou authenticated")
	cmd.Flags().StringVar(&friendly, "friendly-url", "", "URL amigável, relativa à raiz do servidor (\"\" remove)")
	cmd.Flags().StringVar(&parent, "parent", "", "código da página-pai (\"\" tira do pai)")
	cmd.Flags().BoolVar(&enableChildren, "enable-children", false, "aceita subpáginas (--enable-children=false desliga)")
	cmd.Flags().StringVar(&layout, "layout", "", "código do layout novo (veja: fluigcli layout list --all)")
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
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
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			if pg.Status == fluig.PageStatusDraft {
				return output.Usagef("a página %q tem um rascunho aberto; publique (fluigcli page publish %s) ou descarte (fluigcli page discard %s) antes", code, code, code)
			}
			v, err := loadPageVersions(ctx, client, pg)
			if err != nil {
				return err
			}
			w, _, err := v.find(id)
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
		view, edit    principals
		passwordStdin bool
		takeOver      bool
	)
	cmd := &cobra.Command{
		Use:   "permission <página>",
		Short: "Troca quem pode ver e quem pode editar a página (grava no rascunho; publique depois)",
		Long: "Troca a lista de quem pode ver a página pelos --view-* informados e a de\n" +
			"quem pode editar e remover pelos --edit-*. A família não informada fica\n" +
			"igual. O Fluig grava a mudança num rascunho. Publique depois com:\n" +
			"fluigcli page publish <página>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			if view.empty() && edit.empty() {
				return output.Usagef("informe ao menos um --view-group/--view-role/--view-user ou --edit-group/--edit-role/--edit-user")
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
			if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
				return err
			}
			if err := view.validate(ctx, client); err != nil {
				return err
			}
			if err := edit.validate(ctx, client); err != nil {
				return err
			}
			current, err := client.PagePermissions(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			perms := mergePermissions(current, &view, &edit)
			if err := client.UpdatePagePermissions(ctx, pg, perms); err != nil {
				return concurrentEditError(code, err)
			}
			p.Successf("permissões de %q gravadas no rascunho: %s", code, formatPerms(perms))
			p.Infof("Publique com: fluigcli page publish %s", code)
			p.Done(map[string]any{"page": code, "permissions": perms})
			return nil
		},
	}
	view.register(cmd, "view", "ver")
	edit.register(cmd, "edit", "editar e remover")
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page publish ---

func newPagePublishCmd(app *App) *cobra.Command {
	var (
		description   string
		passwordStdin bool
		takeOver      bool
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
			if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
				return err
			}
			if pg.Status != fluig.PageStatusDraft {
				return output.Usagef("a página %q não tem rascunho para publicar (status %s)", code, pg.Status)
			}
			if err := client.PublishPage(ctx, code, description); err != nil {
				return concurrentEditError(code, err)
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
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
	cmd.Flags().StringVar(&description, "description", "Publicado pelo fluigcli", "descrição da versão publicada")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}

// --- page discard ---

func newPageDiscardCmd(app *App) *cobra.Command {
	var passwordStdin, takeOver bool
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
			if err := guardDraftOwner(ctx, p, client, pg, takeOver); err != nil {
				return err
			}
			if pg.Status != fluig.PageStatusDraft {
				return output.Usagef("a página %q não tem rascunho aberto (status %s)", code, pg.Status)
			}
			if _, err := client.PageWidgets(ctx, code); errors.Is(err, fluig.ErrPageNotPublished) {
				return output.Usagef("a página %q nunca foi publicada; para apagar, use: fluigcli page delete %s", code, code)
			}
			if err := client.DiscardPageDraft(ctx, code); err != nil {
				return concurrentEditError(code, err)
			}
			p.Successf("rascunho de %q descartado", code)
			p.Done(map[string]any{"page": code})
			return nil
		},
	}
	cmd.Flags().BoolVar(&takeOver, "take-over", false, "assume o rascunho quando outro usuário o abriu")
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
