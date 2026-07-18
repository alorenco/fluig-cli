package cli

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

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
	Type   string `json:"type"` // dataset | event | mechanism | form | workflow
	ID     string `json:"id"`
	Path   string `json:"path,omitempty"` // relativo à raiz do projeto
	Status string `json:"status"`
	Diff   string `json:"diff,omitempty"` // diff unificado (só em modified de texto)
}

// diffTarget é um artefato de arquivo único a comparar (dataset/event/mechanism).
type diffTarget struct {
	typ, id, path string
}

// formDiffTarget é uma pasta de formulário local a comparar.
type formDiffTarget struct {
	folder string // nome da pasta em forms/
	dir    string // caminho absoluto da pasta
	only   string // arquivo específico, relativo à pasta ("x.html", "events/y.js"); "" = pasta toda
}

// diffTargets agrupa tudo o que o diff vai comparar.
type diffTargets struct {
	scripts []diffTarget                       // datasets/, events/, mechanisms/
	forms   []formDiffTarget                   // forms/<pasta>
	wf      map[string][]project.ProcessScript // processId → scripts em workflow/scripts/
}

// diffDirs mapeia o tipo de artefato de arquivo único para a pasta da convenção.
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
		Long: "Mostra o que um export mudaria: compara datasets, eventos globais,\n" +
			"mecanismos, formulários e scripts de processo locais com o conteúdo\n" +
			"atual do servidor.\n\n" +
			"Sem argumentos, varre as pastas datasets/, events/, mechanisms/, forms/ e\n" +
			"workflow/scripts/ do projeto e também aponta artefatos que só existem no\n" +
			"servidor. Com caminhos, compara apenas os arquivos (ou pastas de\n" +
			"formulário) informados. Diferenças só de quebra de linha (CRLF/LF) não\n" +
			"contam. Em formulários, anexos binários são comparados byte a byte (sem\n" +
			"diff textual); em scripts de processo, a comparação usa o export nativo\n" +
			"do processo (não requer o componente auxiliar).",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			sweep := len(args) == 0
			var targets diffTargets
			if sweep {
				if targets, err = collectDiffTargets(root); err != nil {
					return err
				}
			} else {
				targets.wf = map[string][]project.ProcessScript{}
				for _, arg := range args {
					if err := classifyArtifactPath(&targets, root, arg); err != nil {
						return err
					}
				}
			}

			server, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}

			// Eventos e mecanismos vêm com o código na própria listagem — uma
			// requisição cobre tudo. Datasets são carregados um a um.
			needType := map[string]bool{}
			for _, t := range targets.scripts {
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
			var serverProcesses []fluig.ProcessSummary
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
				// Processos do servidor (REST v2) — para apontar os que não têm
				// script local. A comparação script a script continua vindo do
				// export nativo em diffProcessScripts.
				if serverProcesses, err = client.ListProcesses(ctx); err != nil {
					return mapFluigError(err)
				}
			}

			localSeen := map[string]map[string]bool{"dataset": {}, "event": {}, "mechanism": {}}
			var entries []diffEntry
			for _, t := range targets.scripts {
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

			// Formulários: pastas locais vs. anexos + eventos do servidor.
			if sweep || len(targets.forms) > 0 {
				formEntries, err := diffForms(ctx, client, root, server.FormScopeKey(), targets.forms, sweep)
				if err != nil {
					return err
				}
				entries = append(entries, formEntries...)
			}

			// Scripts de processo: o export nativo traz os eventos do servidor.
			pids := make([]string, 0, len(targets.wf))
			for pid := range targets.wf {
				pids = append(pids, pid)
			}
			sort.Strings(pids)
			for _, pid := range pids {
				wfEntries, err := diffProcessScripts(ctx, client, p, root, pid, targets.wf[pid], sweep)
				if err != nil {
					return err
				}
				entries = append(entries, wfEntries...)
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
			// Processos sem nenhum script local (o ID sem "." distingue o
			// processo inteiro de um evento pid.evento nas mensagens).
			for _, pr := range serverProcesses {
				if _, ok := targets.wf[pr.ID]; !ok {
					entries = append(entries, diffEntry{Type: "workflow", ID: pr.ID, Status: diffOnlyServer})
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
					if e.Diff == "" {
						p.Successf("── %s %s difere do servidor (conteúdo binário)", e.Type, e.ID)
						continue
					}
					p.Successf("── %s %s difere do servidor:", e.Type, e.ID)
					p.Successf("%s", strings.TrimRight(e.Diff, "\n"))
				case diffOnlyLocal:
					p.Successf("── %s %s só existe localmente (%s) — o export criaria no servidor", e.Type, e.ID, e.Path)
				case diffOnlyServer:
					p.Successf("%s", onlyServerMessage(e))
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

// onlyServerMessage monta a orientação humana de um artefato only-server — o
// caminho de volta varia por tipo (form/workflow não têm o mesmo import).
func onlyServerMessage(e diffEntry) string {
	switch e.Type {
	case "form":
		if strings.Contains(e.ID, "/") {
			// Arquivo/evento dentro de um formulário que existe localmente.
			return "── form " + e.ID + " só existe no servidor — o export da pasta o removeria; importe o formulário para trazê-lo"
		}
		return "── form " + e.ID + " só existe no servidor — importe com: fluigcli form import \"" + e.ID + "\""
	case "workflow":
		if !strings.Contains(e.ID, ".") {
			// Processo inteiro, sem nenhum script local (varredura via ListProcesses).
			return "── workflow " + e.ID + " (processo) não tem scripts locais — se ele tiver eventos, versione-os em workflow/scripts/" + e.ID + ".<evento>.js"
		}
		return "── workflow " + e.ID + " só existe no servidor — crie workflow/scripts/" + e.ID + ".js para versioná-lo (não há import de scripts de processo)"
	default:
		return "── " + e.Type + " " + e.ID + " só existe no servidor — importe com: fluigcli " + e.Type + " import " + e.ID
	}
}

// collectDiffTargets varre as pastas convencionais: .js de datasets/, events/ e
// mechanisms/, pastas de forms/ e scripts de workflow/scripts/.
func collectDiffTargets(root string) (diffTargets, error) {
	targets := diffTargets{wf: map[string][]project.ProcessScript{}}
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
				targets.scripts = append(targets.scripts, diffTarget{d.typ, project.ArtifactName(path), path})
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return targets, output.Genericf("falha ao varrer %s: %v", base, err)
		}
	}

	// forms/: cada subpasta é um formulário.
	formsBase := filepath.Join(root, project.FormsDirName)
	if dirEntries, err := os.ReadDir(formsBase); err == nil {
		for _, e := range dirEntries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				targets.forms = append(targets.forms, formDiffTarget{
					folder: e.Name(), dir: filepath.Join(formsBase, e.Name()),
				})
			}
		}
	} else if !os.IsNotExist(err) {
		return targets, output.Genericf("falha ao varrer %s: %v", formsBase, err)
	}

	// workflow/scripts/: <Processo>.<evento>.js, agrupados por processo.
	wfBase := filepath.Join(root, project.WorkflowScriptsDir)
	err := filepath.WalkDir(wfBase, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".js") {
			return nil
		}
		if pid, ev, ok := project.ParseWorkflowScriptName(path); ok {
			targets.wf[pid] = append(targets.wf[pid], project.ProcessScript{ProcessID: pid, Event: ev, Path: path})
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return targets, output.Genericf("falha ao varrer %s: %v", wfBase, err)
	}
	return targets, nil
}

