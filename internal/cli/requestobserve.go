package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/alorenco/fluig-cli/internal/fluig"
	"github.com/alorenco/fluig-cli/internal/output"
)

// Observação em solicitação SEM movimentar (helper >= 0.11.0). O caso de uso
// que motivou: um agente registra um laudo no histórico da solicitação e a
// deixa na mesma etapa, para a pessoa responsável decidir.

// --- request observe ---

func newRequestObserveCmd(app *App) *cobra.Command {
	var (
		text          string
		textFile      string
		state         int
		movement      int
		thread        int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "observe <número>",
		Short: "Registra uma observação na solicitação sem movimentá-la (via fluigcliHelper)",
		Long: "Registra uma observação no histórico da solicitação e a deixa na MESMA\n" +
			"etapa. É o comentário sem move: a REST do Fluig não tem essa operação e o\n" +
			"SOAP exige a senha do usuário. Por isso o caminho é o fluigcliHelper\n" +
			">= 0.11.0 (instale ou atualize com: fluigcli server install-helper).\n\n" +
			"O texto vem de --text ou de --text-file (arquivo ou \"-\" para o stdin, o\n" +
			"modo natural para um laudo em HTML). HTML simples é permitido. O teto é\n" +
			"8.000 caracteres. A observação sai em nome do usuário autenticado.\n\n" +
			"Sem --state e --movement, a observação vai para a tarefa corrente. Se a\n" +
			"solicitação tem tarefas paralelas, o servidor exige os dois: veja as\n" +
			"tarefas com `fluigcli request show <número>`. Solicitação finalizada ou\n" +
			"cancelada não aceita observação (exit 5 com o motivo).\n\n" +
			"Exemplos:\n" +
			"  fluigcli request observe 235189 --text \"Laudo: documentação conferida.\"\n" +
			"  cat laudo.html | fluigcli request observe 235189 --text-file - --json\n" +
			"  fluigcli request observe 235189 --text \"ok\" --state 34 --movement 5",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			id, err := strconv.Atoi(args[0])
			if err != nil || id <= 0 {
				return output.Usagef("número de solicitação inválido %q", args[0])
			}
			if (state == 0) != (movement == 0) {
				return output.Usagef("--state e --movement andam juntos: informe os dois ou nenhum")
			}
			body, err := observationText(text, textFile)
			if err != nil {
				return err
			}
			ctx := context.Background()
			_, client, err := app.connectWrite(ctx, passwordStdin, "registrar observação")
			if err != nil {
				return err
			}
			obs, err := client.CreateRequestObservation(ctx, id, fluig.ObservationOptions{
				Text: body, StateSequence: state, MovementSequence: movement, ThreadSequence: thread,
			})
			if err != nil {
				return mapFluigError(explainObservationConflict(err, id))
			}
			p.Successf("observação %d registrada na solicitação %d (etapa %d, movimento %d) em nome de %s. A solicitação não foi movimentada.",
				obs.ID, obs.ProcessInstanceID, obs.StateSequence, obs.MovementSequence, obs.ColleagueID)
			p.Done(map[string]any{"observation": obs})
			return nil
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "texto da observação (HTML simples permitido; até 8.000 caracteres)")
	cmd.Flags().StringVar(&textFile, "text-file", "", `arquivo com o texto da observação ("-" lê do stdin)`)
	cmd.Flags().IntVar(&state, "state", 0, "etapa (stateSequence) da tarefa; default = tarefa corrente (exige --movement)")
	cmd.Flags().IntVar(&movement, "movement", 0, "movimento (movementSequence) da tarefa; default = tarefa corrente (exige --state)")
	cmd.Flags().IntVar(&thread, "thread", 0, "thread da tarefa (default 0 = fluxo principal)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// observationText resolve o texto de --text ou --text-file (arquivo ou "-").
// Exige exatamente uma das duas fontes e recusa texto vazio antes de chamar o
// servidor.
func observationText(text, textFile string) (string, error) {
	switch {
	case text != "" && textFile != "":
		return "", output.Usagef("use --text OU --text-file, não os dois")
	case text == "" && textFile == "":
		return "", output.Usagef("informe o texto com --text ou --text-file (\"-\" lê do stdin)")
	}
	body := text
	if textFile != "" {
		var raw []byte
		var err error
		if textFile == "-" {
			raw, err = io.ReadAll(os.Stdin)
		} else {
			raw, err = os.ReadFile(textFile)
		}
		if err != nil {
			return "", output.Usagef("--text-file: %v", err)
		}
		body = string(raw)
	}
	if strings.TrimSpace(body) == "" {
		return "", output.Usagef("o texto da observação está vazio")
	}
	return body, nil
}

// explainObservationConflict completa o 409 de paralelismo com o caminho na
// CLI: quais flags passar e onde ver as tarefas. Os demais conflitos
// (finalizada, cancelada, sem tarefa ativa) já vêm explicados pelo helper.
func explainObservationConflict(err error, id int) error {
	var conflict *fluig.RequestConflictError
	if !errors.As(err, &conflict) {
		return err
	}
	switch {
	case strings.Contains(conflict.Message, "paralelas"):
		return fmt.Errorf("%w. Informe --state e --movement da tarefa desejada (veja: fluigcli request show %d)", err, id)
	case strings.Contains(conflict.Message, "informe stateSequence"):
		// GET sem etapa numa solicitação encerrada: não há tarefa corrente.
		return fmt.Errorf("%w (--state; veja as etapas com: fluigcli request show %d)", err, id)
	}
	return err
}

// --- request observations ---

func newRequestObservationsCmd(app *App) *cobra.Command {
	var (
		state         int
		thread        int
		passwordStdin bool
	)
	cmd := &cobra.Command{
		Use:   "observations <número>",
		Short: "Lista as observações de uma etapa da solicitação (via fluigcliHelper)",
		Long: "Lista as observações registradas numa etapa da solicitação, da mais antiga\n" +
			"para a mais recente. É a conferência do `request observe`. Requer o\n" +
			"fluigcliHelper >= 0.11.0.\n\n" +
			"Sem --state, mostra a etapa da tarefa corrente. Solicitação finalizada ou\n" +
			"com tarefas paralelas exige --state (veja: fluigcli request show <número>).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := app.printerFor(cmd)
			id, err := strconv.Atoi(args[0])
			if err != nil || id <= 0 {
				return output.Usagef("número de solicitação inválido %q", args[0])
			}
			ctx := context.Background()
			_, client, err := app.connect(ctx, passwordStdin)
			if err != nil {
				return err
			}
			list, err := client.ListRequestObservations(ctx, id, state, thread)
			if err != nil {
				return mapFluigError(explainObservationConflict(err, id))
			}
			if len(list) == 0 {
				p.Infof("Nenhuma observação nesta etapa da solicitação %d. Registre uma com: fluigcli request observe %d --text \"...\"", id, id)
			} else {
				rows := make([][]string, 0, len(list))
				for _, o := range list {
					rows = append(rows, []string{
						fmtObservationTime(o.ObservationDate),
						o.ColleagueID,
						strconv.Itoa(o.StateSequence),
						strconv.Itoa(o.MovementSequence),
						observationPreview(o.Observation, 80),
					})
				}
				p.Table(output.Table{
					Headers: []string{"Data", "Autor", "Etapa", "Mov.", "Observação"},
					Rows:    rows,
					Style:   output.BoldHeaderStyle(nil),
				})
			}
			p.Done(map[string]any{"processInstanceId": id, "count": len(list), "observations": list})
			return nil
		},
	}
	cmd.Flags().IntVar(&state, "state", 0, "etapa (stateSequence); default = tarefa corrente")
	cmd.Flags().IntVar(&thread, "thread", 0, "thread da tarefa (default 0 = fluxo principal)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "lê a senha do stdin")
	return cmd
}

// fmtObservationTime reduz o ISO do helper ("2026-09-17T14:03:11.000-04:00")
// para a hora local do servidor na tabela; texto inesperado sai como veio.
func fmtObservationTime(iso string) string {
	for _, layout := range []string{"2006-01-02T15:04:05.000Z07:00", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, iso); err == nil {
			return t.Format("2006-01-02 15:04")
		}
	}
	return iso
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// observationPreview tira as tags HTML, junta o espaço em branco e corta em
// max caracteres para a célula da tabela. O --json leva o texto íntegro.
func observationPreview(html string, max int) string {
	plain := htmlTagRe.ReplaceAllString(html, " ")
	plain = strings.Join(strings.Fields(plain), " ")
	if utf8.RuneCountInString(plain) <= max {
		return plain
	}
	runes := []rune(plain)
	return strings.TrimSpace(string(runes[:max-1])) + "…"
}
