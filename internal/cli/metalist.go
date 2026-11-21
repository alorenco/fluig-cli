package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// newMetalistCmd agrupa as listas customizadas do Painel de Controle
// (metalist). ROADMAP §5.23. A API é a legada do portal; os quirks medidos
// estão em internal/fluig/metalist.go.
func newMetalistCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metalist",
		Short: "Listas customizadas (metalist): consulta e registros",
		Long: "Consulta as listas customizadas do Painel de Controle (Listas) e\n" +
			"mantém os registros delas.\n\n" +
			"Uma lista é identificada pelo id ou pelo nome. Use metalist list para\n" +
			"ver os dois.",
	}
	cmd.AddCommand(newMetalistListCmd(app))
	cmd.AddCommand(newMetalistShowCmd(app))
	cmd.AddCommand(newMetalistRecordsCmd(app))
	return cmd
}

// resolveMetaList resolve <id|nome>. Um número é o id. Um nome é procurado
// na listagem: primeiro igual, depois sem diferenciar maiúsculas.
func (a *App) resolveMetaList(ctx context.Context, client *fluig.Client, arg string) (*fluig.MetaList, error) {
	if id, err := strconv.Atoi(arg); err == nil && id > 0 {
		m, err := client.GetMetaList(ctx, id)
		if err != nil {
			return nil, mapMetaListNotFound(err, arg)
		}
		return m, nil
	}
	lists, err := client.ListMetaLists(ctx)
	if err != nil {
		return nil, mapFluigError(err)
	}
	var matches []fluig.MetaListSummary
	for _, l := range lists {
		if l.Name == arg {
			matches = []fluig.MetaListSummary{l}
			break
		}
		if strings.EqualFold(l.Name, arg) {
			matches = append(matches, l)
		}
	}
	switch len(matches) {
	case 0:
		return nil, output.NotFoundf("lista %q não encontrada no servidor (use metalist list)", arg)
	case 1:
		m, err := client.GetMetaList(ctx, matches[0].ID)
		if err != nil {
			return nil, mapMetaListNotFound(err, arg)
		}
		return m, nil
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, strconv.Itoa(m.ID))
	}
	return nil, output.Usagef("o nome %q corresponde a mais de uma lista (ids %s). Informe o id", arg, strings.Join(ids, ", "))
}

func mapMetaListNotFound(err error, arg string) error {
	mapped := mapFluigError(err)
	if e := output.AsError(mapped); e != nil && e.Exit == output.ExitNotFound {
		return output.NotFoundf("lista %q não encontrada no servidor (use metalist list)", arg).WithCause(err)
	}
	return mapped
}

// --- metalist list ---

func newMetalistListCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista as listas customizadas",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			lists, err := client.ListMetaLists(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			if len(lists) == 0 {
				p.Infof("O servidor não tem listas customizadas. Crie uma em Painel de Controle > Listas.")
				p.Done(map[string]any{"lists": lists})
				return nil
			}
			rows := make([][]string, 0, len(lists))
			for _, l := range lists {
				rows = append(rows, []string{strconv.Itoa(l.ID), l.Name})
			}
			p.Table(output.Table{Headers: []string{"ID", "Nome"}, Rows: rows, Style: output.BoldHeaderStyle(nil)})
			p.Done(map[string]any{"lists": lists})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- metalist show ---

func newMetalistShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <id|nome>",
		Short: "Mostra as colunas de uma lista",
		Long: "Mostra a lista e as colunas dela.\n\n" +
			"A coluna \"Coluna\" é o nome que os registros usam. Use esse nome em\n" +
			"--field e em --where. Uma coluna desativada não tem nome e não aceita\n" +
			"valores.\n\n" +
			"Tipos: texto, area-texto, inteiro, data (dd/mm/aaaa), numerico e\n" +
			"metalista (referência a um registro de outra lista).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			p.Successf("Lista %d: %s", m.ID, m.Name)
			if m.Description != "" {
				p.Successf("Descrição: %s", m.Description)
			}
			if len(m.Columns) == 0 {
				p.Infof("A lista não tem colunas.")
				p.Done(map[string]any{"list": m})
				return nil
			}
			rows := make([][]string, 0, len(m.Columns))
			for _, c := range m.Columns {
				ref := ""
				if c.Type == fluig.MetaListTypeMetaList {
					ref = fmt.Sprintf("%s (%d)", c.RefListName, c.RefListID)
				}
				name := c.Name
				if name == "" {
					name = "-"
				}
				rows = append(rows, []string{name, c.Title, c.TypeName, simNao(c.Label), ref, simNao(c.Disabled)})
			}
			p.Table(output.Table{
				Headers: []string{"Coluna", "Título", "Tipo", "Rótulo", "Referência", "Desativada"},
				Rows:    rows,
				Style:   output.BoldHeaderStyle(nil),
			})
			p.Done(map[string]any{"list": m})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- metalist records ---

func newMetalistRecordsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "records",
		Short: "Registros de uma lista: consulta e CRUD",
	}
	cmd.AddCommand(newMetalistRecordsListCmd(app))
	cmd.AddCommand(newMetalistRecordsShowCmd(app))
	cmd.AddCommand(newMetalistRecordsCreateCmd(app))
	cmd.AddCommand(newMetalistRecordsUpdateCmd(app))
	cmd.AddCommand(newMetalistRecordsDeleteCmd(app))
	return cmd
}

// metaListValueString formata um valor de registro para a tabela e o --where.
func metaListValueString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// parseWhere lê --where COLUNA=VALOR. A coluna é resolvida como no --field.
func parseWhere(m *fluig.MetaList, raw []string) (map[string]string, error) {
	out := map[string]string{}
	for _, w := range raw {
		k, v, ok := strings.Cut(w, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, output.Usagef("--where inválido %q: use COLUNA=VALOR", w)
		}
		col, err := findMetaListColumn(m, strings.TrimSpace(k))
		if err != nil {
			return nil, err
		}
		out[col.Name] = v
	}
	return out, nil
}

