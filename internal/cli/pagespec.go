package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// pageSpec é o arquivo versionável de uma página (wcm/page/<código>.json).
// Widgets traz só as widgets de CONTEÚDO (slots do layout). As widgets de
// sistema (cabeçalho, menu, busca) vêm do layout e ficam de fora.
type pageSpec struct {
	Code            string                 `json:"code"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description,omitempty"`
	Layout          string                 `json:"layout"`
	Authorization   string                 `json:"authorization,omitempty"` // AUTHENTICATED | PUBLIC
	FriendlyURL     string                 `json:"friendlyUrl,omitempty"`
	Parent          string                 `json:"parent,omitempty"`
	EnabledChildren bool                   `json:"enabledChildren,omitempty"`
	Permissions     []fluig.PagePermission `json:"permissions,omitempty"`
	Widgets         []pageSpecWidget       `json:"widgets"`
}

// pageSpecWidget é uma widget de conteúdo. A ordem dentro do slot é a ordem
// do arquivo.
type pageSpecWidget struct {
	Widget      string            `json:"widget"`
	Slot        string            `json:"slot"`
	Preferences map[string]string `json:"preferences,omitempty"`
}

func readPageSpec(path string) (*pageSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, output.NotFoundf("arquivo %q não encontrado", path)
		}
		return nil, output.LocalIOf("falha ao ler %s: %v", path, err)
	}
	var spec pageSpec
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return nil, output.Usagef("%s não é uma especificação de página válida: %v", path, err)
	}
	if err := spec.validate(); err != nil {
		return nil, output.Usagef("%s: %s", path, err.Error())
	}
	return &spec, nil
}

func (s *pageSpec) validate() error {
	switch {
	case s.Code == "":
		return errors.New("falta \"code\"")
	case s.Title == "":
		return errors.New("falta \"title\"")
	case s.Layout == "":
		return errors.New("falta \"layout\"")
	case s.Parent == s.Code:
		return errors.New("a página não pode ser pai dela mesma")
	}
	if s.Authorization == "" {
		s.Authorization = "AUTHENTICATED"
	}
	s.Authorization = strings.ToUpper(s.Authorization)
	if s.Authorization != "AUTHENTICATED" && s.Authorization != "PUBLIC" {
		return fmt.Errorf("\"authorization\" aceita AUTHENTICATED ou PUBLIC (veio %q)", s.Authorization)
	}
	s.FriendlyURL = strings.TrimPrefix(s.FriendlyURL, "/")
	if strings.ContainsAny(s.FriendlyURL, " /?#") {
		return fmt.Errorf("\"friendlyUrl\" aceita um único trecho de caminho (veio %q)", s.FriendlyURL)
	}
	for i, pp := range s.Permissions {
		n := 0
		for _, v := range []string{pp.Group, pp.Role, pp.User} {
			if v != "" {
				n++
			}
		}
		if n != 1 {
			return fmt.Errorf("a permissão %d precisa de exatamente um entre group, role e user", i+1)
		}
		if pp.Permission != fluig.PagePermView && pp.Permission != fluig.PagePermUpdate && pp.Permission != fluig.PagePermRemove {
			return fmt.Errorf("a permissão %d tem o tipo %q; use pageView, pageUpdate ou pageRemove", i+1, pp.Permission)
		}
	}
	for i, w := range s.Widgets {
		if w.Widget == "" || w.Slot == "" {
			return fmt.Errorf("a widget %d precisa de \"widget\" e \"slot\"", i+1)
		}
	}
	return nil
}

// permKey identifica uma permissão para comparar conjuntos.
func permKey(pp fluig.PagePermission) string {
	return pp.Permission + "|" + pp.Group + "|" + pp.Role + "|" + pp.User
}

func samePerms(a, b []fluig.PagePermission) bool {
	ka := make([]string, 0, len(a))
	for _, p := range a {
		ka = append(ka, permKey(p))
	}
	kb := make([]string, 0, len(b))
	for _, p := range b {
		kb = append(kb, permKey(p))
	}
	sort.Strings(ka)
	sort.Strings(kb)
	return slices.Equal(ka, kb)
}

