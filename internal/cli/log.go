package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

func newLogCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Logs do servidor Fluig: listar, acompanhar e baixar (via fluigcliHelper)",
		Long: "Lê os logs do servidor de aplicação do Fluig (server.log e rotacionados)\n" +
			"remotamente, sem acesso SSH — requer o componente auxiliar fluigcliHelper\n" +
			"≥ 0.3.0 (instale/atualize com: fluigcli server install-helper).",
	}
	cmd.AddCommand(newLogFilesCmd(app))
	cmd.AddCommand(newLogTailCmd(app))
	cmd.AddCommand(newLogDownloadCmd(app))
	return cmd
}

// --- log files ---

// logFilesDefaultLimit limita a listagem no caso comum. O diretório de log do
// Fluig é grande (622 arquivos na homologação, sendo 395 CSVs de monitoramento),
// e quem lista quer escolher um arquivo para o tail/download — quase sempre um
// dos mais recentes. O limite é explícito: a CLI diz quantos ficaram de fora.
const logFilesDefaultLimit = 20

func newLogFilesCmd(app *App) *cobra.Command {
	var (
		all           bool
		pattern       string
		limit         int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Lista os arquivos do diretório de log do servidor",
		Long: "Lista os arquivos do diretório de log, do mais recente para o mais antigo.\n\n" +
			"Por padrão a listagem traz só os ARQUIVOS DE LOG (server.log, as rotações e\n" +
			"os outros *.log) e no máximo 20 deles. O diretório do Fluig também guarda\n" +
			"centenas de CSVs de monitoramento, que poluem a leitura. A CLI informa\n" +
			"quantos arquivos ficaram de fora — nada é omitido em silêncio.\n\n" +
			"Use --all para ver tudo, --pattern para filtrar por nome (ex.: 'chrono.log*')\n" +
			"e --limit 0 para não limitar.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			files, err := client.ListServerLogs(ctx)
			if err != nil {
				return mapFluigError(err)
			}
			total := len(files)
			if total == 0 {
				p.Infof("Nenhum arquivo no diretório de log do servidor.")
				p.Done(map[string]any{"files": []fluig.ServerLogFile{}, "total": 0, "omitted": 0})
				return nil
			}

			selected, err := selectLogFiles(files, pattern, all)
			if err != nil {
				return err
			}
			sortLogFilesByRecency(selected)
			shown := selected
			if limit > 0 && len(shown) > limit {
				shown = shown[:limit]
			}

			if len(shown) == 0 {
				p.Infof("Nenhum arquivo de log casa com o filtro (%d arquivos no diretório; use --all).", total)
			} else {
				rows := make([][]string, 0, len(shown))
				for _, f := range shown {
					rows = append(rows, []string{f.Name, fmtLogSize(f.Size), fmtRequestTime(f.LastModified)})
				}
				// Padrão de listagem (ver CLAUDE.md): o log corrente (default
				// do tail/download) em verde.
				p.Table(output.Table{
					Headers: []string{"Arquivo", "Tamanho", "Modificado"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 0 && shown[row].Name == fluig.DefaultServerLog {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			if omitted := total - len(shown); omitted > 0 {
				p.Infof("%d de %d arquivos — %d fora da listagem. Use --all, --pattern <glob> ou --limit 0.",
					len(shown), total, omitted)
			}
			p.Done(map[string]any{
				"files": shown, "total": total, "omitted": total - len(shown),
			})
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "lista todos os arquivos do diretório, inclusive os CSVs de monitoramento")
	cmd.Flags().StringVar(&pattern, "pattern", "", "filtra por nome (glob, ex.: 'server.log*')")
	cmd.Flags().IntVar(&limit, "limit", logFilesDefaultLimit, "número máximo de arquivos (0 = todos)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// isLogFileName reconhece arquivo de log: `*.log` ou uma rotação `*.log.<algo>`.
// Não confundir com os CSVs de monitoramento que têm "log" no meio do nome
// (ex.: FreemarkerPageRenderer.renderWidget.logcontrol.csv).
func isLogFileName(name string) bool {
	return strings.HasSuffix(name, ".log") || strings.Contains(name, ".log.")
}

// selectLogFiles aplica o filtro da listagem: um --pattern explícito manda (o
// usuário sabe o que quer); sem ele, só arquivos de log, a menos que --all.
func selectLogFiles(files []fluig.ServerLogFile, pattern string, all bool) ([]fluig.ServerLogFile, error) {
	if pattern != "" {
		if _, err := filepath.Match(pattern, "teste"); err != nil {
			return nil, output.Usagef("--pattern inválido %q: %v", pattern, err)
		}
	}
	out := make([]fluig.ServerLogFile, 0, len(files))
	for _, f := range files {
		switch {
		case pattern != "":
			if ok, _ := filepath.Match(pattern, f.Name); !ok {
				continue
			}
		case !all && !isLogFileName(f.Name):
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// sortLogFilesByRecency ordena do mais recente para o mais antigo. É a ordem
// útil: quem lista quer o log corrente e as últimas rotações. Arquivo sem data
// vai para o fim, com desempate por nome (ordem estável).
func sortLogFilesByRecency(files []fluig.ServerLogFile) {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i].LastModified, files[j].LastModified
		switch {
		case a == nil && b == nil:
			return files[i].Name < files[j].Name
		case a == nil:
			return false
		case b == nil:
			return true
		case a.Equal(*b):
			return files[i].Name < files[j].Name
		}
		return a.After(*b)
	})
}

// --- log tail ---

func newLogTailCmd(app *App) *cobra.Command {
	var (
		file          string
		lines         int
		skip          int
		level         string
		grep          []string
		since         string
		until         string
		follow        bool
		ndjson        bool
		untilMatch    string
		forDuration   time.Duration
		idleTimeout   time.Duration
		maxEntries    int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Mostra as últimas entradas do log (com filtros, janela de tempo e --follow)",
		Long: "Mostra as últimas entradas do server.log (ou de outro arquivo, com --file).\n" +
			"Uma ENTRADA é a linha com timestamp + as continuações dela — um stack trace\n" +
			"inteiro conta como uma entrada só e vem completo.\n\n" +
			"--level filtra por severidade mínima (--level warn = WARN, ERROR e FATAL);\n" +
			"--grep filtra por substring (sem diferenciar maiúsculas). Repita o --grep\n" +
			"para procurar VÁRIOS textos: a entrada passa se casar com qualquer um deles\n" +
			"(OU). O filtro roda no servidor e exige o fluigcliHelper >= 0.8.0 quando há\n" +
			"mais de um padrão. --follow segue acompanhando o arquivo (como tail -f;\n" +
			"Ctrl+C para sair).\n\n" +
			"--since/--until buscam uma JANELA DE TEMPO em vez das últimas entradas.\n" +
			"Formatos: duração (30m, 2h), hora de hoje (18:19), data (2026-07-24) ou\n" +
			"data e hora (2026-07-24T18:19). Os horários são os do LOG, ou seja, a hora\n" +
			"local do servidor — a CLI usa o fuso que o fluigcliHelper informa para\n" +
			"resolver as durações e as horas de hoje.\n\n" +
			"Para MONITOR de agente, use --follow --ndjson: cada linha do stdout é um\n" +
			"JSON completo de uma entrada. É formato de streaming, diferente do envelope\n" +
			"único do --json. Combine com uma condição de parada (--until-match, --for,\n" +
			"--idle-timeout ou --max-entries) para o comando terminar sozinho em vez de\n" +
			"esperar Ctrl+C. Com --until-match, terminar sem casar é exit 4.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if follow && app.JSON {
				return output.Usagef("--follow é um modo contínuo e não suporta --json (use --follow --ndjson para streaming, ou tail sem --follow)")
			}
			if ndjson && app.JSON {
				return output.Usagef("--ndjson e --json são formatos diferentes: o --json emite um envelope único e o --ndjson um objeto por linha")
			}
			// As condições de parada e o streaming só existem no modo contínuo.
			for flag, ligada := range map[string]bool{
				"ndjson": ndjson, "until-match": untilMatch != "",
				"for": forDuration > 0, "idle-timeout": idleTimeout > 0, "max-entries": maxEntries > 0,
			} {
				if ligada && !follow {
					return output.Usagef("--%s só faz sentido com --follow (é o modo contínuo)", flag)
				}
			}
			ranged := since != "" || until != ""
			if ranged && follow {
				return output.Usagef("--since/--until buscam uma janela fechada e não combinam com --follow")
			}
			if ranged {
				for _, f := range []string{"lines", "skip"} {
					if cmd.Flags().Changed(f) {
						return output.Usagef("--%s conta as últimas entradas e não se aplica a --since/--until (a janela define o recorte)", f)
					}
				}
			}
			lv, err := normalizeEnum("--level", level, "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL")
			if err != nil {
				return err
			}
			if ranged {
				// Recortar uma janela num server.log grande é leitura pesada: o
				// servidor varre o arquivo inteiro para achar o intervalo.
				app.raiseReadTimeout()
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			// Vários padrões dependem do servidor: filtrar no cliente perderia
			// entradas em silêncio quando a resposta vem truncada.
			if len(grep) > 1 {
				if err := client.EnsureMultiGrep(ctx); err != nil {
					return mapFluigError(err)
				}
			}
			if ranged {
				return runLogRange(ctx, app, p, client, file, since, until, lv, grep)
			}
			tail, err := client.TailServerLog(ctx, fluig.ServerLogTailOptions{
				File: file, Lines: lines, Skip: skip, Level: lv, Grep: grep,
			})
			if err != nil {
				return mapFluigError(err)
			}
			if len(tail.Entries) == 0 && !follow {
				p.Infof("Nenhuma entrada de log casa com os filtros em %s.", tail.File)
			}
			// Em NDJSON o backlog NÃO sai: o stdout é exclusivo do stream (uma
			// linha de texto quebraria quem consome) e, mais importante, o
			// --until-match tem de valer para o que acontecer DEPOIS do início —
			// casar com uma entrada antiga daria falso positivo ao monitor.
			if !ndjson {
				for _, entry := range tail.Entries {
					printLogEntry(p, entry)
				}
			}
			if tail.Truncated && !ndjson {
				p.Warnf("saída truncada pelo limite de tamanho — refine com --grep/--level ou reduza -n")
			}
			if !follow {
				p.Done(map[string]any{
					"file": tail.File, "size": tail.Size,
					"entries": tail.Entries, "records": fluig.ParseLogEntries(tail.Entries),
					"truncated": tail.Truncated,
				})
				return nil
			}
			return followServerLog(ctx, p, client, followParams{
				file: tail.File, offset: tail.Size, level: lv, grep: grep,
				ndjson: ndjson, untilMatch: untilMatch,
				forDuration: forDuration, idleTimeout: idleTimeout, maxEntries: maxEntries,
			})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "arquivo de log (default: server.log; veja log files)")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "número de entradas (stack trace conta como uma)")
	cmd.Flags().IntVar(&skip, "skip", 0, "pula as N entradas mais recentes (paginação para trás)")
	cmd.Flags().StringVar(&level, "level", "", "severidade mínima: trace, debug, info, warn, error ou fatal")
	cmd.Flags().StringArrayVar(&grep, "grep", nil, "filtra por substring (case-insensitive, entrada completa; repita para OU — exige helper ≥ 0.8.0)")
	cmd.Flags().StringVar(&since, "since", "", "início da janela: duração (30m), hora de hoje (18:19), data ou data e hora")
	cmd.Flags().StringVar(&until, "until", "", "fim da janela (mesmos formatos do --since)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "segue acompanhando o log (como tail -f)")
	cmd.Flags().BoolVar(&ndjson, "ndjson", false, "com --follow: um JSON por linha (streaming para monitor de agente)")
	cmd.Flags().StringVar(&untilMatch, "until-match", "", "com --follow: encerra (exit 0) na primeira entrada que contém o texto")
	cmd.Flags().DurationVar(&forDuration, "for", 0, "com --follow: acompanha por no máximo esse tempo (ex.: 5m)")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 0, "com --follow: encerra depois desse tempo sem entrada nova")
	cmd.Flags().IntVar(&maxEntries, "max-entries", 0, "com --follow: encerra depois de emitir N entradas")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// runLogRange atende o `log tail --since/--until`: busca a janela fechada pela
// rota /range do helper (a mesma que o painel do `dev` usa).
func runLogRange(ctx context.Context, app *App, p *output.Printer, client *fluig.Client, file, since, until, level string, grep []string) error {
	now, zoneKnown := serverNow(ctx, client)
	from, fromUsedNow, err := resolveLogBound("--since", since, false, now)
	if err != nil {
		return err
	}
	to, toUsedNow, err := resolveLogBound("--until", until, true, now)
	if err != nil {
		return err
	}
	// Sem o fuso do servidor (helper < 0.4.0), duração e hora de hoje saem do
	// relógio local — o que erra a janela se as máquinas estiverem em fusos
	// diferentes. Avisa em vez de fingir precisão.
	if !zoneKnown && (fromUsedNow || toUsedNow) {
		p.Warnf("o fluigcliHelper não informou o fuso do servidor — a janela foi calculada com o horário desta máquina")
	}

	res, err := client.RangeServerLog(ctx, fluig.ServerLogRangeOptions{
		File: file, From: from, To: to, Level: level, Grep: grep,
	})
	if err != nil {
		return app.mapErr(err)
	}
	if len(res.Entries) == 0 {
		p.Infof("Nenhuma entrada de log entre %s e %s em %s.", boundLabel(from, "o início do arquivo"), boundLabel(to, "o fim do arquivo"), res.File)
	}
	for _, entry := range res.Entries {
		printLogEntry(p, entry)
	}
	if res.Truncated {
		p.Warnf("saída truncada pelo limite de tamanho — estreite a janela ou refine com --grep/--level")
	}
	p.Done(map[string]any{
		"file": res.File, "from": from, "to": to,
		"entries": res.Entries, "records": fluig.ParseLogEntries(res.Entries),
		"truncated": res.Truncated,
	})
	return nil
}

// serverNow devolve o "agora" na hora local do SERVIDOR. O timestamp do log não
// tem offset, então a janela precisa ser expressa no fuso do servidor — que o
// fluigcliHelper informa a partir da 0.4.0. Sem essa informação, devolve a hora
// local desta máquina e false.
func serverNow(ctx context.Context, client *fluig.Client) (time.Time, bool) {
	info, err := client.HelperStatus(ctx)
	if err != nil || info.ZoneOffsetMinutes == nil {
		return time.Now(), false
	}
	zone := info.ZoneID
	if zone == "" {
		zone = "servidor"
	}
	return time.Now().In(time.FixedZone(zone, *info.ZoneOffsetMinutes*60)), true
}

// Formato que a rota /range espera: hora local do servidor, sem offset. Com 16
// caracteres (sem segundos) o helper completa :00 no início e :59 no fim.
const (
	logBoundLayoutMinute = "2006-01-02T15:04"
	logBoundLayoutSecond = "2006-01-02T15:04:05"
)

// resolveLogBound traduz o valor de --since/--until para o formato da API.
// Aceita duração (30m), hora de hoje (18:19[:05]), data (2026-07-24) e data e
// hora (2026-07-24T18:19[:05], com T ou espaço). Vazio = sem limite naquele
// lado. `end` distingue o fim da janela: numa data sem hora, o fim é o dia
// inteiro. O segundo retorno diz se o valor dependeu do relógio (`now`) — é o
// caso em que o fuso do servidor importa.
func resolveLogBound(flag, value string, end bool, now time.Time) (string, bool, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", false, nil
	}
	if d, err := time.ParseDuration(v); err == nil {
		if d <= 0 {
			return "", false, output.Usagef("%s: a duração %q precisa ser positiva (a janela olha para trás, ex.: 30m)", flag, v)
		}
		return now.Add(-d).Format(logBoundLayoutSecond), true, nil
	}
	// Hora de hoje: HH:MM[:SS] — "hoje" é o dia do SERVIDOR.
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, v); err == nil {
			return now.Format("2006-01-02") + "T" + t.Format(layout), true, nil
		}
	}
	// Data sem hora: o início é 00:00 e o fim é o dia inteiro.
	if t, err := time.Parse("2006-01-02", v); err == nil {
		hora := "T00:00"
		if end {
			hora = "T23:59"
		}
		return t.Format("2006-01-02") + hora, false, nil
	}
	// Data e hora completas: repassa normalizando o separador.
	normalized := strings.Replace(v, " ", "T", 1)
	for _, layout := range []string{logBoundLayoutSecond, logBoundLayoutMinute} {
		if t, err := time.Parse(layout, normalized); err == nil {
			return t.Format(layout), false, nil
		}
	}
	return "", false, output.Usagef("%s: valor %q não reconhecido — use duração (30m, 2h), hora de hoje (18:19), "+
		"data (2026-07-24) ou data e hora (2026-07-24T18:19)", flag, value)
}

// boundLabel descreve um limite vazio na mensagem humana.
func boundLabel(bound, vazio string) string {
	if bound == "" {
		return vazio
	}
	return bound
}

// logFollowInterval é o intervalo do polling do --follow. É variável para os
// testes poderem encurtá-lo (o valor real é o de produção).
var logFollowInterval = 2 * time.Second

// followParams reúne o que o modo contínuo precisa: de onde ler, o que filtrar,
// como emitir e quando parar.
type followParams struct {
	file   string
	offset int64
	level  string
	grep   []string

	ndjson      bool          // um JSON por linha em vez de texto
	untilMatch  string        // encerra na primeira entrada que contém o texto
	forDuration time.Duration // teto de tempo total
	idleTimeout time.Duration // teto de tempo sem entrada nova
	maxEntries  int           // teto de entradas emitidas
}

// hasStopCondition informa se o comando termina sozinho.
func (fp followParams) hasStopCondition() bool {
	return fp.untilMatch != "" || fp.forDuration > 0 || fp.idleTimeout > 0 || fp.maxEntries > 0
}

// notice manda um recado humano pelo canal certo. Em NDJSON o stdout é
// EXCLUSIVO dos dados — uma linha que não seja JSON quebraria quem consome o
// stream —, então o recado vai para o stderr.
func (fp followParams) notice(p *output.Printer, format string, args ...any) {
	if fp.ndjson {
		p.Warnf(format, args...)
		return
	}
	p.Infof(format, args...)
}

// logEntryAccumulator junta as LINHAS que chegam do polling em ENTRADAS
// completas. Uma entrada só termina quando a próxima começa (ou quando o log
// silencia), porque o stack trace vem em linhas de continuação.
type logEntryAccumulator struct{ pending []string }

// add devolve as entradas fechadas pelas linhas novas.
func (a *logEntryAccumulator) add(lines []string) []string {
	var done []string
	for _, line := range lines {
		if fluig.IsLogEntryStart(line) && len(a.pending) > 0 {
			done = append(done, strings.Join(a.pending, "\n"))
			a.pending = nil
		}
		a.pending = append(a.pending, line)
	}
	return done
}

// flush fecha a entrada pendente (o log silenciou ou o comando vai terminar).
func (a *logEntryAccumulator) flush() string {
	if len(a.pending) == 0 {
		return ""
	}
	entry := strings.Join(a.pending, "\n")
	a.pending = nil
	return entry
}

// followServerLog acompanha o arquivo por polling de offset. Rotação
// (tamanho menor que o offset) recomeça do zero; erros transitórios são
// tolerados até um limite de falhas consecutivas.
//
// O acompanhamento é por ENTRADA, não por linha: assim o stack trace nunca sai
// partido e os filtros valem para a entrada inteira. O custo é a latência da
// última entrada, que espera a próxima ou o silêncio do log (um ciclo de poll).
func followServerLog(ctx context.Context, p *output.Printer, client *fluig.Client, fp followParams) error {
	switch {
	case fp.ndjson:
		fp.notice(p, "acompanhando %s em NDJSON a partir de agora (uma entrada por linha; o histórico não entra no stream)", fp.file)
	case fp.hasStopCondition():
		fp.notice(p, "— acompanhando %s (encerra sozinho; Ctrl+C também sai) —", fp.file)
	default:
		fp.notice(p, "— acompanhando %s (Ctrl+C para sair) —", fp.file)
	}

	filter := newLogEntryFilter(fp.level, fp.grep)
	match := strings.ToLower(fp.untilMatch)
	acc := &logEntryAccumulator{}
	emitted := 0
	failures := 0
	start := time.Now()
	lastEntry := start
	offset := fp.offset

	// emit devolve true quando o --until-match casou (fim de festa, exit 0).
	emit := func(entry string) bool {
		if entry == "" || !filter.match(entry) {
			return false
		}
		emitted++
		lastEntry = time.Now()
		if fp.ndjson {
			writeNDJSON(p, entry)
		} else {
			printLogEntry(p, entry)
		}
		return match != "" && strings.Contains(strings.ToLower(entry), match)
	}

	for {
		time.Sleep(logFollowInterval)

		chunk, err := client.ReadServerLog(ctx, fp.file, offset)
		if err != nil {
			failures++
			if failures >= 5 {
				return mapFluigError(err)
			}
			p.Warnf("falha ao ler o log (%d/5): %v", failures, err)
			continue
		}
		failures = 0
		if chunk.Size < offset {
			fp.notice(p, "— arquivo rotacionado, recomeçando do início —")
			offset = 0
			continue
		}
		offset = chunk.To

		if chunk.Content == "" {
			// Log em silêncio: a entrada pendente já pode ser considerada
			// completa. É isto que garante latência limitada.
			if emit(acc.flush()) {
				return nil
			}
		} else {
			for _, entry := range acc.add(strings.Split(strings.TrimSuffix(chunk.Content, "\n"), "\n")) {
				if emit(entry) {
					return nil
				}
				if fp.maxEntries > 0 && emitted >= fp.maxEntries {
					return followStopped(p, fp, emitted, "limite de %d entradas atingido", fp.maxEntries)
				}
			}
		}

		if fp.maxEntries > 0 && emitted >= fp.maxEntries {
			return followStopped(p, fp, emitted, "limite de %d entradas atingido", fp.maxEntries)
		}
		if fp.forDuration > 0 && time.Since(start) >= fp.forDuration {
			emit(acc.flush())
			return followStopped(p, fp, emitted, "tempo de acompanhamento (%s) esgotado", fp.forDuration)
		}
		if fp.idleTimeout > 0 && time.Since(lastEntry) >= fp.idleTimeout {
			emit(acc.flush())
			return followStopped(p, fp, emitted, "nenhuma entrada nova por %s", fp.idleTimeout)
		}
	}
}

// followStopped encerra o acompanhamento por uma condição de parada. Com
// --until-match, terminar sem casar é resultado NEGATIVO: exit 4 (não
// encontrado), para o agente distinguir "vi o que esperava" de "desisti".
func followStopped(p *output.Printer, fp followParams, emitted int, motivo string, args ...any) error {
	razao := fmt.Sprintf(motivo, args...)
	if fp.untilMatch != "" {
		return output.NotFoundf("%s sem nenhuma entrada com %q (%d entradas vistas)", razao, fp.untilMatch, emitted)
	}
	fp.notice(p, "— %s (%d entradas) —", razao, emitted)
	return nil
}

// writeNDJSON emite uma entrada como um objeto JSON por linha. Este é o formato
// de STREAMING (não é o envelope do --json, que é único por execução — ver a
// regra do contrato de saída).
func writeNDJSON(p *output.Printer, entry string) {
	data, err := json.Marshal(fluig.ParseLogEntry(entry))
	if err != nil {
		p.Warnf("falha ao serializar a entrada de log: %v", err)
		return
	}
	fmt.Fprintln(p.Stdout, string(data))
}

// --- log download ---

func newLogDownloadCmd(app *App) *cobra.Command {
	var (
		file          string
		out           string
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Baixa um arquivo de log inteiro",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			name := file
			if name == "" {
				name = fluig.DefaultServerLog
			}
			dest := out
			if dest == "" {
				dest = name
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Clean(dest)), 0o755); err != nil {
				return output.Usagef("não foi possível criar o diretório de destino: %v", err)
			}
			f, err := os.Create(dest)
			if err != nil {
				return output.Usagef("não foi possível criar %s: %v", dest, err)
			}
			n, err := client.DownloadServerLog(ctx, name, f)
			closeErr := f.Close()
			if err != nil {
				os.Remove(dest)
				return mapFluigError(err)
			}
			if closeErr != nil {
				return output.ServerErrorf("falha ao gravar %s: %v", dest, closeErr)
			}
			p.Successf("Baixado %s → %s (%s)", name, dest, fmtLogSize(n))
			p.Done(map[string]any{"file": name, "path": dest, "size": n})
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "arquivo de log (default: server.log; veja log files)")
	cmd.Flags().StringVarP(&out, "output", "o", "", "caminho local de destino (default: nome do arquivo)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- helpers ---

// printLogEntry imprime uma entrada (possivelmente multi-linha) colorindo o
// cabeçalho pelo nível.
func printLogEntry(p *output.Printer, entry string) {
	for _, line := range strings.Split(entry, "\n") {
		printLogLine(p, line)
	}
}

func printLogLine(p *output.Printer, line string) {
	if output.ColorEnabled() {
		switch fluig.LineLevelRank(line) {
		case 4, 5: // ERROR/FATAL
			line = output.Red(line)
		case 3: // WARN
			line = output.Yellow(line)
		}
	}
	p.Successf("%s", line)
}

// logEntryFilter aplica --level/--grep no cliente durante o --follow. Agora a
// decisão é por ENTRADA completa: o nível sai da linha de cabeçalho e o texto do
// --grep é procurado na entrada inteira (inclusive dentro do stack trace) —
// mesma semântica dos filtros que o helper aplica no servidor.
type logEntryFilter struct {
	minLevel int      // -1 = sem filtro
	greps    []string // já em minúsculas; vários = OU (igual ao servidor)
}

func newLogEntryFilter(level string, greps []string) *logEntryFilter {
	f := &logEntryFilter{minLevel: -1}
	for _, g := range greps {
		if g != "" {
			f.greps = append(f.greps, strings.ToLower(g))
		}
	}
	if level != "" {
		f.minLevel = fluig.LogLevelRank(level)
	}
	return f
}

func (f *logEntryFilter) match(entry string) bool {
	if entry == "" {
		return false
	}
	if f.minLevel >= 0 {
		head, _, _ := strings.Cut(entry, "\n")
		if fluig.LineLevelRank(head) < f.minLevel {
			return false
		}
	}
	if len(f.greps) == 0 {
		return true
	}
	lower := strings.ToLower(entry)
	for _, g := range f.greps {
		if strings.Contains(lower, g) {
			return true
		}
	}
	return false
}

// fmtLogSize formata bytes para leitura humana.
func fmtLogSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
