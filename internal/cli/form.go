package cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/fluig/soap"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newFormCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "form",
		Short: "Lista, importa e exporta formulários (import = servidor→local; export = local→servidor)",
	}
	cmd.AddCommand(newFormListCmd(app))
	cmd.AddCommand(newFormImportCmd(app))
	cmd.AddCommand(newFormExportCmd(app))
	cmd.AddCommand(newFormLinkCmd(app))
	return cmd
}

// --- form list ---

func newFormListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os formulários do servidor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			userCode, err := client.ResolveUserCode(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			forms, err := client.ListForms(ctx, userCode)
			if err != nil {
				return mapFluigError(err)
			}
			rows := make([][]string, 0, len(forms))
			for _, f := range forms {
				rows = append(rows, []string{
					strconv.Itoa(f.DocumentID), f.Description, f.DatasetName, strconv.Itoa(f.Version),
				})
			}
			if len(forms) == 0 {
				p.Infof("Nenhum formulário no servidor.")
			} else {
				// Padrão de listagem (ver CLAUDE.md).
				p.Table(output.Table{
					Headers: []string{"ID", "Nome", "Dataset", "Versão"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"forms": forms})
			return nil
		},
	}
}

// --- form import (servidor → local) ---

func newFormImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		folderFlag    string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <documentId|nome>... | --all",
		Short: "Baixa formulários do servidor para pastas locais (servidor → local)",
		Long: "Baixa formulários do servidor para pastas locais.\n\n" +
			"O vínculo pasta local ↔ formulário do servidor é gravado em\n" +
			".fluigcli/forms.json, então exports futuros reencontram o formulário\n" +
			"mesmo que a pasta tenha nome diferente do nome no servidor.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais formulários (documentId ou nome) ou use --all")
			}
			if folderFlag != "" && (all || len(args) != 1) {
				return output.Usagef("--folder só pode ser usado ao importar um único formulário")
			}
			ctx := context.Background()
			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			userCode, err := client.ResolveUserCode(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			fmap, err := project.LoadFormMap(root, server.FormScopeKey())
			if err != nil {
				return output.Genericf("falha ao ler .fluigcli/forms.json: %v", err)
			}

			forms, err := client.ListForms(ctx, userCode)
			if err != nil {
				return mapFluigError(err)
			}

			var targets []fluig.Form
			if all {
				targets = forms
			} else {
				for _, arg := range args {
					f, ok := matchForm(forms, arg)
					if !ok {
						p.Warnf("formulário %q não encontrado", arg)
						continue
					}
					targets = append(targets, f)
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, f := range targets {
				localFolder := resolveImportFolder(fmap, f, folderFlag)
				if err := app.importOneForm(ctx, client, userCode, root, localFolder, f); err != nil {
					failures++
					lastErr = mapFluigError(err)
					results = append(results, itemResult{ID: f.Description, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("formulário %q: %s", f.Description, output.AsError(lastErr).Message)
					continue
				}
				fmap.Upsert(project.FormLink{Folder: localFolder, DocumentID: f.DocumentID, Name: f.Description, DatasetName: f.DatasetName})
				results = append(results, itemResult{ID: f.Description, Action: "imported", Success: true})
				p.Successf("formulário %q importado em forms/%s", f.Description, localFolder)
			}
			if err := fmap.Save(); err != nil {
				p.Warnf("não foi possível salvar .fluigcli/forms.json: %v", err)
			}
			total := len(targets)
			if !all && len(args) > total {
				failures += len(args) - total
				total = len(args)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, total)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os formulários do servidor")
	cmd.Flags().StringVar(&folderFlag, "folder", "", "nome da pasta local (bootstrap do mapeamento; só com um formulário)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// resolveImportFolder decide a pasta local: --folder > mapeamento (por id/nome)
// > documentDescription.
func resolveImportFolder(fmap *project.FormMap, f fluig.Form, folderFlag string) string {
	if folderFlag != "" {
		return folderFlag
	}
	if l, ok := fmap.ByDocumentID(f.DocumentID); ok {
		return l.Folder
	}
	if l, ok := fmap.ByName(f.Description); ok {
		return l.Folder
	}
	return f.Description
}

// matchForm casa um argumento (documentId numérico ou nome) com um formulário.
func matchForm(forms []fluig.Form, arg string) (fluig.Form, bool) {
	if id, err := strconv.Atoi(arg); err == nil {
		for _, f := range forms {
			if f.DocumentID == id {
				return f, true
			}
		}
	}
	for _, f := range forms {
		if f.Description == arg {
			return f, true
		}
	}
	return fluig.Form{}, false
}

// importOneForm baixa anexos e eventos para forms/<localFolder>/{,events/}.
func (a *App) importOneForm(ctx context.Context, client *fluig.Client, userCode, root, localFolder string, f fluig.Form) error {
	// A pasta pode vir do documentDescription do servidor — confina em forms/.
	formDir, err := project.SafeJoin(filepath.Join(root, project.FormsDirName), localFolder)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(formDir, 0o755); err != nil {
		return err
	}

	names, err := client.FormAttachments(ctx, f.DocumentID)
	if err != nil {
		return err
	}
	for _, name := range names {
		// Nome do anexo vem do servidor — rejeita traversal (zip-slip/../).
		dst, err := project.SafeJoin(formDir, name)
		if err != nil {
			return err
		}
		file, err := client.DownloadFormFile(ctx, f.DocumentID, userCode, f.Version, name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, file.Content, 0o644); err != nil {
			return err
		}
	}

	events, err := client.FormEvents(ctx, f.DocumentID)
	if err != nil {
		return err
	}
	if len(events) > 0 {
		eventsDir := project.FormEventsDir(formDir)
		if err := os.MkdirAll(eventsDir, 0o755); err != nil {
			return err
		}
		for _, e := range events {
			dst, err := project.SafeJoin(eventsDir, e.ID+".js") // id do evento vem do servidor
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, []byte(e.Code), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- form export (local → servidor) ---

func newFormExportCmd(app *App) *cobra.Command {
	var (
		markNew         bool
		nameFlag        string
		documentID      int
		parentID        int
		datasetName     string
		cardDescription string
		persistenceType string
		versionMode     string
		passwordStdin   bool
	)
	cmd := &cobra.Command{
		Use:   "export <pasta>",
		Short: "Envia um formulário local para o servidor (local → servidor)",
		Long: "Envia uma pasta de formulário para o servidor.\n\n" +
			"O formulário-alvo é resolvido por: --document-id > --name > mapeamento\n" +
			"(.fluigcli/forms.json) > nome da pasta. Após o envio, o vínculo é gravado\n" +
			"no mapeamento para os próximos exports.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)

			folder := args[0]
			info, err := os.Stat(folder)
			if err != nil || !info.IsDir() {
				return output.NotFoundf("pasta de formulário %q não encontrada", folder)
			}
			folderKey := filepath.Base(filepath.Clean(folder))

			upload, err := readFormUpload(folder)
			if err != nil {
				return err
			}
			if len(upload.Files) == 0 {
				return output.Usagef("a pasta %q não tem arquivos para enviar", folder)
			}

			persist, err := parsePersistence(persistenceType)
			if err != nil {
				return err
			}
			versionOption, err := parseVersionMode(versionMode)
			if err != nil {
				return err
			}

			ctx := context.Background()
			server, client, err := app.connectWrite(ctx, passwordStdin, "publicar o formulário")
			if err != nil {
				return err
			}
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			pub, err := client.ResolveUserCode(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			fmap, err := project.LoadFormMap(root, server.FormScopeKey())
			if err != nil {
				return output.Genericf("falha ao ler .fluigcli/forms.json: %v", err)
			}

			forms, err := client.ListForms(ctx, pub)
			if err != nil {
				return mapFluigError(err)
			}
			existing, found := resolveExportTarget(forms, fmap, folderKey, nameFlag, documentID)

			// Nome do formulário no servidor (para escolher o principal e a criação).
			formName := folderKey
			if found {
				formName = existing.Description
			} else if nameFlag != "" {
				formName = nameFlag
			}
			names := make([]string, 0, len(upload.Files))
			for _, ff := range upload.Files {
				names = append(names, ff.Name)
			}
			upload.PrincipalFile = fluig.ChoosePrincipalFile(names, folderKey, formName)
			if upload.PrincipalFile == "" {
				p.Warnf("nenhum .html/.htm na pasta — o formulário será enviado sem arquivo principal")
			}

			if found && !markNew {
				ds := datasetName
				if ds == "" {
					ds = existing.DatasetName
				}
				res, err := client.UpdateForm(ctx, pub, existing.DocumentID, existing.CardDescription, existing.Description, ds, versionOption, upload)
				if err != nil {
					return mapFluigError(err)
				}
				docID := documentIDOf(res, existing.DocumentID)
				fmap.Upsert(project.FormLink{Folder: folderKey, DocumentID: docID, Name: existing.Description, DatasetName: ds})
				saveFormMap(p, fmap)
				p.Successf("formulário %q atualizado (documentId %d)", existing.Description, docID)
				p.Done(map[string]any{"action": "updated", "documentId": docID, "name": existing.Description})
				return nil
			}

			// Criação.
			if !app.confirmCreate(formName, markNew) {
				return output.Usagef("formulário %q não existe no servidor; use --new para criá-lo, --name/--document-id para apontar um existente, ou rode form link para vincular as pastas locais", folderKey)
			}
			if parentID == 0 {
				return output.Usagef("--parent-id é obrigatório para criar um formulário (pasta do GED onde ele será salvo)")
			}
			if datasetName == "" {
				return output.Usagef("--dataset-name é obrigatório para criar um formulário")
			}
			card := cardDescription
			if card == "" {
				card = formName
			}
			res, err := client.CreateForm(ctx, pub, formName, card, datasetName, parentID, persist, upload)
			if err != nil {
				return mapFluigError(err)
			}
			docID := documentIDOf(res, 0)
			if docID != 0 {
				fmap.Upsert(project.FormLink{Folder: folderKey, DocumentID: docID, Name: formName, DatasetName: datasetName})
				saveFormMap(p, fmap)
			}
			p.Successf("formulário %q criado (documentId %d)", formName, docID)
			p.Done(map[string]any{"action": "created", "documentId": docID, "name": formName})
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&markNew, "new", false, "cria o formulário se ele ainda não existe no servidor")
	f.StringVar(&nameFlag, "name", "", "nome do formulário no servidor (aponta o alvo / define o nome na criação)")
	f.IntVar(&documentID, "document-id", 0, "documentId do formulário-alvo no servidor")
	f.IntVar(&parentID, "parent-id", 0, "id da pasta do GED onde criar o formulário (obrigatório na criação)")
	f.StringVar(&datasetName, "dataset-name", "", "nome do dataset do formulário (obrigatório na criação)")
	f.StringVar(&cardDescription, "card-description", "", "campo descritor do card (default: o nome do formulário)")
	f.StringVar(&persistenceType, "persistence-type", "db", "persistência na criação: db (tabelas por form) ou single (tabela única)")
	f.StringVar(&versionMode, "version", "new", "no update: keep (mantém a versão) ou new (cria nova)")
	f.BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// resolveExportTarget resolve o formulário-alvo: --document-id > --name >
// mapeamento (pela pasta) > nome da pasta.
func resolveExportTarget(forms []fluig.Form, fmap *project.FormMap, folderKey, nameFlag string, documentID int) (fluig.Form, bool) {
	if documentID != 0 {
		return matchForm(forms, strconv.Itoa(documentID))
	}
	if nameFlag != "" {
		return matchForm(forms, nameFlag)
	}
	if l, ok := fmap.ByFolder(folderKey); ok {
		if f, found := matchForm(forms, strconv.Itoa(l.DocumentID)); found {
			return f, true
		}
	}
	return matchForm(forms, folderKey)
}

func saveFormMap(p *output.Printer, fmap *project.FormMap) {
	if err := fmap.Save(); err != nil {
		p.Warnf("não foi possível salvar .fluigcli/forms.json: %v", err)
	}
}

func documentIDOf(res *soap.WriteResult, fallback int) int {
	if res != nil && res.DocumentID != 0 {
		return res.DocumentID
	}
	return fallback
}

func readFormUpload(folder string) (fluig.FormUpload, error) {
	fc, err := project.ReadFormFolder(folder)
	if err != nil {
		return fluig.FormUpload{}, err
	}
	var up fluig.FormUpload
	for _, path := range fc.Files {
		content, err := os.ReadFile(path)
		if err != nil {
			return fluig.FormUpload{}, err
		}
		up.Files = append(up.Files, fluig.FormFile{Name: filepath.Base(path), Content: content})
	}
	for _, path := range fc.EventFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			return fluig.FormUpload{}, err
		}
		id := project.ArtifactName(path)
		up.Events = append(up.Events, fluig.FormEvent{ID: id, Code: string(content)})
	}
	return up, nil
}

func parsePersistence(mode string) (int, error) {
	switch mode {
	case "db", "":
		return fluig.PersistenceDB, nil
	case "single":
		return fluig.PersistenceSingle, nil
	default:
		return 0, output.Usagef("--persistence-type inválido: %q (use db ou single)", mode)
	}
}

func parseVersionMode(mode string) (string, error) {
	switch mode {
	case "new", "":
		return fluig.VersionNew, nil
	case "keep":
		return fluig.VersionKeep, nil
	default:
		return "", output.Usagef("--version inválido: %q (use keep ou new)", mode)
	}
}
