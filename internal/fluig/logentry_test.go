package fluig

import (
	"encoding/json"
	"strings"
	"testing"
)

// O parse é sobre entradas REAIS da homologação/produção (fixture
// helper_log_tail_formatos.json, sanitizada).
func TestParseLogEntryFixture(t *testing.T) {
	var tail ServerLogTail
	if err := json.Unmarshal(testdata(t, "helper_log_tail_formatos.json"), &tail); err != nil {
		t.Fatal(err)
	}
	recs := ParseLogEntries(tail.Entries)
	if len(recs) != len(tail.Entries) {
		t.Fatalf("records=%d, entries=%d — o índice tem que casar", len(recs), len(tail.Entries))
	}

	// 1) INFO do WildFly, com o padding do %-5p e "--" no nome da thread.
	if got := recs[0]; got.Timestamp != "2026-07-18T15:39:31.759" || got.Level != "INFO" ||
		got.Logger != "org.wildfly.extension.undertow" ||
		got.Thread != "ServerService Thread Pool -- 191" ||
		!strings.HasPrefix(got.Message, "WFLYUT0021: Registered web context") || got.Stack != "" || got.Raw != "" {
		t.Errorf("entrada 1: %+v", got)
	}

	// 2) ERROR com stack: o cabeçalho vira campos e o resto vai para Stack.
	got := recs[1]
	if got.Level != "ERROR" || got.Logger != "com.fluig.auth.FluigAuthenticationMechanism" || got.Thread != "default task-22" {
		t.Errorf("entrada 2 (cabeçalho): %+v", got)
	}
	if !strings.HasPrefix(got.Message, "Session expired: com.totvs") {
		t.Errorf("entrada 2 (mensagem): %q", got.Message)
	}
	if !strings.HasPrefix(got.Stack, "\tat com.fluig.auth//") || !strings.Contains(got.Stack, "... 42 more") {
		t.Errorf("entrada 2 (stack): %q", got.Stack)
	}
	if strings.Contains(got.Stack, got.Message) {
		t.Errorf("o cabeçalho não deve se repetir dentro do stack: %q", got.Stack)
	}

	// 3) Log de script (o caso do dia a dia de quem depura dataset/evento).
	if got := recs[2]; got.Logger != "com.fluig.sdk.api.log.ScriptingLog" || !strings.Contains(got.Message, "TypeError") {
		t.Errorf("entrada 3: %+v", got)
	}

	// 4) Thread com parênteses dentro do nome (ActiveMQ) — o caso que quebra
	// um parse ingênuo.
	if got := recs[3]; got.Level != "WARN" || got.Thread != "Thread-2039 (ActiveMQ-client-global-threads)" ||
		got.Message != "Nenhum destinatário para o alerta 42" {
		t.Errorf("entrada 4: %+v", got)
	}

	// 5) Entrada sem cabeçalho (o helper degrada assim): tudo em Raw, nada
	// perdido e nenhum campo inventado.
	if got := recs[4]; got.Raw == "" || got.Timestamp != "" || got.Level != "" || got.Message != "" {
		t.Errorf("entrada 5 deveria vir só em Raw: %+v", got)
	}
}