// classifyArtifactPath deduz o tipo do artefato pela pasta da convenção em que
// o caminho está e o acumula em targets.
func classifyArtifactPath(targets *diffTargets, root, arg string) error {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return output.Usagef("caminho inválido %q: %v", arg, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return output.Usagef("%s está fora do projeto (%s)", arg, root)
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for _, d := range diffDirs {
		if segs[0] == d.dir {
			if !strings.HasSuffix(abs, ".js") {
				return output.Usagef("%s não é um artefato .js", arg)
			}
			targets.scripts = append(targets.scripts, diffTarget{d.typ, project.ArtifactName(abs), abs})
			return nil
		}
	}

	// forms/<pasta>[/arquivo | /events/evento.js]
	if segs[0] == project.FormsDirName {
		if len(segs) < 2 {
			return output.Usagef("informe a pasta do formulário: %s/<pasta>", project.FormsDirName)
		}
		folder := segs[1]
		dir := filepath.Join(root, project.FormsDirName, folder)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return output.NotFoundf("pasta de formulário %q não encontrada", filepath.Join(project.FormsDirName, folder))
		}
		only := ""
		if len(segs) > 2 {
			only = strings.Join(segs[2:], "/")
			if info, err := os.Stat(abs); err != nil || info.IsDir() {
				return output.NotFoundf("arquivo %q não encontrado", arg)
			}
		}
		targets.forms = append(targets.forms, formDiffTarget{folder: folder, dir: dir, only: only})
		return nil
	}

	// workflow/scripts/<Processo>.<evento>.js
	if filepath.ToSlash(rel) != "" && strings.HasPrefix(filepath.ToSlash(rel), filepath.ToSlash(project.WorkflowScriptsDir)+"/") {
		if !strings.HasSuffix(abs, ".js") {
			return output.Usagef("%s não é um script .js de processo", arg)
		}
		pid, ev, ok := project.ParseWorkflowScriptName(abs)
		if !ok {
			return output.Usagef("nome de script inválido %q (esperado <Processo>.<evento>.js)", filepath.Base(arg))
		}
		targets.wf[pid] = append(targets.wf[pid], project.ProcessScript{ProcessID: pid, Event: ev, Path: abs})
		return nil
	}

	return output.Usagef(
		"%s: o diff cobre as pastas %s/, %s/, %s/, %s/ e %s/",
		arg, project.DatasetsDirName, project.EventsDirName, project.MechanismsDirName,
		project.FormsDirName, filepath.ToSlash(project.WorkflowScriptsDir))
}

