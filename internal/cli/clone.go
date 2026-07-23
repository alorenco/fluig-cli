package cli

import (
	"context"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/config"
	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
	"github.com/alorenco/fluig-cli/internal/project"
)

// cloneTypeDef descreve um tipo de artefato clonável (ordem = ordem canônica
// de exibição e execução).
type cloneTypeDef struct {
	key   string // nome da flag/seleção (inglês, plural)
	label string // rótulo humano pt-BR
}

var cloneTypeDefs = []cloneTypeDef{
	{"forms", "Formulários"},
	{"datasets", "Datasets customizados"},
	{"workflows", "Processos (scripts de eventos)"},
	{"events", "Eventos globais"},
	{"mechanisms", "Mecanismos de atribuição"},
	{"widgets", "Widgets"},
}

func cloneKeys() []string {
	keys := make([]string, len(cloneTypeDefs))
	for i, d := range cloneTypeDefs {
		keys[i] = d.key
	}
	return keys
}

func cloneLabel(key string) string {
	for _, d := range cloneTypeDefs {
		if d.key == key {
			return d.label
		}
	}
	return key
}

// cloneInventory é o resultado do discovery: tudo o que o servidor tem de
// cada tipo, buscado uma vez e reusado na importação.
type cloneInventory struct {
	userCode  string
	forms     []fluig.Form
	datasets  []fluig.DatasetSummary // já filtrado: só os customizados
	processes []fluig.ProcessSummary
	events    []fluig.GlobalEvent
	mechs     []fluig.Mechanism
	widgets   []fluig.Widget
	helper    bool // fluigcliHelper instalado (condição para widgets)
}

func (inv *cloneInventory) count(key string) int {
	switch key {
	case "forms":
		return len(inv.forms)
	case "datasets":
		return len(inv.datasets)
	case "workflows":
		return len(inv.processes)
	case "events":
		return len(inv.events)
	case "mechanisms":
		return len(inv.mechs)
	case "widgets":
		return len(inv.widgets)
	}
	return 0
}

func (inv *cloneInventory) available(key string) bool {
	return key != "widgets" || inv.helper
}

// counts monta o mapa tipo → quantidade do envelope (widgets só quando o
// helper permite contá-los).
func (inv *cloneInventory) counts() map[string]int {
	m := make(map[string]int, len(cloneTypeDefs))
	for _, d := range cloneTypeDefs {
		if !inv.available(d.key) {
			continue
		}
		m[d.key] = inv.count(d.key)
	}
	return m
}

