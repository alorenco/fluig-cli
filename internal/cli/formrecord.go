package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// newFormRecordsCmd agrupa o CRUD de REGISTROS (dados) de formulário — o
// layout fica com form import/export.
func newFormRecordsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "records",
		Short: "Registros (dados) de um formulário: consulta e CRUD",
	}
	cmd.AddCommand(newFormRecordsListCmd(app))
	cmd.AddCommand(newFormRecordsShowCmd(app))
	cmd.AddCommand(newFormRecordsCreateCmd(app))
	cmd.AddCommand(newFormRecordsUpdateCmd(app))
	cmd.AddCommand(newFormRecordsDeleteCmd(app))
	return cmd
}

// resolveFormID resolve <documentId|nome>: numérico vai direto; nome consulta
// a listagem de formulários do servidor.
func (a *App) resolveFormID(ctx context.Context, client *fluig.Client, arg string) (int, error) {
	if id, err := strconv.Atoi(arg); err == nil && id > 0 {
		return id, nil
	}
	userCode, err := client.ResolveUserCode(ctx)
	if err != nil {
		return 0, mapFluigError(err)
	}
	forms, err := client.ListForms(ctx, userCode)
	if err != nil {
		return 0, mapFluigError(err)
	}
	if f, ok := matchForm(forms, arg); ok {
		return f.DocumentID, nil
	}
	return 0, output.NotFoundf("formulário %q não encontrado no servidor (use form list)", arg)
}

// sortedFieldKeys devolve os campos de um registro em ordem estável.
func sortedFieldKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- form records list ---

