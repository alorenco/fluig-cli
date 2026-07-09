package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// linkSuggestion é a sugestão de vínculo de uma pasta local sem mapeamento.
type linkSuggestion struct {
	Folder string
	Form   fluig.Form // zero quando não há sugestão
	Source string     // origem da sugestão, para o usuário confiar (ou não)
}

// suggestFormLinks propõe um formulário do servidor para cada pasta local sem
// vínculo. Fontes, na ordem: nome já vinculado à pasta em OUTRO servidor
// (mapeamento inicial de um ambiente novo), nome exato da pasta e nome da
// pasta ignorando caixa. Sugestão só quando o match é único e o formulário
// ainda não está vinculado a outra pasta.
func suggestFormLinks(folders []string, forms []fluig.Form, fmap *project.FormMap) []linkSuggestion {
	taken := map[int]bool{}
	for _, f := range forms {
		if l, ok := fmap.ByDocumentID(f.DocumentID); ok && l.DocumentID == f.DocumentID {
			taken[f.DocumentID] = true
		}
	}
	unique := func(match func(fluig.Form) bool) (fluig.Form, bool) {
		var found fluig.Form
		n := 0
		for _, f := range forms {
			if !taken[f.DocumentID] && match(f) {
				found = f
				n++
			}
		}
		return found, n == 1
	}

	var out []linkSuggestion
	for _, folder := range folders {
		s := linkSuggestion{Folder: folder}
		if name, srvKey, ok := fmap.FolderNameHint(folder); ok {
			if f, ok := unique(func(f fluig.Form) bool { return f.Description == name }); ok {
				s.Form, s.Source = f, "vínculo em "+srvKey
			}
		}
		if s.Form.DocumentID == 0 {
			if f, ok := unique(func(f fluig.Form) bool { return f.Description == folder }); ok {
				s.Form, s.Source = f, "nome exato"
			}
		}
		if s.Form.DocumentID == 0 {
			if f, ok := unique(func(f fluig.Form) bool { return strings.EqualFold(f.Description, folder) }); ok {
				s.Form, s.Source = f, "nome (ignorando caixa)"
			}
		}
		if s.Form.DocumentID != 0 {
			taken[s.Form.DocumentID] = true // uma sugestão por formulário
		}
		out = append(out, s)
	}
	return out
}