// diffForms compara as pastas locais de formulário com o servidor e, na
// varredura, aponta formulários que só existem no servidor.
func diffForms(ctx context.Context, client *fluig.Client, root, formScope string, targets []formDiffTarget, sweep bool) ([]diffEntry, error) {
	userCode, err := client.ResolveUserCode(ctx)
	if err != nil {
		return nil, mapFluigError(err)
	}
	forms, err := client.ListForms(ctx, userCode)
	if err != nil {
		return nil, mapFluigError(err)
	}
	fmap, err := project.LoadFormMap(root, formScope)
	if err != nil {
		return nil, output.Genericf("falha ao ler .fluigcli/forms.json: %v", err)
	}

	matched := map[int]bool{}
	var entries []diffEntry
	for _, t := range targets {
		f, found := resolveExportTarget(forms, fmap, t.folder, "", 0)
		if !found {
			entries = append(entries, diffEntry{
				Type: "form", ID: t.folder, Path: relTo(root, t.dir), Status: diffOnlyLocal,
			})
			continue
		}
		matched[f.DocumentID] = true
		formEntries, err := diffOneForm(ctx, client, root, t, f, userCode)
		if err != nil {
			return nil, err
		}
		entries = append(entries, formEntries...)
	}

	if sweep {
		for _, f := range forms {
			if !matched[f.DocumentID] {
				entries = append(entries, diffEntry{Type: "form", ID: f.Description, Status: diffOnlyServer})
			}
		}
	}
	return entries, nil
}

