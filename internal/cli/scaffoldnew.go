package cli

// Subcomandos `new` dos artefatos além de widgets (dataset, event, mechanism,
// form, workflow new-script) — scaffolds locais que espelham o `widget new`:
// geram o esqueleto no layout convencional do projeto, sem tocar o servidor.
// A geração em si fica em internal/scaffold.

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/scaffold"
)

// scaffoldErr traduz os erros sentinela do scaffold para erro de uso (exit 2).
func scaffoldErr(err error) error {
	if errors.Is(err, scaffold.ErrInvalidCode) || errors.Is(err, scaffold.ErrFileExists) ||
		errors.Is(err, scaffold.ErrDirExists) || errors.Is(err, scaffold.ErrUnknownEvent) {
		return output.Usagef("%s", err)
	}
	return err
}

// singleArtifactSpec parametriza o `new` dos artefatos de arquivo único.
type singleArtifactSpec struct {
	kind    string // rótulo humano nas mensagens
	jsonKey string // chave do nome no envelope --json
	subdir  string // pasta convencional do artefato
	short   string
	long    string
	hint    func(rel string) string // próximo passo após criar
	create  func(path, name string) error
}

// newSingleArtifactNewCmd fabrica um subcomando `new <name>` para dataset,
// evento global e mecanismo (mesmo fluxo, só muda a pasta e o template).
func newSingleArtifactNewCmd(app *App, spec singleArtifactSpec) *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: spec.short,
		Long:  spec.long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			name := args[0]
			if err := scaffold.ValidateArtifactName(name); err != nil {
				return output.Usagef("%s", err)
			}
			// import/export procuram o artefato recursivamente na pasta —
			// um homônimo em subpasta viraria ambiguidade silenciosa.
			existing, _, err := project.FindArtifactFile(root, spec.subdir, name)
			if err != nil {
				return err
			}
			if existing != "" {
				relExisting, _ := filepath.Rel(root, existing)
				return output.Usagef("%s %q já existe em %s", spec.kind, name, filepath.ToSlash(relExisting))
			}
			path, err := project.DefaultArtifactPath(root, spec.subdir, name)
			if err != nil {
				return output.Usagef("%s", err)
			}
			if err := spec.create(path, name); err != nil {
				return scaffoldErr(err)
			}
			rel := filepath.ToSlash(filepath.Join(spec.subdir, name+".js"))
			p.Successf("%s %q criado em %s", spec.kind, name, rel)
			p.Infof("%s", spec.hint(rel))
			p.Done(map[string]any{spec.jsonKey: name, "file": rel})
			return nil
		},
	}
}

func newDatasetNewCmd(app *App) *cobra.Command {
	return newSingleArtifactNewCmd(app, singleArtifactSpec{
		kind:    "dataset",
		jsonKey: "dataset",
		subdir:  project.DatasetsDirName,
		short:   "Cria um dataset customizado local a partir de um esqueleto (scaffold)",
		long: "Gera datasets/<name>.js com o esqueleto de dataset customizado\n" +
			"(defineStructure, createDataset e sincronização comentada). Nada é enviado\n" +
			"ao servidor — publique com `fluigcli dataset export datasets/<name>.js --new`.",
		hint: func(rel string) string {
			return fmt.Sprintf("Edite o código e publique com `fluigcli dataset export %s --new`.", rel)
		},
		create: scaffold.CreateDatasetFile,
	})
}

func newEventNewCmd(app *App) *cobra.Command {
	return newSingleArtifactNewCmd(app, singleArtifactSpec{
		kind:    "evento global",
		jsonKey: "event",
		subdir:  project.EventsDirName,
		short:   "Cria um evento global local a partir de um esqueleto (scaffold)",
		long: "Gera events/<name>.js com a função do evento (o nome do arquivo é o id do\n" +
			"evento global — os parâmetros variam por evento; ajuste a assinatura). Nada\n" +
			"é enviado ao servidor — publique com `fluigcli event export events/<name>.js`.",
		hint: func(rel string) string {
			return fmt.Sprintf("Edite o código e publique com `fluigcli event export %s`.", rel)
		},
		create: scaffold.CreateGlobalEventFile,
	})
}