// localFormFolders lista as pastas de forms/ (ordem alfabética, sem ocultas).
func localFormFolders(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, project.FormsDirName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

func newFormLinkCmd(app *App) *cobra.Command {
	var (
		auto          bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Vincula as pastas locais aos formulários do servidor (mapeamento inicial)",
		Long: "Percorre as pastas de forms/ sem vínculo no servidor ativo e liga cada uma\n" +
			"a um formulário do servidor, gravando em .fluigcli/forms.json (por servidor).\n\n" +
			"Sugestões automáticas: nome já vinculado à pasta em outro servidor (ao\n" +
			"configurar um ambiente novo), nome exato da pasta e nome ignorando caixa.\n" +
			"No modo interativo, Enter aceita a sugestão, um termo busca na lista do\n" +
			"servidor, o número escolhe e \"s\" pula. Com --auto, só as sugestões\n" +
			"inequívocas são gravadas (para scripts e agentes; combina com --json).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if app.JSON && !auto {
				return output.Usagef("o modo interativo não suporta --json; use form link --auto")
			}
			if !auto && !app.Interactive() {
				return output.Usagef("sem TTY, use form link --auto")
			}

			ctx := context.Background()
			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			p.Server = server.Name
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			folders, err := localFormFolders(root)
			if err != nil {
				return output.Genericf("não consegui ler forms/: %v", err)
			}
			if len(folders) == 0 {
				return output.Usagef("nenhuma pasta em forms/ — baixe formulários com form import ou crie a pasta")
			}
			fmap, err := project.LoadFormMap(root, server.FormScopeKey())
			if err != nil {
				return output.Genericf("falha ao ler .fluigcli/forms.json: %v", err)
			}
			var unlinked []string
			for _, f := range folders {
				if _, ok := fmap.ByFolder(f); !ok {
					unlinked = append(unlinked, f)
				}
			}
			if len(unlinked) == 0 {
				p.Infof("Todas as %d pastas de forms/ já têm vínculo em %q.", len(folders), server.Name)
				p.Done(map[string]any{"linked": []any{}, "skipped": []any{}, "alreadyLinked": len(folders)})
				return nil
			}

			userCode, err := client.ResolveUserCode(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			forms, err := client.ListForms(ctx, userCode)
			if err != nil {
				return mapFluigError(err)
			}
			suggestions := suggestFormLinks(unlinked, forms, fmap)

			linked, skipped := []string{}, []string{}
			link := func(folder string, f fluig.Form) {
				fmap.Upsert(project.FormLink{Folder: folder, DocumentID: f.DocumentID,
					Name: f.Description, DatasetName: f.DatasetName})
				linked = append(linked, folder)
			}

			if auto {
				for _, s := range suggestions {
					if s.Form.DocumentID != 0 {
						link(s.Folder, s.Form)
						p.Infof("✓ %s → %q (documentId %d, %s)", s.Folder, s.Form.Description, s.Form.DocumentID, s.Source)
					} else {
						skipped = append(skipped, s.Folder)
					}
				}
			} else {
				p.Infof("%d pasta(s) sem vínculo em %q. Enter aceita a sugestão; termo busca; s pula.", len(unlinked), server.Name)
				if err := runFormLinkPrompt(p, suggestions, forms, fmap, link, &skipped); err != nil {
					return err
				}
			}

			if len(linked) > 0 {
				if err := fmap.Save(); err != nil {
					return output.Genericf("não consegui salvar .fluigcli/forms.json: %v", err)
				}
			}
			p.Successf("%d vinculado(s), %d sem vínculo.", len(linked), len(skipped))
			for _, f := range skipped {
				p.Infof("  pulado: %s (aponte com form export --name/--document-id, ou rode o link de novo)", f)
			}
			p.Done(map[string]any{"linked": linked, "skipped": skipped})
			return nil
		},
	}
	cmd.Flags().BoolVar(&auto, "auto", false, "grava só as sugestões inequívocas, sem prompt (para scripts/agentes)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// runFormLinkPrompt é o loop interativo do form link: uma pergunta por pasta.
func runFormLinkPrompt(p *output.Printer, suggestions []linkSuggestion, forms []fluig.Form,
	fmap *project.FormMap, link func(string, fluig.Form), skipped *[]string) error {
	for _, s := range suggestions {
		var shown []fluig.Form // última lista numerada exibida (após busca)
		label := fmt.Sprintf("%s → (sem sugestão) · termo busca · s pula", s.Folder)
		if s.Form.DocumentID != 0 {
			label = fmt.Sprintf("%s → %q (%s) · Enter vincula · termo busca · s pula", s.Folder, s.Form.Description, s.Source)
		}
		for {
			in, err := promptLine(label, "")
			if err != nil {
				return err
			}
			in = strings.TrimSpace(in)
			switch {
			case in == "" && s.Form.DocumentID != 0:
				link(s.Folder, s.Form)
			case in == "" || strings.EqualFold(in, "s"):
				*skipped = append(*skipped, s.Folder)
			case isNumber(in):
				n, _ := strconv.Atoi(in)
				if len(shown) == 0 || n < 1 || n > len(shown) {
					p.Warnf("número fora da lista — busque um termo primeiro")
					continue
				}
				link(s.Folder, shown[n-1])
			default:
				shown = filterForms(forms, fmap, in)
				if len(shown) == 0 {
					p.Warnf("nenhum formulário do servidor casa com %q", in)
					continue
				}
				for i, f := range shown {
					p.Infof("  %2d. %s (documentId %d)", i+1, f.Description, f.DocumentID)
				}
				continue
			}
			break
		}
	}
	return nil
}

// filterForms devolve os formulários (ainda sem vínculo) cujo nome contém o
// termo, ignorando caixa.
func filterForms(forms []fluig.Form, fmap *project.FormMap, term string) []fluig.Form {
	term = strings.ToLower(term)
	var out []fluig.Form
	for _, f := range forms {
		if _, ok := fmap.ByDocumentID(f.DocumentID); ok {
			continue
		}
		if strings.Contains(strings.ToLower(f.Description), term) {
			out = append(out, f)
		}
	}
	return out
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// hintFormLink sugere o form link quando o projeto tem pastas de formulário e
// o servidor recém-configurado/testado ainda não tem nenhum vínculo. Falhas
// são silenciosas — é só uma dica.
func hintFormLink(a *App, p *output.Printer, server *config.Server) {
	root := a.ProjectRoot()
	if root == "" {
		return
	}
	folders, err := localFormFolders(root)
	if err != nil || len(folders) == 0 {
		return
	}
	fmap, err := project.LoadFormMap(root, server.FormScopeKey())
	if err != nil {
		return
	}
	for _, f := range folders {
		if _, ok := fmap.ByFolder(f); ok {
			return // já há vínculo neste servidor
		}
	}
	p.Infof("Dica: o projeto tem %d pasta(s) em forms/ sem vínculo com %q — rode: fluigcli form link --server %s", len(folders), server.Name, server.Name)
}
