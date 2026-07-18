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
		follow        bool
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Mostra as últimas entradas do log (com filtros e --follow)",
		Long: "Mostra as últimas entradas do server.log (ou de outro arquivo, com --file).\n" +
			"Uma ENTRADA é a linha com timestamp + as continuações dela — um stack trace\n" +
			"inteiro conta como uma entrada só e vem completo.\n\n" +
			"--level filtra por severidade mínima (--level warn = WARN, ERROR e FATAL);\n" +
			"--grep filtra por substring (sem diferenciar maiúsculas); --follow segue\n" +
			"acompanhando o arquivo (como tail -f; Ctrl+C para sair).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			if follow && app.JSON {
				return output.Usagef("--follow é um modo contínuo e não suporta --json (use tail sem --follow)")
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
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "segue acompanhando o log (como tail -f)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
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
