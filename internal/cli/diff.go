package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
	"github.com/alorenco/fluig-cli/internal/textdiff"
)

// Status possíveis de um artefato no diff (contrato --json).
const (
	diffEqual      = "equal"
	diffModified   = "modified"
	diffOnlyLocal  = "only-local"
	diffOnlyServer = "only-server"
)

// diffEntry é o resultado do diff de um artefato.
type diffEntry struct {
	Type   string `json:"type"` // dataset | event | mechanism
	ID     string `json:"id"`
	Path   string `json:"path,omitempty"` // relativo à raiz do projeto
	Status string `json:"status"`
	Diff   string `json:"diff,omitempty"` // diff unificado (só em modified)
}

// diffTarget é um artefato local a comparar.
type diffTarget struct {
	typ, id, path string
}

// diffDirs mapeia o tipo de artefato para a pasta da convenção do projeto.
var diffDirs = []struct{ typ, dir string }{
	{"dataset", project.DatasetsDirName},
	{"event", project.EventsDirName},
	{"mechanism", project.MechanismsDirName},
}

func newDiffCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "diff [<path>...]",
		Short: "Compara artefatos locais com o servidor (nada é alterado)",
		Long: "Mostra o que um export mudaria: compara datasets, eventos globais e\n" +
			"mecanismos locais com o conteúdo atual do servidor.\n\n" +
			"Sem argumentos, varre as pastas datasets/, events/ e mechanisms/ do projeto\n" +
			"e também aponta artefatos que só existem no servidor. Com caminhos, compara\n" +
			"apenas os arquivos informados. Diferenças só de quebra de linha (CRLF/LF)\n" +
			"não contam. Formulários e scripts de workflow ainda não são cobertos.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			sweep := len(args) == 0
			var targets []diffTarget
			if sweep {
				if targets, err = collectDiffTargets(root); err != nil {
					return err
				}
			} else {
				for _, arg := range args {
					t, err := classifyArtifactPath(root, arg)
					if err != nil {
						return err
					}
					targets = append(targets, t)
				}
			}

			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}

			// Eventos e mecanismos vêm com o código na própria listagem — uma
			// requisição cobre tudo. Datasets são carregados um a um.
			needType := map[string]bool{}
			for _, t := range targets {
				needType[t.typ] = true
			}
			events := map[string]string{}
			if sweep || needType["event"] {
				list, err := client.ListGlobalEvents(ctx)
				if err != nil {
					return mapFluigError(err)
				}
				for _, ev := range list {
					events[ev.ID] = ev.Code
				}
			}
			mechs := map[string]string{}
			if sweep || needType["mechanism"] {
				list, err := client.ListMechanisms(ctx)
				if err != nil {
					return mapFluigError(err)
				}
				for _, m := range list {
					mechs[m.ID] = m.Code
				}
			}
			serverDatasets := map[string]bool{}
			if sweep {
				list, err := client.ListDatasets(ctx)
				if err != nil {
					return mapFluigError(err)
				}
				for _, d := range list {
					if d.Custom {
						serverDatasets[d.ID] = true
					}
				}
			}

			localSeen := map[string]map[string]bool{"dataset": {}, "event": {}, "mechanism": {}}
			var entries []diffEntry
			for _, t := range targets {
				localSeen[t.typ][t.id] = true
				raw, err := os.ReadFile(t.path)
				if err != nil {
					return output.Usagef("não consegui ler %s: %v", t.path, err)
				}
				local := normalizeEOL(string(raw))

				remote, found := "", false
				switch t.typ {
				case "dataset":
					ds, err := client.LoadDataset(ctx, t.id)
					switch {
					case errors.Is(err, fluig.ErrNotFound):
					case err != nil:
						return mapFluigError(err)
					default:
						remote, found = ds.Impl, true
					}
				case "event":
					remote, found = events[t.id]
				case "mechanism":
					remote, found = mechs[t.id]
				}

				entry := diffEntry{Type: t.typ, ID: t.id, Path: relTo(root, t.path)}
				switch {
				case !found:
					entry.Status = diffOnlyLocal
				case normalizeEOL(remote) == local:
					entry.Status = diffEqual
				default:
					entry.Status = diffModified
					entry.Diff = textdiff.Unified(
						"servidor:"+t.id, "local:"+entry.Path, normalizeEOL(remote), local)
				}
				entries = append(entries, entry)
			}

			// Artefatos que só existem no servidor (apenas na varredura).
			for id := range serverDatasets {
				if !localSeen["dataset"][id] {
					entries = append(entries, diffEntry{Type: "dataset", ID: id, Status: diffOnlyServer})
				}
			}
			for id := range events {
				if sweep && !localSeen["event"][id] {
					entries = append(entries, diffEntry{Type: "event", ID: id, Status: diffOnlyServer})
				}
			}
			for id := range mechs {
				if sweep && !localSeen["mechanism"][id] {
					entries = append(entries, diffEntry{Type: "mechanism", ID: id, Status: diffOnlyServer})
				}
			}

			sort.Slice(entries, func(i, j int) bool {
				if entries[i].Type != entries[j].Type {
					return entries[i].Type < entries[j].Type
				}
				return entries[i].ID < entries[j].ID
			})

			counts := map[string]int{}
			for _, e := range entries {
				counts[e.Status]++
				switch e.Status {
				case diffModified:
					p.Successf("── %s %s difere do servidor:", e.Type, e.ID)
					p.Successf("%s", strings.TrimRight(e.Diff, "\n"))
				case diffOnlyLocal:
					p.Successf("── %s %s só existe localmente (%s) — o export criaria no servidor", e.Type, e.ID, e.Path)
				case diffOnlyServer:
					p.Successf("── %s %s só existe no servidor — importe com: fluigcli %s import %s", e.Type, e.ID, e.Type, e.ID)
				}
			}
			p.Infof("%d igual(is), %d diferente(s), %d só local(is), %d só no servidor",
				counts[diffEqual], counts[diffModified], counts[diffOnlyLocal], counts[diffOnlyServer])
			p.Done(map[string]any{"artifacts": entries, "counts": counts})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// collectDiffTargets varre as pastas convencionais atrás de arquivos .js.