func newCloneCmd(app *App) *cobra.Command {
	var (
		all           bool
		only          []string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "clone [--all | --only <tipos>]",
		Short: "Clona os artefatos do servidor para o projeto local (onboarding)",
		Long: "Clona os artefatos do servidor para o projeto local — o onboarding de quem\n" +
			"chega a uma instância existente com uma pasta vazia. Consulta o servidor,\n" +
			"mostra o inventário de cada tipo e importa os selecionados (a mesma\n" +
			"semântica do import de cada grupo). Tipos:\n\n" +
			"  forms       formulários (forms/<pasta>/, com anexos e eventos)\n" +
			"  datasets    datasets customizados (datasets/<id>.js)\n" +
			"  workflows   scripts de eventos dos processos (workflow/scripts/) —\n" +
			"              o diagrama do processo fica no servidor\n" +
			"  events      eventos globais (events/<id>.js)\n" +
			"  mechanisms  mecanismos de atribuição (mechanisms/<id>.js)\n" +
			"  widgets     widgets (wcm/widget/<code>/) — requer o fluigcliHelper;\n" +
			"              widget SPA vem como o bundle publicado, sem o fonte\n\n" +
			"Sem flags (em terminal interativo), pergunta o que clonar após mostrar o\n" +
			"inventário. Em modo não-interativo use --all ou --only. Re-executar\n" +
			"sobrescreve os arquivos locais — commite antes. Páginas, comunidades,\n" +
			"parâmetros e documentos do GED ficam fora do escopo.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			onlyTokens := splitCSV(only)
			if all && len(onlyTokens) > 0 {
				return output.Usagef("use --only ou --all, não os dois")
			}
			var selected []string
			if len(onlyTokens) > 0 {
				keys, err := parseCloneTypes(onlyTokens)
				if err != nil {
					return err
				}
				selected = keys
			}
			if !all && selected == nil && !app.Interactive() {
				return output.Usagef("informe o que clonar: --all ou --only <tipos> (tipos: %s)",
					strings.Join(cloneKeys(), ", "))
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

			p.Infof("Consultando o servidor %s…", server.Name)
			inv, err := app.cloneDiscover(ctx, client)
			if err != nil {
				return err
			}
			printCloneInventory(p, inv, server.Name)
			p.Infof("Projeto destino: %s", root)

			switch {
			case all:
				for _, d := range cloneTypeDefs {
					if d.key == "widgets" && !inv.helper {
						p.Warnf("widgets pulados: fluigcliHelper não instalado — instale com `fluigcli server install-helper %s` e rode `fluigcli clone --only widgets`", server.Name)
						continue
					}
					selected = append(selected, d.key)
				}
			case selected != nil:
				// --only explícito: pedir widgets sem o helper é erro (exit 7).
			default:
				selected, err = promptCloneSelection(inv)
				if err != nil {
					return err
				}
			}
			for _, key := range selected {
				if key == "widgets" && !inv.helper {
					return output.MissingHelperf("widgets exigem o componente fluigcliHelper — instale com: fluigcli server install-helper %s", server.Name)
				}
			}

			results := map[string][]itemResult{}
			var lastErr error
			failures, total := 0, 0
			for _, key := range selected {
				n := inv.count(key)
				if n == 0 {
					continue // já apareceu zerado no inventário
				}
				p.Infof("Clonando %s (%d)…", cloneLabel(key), n)
				r, f, e := app.cloneRun(ctx, client, server, root, key, inv)
				results[key] = r
				failures += f
				total += len(r)
				if e != nil {
					lastErr = e
				}
			}
			if total == 0 {
				p.Infof("Nada para clonar.")
			}

			data := map[string]any{
				"root":      root,
				"selected":  selected,
				"available": inv.counts(),
				"results":   results,
			}
			if !inv.helper {
				data["unavailable"] = map[string]string{"widgets": "fluigcliHelper não instalado"}
			}
			return finishBatch(p, lastErr, data, failures, total)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "clona todos os tipos disponíveis, sem perguntar")
	cmd.Flags().StringSliceVar(&only, "only", nil, "clona só os tipos informados (ex.: forms,datasets)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// cloneDiscover busca o inventário completo do servidor (uma listagem por
// tipo); as listas são reusadas na fase de importação.
func (a *App) cloneDiscover(ctx context.Context, client *fluig.Client) (*cloneInventory, error) {
	inv := &cloneInventory{}
	var err error
	if inv.userCode, err = client.ResolveUserCode(ctx); err != nil {
		return nil, mapFluigError(err)
	}
	if inv.forms, err = client.ListForms(ctx, inv.userCode); err != nil {
		return nil, mapFluigError(err)
	}
	datasets, err := client.ListDatasets(ctx)
	if err != nil {
		return nil, mapFluigError(err)
	}
	for _, d := range datasets {
		if d.Custom {
			inv.datasets = append(inv.datasets, d)
		}
	}
	if inv.processes, err = client.ListProcesses(ctx); err != nil {
		return nil, mapFluigError(err)
	}
	if inv.events, err = client.ListGlobalEvents(ctx); err != nil {
		return nil, mapFluigError(err)
	}
	if inv.mechs, err = client.ListMechanisms(ctx); err != nil {
		return nil, mapFluigError(err)
	}
	if inv.helper, err = client.HelperInstalled(ctx); err != nil {
		return nil, mapFluigError(err)
	}
	if inv.helper {
		if inv.widgets, err = client.ListWidgets(ctx); err != nil {
			return nil, mapFluigError(err)
		}
	}
	return inv, nil
}

// printCloneInventory mostra a tabela de discovery (só no modo humano — em
// JSON as contagens vão no envelope).
func printCloneInventory(p *output.Printer, inv *cloneInventory, serverName string) {
	rows := make([][]string, 0, len(cloneTypeDefs))
	for i, d := range cloneTypeDefs {
		count := strconv.Itoa(inv.count(d.key))
		obs := ""
		if !inv.available(d.key) {
			count = "—"
			obs = "requer o fluigcliHelper (fluigcli server install-helper " + serverName + ")"
		}
		rows = append(rows, []string{strconv.Itoa(i + 1), d.key, d.label, count, obs})
	}
	p.Table(output.Table{
		Headers: []string{"#", "Tipo", "Descrição", "Itens", "Obs."},
		Rows:    rows,
		Style:   output.BoldHeaderStyle(nil),
	})
}

// promptCloneSelection pergunta os tipos no modo interativo. Enter = todos os
// disponíveis (helper presente e contagem > 0).
func promptCloneSelection(inv *cloneInventory) ([]string, error) {
	line, err := promptLine("Tipos a clonar (números ou nomes separados por vírgula; Enter = todos os disponíveis)", "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		var keys []string
		for _, d := range cloneTypeDefs {
			if inv.available(d.key) && inv.count(d.key) > 0 {
				keys = append(keys, d.key)
			}
		}
		return keys, nil
	}
	return parseCloneTypes(splitCSV([]string{line}))
}

// parseCloneTypes normaliza tokens (chave, singular ou número da tabela) para
// as chaves canônicas, deduplicando e preservando a ordem canônica.
func parseCloneTypes(tokens []string) ([]string, error) {
	set := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		key, ok := matchCloneType(tok)
		if !ok {
			return nil, output.Usagef("tipo desconhecido: %q (tipos: %s)", tok, strings.Join(cloneKeys(), ", "))
		}
		set[key] = true
	}
	var out []string
	for _, d := range cloneTypeDefs {
		if set[d.key] {
			out = append(out, d.key)
		}
	}
	return out, nil
}

func matchCloneType(tok string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(tok))
	if n, err := strconv.Atoi(t); err == nil {
		if n >= 1 && n <= len(cloneTypeDefs) {
			return cloneTypeDefs[n-1].key, true
		}
		return "", false
	}
	for _, d := range cloneTypeDefs {
		if t == d.key || t == strings.TrimSuffix(d.key, "s") {
			return d.key, true
		}
	}
	return "", false
}

// cloneRun importa todos os itens de UM tipo, reusando os mesmos executores
// dos comandos import de cada grupo.
func (a *App) cloneRun(ctx context.Context, client *fluig.Client, server *config.Server, root, key string, inv *cloneInventory) (results []itemResult, failures int, lastErr error) {
	p := a.printer
	fail := func(id, kind string, err error) {
		failures++
		lastErr = mapFluigError(err)
		results = append(results, itemResult{ID: id, Action: "failed", Success: false, Error: output.AsError(lastErr).Message})
		p.Warnf("%s %q: %s", kind, id, output.AsError(lastErr).Message)
	}
	switch key {
	case "forms":
		fmap, err := project.LoadFormMap(root, server.FormScopeKey())
		if err != nil {
			fail("forms", "formulários", output.Genericf("falha ao ler .fluigcli/forms.json: %v", err))
			return results, failures, lastErr
		}
		for _, f := range inv.forms {
			folder := resolveImportFolder(fmap, f, "")
			if err := a.importOneForm(ctx, client, inv.userCode, root, folder, f); err != nil {
				fail(f.Description, "formulário", err)
				continue
			}
			fmap.Upsert(project.FormLink{Folder: folder, DocumentID: f.DocumentID, Name: f.Description, DatasetName: f.DatasetName})
			results = append(results, itemResult{ID: f.Description, Action: "imported", Success: true})
			p.Successf("formulário %q importado em forms/%s", f.Description, folder)
		}
		saveFormMap(p, fmap)
	case "datasets":
		for _, d := range inv.datasets {
			action, err := a.importOneDataset(ctx, client, root, d.ID)
			if err != nil {
				fail(d.ID, "dataset", err)
				continue
			}
			results = append(results, itemResult{ID: d.ID, Action: action, Success: true})
			p.Successf("dataset %q %s", d.ID, action)
		}
	case "workflows":
		for _, pr := range inv.processes {
			r, f, e := a.importProcessScripts(ctx, client, root, pr.ID, nil)
			results = append(results, r...)
			failures += f
			if e != nil {
				lastErr = e
			}
		}
	case "events":
		for _, ev := range inv.events {
			action, err := writeArtifactFile(a, root, project.EventsDirName, ev.ID, ev.Code)
			if err != nil {
				fail(ev.ID, "evento", err)
				continue
			}
			results = append(results, itemResult{ID: ev.ID, Action: action, Success: true})
			p.Successf("evento %q %s", ev.ID, action)
		}
	case "mechanisms":
		for _, m := range inv.mechs {
			action, err := writeArtifactFile(a, root, project.MechanismsDirName, m.ID, m.Code)
			if err != nil {
				fail(m.ID, "mecanismo", err)
				continue
			}
			results = append(results, itemResult{ID: m.ID, Action: action, Success: true})
			p.Successf("mecanismo %q %s", m.ID, action)
		}
	case "widgets":
		for _, w := range inv.widgets {
			if err := a.importOneWidget(ctx, client, root, w); err != nil {
				fail(w.Code, "widget", err)
				continue
			}
			results = append(results, itemResult{ID: w.Code, Action: "imported", Success: true})
			p.Successf("widget %q importado em wcm/widget/%s", w.Code, w.Code)
		}
	}
	return results, failures, lastErr
}
