package fluig

import (
	"regexp"
	"strings"
)

// LogEntry é uma entrada do log do servidor já decomposta. O formato de origem é
// o padrão do WildFly (`%d{yyyy-MM-dd HH:mm:ss,SSS} %-5p [%c] (%t) %s%e%n`),
// mas o padrão é configurável por servidor: entrada que não casa vem em Raw,
// com os outros campos vazios. O parse NUNCA falha nem descarta conteúdo.
//
// ⚠️ Timestamp é a hora LOCAL DO SERVIDOR, sem fuso — o log não carrega offset.
// Quem precisa do fuso pergunta ao helper (HelperStatus.ZoneID).
type LogEntry struct {
	Timestamp string `json:"timestamp,omitempty"` // "2026-07-25T17:58:16.089"
	Level     string `json:"level,omitempty"`     // como veio no log (INFO, WARN, ERROR, SEVERE…)
	Logger    string `json:"logger,omitempty"`    // conteúdo dos [colchetes]
	Thread    string `json:"thread,omitempty"`    // conteúdo dos (parênteses)
	Message   string `json:"message,omitempty"`   // o resto da linha de cabeçalho
	Stack     string `json:"stack,omitempty"`     // linhas de continuação (stack trace), sem o cabeçalho
	Raw       string `json:"raw,omitempty"`       // preenchido SÓ quando o cabeçalho não foi reconhecido
}

// logHeaderRe casa o cabeçalho do WildFly. O nível pode vir com padding
// (`%-5p` gera "INFO " e "WARN "), o logger vem em colchetes e a thread em
// parênteses — a thread pode ter parênteses dentro (ex.: "Thread-2039
// (ActiveMQ-client-global-threads)"), por isso o grupo é guloso até o último
// ")" seguido de espaço.
var logHeaderRe = regexp.MustCompile(
	`^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})[,.](\d{3})\s+([A-Z]+)\s*` +
		`(?:\[([^\]]*)\]\s*)?` +
		`(?:\((.*)\)\s?)?` +
		`(.*)$`)

// logEntryStartRe reconhece o início de uma entrada (a linha com data). É o
// mesmo critério do helper — o resto são continuações da entrada anterior.
var logEntryStartRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[ T]`)

// IsLogEntryStart informa se a linha ABRE uma entrada de log.
func IsLogEntryStart(line string) bool { return logEntryStartRe.MatchString(line) }

// logLevelRanks ordena as severidades, incluindo os apelidos do java.util.logging
// (FINEST/FINE/CONFIG/SEVERE) que aparecem em log de componente antigo.
var logLevelRanks = map[string]int{
	"TRACE": 0, "FINEST": 0, "FINER": 0,
	"DEBUG": 1, "FINE": 1,
	"INFO": 2, "CONFIG": 2,
	"WARN": 3, "WARNING": 3,
	"ERROR": 4, "SEVERE": 4,
	"FATAL": 5,
}

// LogLevelRank devolve a ordem de uma severidade (-1 = desconhecida).
func LogLevelRank(level string) int {
	if rank, ok := logLevelRanks[strings.ToUpper(strings.TrimSpace(level))]; ok {
		return rank
	}
	return -1
}

// LineLevelRank devolve a severidade de uma linha de cabeçalho (-1 = sem
// severidade reconhecível). Olha só os primeiros tokens, onde o nível fica.
func LineLevelRank(line string) int {
	tokens := strings.Fields(line)
	for i := 0; i < len(tokens) && i < 4; i++ {
		if rank := LogLevelRank(tokens[i]); rank >= 0 {
			return rank
		}
	}
	return -1
}

// ParseLogEntry decompõe uma entrada. Cabeçalho irreconhecível devolve a
// entrada inteira em Raw — nada é descartado.
func ParseLogEntry(entry string) LogEntry {
	head, stack, _ := strings.Cut(entry, "\n")
	m := logHeaderRe.FindStringSubmatch(head)
	if m == nil {
		return LogEntry{Raw: entry}
	}
	return LogEntry{
		Timestamp: m[1] + "T" + m[2] + "." + m[3],
		Level:     m[4],
		Logger:    m[5],
		Thread:    m[6],
		Message:   strings.TrimSpace(m[7]),
		Stack:     stack,
	}
}

// ParseLogEntries decompõe uma lista de entradas preservando a ordem. O índice
// casa com o da lista de origem (entries[i] ↔ records[i]).
func ParseLogEntries(entries []string) []LogEntry {
	out := make([]LogEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, ParseLogEntry(e))
	}
	return out
}
