package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

func newWatchCmd(app *App) *cobra.Command {
	var (
		debounce      time.Duration
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Publica automaticamente ao salvar (datasets, eventos, mecanismos, formulários e scripts de processo)",
		Long: "Observa as pastas do projeto e publica o artefato no servidor a cada\n" +
			"salvamento — o ciclo editar→exportar→testar vira só editar→testar.\n\n" +
			"Cobertura: datasets/, events/, mechanisms/, forms/ (a pasta inteira do\n" +
			"formulário é a unidade — sempre com a versão mantida) e workflow/scripts/\n" +
			"(atualização cirúrgica via fluiggersWidget, sem bump de versão).\n\n" +
			"Regras de segurança: só roda em servidor marcado como dev ou hml (produção\n" +
			"é recusada, sem exceção); só ATUALIZA artefatos que já existem no servidor\n" +
			"— arquivo novo gera um aviso com o comando de criação; salvamento sem\n" +
			"mudança de conteúdo não publica nada.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if app.JSON {
				return output.Usagef("watch é um modo interativo e não suporta --json; em automação, use os comandos export")
			}

			root, err := app.projectRootForFiles()
			if err != nil {
				return err
			}
			server, err := app.resolveServer("")
			if err != nil {
				return err
			}
			p.Server = server.Name
			switch server.Env {
			case config.EnvDev, config.EnvHml:
			case config.EnvProd:
				return output.Usagef("o watch não roda em servidor de PRODUÇÃO (%q) — publique conscientemente com export", server.Name)
			default:
				return output.Usagef("o watch exige servidor marcado como dev ou hml; marque com: fluigcli server update %s --env hml", server.Name)
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			client, err := app.authenticate(ctx, server, passwordStdin)
			if err != nil {
				return err
			}
			return app.runWatch(ctx, client, root, server.Name, debounce)
		},
	}
	cmd.Flags().DurationVar(&debounce, "debounce", 500*time.Millisecond, "espera após o salvamento antes de publicar (agrupa rajadas do editor)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// watchUnit é a unidade de publicação do watch: um arquivo (dataset, evento,
// mecanismo, script de processo) ou uma pasta inteira (formulário).
type watchUnit struct {
	typ  string // dataset | event | mechanism | form | workflow
	id   string // nome nas mensagens: id do artefato, pasta do form, Processo.evento
	path string // arquivo — ou a pasta do formulário (chave do debounce)
}

// watchDirs são as pastas observadas, relativas à raiz do projeto.
var watchDirs = []string{
	project.DatasetsDirName,
	project.EventsDirName,
	project.MechanismsDirName,
	project.FormsDirName,
	project.WorkflowScriptsDir,
}

// watchSession carrega o estado do loop de watch.
type watchSession struct {
	app    *App
	client *fluig.Client
	root   string
	// published guarda o hash do conteúdo na última publicação de cada
	// unidade, para pular salvamentos sem mudança — essencial em formulários
	// e scripts de processo, cujo conteúdo atual não pode ser lido barato do
	// servidor.
	published map[string]string
	// helperOK cacheia o status da fluiggersWidget (checado no primeiro
	// script de processo salvo).
	helperOK *bool
}

// runWatch é o loop do watch: observa as pastas convencionais, agrupa eventos
// por unidade (debounce; a pasta do formulário agrupa todos os seus arquivos)
// e publica cada salvamento. Erros de publicação viram aviso — o watch segue
// vivo até o contexto ser cancelado (Ctrl+C).
func (a *App) runWatch(ctx context.Context, client *fluig.Client, root, serverName string, debounce time.Duration) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return output.Genericf("não consegui iniciar o observador de arquivos: %v", err)
	}
	defer w.Close()

	var watched []string
	for _, dir := range watchDirs {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		if err := watchRecursive(w, base); err != nil {
			return output.Genericf("não consegui observar %s: %v", base, err)
		}
		watched = append(watched, filepath.ToSlash(dir)+"/")
	}
	if len(watched) == 0 {
		return output.Usagef("nenhuma pasta da convenção (datasets/, events/, mechanisms/, forms/, workflow/scripts/) encontrada em %s", root)
	}

	a.printer.Infof("Observando %s em %q — Ctrl+C para parar.", strings.Join(watched, ", "), serverName)

	s := &watchSession{app: a, client: client, root: root, published: map[string]string{}}
	pending := map[string]*time.Timer{}
	fire := make(chan watchUnit, 32)
	for {
		select {
		case <-ctx.Done():
			a.printer.Infof("Watch encerrado.")
			return nil

		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
				continue
			}
			// Pasta nova criada dentro da árvore observada: passa a observá-la.
			if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
				_ = watchRecursive(w, ev.Name)
				continue
			}
			unit, ok := classifyWatchPath(root, ev.Name)
			if !ok {
				continue
			}
			// Debounce por unidade: rajadas do editor (write+rename) e vários
			// arquivos do mesmo formulário viram uma publicação só.
			if t, ok := pending[unit.path]; ok {
				t.Stop()
			}
			u := unit
			pending[u.path] = time.AfterFunc(debounce, func() {
				select {
				case fire <- u:
				case <-ctx.Done():
				}
			})

		case u := <-fire:
			delete(pending, u.path)
			if _, err := os.Stat(u.path); err != nil {
				continue // apagado/renomeado entre o evento e o disparo
			}
			s.publish(ctx, u)

		case werr, ok := <-w.Errors:
			if !ok {
				return nil
			}
			a.printer.Warnf("observador de arquivos: %v", werr)
		}
	}
}