func newFormRecordsListCmd(app *App) *cobra.Command {
	var (
		filter        string
		fields        []string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "list <documentId|nome>",
		Short: "Lista os registros de um formulário",
		Long: "Lista os registros (cards) de um formulário. Sem --fields, a tabela\n" +
			"mostra só card/versão — indique as colunas com --fields campo1,campo2\n" +
			"(o --json sempre traz todos os valores).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			formID, err := app.resolveFormID(ctx, client, args[0])
			if err != nil {
				return err
			}
			records, err := client.ListFormRecords(ctx, formID, filter, limit)
			if err != nil {
				return mapFluigError(err)
			}
			if len(records) == 0 {
				p.Infof("O formulário %d não tem registros (com esse filtro).", formID)
				p.Done(map[string]any{"formId": formID, "records": records})
				return nil
			}

			cols := splitCSV(fields)
			headers := append([]string{"Card", "Versão"}, cols...)
			rows := make([][]string, 0, len(records))
			for _, r := range records {
				row := []string{strconv.FormatInt(r.CardID, 10), strconv.Itoa(r.Version)}
				for _, c := range cols {
					row = append(row, r.Values[c])
				}
				rows = append(rows, row)
			}
			p.Table(output.Table{Headers: headers, Rows: rows, Style: output.BoldHeaderStyle(nil)})
			if len(cols) == 0 {
				p.Infof("Dica: escolha colunas com --fields campo1,campo2 (ou use records show <card> para o registro completo).")
			}
			p.Done(map[string]any{"formId": formID, "records": records})
			return nil
		},
	}
	cmd.Flags().StringVar(&filter, "filter", "", "expressão $filter da API (repassada crua)")
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "campos a mostrar como colunas da tabela")
	cmd.Flags().IntVar(&limit, "limit", 50, "número máximo de registros (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- form records show ---

func newFormRecordsShowCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "show <documentId|nome> <cardId>",
		Short: "Mostra um registro completo (com as linhas filhas, se houver)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			cardID, err := strconv.Atoi(args[1])
			if err != nil || cardID <= 0 {
				return output.Usagef("cardId inválido %q", args[1])
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			formID, err := app.resolveFormID(ctx, client, args[0])
			if err != nil {
				return err
			}
			rec, err := client.GetFormRecord(ctx, formID, cardID)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("Registro %d do formulário %d (versão %d)", rec.CardID, formID, rec.Version)
			for _, k := range sortedFieldKeys(rec.Values) {
				p.Successf("  %s = %s", k, rec.Values[k])
			}
			for i, child := range rec.Children {
				p.Successf("  filho %d:", i+1)
				for _, k := range sortedFieldKeys(child) {
					p.Successf("    %s = %s", k, child[k])
				}
			}
			p.Done(map[string]any{"formId": formID, "record": rec})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- form records create / update ---

func newFormRecordsCreateCmd(app *App) *cobra.Command {
	var (
		fields        []string
		fieldsFile    string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "create <documentId|nome>",
		Short: "Cria um registro no formulário",
		Long: "Cria um registro com os valores dados (--field campo=valor ou\n" +
			"--fields-file JSON plano; \"-\" lê do stdin). Os eventos do formulário\n" +
			"NÃO rodam neste caminho — os dados entram como enviados.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			values, err := loadFormFields(fieldsFile, fields)
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return output.Usagef("informe os valores com --field campo=valor ou --fields-file")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "criar registro de formulário")
			if err != nil {
				return err
			}
			formID, err := app.resolveFormID(ctx, client, args[0])
			if err != nil {
				return err
			}
			rec, err := client.CreateFormRecord(ctx, formID, values)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("registro %d criado no formulário %d", rec.CardID, formID)
			p.Done(map[string]any{"formId": formID, "record": rec})
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "valor do campo: campo=valor (pode repetir; sobrepõe o --fields-file)")
	cmd.Flags().StringVar(&fieldsFile, "fields-file", "", `valores em JSON plano {"campo":"valor"} (arquivo ou "-" para stdin)`)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

func newFormRecordsUpdateCmd(app *App) *cobra.Command {
	var (
		fields        []string
		fieldsFile    string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "update <documentId|nome> <cardId>",
		Short: "Atualiza um registro do formulário",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			cardID, err := strconv.Atoi(args[1])
			if err != nil || cardID <= 0 {
				return output.Usagef("cardId inválido %q", args[1])
			}
			values, err := loadFormFields(fieldsFile, fields)
			if err != nil {
				return err
			}
			if len(values) == 0 {
				return output.Usagef("informe os valores com --field campo=valor ou --fields-file")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "atualizar registro de formulário")
			if err != nil {
				return err
			}
			formID, err := app.resolveFormID(ctx, client, args[0])
			if err != nil {
				return err
			}
			rec, err := client.UpdateFormRecord(ctx, formID, cardID, values)
			if err != nil {
				return mapFluigError(err)
			}
			p.Successf("registro %d atualizado (formulário %d, versão %d)", cardID, formID, rec.Version)
			p.Done(map[string]any{"formId": formID, "record": rec})
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "valor do campo: campo=valor (pode repetir; sobrepõe o --fields-file)")
	cmd.Flags().StringVar(&fieldsFile, "fields-file", "", `valores em JSON plano {"campo":"valor"} (arquivo ou "-" para stdin)`)
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- form records delete ---

func newFormRecordsDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <documentId|nome> <cardId>...",
		Short: "Exclui registros do formulário",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if err := app.confirm(fmt.Sprintf("Excluir %d registro(s) do formulário %q?", len(args)-1, args[0])); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir registros de formulário")
			if err != nil {
				return err
			}
			formID, err := app.resolveFormID(ctx, client, args[0])
			if err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, arg := range args[1:] {
				cardID, aerr := strconv.Atoi(arg)
				if aerr != nil || cardID <= 0 {
					return output.Usagef("cardId inválido %q", arg)
				}
				if derr := client.DeleteFormRecord(ctx, formID, cardID); derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: arg, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("registro %s: %s", arg, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: arg, Action: "deleted", Success: true})
				p.Successf("registro %s excluído", arg)
			}
			return finishBatch(p, lastErr, map[string]any{"formId": formID, "results": results}, failures, len(args)-1)
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