func sortPerms(perms []fluig.PagePermission) {
	sort.SliceStable(perms, func(i, j int) bool { return permKey(perms[i]) < permKey(perms[j]) })
}

// contentWidgets filtra as widgets dos slots do layout, na ordem slot+ordem.
func contentWidgets(ws []fluig.PageWidget, slots []string) []fluig.PageWidget {
	var out []fluig.PageWidget
	for _, w := range ws {
		if slices.Contains(slots, w.Slot) {
			out = append(out, w)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].Order < out[j].Order
	})
	return out
}

// specWidgetsSorted devolve as widgets da especificação na ordem slot+arquivo.
func specWidgetsSorted(ws []pageSpecWidget) []pageSpecWidget {
	out := slices.Clone(ws)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

func sameWidgetLayout(cur []fluig.PageWidget, spec []pageSpecWidget) bool {
	sw := specWidgetsSorted(spec)
	if len(cur) != len(sw) {
		return false
	}
	for i := range cur {
		if cur[i].Widget != sw[i].Widget || cur[i].Slot != sw[i].Slot {
			return false
		}
	}
	return true
}

// validatePageSpecRefs confere no servidor o que o Fluig não confere:
// layout, página-pai, widgets, grupos/papéis/usuários e slots.
func validatePageSpecRefs(ctx context.Context, app *App, client *fluig.Client, spec *pageSpec) ([]string, *fluig.Page, error) {
	if _, err := client.FindLayout(ctx, spec.Layout); err != nil {
		if errors.Is(err, fluig.ErrNotFound) {
			return nil, nil, output.NotFoundf("layout %q não existe no servidor", spec.Layout)
		}
		return nil, nil, mapFluigError(err)
	}
	var parent *fluig.Page
	if spec.Parent != "" {
		pp, err := client.GetPage(ctx, spec.Parent)
		if err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				return nil, nil, output.NotFoundf("página-pai %q não existe no servidor; aplique a página-pai antes", spec.Parent)
			}
			return nil, nil, mapFluigError(err)
		}
		if !pp.EnabledChildren {
			return nil, nil, output.Usagef("a página-pai %q não aceita subpáginas; ponha \"enabledChildren\": true na especificação dela", spec.Parent)
		}
		parent = pp
	}
	seen := map[string]bool{}
	for _, w := range spec.Widgets {
		if seen[w.Widget] {
			continue
		}
		seen[w.Widget] = true
		if _, err := client.FindWidgetNative(ctx, w.Widget); err != nil {
			if errors.Is(err, fluig.ErrNotFound) {
				return nil, nil, output.NotFoundf("widget %q não existe no servidor (publique com: fluigcli widget export)", w.Widget)
			}
			return nil, nil, mapFluigError(err)
		}
	}
	var pr principals
	for _, pp := range spec.Permissions {
		switch {
		case pp.Group != "":
			pr.groups = append(pr.groups, pp.Group)
		case pp.Role != "":
			pr.roles = append(pr.roles, pp.Role)
		default:
			pr.users = append(pr.users, pp.User)
		}
	}
	pr.groups, pr.roles, pr.users = uniq(pr.groups), uniq(pr.roles), uniq(pr.users)
	if err := pr.validate(ctx, client); err != nil {
		return nil, nil, err
	}
	slots, err := app.layoutSlots(ctx, client, spec.Layout)
	if err != nil {
		return nil, nil, mapFluigError(err)
	}
	if slots != nil {
		for _, w := range spec.Widgets {
			if !slices.Contains(slots, w.Slot) {
				return nil, nil, output.Usagef("o layout %s não tem o slot %q (widget %s); slots disponíveis: %s", spec.Layout, w.Slot, w.Widget, strings.Join(slots, ", "))
			}
		}
	}
	return slots, parent, nil
}

func uniq(in []string) []string {
	slices.Sort(in)
	return slices.Compact(in)
}

