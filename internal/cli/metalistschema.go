package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// Fase 2 do ROADMAP §5.23: criar, alterar e remover a LISTA (as colunas).
// A API é frágil (medido em 2026-11-21): a edição não é atômica, a colisão de
// nome de coluna é ignorada em silêncio e um título com "/" quebra a lista.
// Por isso, toda validação acontece aqui, antes do primeiro POST.

const metaListMaxDescription = 254

const metaListColumnSpecHelp = "Coluna: --column \"TÍTULO:TIPO\". Para o tipo metalista, informe a lista\n" +
	"referenciada: --column \"TÍTULO:metalista:LISTA\" (id ou nome).\n" +
	"Tipos: texto, area-texto, inteiro, data, numerico e metalista.\n\n" +
	"O servidor gera o nome da coluna a partir do título: maiúsculas, sem\n" +
	"acento, espaço vira _. Por exemplo: \"Data Ref\" vira DATA_REF. O título\n" +
	"aceita letras, dígitos, espaço, _ e -. O nome gerado tem no máximo 30\n" +
	"caracteres. O servidor recusa palavras reservadas, como ID, DATE, USER e\n" +
	"VALUE."

// metaListColumnSpec é uma coluna lida de --column, já validada.
type metaListColumnSpec struct {
	col  fluig.MetaListNewColumn
	name string // nome gerado (columnNameId)
}

// parseMetaListColumnSpec lê "TÍTULO:TIPO[:LISTA]". A lista referenciada é
// resolvida no servidor.
func (a *App) parseMetaListColumnSpec(ctx context.Context, client *fluig.Client, spec string) (metaListColumnSpec, error) {
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return metaListColumnSpec{}, output.Usagef("coluna inválida %q: use TÍTULO:TIPO ou TÍTULO:metalista:LISTA", spec)
	}
	title := strings.TrimSpace(parts[0])
	typ, err := fluig.ParseMetaListType(parts[1])
	if err != nil {
		return metaListColumnSpec{}, output.Usagef("coluna %q: %v", spec, err)
	}
	name, err := fluig.MetaListColumnName(title)
	if err != nil {
		return metaListColumnSpec{}, output.Usagef("coluna %q: %v", spec, err)
	}
	out := metaListColumnSpec{col: fluig.MetaListNewColumn{Title: title, Type: typ}, name: name}
	if typ == fluig.MetaListTypeMetaList {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			return metaListColumnSpec{}, output.Usagef("coluna %q: o tipo metalista exige a lista referenciada (TÍTULO:metalista:LISTA)", spec)
		}
		ref, err := a.resolveMetaList(ctx, client, strings.TrimSpace(parts[2]))
		if err != nil {
			return metaListColumnSpec{}, err
		}
		out.col.RefListID, out.col.RefListName = ref.ID, ref.Name
	} else if len(parts) == 3 {
		return metaListColumnSpec{}, output.Usagef("coluna %q: só o tipo metalista aceita a lista referenciada", spec)
	}
	return out, nil
}

// applyLabels marca como rótulo as colunas novas cujo título ou nome está em
// labels. Devolve os itens de labels que não casaram com coluna nova.
func applyLabels(specs []metaListColumnSpec, labels []string) ([]string, error) {
	var rest []string
	for _, l := range labels {
		found := false
		for i := range specs {
			if strings.EqualFold(specs[i].col.Title, l) || strings.EqualFold(specs[i].name, l) {
				if specs[i].col.Type == fluig.MetaListTypeMetaList {
					return nil, output.Usagef("a coluna %q é do tipo metalista e não pode ser rótulo", l)
				}
				specs[i].col.Label = true
				found = true
			}
		}
		if !found {
			rest = append(rest, l)
		}
	}
	return rest, nil
}

// metaListNameTaken confere se outra lista já usa o nome. O servidor aceita
// nome repetido, e o nome repetido torna a lista ambígua na CLI.
func metaListNameTaken(lists []fluig.MetaListSummary, name string, exceptID int) *fluig.MetaListSummary {
	for i := range lists {
		if lists[i].ID != exceptID && strings.EqualFold(strings.TrimSpace(lists[i].Name), strings.TrimSpace(name)) {
			return &lists[i]
		}
	}
	return nil
}