func collectDiffTargets(root string) ([]diffTarget, error) {
	var targets []diffTarget
	for _, d := range diffDirs {
		base := filepath.Join(root, d.dir)
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return fs.SkipAll
				}
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
				targets = append(targets, diffTarget{d.typ, project.ArtifactName(path), path})
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, output.Genericf("falha ao varrer %s: %v", base, err)
		}
	}
	return targets, nil
}

// classifyArtifactPath deduz o tipo do artefato pela pasta da convenção em que
// o caminho está (datasets/, events/ ou mechanisms/).
func classifyArtifactPath(root, arg string) (diffTarget, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return diffTarget{}, output.Usagef("caminho inválido %q: %v", arg, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return diffTarget{}, output.Usagef("%s está fora do projeto (%s)", arg, root)
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for _, d := range diffDirs {
		if segs[0] == d.dir {
			if !strings.HasSuffix(abs, ".js") {
				return diffTarget{}, output.Usagef("%s não é um artefato .js", arg)
			}
			return diffTarget{d.typ, project.ArtifactName(abs), abs}, nil
		}
	}
	return diffTarget{}, output.Usagef(
		"%s: o diff cobre por enquanto as pastas %s/, %s/ e %s/",
		arg, project.DatasetsDirName, project.EventsDirName, project.MechanismsDirName)
}

// normalizeEOL iguala quebras de linha (CRLF → LF) e ignora a quebra final,
// para o diff não acusar diferença que o export/import não preserva de
// qualquer forma (servidor usa CRLF; repositórios normalmente LF).
func normalizeEOL(s string) string {
	return strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// relTo devolve path relativo a root (ou o próprio path se não der).
func relTo(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}