// applyPageSpec cria ou alinha a página com a especificação e publica. Com
// dryRun, só descreve. Página com rascunho aberto é recusada: o apply não
// publica mudança de outra pessoa.
func applyPageSpec(ctx context.Context, app *App, client *fluig.Client, spec *pageSpec, dryRun bool) ([]string, error) {
	slots, parent, err := validatePageSpecRefs(ctx, app, client, spec)
	if err != nil {
		return nil, err
	}
	cur, err := client.GetPage(ctx, spec.Code)
	if err != nil && !errors.Is(err, fluig.ErrNotFound) {
		return nil, mapFluigError(err)
	}
	public := spec.Authorization == "PUBLIC"

	if cur == nil {
		actions := []string{fmt.Sprintf("criar a página %s (layout %s)", spec.Code, spec.Layout)}
		for _, w := range specWidgetsSorted(spec.Widgets) {
			actions = append(actions, fmt.Sprintf("pôr a widget %s no %s", w.Widget, w.Slot))
		}
		if spec.FriendlyURL != "" || spec.EnabledChildren {
			actions = append(actions, "ajustar URL amigável/subpáginas")
		}
		actions = append(actions, "publicar")
		if slots == nil && len(spec.Widgets) > 0 {
			actions = append(actions, "aviso: slots do layout não conferidos (nenhuma página publicada com o layout)")
		}
		if dryRun {
			return actions, nil
		}
		perms := spec.Permissions
		if len(perms) == 0 {
			perms = client.DefaultPagePermissions()
		}
		err := client.CreatePage(ctx, fluig.PageCreateInput{
			Code: spec.Code, Title: spec.Title, Description: orDefault(spec.Description, spec.Title),
			Layout: spec.Layout, Public: public, Permissions: perms, Parent: parent,
		})
		if err != nil {
			return nil, mapFluigError(err)
		}
		if spec.FriendlyURL != "" || spec.EnabledChildren {
			pg, err := client.GetPage(ctx, spec.Code)
			if err != nil {
				return nil, mapFluigError(err)
			}
			u := fluig.PageUpdate{}
			if spec.FriendlyURL != "" {
				u.FriendlyURL = &spec.FriendlyURL
			}
			if spec.EnabledChildren {
				t := true
				u.EnabledChildren = &t
			}
			if err := client.UpdatePage(ctx, pg, u, perms); err != nil {
				return nil, mapFluigError(err)
			}
		}
		for _, w := range specWidgetsSorted(spec.Widgets) {
			if _, err := client.AddPageWidget(ctx, spec.Code, w.Slot, w.Widget); err != nil {
				return nil, mapFluigError(err)
			}
		}
		if err := client.PublishPage(ctx, spec.Code, "fluigcli page apply"); err != nil {
			return nil, mapFluigError(err)
		}
		prefActions, err := applySpecPreferences(ctx, client, spec, slots, false)
		if err != nil {
			return nil, err
		}
		return append(actions, prefActions...), nil
	}

	if cur.Status == fluig.PageStatusDraft {
		return nil, output.Usagef("a página %q tem um rascunho aberto; publique (fluigcli page publish %s) ou descarte (fluigcli page discard %s) antes do apply", spec.Code, spec.Code, spec.Code)
	}
	var actions []string
	var u fluig.PageUpdate
	infoChanged := false
	setStr := func(label, from, to string, dst **string) {
		if from != to {
			v := to
			*dst = &v
			infoChanged = true
			actions = append(actions, fmt.Sprintf("%s: %q → %q", label, from, to))
		}
	}
	setStr("título", cur.Title, spec.Title, &u.Title)
	if spec.Description != "" {
		setStr("descrição", cur.Description, spec.Description, &u.Description)
	}
	setStr("URL amigável", strings.TrimPrefix(cur.FriendlyURL, "/"), spec.FriendlyURL, &u.FriendlyURL)
	if cur.Authorization != spec.Authorization {
		u.Public, infoChanged = &public, true
		actions = append(actions, fmt.Sprintf("autorização: %s → %s", cur.Authorization, spec.Authorization))
	}
	if cur.ParentCode != spec.Parent {
		id := 0
		if parent != nil {
			id = parent.ID
		}
		u.ParentID, infoChanged = &id, true
		actions = append(actions, fmt.Sprintf("página-pai: %q → %q", cur.ParentCode, spec.Parent))
	}
	if cur.EnabledChildren != spec.EnabledChildren {
		v := spec.EnabledChildren
		u.EnabledChildren, infoChanged = &v, true
		actions = append(actions, fmt.Sprintf("subpáginas: %v → %v", cur.EnabledChildren, spec.EnabledChildren))
	}
	curPerms, err := client.PagePermissions(ctx, spec.Code)
	if err != nil {
		return nil, mapFluigError(err)
	}
	perms := curPerms
	if len(spec.Permissions) > 0 && !samePerms(curPerms, spec.Permissions) {
		perms, infoChanged = spec.Permissions, true
		actions = append(actions, "permissões: "+formatPerms(spec.Permissions))
	}
	layoutChanged := cur.Layout != spec.Layout
	if layoutChanged {
		actions = append(actions, fmt.Sprintf("layout: %s → %s", cur.Layout, spec.Layout))
	}
	published, err := client.PageWidgets(ctx, spec.Code)
	if err != nil && !errors.Is(err, fluig.ErrPageNotPublished) {
		return nil, mapFluigError(err)
	}
	contentSlots := slots
	if contentSlots == nil {
		// Sem slots conhecidos, só os slots da especificação contam como
		// conteúdo: o apply nunca mexe numa widget fora deles.
		for _, w := range spec.Widgets {
			contentSlots = append(contentSlots, w.Slot)
		}
		contentSlots = uniq(contentSlots)
	}
	widgetsChanged := !sameWidgetLayout(contentWidgets(published, contentSlots), spec.Widgets)
	if widgetsChanged {
		actions = append(actions, "widgets: "+describeSpecWidgets(spec.Widgets))
	}
	structural := infoChanged || layoutChanged || widgetsChanged
	if structural {
		actions = append(actions, "publicar")
	}
	if dryRun {
		prefActions, err := applySpecPreferences(ctx, client, spec, contentSlots, true)
		if err != nil {
			return nil, err
		}
		actions = append(actions, prefActions...)
		if len(actions) == 0 {
			return []string{"página " + spec.Code + " já está igual à especificação"}, nil
		}
		return actions, nil
	}
	if infoChanged {
		if err := client.UpdatePage(ctx, cur, u, perms); err != nil {
			return nil, mapFluigError(err)
		}
	}
	if layoutChanged {
		if err := client.ChangePageLayout(ctx, spec.Code, spec.Layout); err != nil {
			return nil, mapFluigError(err)
		}
	}
	if widgetsChanged {
		if err := client.CreatePageDraft(ctx, spec.Code); err != nil && !errors.Is(err, fluig.ErrPageDraftExists) {
			return nil, mapFluigError(err)
		}
		draft, err := client.DraftWidgets(ctx, cur.ID, contentSlots)
		if err != nil {
			return nil, mapFluigError(err)
		}
		for _, w := range draft {
			if err := client.RemovePageWidget(ctx, spec.Code, w.InstanceID); err != nil {
				return nil, mapFluigError(err)
			}
		}
		for _, w := range specWidgetsSorted(spec.Widgets) {
			if _, err := client.AddPageWidget(ctx, spec.Code, w.Slot, w.Widget); err != nil {
				return nil, mapFluigError(err)
			}
		}
	}
	if structural {
		if err := client.PublishPage(ctx, spec.Code, "fluigcli page apply"); err != nil {
			return nil, mapFluigError(err)
		}
	}
	prefActions, err := applySpecPreferences(ctx, client, spec, contentSlots, false)
	if err != nil {
		return nil, err
	}
	actions = append(actions, prefActions...)
	if len(actions) == 0 {
		return []string{"página " + spec.Code + " já está igual à especificação"}, nil
	}
	return actions, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func describeSpecWidgets(ws []pageSpecWidget) string {
	if len(ws) == 0 {
		return "nenhuma"
	}
	parts := make([]string, 0, len(ws))
	for _, w := range specWidgetsSorted(ws) {
		parts = append(parts, w.Widget+"@"+w.Slot)
	}
	return strings.Join(parts, ", ")
}

// applySpecPreferences alinha as preferências das widgets publicadas com a
// especificação. A widget é casada pela posição: slot + ordem + código. Só
// as widgets com "preferences" na especificação são tocadas.
func applySpecPreferences(ctx context.Context, client *fluig.Client, spec *pageSpec, contentSlots []string, dryRun bool) ([]string, error) {
	published, err := client.PageWidgets(ctx, spec.Code)
	if err != nil {
		if errors.Is(err, fluig.ErrPageNotPublished) || errors.Is(err, fluig.ErrNotFound) {
			return nil, nil
		}
		return nil, mapFluigError(err)
	}
	cur := contentWidgets(published, contentSlots)
	want := specWidgetsSorted(spec.Widgets)
	var actions []string
	for i, w := range want {
		if w.Preferences == nil {
			continue
		}
		if i >= len(cur) || cur[i].Widget != w.Widget || cur[i].Slot != w.Slot {
			if dryRun {
				actions = append(actions, fmt.Sprintf("preferências de %s@%s (depois de publicar)", w.Widget, w.Slot))
				continue
			}
			return nil, output.ServerErrorf("a widget %s@%s não apareceu na versão publicada; confira com fluigcli page show %s", w.Widget, w.Slot, spec.Code)
		}
		have, err := client.WidgetPreferences(ctx, cur[i].InstanceID)
		if err != nil {
			return nil, mapFluigError(err)
		}
		if maps.Equal(have, w.Preferences) {
			continue
		}
		actions = append(actions, fmt.Sprintf("preferências de %s@%s (%d chave(s))", w.Widget, w.Slot, len(w.Preferences)))
		if dryRun {
			continue
		}
		if err := client.SetWidgetPreferences(ctx, cur[i].InstanceID, w.Preferences); err != nil {
			return nil, mapFluigError(err)
		}
	}
	return actions, nil
}

// layoutSlots devolve os slots de conteúdo do layout. Primeiro pela página
// publicada que usa o layout (servidor); sem ela, pelo layout.ftl do projeto
// (wcm/layout/<código>/src/main/resources/layout.ftl). nil = desconhecidos.
func (a *App) layoutSlots(ctx context.Context, client *fluig.Client, layout string) ([]string, error) {
	slots, err := client.LayoutSlots(ctx, layout)
	if err != nil || slots != nil {
		return slots, err
	}
	root, rerr := a.projectRootForFiles()
	if rerr != nil {
		return nil, nil
	}
	ftl, rerr := os.ReadFile(filepath.Join(project.LayoutDir(root, layout), "src", "main", "resources", "layout.ftl"))
	if rerr != nil {
		return nil, nil
	}
	if local := fluig.ParseLayoutSlots(string(ftl)); len(local) > 0 {
		return local, nil
	}
	return nil, nil
}

// --- page export ---

func newPageExportCmd(app *App) *cobra.Command {
	var (
		outPath       string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "export <página>",
		Short: "Grava a especificação da página publicada num arquivo JSON versionável",
		Long: "Grava a especificação da versão PUBLICADA da página em\n" +
			"wcm/page/<código>.json (ou no --output): título, layout, autorização, URL\n" +
			"amigável, página-pai, permissões e as widgets de conteúdo com as\n" +
			"preferências. O arquivo vai para o Git e é aplicado em outro servidor com\n" +
			"page apply ou com o passo \"page\" do deploy.\n\n" +
			"As widgets de sistema (cabeçalho, menu, busca) ficam de fora: o layout as\n" +
			"traz. Revise as preferências antes de aplicar em outro ambiente: elas\n" +
			"podem conter URLs ou códigos do ambiente de origem.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			code := args[0]
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			pg, err := client.GetPage(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			published, err := client.PageWidgets(ctx, code)
			if errors.Is(err, fluig.ErrPageNotPublished) {
				return output.Usagef("a página %q nunca foi publicada; publique antes do export", code)
			}
			if err != nil {
				return mapFluigError(err)
			}
			slots, err := app.layoutSlots(ctx, client, pg.Layout)
			if err != nil {
				return mapFluigError(err)
			}
			if slots == nil {
				return output.ServerErrorf("não foi possível ler os slots do layout %s (nenhuma página publicada com ele pôde ser renderizada)", pg.Layout)
			}
			if pg.Status == fluig.PageStatusDraft {
				p.Warnf("a página %q tem rascunho aberto; o export grava a versão publicada", code)
			}
			perms, err := client.PagePermissions(ctx, code)
			if err != nil {
				return mapFluigError(err)
			}
			sortPerms(perms)
			spec := pageSpec{
				Code: pg.Code, Title: pg.Title, Description: pg.Description, Layout: pg.Layout,
				Authorization: pg.Authorization, FriendlyURL: strings.TrimPrefix(pg.FriendlyURL, "/"),
				Parent: pg.ParentCode, EnabledChildren: pg.EnabledChildren, Permissions: perms,
				Widgets: []pageSpecWidget{},
			}
			for _, w := range contentWidgets(published, slots) {
				prefs, err := client.WidgetPreferences(ctx, w.InstanceID)
				if err != nil {
					return mapFluigError(err)
				}
				sw := pageSpecWidget{Widget: w.Widget, Slot: w.Slot}
				if len(prefs) > 0 {
					sw.Preferences = prefs
				}
				spec.Widgets = append(spec.Widgets, sw)
			}
			path := outPath
			if path == "" {
				root, err := app.projectRootForFiles()
				if err != nil {
					return err
				}
				path = filepath.Join(root, project.PagesDir, code+".json")
			}
			if err := writeJSONFile(path, spec); err != nil {
				return err
			}
			p.Successf("página %q exportada para %s (%d widget(s) de conteúdo)", code, path, len(spec.Widgets))
			p.Done(map[string]any{"path": path, "spec": spec})
			return nil
		},
	}
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "arquivo de saída (default: wcm/page/<código>.json no projeto)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- page apply ---

func newPageApplyCmd(app *App) *cobra.Command {
	var (
		dryRun        bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "apply <arquivo.json>",
		Short: "Cria ou alinha a página com a especificação do arquivo e publica (idempotente)",
		Long: "Cria a página quando ela não existe, ou alinha a página existente com a\n" +
			"especificação: informações, permissões, layout, widgets de conteúdo e\n" +
			"preferências. Quando algo muda, o apply publica. Rodar de novo não muda\n" +
			"nada.\n\n" +
			"Antes de gravar, a CLI confere layout, página-pai, widgets, grupos,\n" +
			"papéis, usuários e slots. Página com rascunho aberto é recusada: publique\n" +
			"ou descarte o rascunho antes.\n\n" +
			"Sem \"permissions\" no arquivo, a página nova recebe as permissões padrão\n" +
			"e a página existente mantém as dela.\n\n" +
			"Use --dry-run para ver as mudanças sem gravar.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			spec, err := readPageSpec(args[0])
			if err != nil {
				return err
			}
			ctx := context.Background()
			var client *fluig.Client
			if dryRun {
				_, client, err = app.connect(ctx, passwordStdin)
			} else {
				_, client, err = app.connectWrite(ctx, passwordStdin, "aplicar página")
			}
			if err != nil {
				return err
			}
			actions, err := applyPageSpec(ctx, app, client, spec, dryRun)
			if err != nil {
				return err
			}
			verb := "aplicada"
			if dryRun {
				verb = "plano (nada gravado)"
			}
			p.Successf("página %q: %s", spec.Code, verb)
			for _, a := range actions {
				p.Successf("  - %s", a)
			}
			p.Done(map[string]any{"code": spec.Code, "dryRun": dryRun, "actions": actions})
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "mostra as mudanças sem gravar")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha de AUTENTICAÇÃO do stdin")
	return cmd
}