func newMechanismNewCmd(app *App) *cobra.Command {
	return newSingleArtifactNewCmd(app, singleArtifactSpec{
		kind:    "mecanismo",
		jsonKey: "mechanism",
		subdir:  project.MechanismsDirName,
		short:   "Cria um mecanismo de atribuição local a partir de um esqueleto (scaffold)",
		long: "Gera mechanisms/<name>.js com o esqueleto do mecanismo customizado (a\n" +
			"função que devolve os userCodes aptos a receber a tarefa). Nada é enviado\n" +
			"ao servidor — publique com `fluigcli mechanism export mechanisms/<name>.js`.",
		hint: func(rel string) string {
			return fmt.Sprintf("Edite o código e publique com `fluigcli mechanism export %s`.", rel)
		},
		create: scaffold.CreateMechanismFile,
	})
}

// --- form new (pasta com HTML + eventos) ---

func newFormNewCmd(app *App) *cobra.Command {
	var title string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Cria um formulário local a partir de um esqueleto (scaffold)",
		Long: "Gera forms/<name>/ com o HTML principal (com a tag <form> que o servidor\n" +
			"exige) e os eventos comuns (events/displayFields.js e validateForm.js,\n" +
			"compatíveis com a simulação do `fluigcli dev`). Nada é enviado ao\n" +
			"servidor — publique com `fluigcli form export forms/<name> --new`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			name := args[0]
			files, err := scaffold.CreateFormDir(project.FormDir(root, name), name, title)
			if err != nil {
				return scaffoldErr(err)
			}
			rel := filepath.ToSlash(filepath.Join(project.FormsDirName, name))
			p.Successf("formulário %q criado em %s (%d arquivos)", name, rel, len(files))
			p.Infof("Preview com `fluigcli dev` (em /_dev/forms/); publique com `fluigcli form export %s --new`.", rel)
			p.Done(map[string]any{"form": name, "dir": rel, "files": files})
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "título do formulário (padrão: o próprio nome)")
	return cmd
}

// --- workflow new-script (evento de processo com a assinatura do catálogo) ---

func newWorkflowNewScriptCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new-script <processId> <evento>",
		Short: "Cria o script local de um evento de processo (scaffold)",
		Long: "Gera workflow/scripts/<processId>.<evento>.js com a assinatura correta do\n" +
			"evento escolhido. Nada é enviado ao servidor — publique com\n" +
			"`fluigcli workflow export <processId>` ou `fluigcli workflow publish <processId>`.\n\n" +
			"Eventos disponíveis:\n" + processEventsHelp(),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			processID, eventName := args[0], args[1]
			ev, ok := scaffold.FindProcessEvent(eventName)
			if !ok {
				return output.Usagef("evento de processo desconhecido: %q (disponíveis: %s)",
					eventName, strings.Join(scaffold.ProcessEventNames(), ", "))
			}
			// export/publish varrem workflow/scripts recursivamente — um
			// homônimo em subpasta viraria duplicata com warning.
			scriptName := processID + "." + ev.Name
			existing, _, err := project.FindArtifactFile(root, project.WorkflowScriptsDir, scriptName)
			if err != nil {
				return err
			}
			if existing != "" {
				relExisting, _ := filepath.Rel(root, existing)
				return output.Usagef("script %q já existe em %s", scriptName, filepath.ToSlash(relExisting))
			}
			path, err := project.SafeJoin(filepath.Join(root, project.WorkflowScriptsDir), scriptName+".js")
			if err != nil {
				return output.Usagef("%s", err)
			}
			if _, err := scaffold.CreateProcessScriptFile(path, processID, ev.Name); err != nil {
				return scaffoldErr(err)
			}
			rel := filepath.ToSlash(filepath.Join(project.WorkflowScriptsDir, scriptName+".js"))
			p.Successf("script %q criado em %s", scriptName, rel)
			p.Infof("Publique com `fluigcli workflow export %s` (cirúrgico, exige o componente auxiliar) ou `fluigcli workflow publish %s` (nativo, cria nova versão).", processID, processID)
			p.Done(map[string]any{"process": processID, "event": ev.Name, "file": rel})
			return nil
		},
	}
	return cmd
}

// processEventsHelp monta a lista de eventos do catálogo para o help.
func processEventsHelp() string {
	var b strings.Builder
	for _, e := range scaffold.ProcessEvents() {
		fmt.Fprintf(&b, "  %s(%s) — %s\n", e.Name, e.Params, e.Doc)
	}
	return strings.TrimRight(b.String(), "\n")
}