// diffOneForm compara uma pasta local com o formulário do servidor, arquivo a
// arquivo (anexos + eventos). Anexos binários são comparados byte a byte.
func diffOneForm(ctx context.Context, client *fluig.Client, root string, t formDiffTarget, f fluig.Form, userCode string) ([]diffEntry, error) {
	fc, err := project.ReadFormFolder(t.dir)
	if err != nil {
		return nil, output.Genericf("falha ao ler a pasta %s: %v", t.dir, err)
	}
	localFiles := map[string]string{} // nome do anexo → caminho local
	for _, path := range fc.Files {
		localFiles[filepath.Base(path)] = path
	}
	localEvents := map[string]string{} // id do evento → caminho local
	for _, path := range fc.EventFiles {
		localEvents[project.ArtifactName(path)] = path
	}

	names, err := client.FormAttachments(ctx, f.DocumentID)
	if err != nil {
		return nil, mapFluigError(err)
	}
	serverFiles := map[string]bool{}
	for _, n := range names {
		serverFiles[n] = true
	}
	sEvents, err := client.FormEvents(ctx, f.DocumentID)
	if err != nil {
		return nil, mapFluigError(err)
	}
	serverEvents := map[string]string{}
	for _, e := range sEvents {
		serverEvents[e.ID] = e.Code
	}

	include := func(rel string) bool { return t.only == "" || t.only == rel }
	var entries []diffEntry

	// Anexos locais (ordem estável).
	fileNames := make([]string, 0, len(localFiles))
	for name := range localFiles {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	for _, name := range fileNames {
		if !include(name) {
			continue
		}
		path := localFiles[name]
		entry := diffEntry{Type: "form", ID: t.folder + "/" + name, Path: relTo(root, path)}
		if !serverFiles[name] {
			entry.Status = diffOnlyLocal
			entries = append(entries, entry)
			continue
		}
		remote, err := client.DownloadFormFile(ctx, f.DocumentID, userCode, f.Version, name)
		if err != nil {
			return nil, mapFluigError(err)
		}
		local, err := os.ReadFile(path)
		if err != nil {
			return nil, output.Usagef("não consegui ler %s: %v", path, err)
		}
		fillContentDiff(&entry, "servidor:"+name, "local:"+entry.Path, remote.Content, local)
		entries = append(entries, entry)
	}

	// Eventos locais.
	eventIDs := make([]string, 0, len(localEvents))
	for id := range localEvents {
		eventIDs = append(eventIDs, id)
	}
	sort.Strings(eventIDs)
	for _, id := range eventIDs {
		rel := "events/" + id + ".js"
		if !include(rel) {
			continue
		}
		path := localEvents[id]
		entry := diffEntry{Type: "form", ID: t.folder + "/" + rel, Path: relTo(root, path)}
		remote, found := serverEvents[id]
		local, err := os.ReadFile(path)
		if err != nil {
			return nil, output.Usagef("não consegui ler %s: %v", path, err)
		}
		switch {
		case !found:
			entry.Status = diffOnlyLocal
		case normalizeEOL(remote) == normalizeEOL(string(local)):
			entry.Status = diffEqual
		default:
			entry.Status = diffModified
			entry.Diff = textdiff.Unified("servidor:"+id, "local:"+entry.Path,
				normalizeEOL(remote), normalizeEOL(string(local)))
		}
		entries = append(entries, entry)
	}

	// Lado do servidor sem contraparte local: o export da pasta os removeria.
	// Só na comparação da pasta inteira (com --only seria ruído).
	if t.only == "" {
		sort.Strings(names)
		for _, name := range names {
			if _, ok := localFiles[name]; !ok {
				entries = append(entries, diffEntry{Type: "form", ID: t.folder + "/" + name, Status: diffOnlyServer})
			}
		}
		ids := make([]string, 0, len(serverEvents))
		for id := range serverEvents {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if _, ok := localEvents[id]; !ok {
				entries = append(entries, diffEntry{Type: "form", ID: t.folder + "/events/" + id + ".js", Status: diffOnlyServer})
			}
		}
	}
	return entries, nil
}

// diffProcessScripts compara os scripts locais de um processo com os eventos do
// export nativo do servidor.
func diffProcessScripts(ctx context.Context, client *fluig.Client, p *output.Printer, root, pid string, scripts []project.ProcessScript, sweep bool) ([]diffEntry, error) {
	serverEvents, err := client.ProcessEventScripts(ctx, pid)
	processMissing := false
	switch {
	case errors.Is(err, fluig.ErrNotFound):
		processMissing = true
		p.Warnf("processo %q não existe no servidor — o export de scripts não o cria (use o Fluig Studio)", pid)
	case err != nil:
		return nil, mapFluigError(err)
	}

	var entries []diffEntry
	localSeen := map[string]bool{}
	sort.Slice(scripts, func(i, j int) bool { return scripts[i].Event < scripts[j].Event })
	for _, s := range scripts {
		localSeen[s.Event] = true
		raw, err := os.ReadFile(s.Path)
		if err != nil {
			return nil, output.Usagef("não consegui ler %s: %v", s.Path, err)
		}
		local := normalizeEOL(string(raw))
		entry := diffEntry{Type: "workflow", ID: pid + "." + s.Event, Path: relTo(root, s.Path)}
		remote, found := "", false
		if !processMissing {
			remote, found = serverEvents[s.Event]
		}
		switch {
		case !found:
			entry.Status = diffOnlyLocal
		case normalizeEOL(remote) == local:
			entry.Status = diffEqual
		default:
			entry.Status = diffModified
			entry.Diff = textdiff.Unified("servidor:"+entry.ID, "local:"+entry.Path,
				normalizeEOL(remote), local)
		}
		entries = append(entries, entry)
	}

	// Eventos do servidor sem script local (apenas na varredura).
	if sweep && !processMissing {
		ids := make([]string, 0, len(serverEvents))
		for ev := range serverEvents {
			ids = append(ids, ev)
		}
		sort.Strings(ids)
		for _, ev := range ids {
			if !localSeen[ev] && strings.TrimSpace(serverEvents[ev]) != "" {
				entries = append(entries, diffEntry{Type: "workflow", ID: pid + "." + ev, Status: diffOnlyServer})
			}
		}
	}
	return entries, nil
}

// fillContentDiff decide equal/modified para conteúdo possivelmente binário:
// texto ganha diff unificado com EOL normalizado; binário compara byte a byte
// (Diff fica vazio).
func fillContentDiff(entry *diffEntry, remoteName, localName string, remote, local []byte) {
	if isTextContent(remote) && isTextContent(local) {
		r, l := normalizeEOL(string(remote)), normalizeEOL(string(local))
		if r == l {
			entry.Status = diffEqual
			return
		}
		entry.Status = diffModified
		entry.Diff = textdiff.Unified(remoteName, localName, r, l)
		return
	}
	if bytes.Equal(remote, local) {
		entry.Status = diffEqual
		return
	}
	entry.Status = diffModified // binário: sem diff textual
}

// isTextContent considera texto o que é UTF-8 válido e não contém NUL.
func isTextContent(b []byte) bool {
	return utf8.Valid(b) && !bytes.ContainsRune(b, 0)
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