func newMetalistRecordsListCmd(app *App) *cobra.Command {
	var (
		where         []string
		columns       []string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list <id|nome>",
		Short: "Lista os registros de uma lista",
		Long: "Lista os registros de uma lista. O servidor devolve todas as linhas\n" +
			"de uma vez.\n\n" +
			"Uma coluna do tipo metalista mostra o rótulo do registro referenciado.\n" +
			"Para ver o id da referência, use metalist records show.\n\n" +
			"Use --where COLUNA=VALOR para filtrar. O filtro compara o valor inteiro,\n" +
			"sem diferenciar maiúsculas. Repita a flag para combinar filtros.\n" +
			"Use --columns para escolher as colunas da tabela. O --json traz todas.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			filters, err := parseWhere(m, where)
			if err != nil {
				return err
			}
			cols := m.ActiveColumns()
			if sel := splitCSV(columns); len(sel) > 0 {
				cols = cols[:0:0]
				for _, name := range sel {
					col, err := findMetaListColumn(m, name)
					if err != nil {
						return err
					}
					cols = append(cols, col)
				}
			}

			all, err := client.ListMetaListRecords(ctx, m.ID)
			if err != nil {
				return mapFluigError(err)
			}
			records := make([]fluig.MetaListRecord, 0, len(all))
			for _, r := range all {
				if matchesWhere(r, filters) {
					records = append(records, r)
				}
			}
			data := map[string]any{"listId": m.ID, "listName": m.Name, "records": records}
			if len(records) == 0 {
				if len(filters) > 0 {
					p.Infof("Nenhum registro da lista %q atende ao filtro.", m.Name)
				} else {
					p.Infof("A lista %q não tem registros. Inclua um com metalist records create.", m.Name)
				}
				p.Done(data)
				return nil
			}
			headers := []string{"ID"}
			for _, c := range cols {
				headers = append(headers, c.Name)
			}
			rows := make([][]string, 0, len(records))
			for _, r := range records {
				row := []string{strconv.Itoa(r.ID)}
				for _, c := range cols {
					row = append(row, metaListValueString(r.Values[c.Name]))
				}
				rows = append(rows, row)
			}
			p.Table(output.Table{Headers: headers, Rows: rows, Style: output.BoldHeaderStyle(nil)})
			p.Done(data)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&where, "where", nil, "filtro COLUNA=VALOR (valor inteiro, sem diferenciar maiúsculas; pode repetir)")
	cmd.Flags().StringSliceVar(&columns, "columns", nil, "colunas a mostrar na tabela (padrão: todas)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

func matchesWhere(r fluig.MetaListRecord, filters map[string]string) bool {
	for col, want := range filters {
		if !strings.EqualFold(strings.TrimSpace(metaListValueString(r.Values[col])), strings.TrimSpace(want)) {
			return false
		}
	}
	return true
}

// --- metalist records show ---

func newMetalistRecordsShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <id|nome> <recordId>",
		Short: "Mostra um registro da lista",
		Long: "Mostra um registro da lista. Uma coluna do tipo metalista mostra o id\n" +
			"do registro referenciado. Use esse id em --field.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			recordID, err := parseRecordID(args[1])
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			rec, err := client.GetMetaListRecord(ctx, m.ID, recordID)
			if err != nil {
				return mapFluigError(err)
			}
			printMetaListRecord(p, m, rec)
			p.Done(map[string]any{"listId": m.ID, "listName": m.Name, "record": metaListActiveRecord(m, rec)})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// metaListActiveRecord tira do registro as colunas desativadas. O findRecord
// ainda devolve o valor delas, mas a lista não as mostra nem as aceita.
func metaListActiveRecord(m *fluig.MetaList, rec *fluig.MetaListRecord) *fluig.MetaListRecord {
	active := map[string]bool{}
	for _, c := range m.ActiveColumns() {
		active[c.Name] = true
	}
	values := make(map[string]any, len(rec.Values))
	for k, v := range rec.Values {
		if active[k] {
			values[k] = v
		}
	}
	return &fluig.MetaListRecord{ID: rec.ID, Values: values}
}

func printMetaListRecord(p *output.Printer, m *fluig.MetaList, rec *fluig.MetaListRecord) {
	p.Successf("Registro %d da lista %q", rec.ID, m.Name)
	for _, c := range m.ActiveColumns() {
		p.Successf("  %s = %s", c.Name, metaListValueString(rec.Values[c.Name]))
	}
}

func parseRecordID(arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil || id <= 0 {
		return 0, output.Usagef("recordId inválido %q", arg)
	}
	return id, nil
}

// findMetaListColumn acha a coluna pelo nome (columnNameId) ou pelo título,
// sem diferenciar maiúsculas.
func findMetaListColumn(m *fluig.MetaList, name string) (fluig.MetaListColumn, error) {
	for _, c := range m.Columns {
		if c.Name != "" && strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	for _, c := range m.Columns {
		if strings.EqualFold(c.Title, name) {
			if c.Disabled || c.Name == "" {
				return fluig.MetaListColumn{}, output.Usagef("a coluna %q da lista %q está desativada", name, m.Name)
			}
			return c, nil
		}
	}
	names := make([]string, 0, len(m.Columns))
	for _, c := range m.ActiveColumns() {
		names = append(names, c.Name)
	}
	return fluig.MetaListColumn{}, output.Usagef("a lista %q não tem a coluna %q. Colunas: %s", m.Name, name, strings.Join(names, ", "))
}

// buildMetaListValues valida os valores antes da escrita. O servidor aceita
// coluna inexistente, data em outro formato e FK inexistente sem avisar.
func (a *App) buildMetaListValues(ctx context.Context, client *fluig.Client, m *fluig.MetaList, raw map[string]string) ([]fluig.MetaListValue, error) {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	refIDs := map[int]map[int]bool{}
	seen := map[string]string{}
	out := make([]fluig.MetaListValue, 0, len(raw))
	for _, k := range keys {
		col, err := findMetaListColumn(m, k)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[col.Name]; dup {
			return nil, output.Usagef("a coluna %s foi informada duas vezes (%q e %q)", col.Name, prev, k)
		}
		seen[col.Name] = k
		v, err := fluig.NormalizeMetaListValue(col.Type, raw[k])
		if err != nil {
			return nil, output.Usagef("coluna %s (%s): %v", col.Name, col.TypeName, err)
		}
		if col.Type == fluig.MetaListTypeMetaList && v != "" {
			ids, ok := refIDs[col.RefListID]
			if !ok {
				labels, err := client.ListMetaListLabelRecords(ctx, col.RefListID)
				if err != nil {
					return nil, mapFluigError(err)
				}
				ids = map[int]bool{}
				for _, l := range labels {
					ids[l.ID] = true
				}
				refIDs[col.RefListID] = ids
			}
			id, _ := strconv.Atoi(v)
			if !ids[id] {
				return nil, output.Usagef("coluna %s: o registro %d não existe na lista referenciada %q (%d)", col.Name, id, col.RefListName, col.RefListID)
			}
		}
		out = append(out, fluig.MetaListValue{Column: col.Name, Value: v, Type: col.Type})
	}
	return out, nil
}

const metaListFieldsHelp = "Informe os valores com --field COLUNA=VALOR ou --fields-file (JSON plano;\n" +
	"\"-\" lê do stdin). COLUNA é o nome ou o título da coluna (veja metalist show).\n\n" +
	"A CLI valida os valores antes de gravar, porque o servidor aceita dado\n" +
	"inválido sem avisar:\n" +
	"  - inteiro: só dígitos;\n" +
	"  - numerico: aceita 12.5 ou 12,5;\n" +
	"  - data: aceita dd/mm/aaaa ou aaaa-mm-dd;\n" +
	"  - metalista: o id de um registro que existe na lista referenciada.\n" +
	"Um valor vazio limpa a coluna."

// --- metalist records create ---

func newMetalistRecordsCreateCmd(app *App) *cobra.Command {
	var (
		fields        []string
		fieldsFile    string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <id|nome>",
		Short: "Inclui um registro na lista",
		Long:  "Inclui um registro na lista.\n\n" + metaListFieldsHelp,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			raw, err := loadFormFields(fieldsFile, fields)
			if err != nil {
				return err
			}
			if len(raw) == 0 {
				return output.Usagef("informe os valores com --field COLUNA=VALOR ou --fields-file")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "incluir registro de lista")
			if err != nil {
				return err
			}
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			values, err := app.buildMetaListValues(ctx, client, m, raw)
			if err != nil {
				return err
			}
			// O servidor não devolve o id. A CLI compara os ids antes e depois.
			before, err := client.ListMetaListRecords(ctx, m.ID)
			if err != nil {
				return mapFluigError(err)
			}
			if err := client.CreateMetaListRecord(ctx, m.ID, values); err != nil {
				return mapFluigError(err)
			}
			after, err := client.ListMetaListRecords(ctx, m.ID)
			if err != nil {
				return mapFluigError(err)
			}
			newID := newMetaListRecordID(before, after)
			data := map[string]any{"listId": m.ID, "listName": m.Name}
			if newID == 0 {
				p.Warnf("O registro foi incluído, mas a CLI não identificou o id dele. Confira com metalist records list.")
				data["record"] = nil
				p.Done(data)
				return nil
			}
			rec, err := client.GetMetaListRecord(ctx, m.ID, newID)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("Registro %d incluído na lista %q", newID, m.Name)
			data["record"] = metaListActiveRecord(m, rec)
			p.Done(data)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "valor da coluna: COLUNA=VALOR (pode repetir; sobrepõe o --fields-file)")
	cmd.Flags().StringVar(&fieldsFile, "fields-file", "", `valores em JSON plano {"COLUNA":"valor"} (arquivo ou "-" para stdin)`)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// newMetaListRecordID devolve o único id que aparece em after e não em
// before. Devolve 0 se não houver exatamente um.
func newMetaListRecordID(before, after []fluig.MetaListRecord) int {
	old := make(map[int]bool, len(before))
	for _, r := range before {
		old[r.ID] = true
	}
	found := 0
	for _, r := range after {
		if !old[r.ID] {
			if found != 0 {
				return 0
			}
			found = r.ID
		}
	}
	return found
}

// --- metalist records update ---

func newMetalistRecordsUpdateCmd(app *App) *cobra.Command {
	var (
		fields        []string
		fieldsFile    string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <id|nome> <recordId>",
		Short: "Altera um registro da lista",
		Long: "Altera as colunas informadas de um registro. As outras colunas ficam\n" +
			"como estão.\n\n" + metaListFieldsHelp,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			recordID, err := parseRecordID(args[1])
			if err != nil {
				return err
			}
			raw, err := loadFormFields(fieldsFile, fields)
			if err != nil {
				return err
			}
			if len(raw) == 0 {
				return output.Usagef("informe os valores com --field COLUNA=VALOR ou --fields-file")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "alterar registro de lista")
			if err != nil {
				return err
			}
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			values, err := app.buildMetaListValues(ctx, client, m, raw)
			if err != nil {
				return err
			}
			// O servidor responde 200 para id inexistente. A CLI confere antes.
			if _, err := client.GetMetaListRecord(ctx, m.ID, recordID); err != nil {
				return mapFluigError(err)
			}
			if err := client.UpdateMetaListRecord(ctx, m.ID, recordID, values); err != nil {
				return mapFluigError(err)
			}
			rec, err := client.GetMetaListRecord(ctx, m.ID, recordID)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("Registro %d da lista %q alterado", recordID, m.Name)
			p.Done(map[string]any{"listId": m.ID, "listName": m.Name, "record": metaListActiveRecord(m, rec)})
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "valor da coluna: COLUNA=VALOR (pode repetir; sobrepõe o --fields-file)")
	cmd.Flags().StringVar(&fieldsFile, "fields-file", "", `valores em JSON plano {"COLUNA":"valor"} (arquivo ou "-" para stdin)`)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- metalist records delete ---

func newMetalistRecordsDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <id|nome> <recordId>...",
		Short: "Exclui registros da lista",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ids := make([]int, 0, len(args)-1)
			for _, arg := range args[1:] {
				id, err := parseRecordID(arg)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if err := app.confirm(fmt.Sprintf("Excluir %d registro(s) da lista %q?", len(ids), args[0])); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir registros de lista")
			if err != nil {
				return err
			}
			m, err := app.resolveMetaList(ctx, client, args[0])
			if err != nil {
				return err
			}
			// O servidor ignora id inexistente sem avisar. A CLI confere antes
			// e depois, para dar o resultado real de cada id.
			before, err := client.ListMetaListRecords(ctx, m.ID)
			if err != nil {
				return mapFluigError(err)
			}
			existing := map[int]bool{}
			for _, r := range before {
				existing[r.ID] = true
			}
			var results []itemResult
			var lastErr error
			failures := 0
			var toDelete []int
			for _, id := range ids {
				if !existing[id] {
					failures++
					lastErr = output.NotFoundf("registro %d não existe na lista %q", id, m.Name)
					results = append(results, itemResult{ID: strconv.Itoa(id), Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					warnItem(p, len(ids), "registro %d: não existe na lista", id)
					continue
				}
				toDelete = append(toDelete, id)
			}
			if len(toDelete) > 0 {
				if err := client.DeleteMetaListRecords(ctx, m.ID, toDelete); err != nil {
					return mapFluigError(err)
				}
				after, err := client.ListMetaListRecords(ctx, m.ID)
				if err != nil {
					return mapFluigError(err)
				}
				remaining := map[int]bool{}
				for _, r := range after {
					remaining[r.ID] = true
				}
				for _, id := range toDelete {
					if remaining[id] {
						failures++
						lastErr = output.ServerErrorf("o servidor não excluiu o registro %d da lista %q", id, m.Name)
						results = append(results, itemResult{ID: strconv.Itoa(id), Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
						warnItem(p, len(ids), "registro %d: o servidor não excluiu", id)
						continue
					}
					results = append(results, itemResult{ID: strconv.Itoa(id), Action: "deleted", Success: true})
					p.Successf("registro %d excluído", id)
				}
			}
			return finishBatch(p, lastErr, map[string]any{"listId": m.ID, "listName": m.Name, "results": results}, failures, len(ids))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