func validateMetaListText(name, description string) error {
	if strings.TrimSpace(name) == "" {
		return output.Usagef("o nome da lista não pode ser vazio")
	}
	if len([]rune(description)) > metaListMaxDescription {
		return output.Usagef("a descrição tem %d caracteres. O limite é %d", len([]rune(description)), metaListMaxDescription)
	}
	return nil
}

// --- metalist create ---

func newMetalistCreateCmd(app *App) *cobra.Command {
	var (
		description   string
		columns       []string
		labels        []string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <nome> --column TÍTULO:TIPO...",
		Short: "Cria uma lista customizada",
		Long: "Cria uma lista com as colunas informadas. O servidor cria uma tabela no\n" +
			"banco para a lista.\n\n" + metaListColumnSpecHelp + "\n\n" +
			"Use --label TÍTULO para marcar uma coluna como rótulo. O rótulo é o texto\n" +
			"que aparece quando outra lista referencia esta. Uma coluna metalista não\n" +
			"pode ser rótulo.\n\n" +
			"A CLI recusa um nome que outra lista já usa.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			name := strings.TrimSpace(args[0])
			if err := validateMetaListText(name, description); err != nil {
				return err
			}
			if len(columns) == 0 {
				return output.Usagef("informe ao menos uma coluna com --column TÍTULO:TIPO")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar lista")
			if err != nil {
				return err
			}
			specs := make([]metaListColumnSpec, 0, len(columns))
			seen := map[string]string{}
			for _, c := range columns {
				spec, err := app.parseMetaListColumnSpec(ctx, client, c)
				if err != nil {
					return err
				}
				if prev, dup := seen[spec.name]; dup {
					return output.Usagef("as colunas %q e %q geram o mesmo nome %s", prev, spec.col.Title, spec.name)
				}
				seen[spec.name] = spec.col.Title
				specs = append(specs, spec)
			}
			rest, err := applyLabels(specs, labels)
			if err != nil {
				return err
			}
			if len(rest) > 0 {
				return output.Usagef("--label %q não corresponde a nenhuma coluna", rest[0])
			}

			before, err := client.ListMetaLists(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			if other := metaListNameTaken(before, name, 0); other != nil {
				return output.Usagef("já existe a lista %q (id %d). Escolha outro nome", other.Name, other.ID)
			}
			cols := make([]fluig.MetaListNewColumn, 0, len(specs))
			for _, s := range specs {
				cols = append(cols, s.col)
			}
			createErr := client.CreateMetaList(ctx, name, description, cols)

			// O servidor não devolve o id. A CLI acha a lista nova pelo nome.
			// Mesmo com erro, a lista pode ter ficado criada (DDL recusado).
			after, err := client.ListMetaLists(ctx)
			if err != nil {
				if createErr != nil {
					return mapFluigError(createErr)
				}
				return mapFluigError(err)
			}
			newID := newMetaListID(before, after, name)
			if createErr != nil {
				if newID != 0 {
					p.Warnf("O servidor recusou a criação, mas a lista %d ficou registrada. Confira com metalist show %d e remova com metalist delete %d.", newID, newID, newID)
				}
				return mapFluigError(createErr)
			}
			if newID == 0 {
				p.Warnf("A lista foi criada, mas a CLI não identificou o id dela. Confira com metalist list.")
				p.Done(map[string]any{"list": nil})
				return nil
			}
			m, err := client.GetMetaList(ctx, newID)
			if err != nil {
				return mapFluigError(err)
			}
			printMetaList(p, m)
			p.Successf("Lista %d criada", m.ID)
			p.Done(map[string]any{"list": m})
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "descrição da lista (até 254 caracteres)")
	cmd.Flags().StringArrayVar(&columns, "column", nil, "coluna TÍTULO:TIPO ou TÍTULO:metalista:LISTA (pode repetir)")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "título de uma coluna que é rótulo (pode repetir)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// newMetaListID devolve o id da lista com o nome dado que aparece em after e
// não em before. Devolve 0 se não houver exatamente uma.
func newMetaListID(before, after []fluig.MetaListSummary, name string) int {
	old := make(map[int]bool, len(before))
	for _, l := range before {
		old[l.ID] = true
	}
	found := 0
	for _, l := range after {
		if !old[l.ID] && l.Name == name {
			if found != 0 {
				return 0
			}
			found = l.ID
		}
	}
	return found
}

// --- metalist update ---

// findMetaListColumnAny acha a coluna pelo nome ou pelo título, inclusive a
// desativada.
func findMetaListColumnAny(m *fluig.MetaList, ref string) (fluig.MetaListColumn, error) {
	for _, c := range m.Columns {
		if c.Name != "" && strings.EqualFold(c.Name, ref) {
			return c, nil
		}
	}
	for _, c := range m.Columns {
		if strings.EqualFold(c.Title, ref) {
			return c, nil
		}
	}
	titles := make([]string, 0, len(m.Columns))
	for _, c := range m.Columns {
		titles = append(titles, c.Title)
	}
	return fluig.MetaListColumn{}, output.Usagef("a lista %q não tem a coluna %q. Colunas: %s", m.Name, ref, strings.Join(titles, ", "))
}

// existingColumnNames devolve os nomes de coluna já ocupados, inclusive os
// das colunas desativadas (a coluna continua no banco). A desativada não tem
// nome no servidor: o nome sai do título, pela mesma regra.
func existingColumnNames(m *fluig.MetaList) map[string]string {
	out := map[string]string{}
	for _, c := range m.Columns {
		name := c.Name
		if name == "" {
			n, err := fluig.MetaListColumnName(c.Title)
			if err != nil {
				continue
			}
			name = n
		}
		out[name] = c.Title
	}
	return out
}

func newMetalistUpdateCmd(app *App) *cobra.Command {
	var (
		name          string
		description   string
		addColumns    []string
		labels        []string
		noLabels      []string
		disable       []string
		enable        []string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <id|nome> [flags]",
		Short: "Altera uma lista: nome, descrição e colunas",
		Long: "Altera o nome e a descrição da lista, inclui colunas e muda o rótulo e a\n" +
			"situação (ativa ou desativada) das colunas existentes.\n\n" +
			"O título e o tipo de uma coluna existente não mudam: a API aceita a\n" +
			"mudança, mas ela corrompe a lista. Para trocar uma coluna, desative a\n" +
			"antiga e inclua uma nova.\n\n" +
			"Uma coluna desativada some dos registros, mas os dados ficam no banco.\n" +
			"Use --enable-column para trazê-la de volta. Identifique uma coluna\n" +
			"existente pelo nome ou pelo título (a desativada, só pelo título).\n\n" +
			metaListColumnSpecHelp,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			nameSet, descSet := cmd.Flags().Changed("name"), cmd.Flags().Changed("description")
			if !nameSet && !descSet && len(addColumns)+len(labels)+len(noLabels)+len(disable)+len(enable) == 0 {
				return output.Usagef("nada a alterar: informe --name, --description, --add-column, --label, --no-label, --disable-column ou --enable-column")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar lista")
			if err != nil {
				return err
			}
			// resolveMetaList recusa lista removida: o updateList a reativaria.
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			if m.FieldsUnavailable {
				return output.ServerErrorf("o servidor devolveu a lista %d sem a definição das colunas (fieldsVO vazio). Isso acontece quando uma coluna metalista aponta para uma lista removida. Corrija pela interface", m.ID)
			}

			newName, newDesc := m.Name, m.Description
			if nameSet {
				newName = strings.TrimSpace(name)
			}
			if descSet {
				newDesc = description
			}
			if err := validateMetaListText(newName, newDesc); err != nil {
				return err
			}

			// Mudanças nas colunas existentes, por título.
			changes := map[string]*fluig.MetaListColumnChange{}
			order := []string{}
			change := func(ref string) (*fluig.MetaListColumnChange, error) {
				col, err := findMetaListColumnAny(m, ref)
				if err != nil {
					return nil, err
				}
				key := strconv.Itoa(col.Seq)
				if ch, ok := changes[key]; ok {
					return ch, nil
				}
				ch := &fluig.MetaListColumnChange{Column: col, Label: col.Label, Disabled: col.Disabled}
				changes[key] = ch
				order = append(order, key)
				return ch, nil
			}
			flagCol := map[string]string{}
			mark := func(ref, flag string) error {
				col, err := findMetaListColumnAny(m, ref)
				if err != nil {
					return err
				}
				key := strconv.Itoa(col.Seq)
				if prev, ok := flagCol[key+"|"+flagGroup(flag)]; ok && prev != flag {
					return output.Usagef("a coluna %q recebeu %s e %s", col.Title, prev, flag)
				}
				flagCol[key+"|"+flagGroup(flag)] = flag
				return nil
			}

			// Colunas novas.
			taken := existingColumnNames(m)
			var specs []metaListColumnSpec
			for _, c := range addColumns {
				spec, err := app.parseMetaListColumnSpec(ctx, client, c)
				if err != nil {
					return err
				}
				if prev, dup := taken[spec.name]; dup {
					return output.Usagef("a coluna %q gera o nome %s, que a coluna %q já usa. O servidor ignoraria a coluna nova sem avisar", spec.col.Title, spec.name, prev)
				}
				taken[spec.name] = spec.col.Title
				specs = append(specs, spec)
			}
			// --label vale para coluna nova ou existente.
			rest, err := applyLabels(specs, labels)
			if err != nil {
				return err
			}
			for _, l := range rest {
				if err := mark(l, "--label"); err != nil {
					return err
				}
				ch, err := change(l)
				if err != nil {
					return err
				}
				if ch.Column.Type == fluig.MetaListTypeMetaList {
					return output.Usagef("a coluna %q é do tipo metalista e não pode ser rótulo", l)
				}
				ch.Label = true
			}
			for _, l := range noLabels {
				if err := mark(l, "--no-label"); err != nil {
					return err
				}
				ch, err := change(l)
				if err != nil {
					return err
				}
				ch.Label = false
			}
			for _, l := range disable {
				if err := mark(l, "--disable-column"); err != nil {
					return err
				}
				ch, err := change(l)
				if err != nil {
					return err
				}
				ch.Disabled = true
			}
			for _, l := range enable {
				if err := mark(l, "--enable-column"); err != nil {
					return err
				}
				ch, err := change(l)
				if err != nil {
					return err
				}
				ch.Disabled = false
			}

			renamed := newName != m.Name
			if renamed {
				lists, err := client.ListMetaLists(ctx)
				if err != nil {
					return mapFluigError(err)
				}
				if other := metaListNameTaken(lists, newName, m.ID); other != nil {
					return output.Usagef("já existe a lista %q (id %d). Escolha outro nome", other.Name, other.ID)
				}
			}

			var chList []fluig.MetaListColumnChange
			for _, k := range order {
				ch := changes[k]
				if ch.Label != ch.Column.Label || ch.Disabled != ch.Column.Disabled {
					chList = append(chList, *ch)
				}
			}
			cols := make([]fluig.MetaListNewColumn, 0, len(specs))
			for _, s := range specs {
				cols = append(cols, s.col)
			}

			changed := false
			// Passo 1: colunas, com o nome e a descrição ATUAIS. A API não é
			// atômica: se uma coluna for recusada, o nome novo já estaria
			// gravado.
			if len(chList)+len(cols) > 0 {
				if err := client.UpdateMetaList(ctx, m.ID, m.Name, m.Description, chList, cols, m.MaxSeq()); err != nil {
					return mapFluigError(err)
				}
				changed = true
			}
			// Passo 2: nome e descrição, sem colunas.
			if renamed || newDesc != m.Description {
				if err := client.UpdateMetaList(ctx, m.ID, newName, newDesc, nil, nil, 0); err != nil {
					if changed {
						p.Warnf("As colunas foram alteradas, mas o nome e a descrição não.")
					}
					return mapFluigError(err)
				}
				changed = true
			}

			after, err := client.GetMetaList(ctx, m.ID)
			if err != nil {
				return mapFluigError(err)
			}
			if err := checkAddedColumns(after, specs); err != nil {
				return err
			}
			printMetaList(p, after)
			if changed {
				p.Successf("Lista %d alterada", after.ID)
			} else {
				p.Infof("A lista %d já estava assim. Nada foi gravado.", after.ID)
			}
			p.Done(map[string]any{"list": after, "changed": changed})
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "nome novo da lista")
	cmd.Flags().StringVar(&description, "description", "", "descrição nova (até 254 caracteres; vazio limpa)")
	cmd.Flags().StringArrayVar(&addColumns, "add-column", nil, "coluna nova TÍTULO:TIPO ou TÍTULO:metalista:LISTA (pode repetir)")
	cmd.Flags().StringArrayVar(&labels, "label", nil, "marca a coluna (nova ou existente) como rótulo (pode repetir)")
	cmd.Flags().StringArrayVar(&noLabels, "no-label", nil, "tira o rótulo da coluna (pode repetir)")
	cmd.Flags().StringArrayVar(&disable, "disable-column", nil, "desativa a coluna (pode repetir)")
	cmd.Flags().StringArrayVar(&enable, "enable-column", nil, "reativa a coluna desativada (pode repetir)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// flagGroup agrupa flags opostas: --label/--no-label e
// --disable-column/--enable-column não podem cair na mesma coluna.
func flagGroup(flag string) string {
	switch flag {
	case "--label", "--no-label":
		return "label"
	}
	return "state"
}

// checkAddedColumns confere que cada coluna nova apareceu. O servidor ignora
// em silêncio uma coluna que não consegue criar.
func checkAddedColumns(m *fluig.MetaList, specs []metaListColumnSpec) error {
	have := map[string]bool{}
	for _, c := range m.Columns {
		if c.Name != "" {
			have[c.Name] = true
		}
	}
	for _, s := range specs {
		if !have[s.name] {
			return output.ServerErrorf("o servidor respondeu ok, mas a coluna %q (%s) não apareceu na lista %d", s.col.Title, s.name, m.ID)
		}
	}
	return nil
}

// --- metalist delete ---

func newMetalistDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <id|nome>...",
		Short: "Remove listas customizadas",
		Long: "Remove as listas. O Fluig não apaga os dados: a lista some da interface\n" +
			"e da API.\n\n" +
			"Uma lista que outra lista referencia (coluna metalista) não sai. O\n" +
			"bloqueio continua mesmo depois de remover a lista que a referencia.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if err := app.confirm(fmt.Sprintf("Remover %d lista(s)?", len(args))); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "remover listas")
			if err != nil {
				return err
			}
			var results []itemResult
			var lastErr error
			failures := 0
			for _, arg := range args {
				m, rerr := app.resolveMetaList(ctx, client, arg)
				if rerr == nil {
					rerr = client.RemoveMetaList(ctx, m.ID)
					if rerr != nil {
						rerr = mapFluigError(rerr)
					}
				}
				if rerr != nil {
					var cliErr *output.Error
					if !errors.As(rerr, &cliErr) {
						rerr = mapFluigError(rerr)
					}
					failures++
					lastErr = rerr
					results = append(results, itemResult{ID: arg, Action: "failed", Success: false, Error: output.AsError(rerr).Message})
					warnItem(p, len(args), "lista %s: %s", arg, output.AsError(rerr).Message)
					continue
				}
				results = append(results, itemResult{ID: strconv.Itoa(m.ID), Action: "deleted", Success: true})
				p.Successf("lista %d (%s) removida", m.ID, m.Name)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
