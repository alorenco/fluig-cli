package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

func newLogFilesCmd(app *App) *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Lista os arquivos do diretório de log do servidor",
		Args:  cobra.NoArgs,
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
			if len(files) == 0 {
				p.Infof("Nenhum arquivo no diretório de log do servidor.")
			} else {
				rows := make([][]string, 0, len(files))
				for _, f := range files {
					rows = append(rows, []string{f.Name, fmtLogSize(f.Size), fmtRequestTime(f.LastModified)})
				}
				// Padrão de listagem (ver CLAUDE.md): o log corrente (default
				// do tail/download) em verde.
				p.Table(output.Table{
					Headers: []string{"Arquivo", "Tamanho", "Modificado"},
					Rows:    rows,
					Style: output.BoldHeaderStyle(func(row, col int, padded string) string {
						if col == 0 && files[row].Name == fluig.DefaultServerLog {
							return output.Green(padded)
						}
						return padded
					}),
				})
			}
			p.Done(map[string]any{"files": files})
			return nil
		},
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// --- log tail ---

func newLogTailCmd(app *App) *cobra.Command {
	var (
		file          string
		lines         int
		skip          int
		level         string
		grep          string
		since         string
		until         string
		follow        bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Mostra as últimas entradas do log (com filtros, janela de tempo e --follow)",
		Long: "Mostra as últimas entradas do server.log (ou de outro arquivo, com --file).\n" +
			"Uma ENTRADA é a linha com timestamp + as continuações dela — um stack trace\n" +
			"inteiro conta como uma entrada só e vem completo.\n\n" +
			"--level filtra por severidade mínima (--level warn = WARN, ERROR e FATAL);\n" +
			"--grep filtra por substring (sem diferenciar maiúsculas); --follow segue\n" +
			"acompanhando o arquivo (como tail -f; Ctrl+C para sair).\n\n" +
			"--since/--until buscam uma JANELA DE TEMPO em vez das últimas entradas.\n" +
			"Formatos: duração (30m, 2h), hora de hoje (18:19), data (2026-07-24) ou\n" +
			"data e hora (2026-07-24T18:19). Os horários são os do LOG, ou seja, a hora\n" +
			"local do servidor — a CLI usa o fuso que o fluigcliHelper informa para\n" +
			"resolver as durações e as horas de hoje.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if follow && app.JSON {
				return output.Usagef("--follow é um modo contínuo e não suporta --json (use tail sem --follow)")
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
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
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
			for _, entry := range tail.Entries {
				printLogEntry(p, entry)
			}
			if tail.Truncated {
				p.Warnf("saída truncada pelo limite de tamanho — refine com --grep/--level ou reduza -n")
			}
			if !follow {
				p.Done(map[string]any{
					"file": tail.File, "size": tail.Size,
					"entries": tail.Entries, "truncated": tail.Truncated,
				})
				return nil
			}
			return followServerLog(ctx, p, client, tail.File, tail.Size, lv, grep)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "arquivo de log (default: server.log; veja log files)")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "número de entradas (stack trace conta como uma)")
	cmd.Flags().IntVar(&skip, "skip", 0, "pula as N entradas mais recentes (paginação para trás)")
	cmd.Flags().StringVar(&level, "level", "", "severidade mínima: trace, debug, info, warn, error ou fatal")
	cmd.Flags().StringVar(&grep, "grep", "", "filtra por substring (case-insensitive, entrada completa)")
	cmd.Flags().StringVar(&since, "since", "", "início da janela: duração (30m), hora de hoje (18:19), data ou data e hora")
	cmd.Flags().StringVar(&until, "until", "", "fim da janela (mesmos formatos do --since)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "segue acompanhando o log (como tail -f)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// runLogRange atende o `log tail --since/--until`: busca a janela fechada pela
// rota /range do helper (a mesma que o painel do `dev` usa).
func runLogRange(ctx context.Context, app *App, p *output.Printer, client *fluig.Client, file, since, until, level, grep string) error {
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
		"entries": res.Entries, "truncated": res.Truncated,
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

const logFollowInterval = 2 * time.Second

// followServerLog acompanha o arquivo por polling de offset. Rotação
// (tamanho menor que o offset) recomeça do zero; erros transitórios são
// tolerados até um limite de falhas consecutivas.
func followServerLog(ctx context.Context, p *output.Printer, client *fluig.Client, file string, offset int64, level, grep string) error {
	p.Infof("— acompanhando %s (Ctrl+C para sair) —", file)
	filter := newLogLineFilter(level, grep)
	failures := 0
	for {
		time.Sleep(logFollowInterval)
		chunk, err := client.ReadServerLog(ctx, file, offset)
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
			p.Infof("— arquivo rotacionado, recomeçando do início —")
			offset = 0
			continue
		}
		offset = chunk.To
		if chunk.Content == "" {
			continue
		}
		for _, line := range strings.Split(strings.TrimSuffix(chunk.Content, "\n"), "\n") {
			if filter.match(line) {
				printLogLine(p, line)
			}
		}
	}
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
		switch logLineLevel(line) {
		case 4, 5: // ERROR/FATAL
			line = output.Red(line)
		case 3: // WARN
			line = output.Yellow(line)
		}
	}
	p.Successf("%s", line)
}

var logEntryStartRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[ T]`)

var logLevelRanks = map[string]int{
	"TRACE": 0, "FINEST": 0, "FINER": 0,
	"DEBUG": 1, "FINE": 1,
	"INFO": 2, "CONFIG": 2,
	"WARN": 3, "WARNING": 3,
	"ERROR": 4, "SEVERE": 4,
	"FATAL": 5,
}

// logLineLevel devolve o nível de uma linha de cabeçalho (-1 = sem nível).
func logLineLevel(line string) int {
	tokens := strings.Fields(line)
	for i := 0; i < len(tokens) && i < 4; i++ {
		if rank, ok := logLevelRanks[strings.ToUpper(tokens[i])]; ok {
			return rank
		}
	}
	return -1
}

// logLineFilter aplica --level/--grep no cliente durante o --follow: a decisão
// é tomada na linha de cabeçalho da entrada e herdada pelas continuações
// (o stack trace acompanha o ERROR que o abriu).
type logLineFilter struct {
	minLevel     int // -1 = sem filtro
	grep         string
	entryMatched bool
}

func newLogLineFilter(level, grep string) *logLineFilter {
	f := &logLineFilter{minLevel: -1, grep: strings.ToLower(grep), entryMatched: true}
	if level != "" {
		if rank, ok := logLevelRanks[strings.ToUpper(level)]; ok {
			f.minLevel = rank
		}
	}
	return f
}

func (f *logLineFilter) match(line string) bool {
	if f.minLevel < 0 && f.grep == "" {
		return true
	}
	if logEntryStartRe.MatchString(line) {
		f.entryMatched = true
		if f.minLevel >= 0 && logLineLevel(line) < f.minLevel {
			f.entryMatched = false
		}
		if f.entryMatched && f.grep != "" && !strings.Contains(strings.ToLower(line), f.grep) {
			f.entryMatched = false
		}
	}
	return f.entryMatched
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