func TestParseLogEntryVariacoes(t *testing.T) {
	cases := []struct {
		nome  string
		entry string
		quer  LogEntry
	}{
		{
			"separador ISO com T e ponto nos milissegundos",
			"2026-07-25T17:58:16.089 ERROR [x.y.Z] (task-1) falhou",
			LogEntry{Timestamp: "2026-07-25T17:58:16.089", Level: "ERROR", Logger: "x.y.Z", Thread: "task-1", Message: "falhou"},
		},
		{
			"sem logger nem thread",
			"2026-07-25 10:00:00,000 INFO  subiu",
			LogEntry{Timestamp: "2026-07-25T10:00:00.000", Level: "INFO", Message: "subiu"},
		},
		{
			"apelido do java.util.logging",
			"2026-07-25 10:00:00,000 SEVERE [a.B] (t) caiu",
			LogEntry{Timestamp: "2026-07-25T10:00:00.000", Level: "SEVERE", Logger: "a.B", Thread: "t", Message: "caiu"},
		},
		{
			"mensagem vazia não inventa conteúdo",
			"2026-07-25 10:00:00,000 INFO  [a.B] (t) ",
			LogEntry{Timestamp: "2026-07-25T10:00:00.000", Level: "INFO", Logger: "a.B", Thread: "t"},
		},
		{
			"linha solta vira Raw",
			"\tat java.base/java.lang.Thread.run(Thread.java:829)",
			LogEntry{Raw: "\tat java.base/java.lang.Thread.run(Thread.java:829)"},
		},
		// Regressão (2026-08-06): a mensagem em pt-BR quase sempre tem
		// parênteses. O parser antigo era guloso e devolvia só o pedaço depois
		// do último ")" — o começo do texto sumia sem aviso.
		{
			"mensagem com parênteses no meio",
			"2026-08-04 09:12:33,120 INFO  [ScriptingLog] (default task-10) FLUIGCLI-TESTE-A: linha com parenteses (1 item) e numero 999111",
			LogEntry{Timestamp: "2026-08-04T09:12:33.120", Level: "INFO", Logger: "ScriptingLog", Thread: "default task-10",
				Message: "FLUIGCLI-TESTE-A: linha com parenteses (1 item) e numero 999111"},
		},
		{
			"mensagem com parênteses, colchetes e dois-pontos",
			"2026-08-04 09:12:33,121 ERROR [ScriptingLog] (default task-10) Notificação [999111]: 1 anexo(s) movido(s) para a pasta 999222",
			LogEntry{Timestamp: "2026-08-04T09:12:33.121", Level: "ERROR", Logger: "ScriptingLog", Thread: "default task-10",
				Message: "Notificação [999111]: 1 anexo(s) movido(s) para a pasta 999222"},
		},
		{
			"mensagem terminada em parêntese (padrão dos datasets do Fluig)",
			"2026-08-04 09:12:33,122 ERROR [ScriptingLog] (default task-10) Erro ao mover: (Linha: 42)",
			LogEntry{Timestamp: "2026-08-04T09:12:33.122", Level: "ERROR", Logger: "ScriptingLog", Thread: "default task-10",
				Message: "Erro ao mover: (Linha: 42)"},
		},
		{
			"thread aninhada E mensagem com parênteses na mesma entrada",
			"2026-08-04 09:12:33,123 WARN  [a.B] (Thread-2039 (ActiveMQ-client-global-threads)) falhou o envio (tentativa 2)",
			LogEntry{Timestamp: "2026-08-04T09:12:33.123", Level: "WARN", Logger: "a.B",
				Thread: "Thread-2039 (ActiveMQ-client-global-threads)", Message: "falhou o envio (tentativa 2)"},
		},
		{
			"acentuação atravessa intacta (não é problema de encoding)",
			"2026-08-04 09:12:33,124 INFO  [ScriptingLog] (default task-10) acentuacao isolada -> ção ã õ é ú",
			LogEntry{Timestamp: "2026-08-04T09:12:33.124", Level: "INFO", Logger: "ScriptingLog", Thread: "default task-10",
				Message: "acentuacao isolada -> ção ã õ é ú"},
		},
		{
			"vazio não quebra",
			"", LogEntry{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			got := ParseLogEntry(tc.entry)
			if got != tc.quer {
				t.Errorf("\n got: %+v\nquer: %+v", got, tc.quer)
			}
		})
	}
}

func TestLogLevelRank(t *testing.T) {
	if LogLevelRank("warn") != LogLevelRank("WARNING") {
		t.Error("WARNING é apelido de WARN")
	}
	if LogLevelRank("ERROR") <= LogLevelRank("INFO") {
		t.Error("ERROR tem de ser mais severo que INFO")
	}
	if LogLevelRank("gigante") != -1 {
		t.Error("severidade desconhecida = -1")
	}
	if LineLevelRank("2026-07-25 10:00:00,000 ERROR [a] (t) x") != LogLevelRank("ERROR") {
		t.Error("LineLevelRank deveria achar o nível no cabeçalho")
	}
	if LineLevelRank("\tat java.base/java.lang.Thread.run") != -1 {
		t.Error("continuação não tem nível")
	}
	if !IsLogEntryStart("2026-07-25 10:00:00,000 INFO x") || IsLogEntryStart("\tat x") {
		t.Error("IsLogEntryStart")
	}
}

// ParseLogEntry só preenche Raw quando não reconhece — é o que distingue
// "não sei ler" de "li e está vazio".
func TestParseLogEntryRawApenasNoDesconhecido(t *testing.T) {
	if got := ParseLogEntry("2026-07-25 10:00:00,000 INFO  [a] (t) ok"); got.Raw != "" {
		t.Errorf("cabeçalho reconhecido não deveria ter Raw: %+v", got)
	}
	if got := ParseLogEntry("sem timestamp"); got.Raw != "sem timestamp" {
		t.Errorf("desconhecido deveria ir para Raw: %+v", got)
	}
}