// classifyWatchPath deduz a unidade de publicação de um arquivo alterado.
// Arquivos temporários de editor (ocultos, *~, *.swp, *.tmp) são ignorados.
func classifyWatchPath(root, name string) (watchUnit, bool) {
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") ||
		strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".tmp") {
		return watchUnit{}, false
	}
	rel, err := filepath.Rel(root, name)
	if err != nil || strings.HasPrefix(rel, "..") {
		return watchUnit{}, false
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	isJS := strings.HasSuffix(base, ".js")

	switch segs[0] {
	case project.DatasetsDirName:
		if isJS {
			return watchUnit{"dataset", project.ArtifactName(name), name}, true
		}
	case project.EventsDirName:
		if isJS {
			return watchUnit{"event", project.ArtifactName(name), name}, true
		}
	case project.MechanismsDirName:
		if isJS {
			return watchUnit{"mechanism", project.ArtifactName(name), name}, true
		}
	case project.FormsDirName:
		// forms/<pasta>/... — qualquer arquivo da pasta publica o formulário.
		if len(segs) >= 3 {
			return watchUnit{"form", segs[1], filepath.Join(root, project.FormsDirName, segs[1])}, true
		}
	case "workflow":
		if len(segs) >= 3 && segs[1] == "scripts" && isJS {
			if _, _, ok := project.ParseWorkflowScriptName(name); ok {
				return watchUnit{"workflow", strings.TrimSuffix(base, ".js"), name}, true
			}
		}
	}
	return watchUnit{}, false
}

// watchRecursive adiciona dir e todos os subdiretórios ao observador (o
// fsnotify não é recursivo por conta própria).
func watchRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}

// publish publica uma unidade salva, com as três proteções do watch: hash
// igual ao da última publicação não vai à rede; artefato inexistente não é
// criado; e nada de bump de versão (forms com VersionKeep, workflow cirúrgico).
func (s *watchSession) publish(ctx context.Context, u watchUnit) {
	ts := time.Now().Format("15:04:05")
	p := s.app.printer

	hash, err := hashPath(u.path)
	if err != nil {
		p.Warnf("%s  %s %q: não consegui ler: %v", ts, u.typ, u.id, err)
		return
	}
	if s.published[u.path] == hash {
		p.Infof("· %s  %s %q sem mudança — nada a publicar", ts, u.typ, u.id)
		return
	}

	action, err := s.update(ctx, u)
	switch {
	case err != nil:
		p.Warnf("%s  %s %q: %s", ts, u.typ, u.id, output.AsError(mapFluigError(err)).Message)
	case action == "missing":
		p.Warnf("%s  %s", ts, missingMessage(u, s.root))
	case action == "no-helper":
		p.Warnf("%s  script %q: a fluiggersWidget não está instalada — instale com: fluigcli server install-helper", ts, u.id)
	case action == "empty":
		p.Warnf("%s  formulário %q: pasta sem arquivos para enviar", ts, u.id)
	case action == "unchanged":
		s.published[u.path] = hash
		p.Infof("· %s  %s %q sem mudança — nada a publicar", ts, u.typ, u.id)
	default:
		s.published[u.path] = hash
		suffix := ""
		if u.typ == "form" {
			suffix = " (versão mantida)"
		}
		p.Successf("✓ %s  %s %q publicado%s", ts, u.typ, u.id, suffix)
	}
}

// missingMessage monta o aviso de artefato inexistente com o comando certo —
// o watch nunca cria nada por conta própria.
func missingMessage(u watchUnit, root string) string {
	switch u.typ {
	case "form":
		return "formulário \"" + u.id + "\" não existe no servidor (nem há vínculo em .fluigcli/forms.json) — " +
			"o watch não cria; use: fluigcli form export forms/" + u.id + " --new"
	case "workflow":
		return "script \"" + u.id + "\": o processo não existe no servidor — crie o processo no Fluig Studio primeiro"
	default:
		hint := ""
		if u.typ == "dataset" {
			hint = " --new"
		}
		return u.typ + " \"" + u.id + "\" não existe no servidor — o watch não cria; use: fluigcli " +
			u.typ + " export " + relTo(root, u.path) + hint
	}
}

