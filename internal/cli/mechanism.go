package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newMechanismCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mechanism",
		Short: "Lista, importa, exporta e exclui mecanismos de atribuição (import = servidor→local; export = local→servidor)",
	}
	cmd.AddCommand(newMechanismListCmd(app))
	cmd.AddCommand(newMechanismImportCmd(app))
	cmd.AddCommand(newMechanismExportCmd(app))
	cmd.AddCommand(newMechanismDeleteCmd(app))
	return cmd
}

// --- mechanism list ---

func newMechanismListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista os mecanismos de atribuição customizados do servidor",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, false)
			if err != nil {
				return err
			}
			mechs, err := client.ListMechanisms(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			ids := make([]string, 0, len(mechs))
			for _, m := range mechs {
				p.Successf("%-40s %s", m.ID, m.Name)
				ids = append(ids, m.ID)
			}
			p.Done(map[string]any{"mechanisms": ids})
			return nil
		},
	}
}

// --- mechanism import (servidor → local) ---

func newMechanismImportCmd(app *App) *cobra.Command {
	var (
		all           bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "import <id>... | --all",
		Short: "Baixa mecanismos do servidor para arquivos locais (servidor → local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if !all && len(args) == 0 {
				return output.Usagef("informe um ou mais ids de mecanismo ou use --all")
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

			mechs, err := client.ListMechanisms(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byID := make(map[string]fluig.Mechanism, len(mechs))
			for _, m := range mechs {
				byID[m.ID] = m
			}

			ids := args
			if all {
				ids = nil
				for _, m := range mechs {
					ids = append(ids, m.ID)
				}
				if len(ids) == 0 {
					p.Infof("Nenhum mecanismo customizado no servidor.")
				}
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, id := range ids {
				m, ok := byID[id]
				if !ok {
					failures++
					lastErr = output.NotFoundf("mecanismo %q não encontrado no servidor", id)
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("mecanismo %q: não encontrado", id)
					continue
				}
				action, werr := writeArtifactFile(app, root, project.MechanismsDirName, id, m.Code)
				if werr != nil {
					failures++
					lastErr = werr
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(werr).Message})
					p.Warnf("mecanismo %q: %s", id, output.AsError(werr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: action, Success: true})
				p.Successf("mecanismo %q %s", id, action)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(ids))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "importa todos os mecanismos customizados do servidor")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- mechanism export (local → servidor) ---

func newMechanismExportCmd(app *App) *cobra.Command {
	var (
		name          string
		description   string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "export <file>...",
		Short: "Envia mecanismos locais para o servidor (local → servidor)",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais arquivos .js de mecanismo")
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "publicar mecanismos")
			if err != nil {
				return err
			}

			mechs, err := client.ListMechanisms(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			byID := make(map[string]fluig.Mechanism, len(mechs))
			for i := range mechs {
				byID[mechs[i].ID] = mechs[i]
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, file := range args {
				id := project.ArtifactName(file)
				action, werr := app.exportOneMechanism(ctx, client, byID, file, id, name, description)
				if werr != nil {
					failures++
					lastErr = werr
					results = append(results, itemResult{ID: id, Action: action, Success: false, Error: output.AsError(werr).Message})
					p.Warnf("mecanismo %q: %s", id, output.AsError(werr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: action, Success: true})
				p.Successf("mecanismo %q %s", id, action)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "nome do mecanismo ao criar (default: o id)")
	cmd.Flags().StringVar(&description, "description", "", "descrição do mecanismo ao criar (default: o nome)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

func (a *App) exportOneMechanism(ctx context.Context, client *fluig.Client, byID map[string]fluig.Mechanism, file, id, name, description string) (string, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return "failed", output.NotFoundf("arquivo %q não encontrado", file)
		}
		return "failed", err
	}
	if existing, ok := byID[id]; ok {
		if err := client.UpdateMechanism(ctx, &existing, string(content)); err != nil {
			return "failed", err
		}
		return "updated", nil
	}
	// Criação: nome/descrição default; controlClass/assignmentType são fixos.
	mechName := name
	if mechName == "" {
		mechName = id
	}
	desc := description
	if desc == "" {
		desc = mechName
	}
	if err := client.CreateMechanism(ctx, id, mechName, desc, string(content)); err != nil {
		return "failed", err
	}
	return "created", nil
}

// --- mechanism delete ---

func newMechanismDeleteCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "delete <id>...",
		Short: "Exclui mecanismos de atribuição no servidor",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if len(args) == 0 {
				return output.Usagef("informe um ou mais ids de mecanismo a excluir")
			}
			if err := app.confirm(fmt.Sprintf("Excluir %d mecanismo(s) no servidor?", len(args))); err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "excluir mecanismos")
			if err != nil {
				return err
			}

			var results []itemResult
			var lastErr error
			failures := 0
			for _, id := range args {
				if derr := client.DeleteMechanism(ctx, id); derr != nil {
					failures++
					lastErr = mapFluigError(derr)
					results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
					p.Warnf("mecanismo %q: %s", id, output.AsError(lastErr).Message)
					continue
				}
				results = append(results, itemResult{ID: id, Action: "deleted", Success: true})
				p.Successf("mecanismo %q excluído", id)
			}
			return finishBatch(p, lastErr, map[string]any{"results": results}, failures, len(args))
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}