// update publica a unidade no servidor. Ações: "updated", "unchanged" (servidor
// já tem esse conteúdo), "missing", "no-helper" ou "empty".
func (s *watchSession) update(ctx context.Context, u watchUnit) (string, error) {
	switch u.typ {
	case "dataset", "event", "mechanism":
		raw, err := os.ReadFile(u.path)
		if err != nil {
			return "", err
		}
		return s.app.updateExisting(ctx, s.client, diffTarget(u), string(raw))
	case "form":
		return s.updateForm(ctx, u)
	case "workflow":
		return s.updateWorkflow(ctx, u)
	}
	return "", output.Genericf("tipo de artefato desconhecido: %q", u.typ)
}

// updateForm reaproveita o fluxo do form export com a versão SEMPRE mantida
// (VersionKeep) — o watch jamais gera versões novas de formulário.
func (s *watchSession) updateForm(ctx context.Context, u watchUnit) (string, error) {
	upload, err := readFormUpload(u.path)
	if err != nil {
		return "", err
	}
	if len(upload.Files) == 0 {
		return "empty", nil
	}
	pub, err := s.client.ResolveUserCode(ctx)
	if err != nil {
		return "", err
	}
	fmap, err := project.LoadFormMap(s.root)
	if err != nil {
		return "", err
	}
	forms, err := s.client.ListForms(ctx, pub)
	if err != nil {
		return "", err
	}
	existing, found := resolveExportTarget(forms, fmap, u.id, "", 0)
	if !found {
		return "missing", nil
	}
	names := make([]string, 0, len(upload.Files))
	for _, ff := range upload.Files {
		names = append(names, ff.Name)
	}
	upload.PrincipalFile = fluig.ChoosePrincipalFile(names, u.id, existing.Description)
	if _, err := s.client.UpdateForm(ctx, pub, existing.DocumentID, existing.CardDescription,
		existing.Description, existing.DatasetName, fluig.VersionKeep, upload); err != nil {
		return "", err
	}
	return "updated", nil
}

// updateWorkflow atualiza cirurgicamente o script salvo (fluiggersWidget) na
// última versão do processo — a atualização não gera bump de versão.
func (s *watchSession) updateWorkflow(ctx context.Context, u watchUnit) (string, error) {
	if s.helperOK == nil {
		ok, err := s.client.HelperInstalled(ctx)
		if err != nil {
			return "", err
		}
		s.helperOK = &ok
	}
	if !*s.helperOK {
		return "no-helper", nil
	}
	processID, scripts, err := resolveWorkflowTargets(s.root, u.path, nil, false)
	if err != nil {
		return "", err
	}
	events, err := readWorkflowEvents(scripts)
	if err != nil {
		return "", err
	}
	version, err := s.client.WorkflowVersion(ctx, processID)
	if err != nil {
		return "", err
	}
	if version == 0 {
		return "missing", nil
	}
	if _, err := s.client.UpdateWorkflowEvents(ctx, processID, version, events); err != nil {
		return "", err
	}
	return "updated", nil
}

// updateExisting atualiza um artefato de arquivo único no servidor. Retorna
// "updated", "unchanged" (conteúdo idêntico, nada gravado) ou "missing" (não
// existe — o watch não cria).
func (a *App) updateExisting(ctx context.Context, client *fluig.Client, t diffTarget, content string) (string, error) {
	switch t.typ {
	case "dataset":
		loaded, err := client.LoadDataset(ctx, t.id)
		if errors.Is(err, fluig.ErrNotFound) {
			return "missing", nil
		}
		if err != nil {
			return "", err
		}
		if normalizeEOL(loaded.Impl) == normalizeEOL(content) {
			return "unchanged", nil
		}
		return "updated", client.UpdateDataset(ctx, loaded, content)

	case "event":
		existing, err := client.ListGlobalEvents(ctx)
		if err != nil {
			return "", err
		}
		for i := range existing {
			if existing[i].ID != t.id {
				continue
			}
			if normalizeEOL(existing[i].Code) == normalizeEOL(content) {
				return "unchanged", nil
			}
			existing[i].Code = content
			return "updated", client.SaveGlobalEvents(ctx, existing)
		}
		return "missing", nil

	case "mechanism":
		mechs, err := client.ListMechanisms(ctx)
		if err != nil {
			return "", err
		}
		for i := range mechs {
			if mechs[i].ID != t.id {
				continue
			}
			if normalizeEOL(mechs[i].Code) == normalizeEOL(content) {
				return "unchanged", nil
			}
			return "updated", client.UpdateMechanism(ctx, &mechs[i], content)
		}
		return "missing", nil
	}
	return "", output.Genericf("tipo de artefato desconhecido: %q", t.typ)
}

// hashPath devolve o sha256 do conteúdo: de um arquivo, ou de uma pasta
// inteira (nomes relativos + conteúdo, em ordem estável).
func hashPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if !info.IsDir() {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	var files []string
	err = filepath.WalkDir(path, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, f := range files {
		rel, _ := filepath.Rel(path, f)
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
