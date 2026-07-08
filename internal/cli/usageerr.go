package cli

import (
	"fmt"
	"regexp"
	"strings"
)

// Padrões das mensagens de erro de uso geradas pelo cobra/pflag (em inglês).
var (
	reUnknownCmd   = regexp.MustCompile(`^unknown command "(.*)" for "(.*)"$`)
	reInvalidArg   = regexp.MustCompile(`^invalid argument "(.*)" for "(.*)" flag: (.*)$`)
	reAcceptsRange = regexp.MustCompile(`^accepts between (\d+) and (\d+) arg\(s\), received (\d+)$`)
	reAcceptsMax   = regexp.MustCompile(`^accepts at most (\d+) arg\(s\), received (\d+)$`)
	reAccepts      = regexp.MustCompile(`^accepts (\d+) arg\(s\), received (\d+)$`)
	reRequiresMin  = regexp.MustCompile(`^requires at least (\d+) arg\(s\), only received (\d+)$`)
	reRequiresMax  = regexp.MustCompile(`^requires at most (\d+) arg\(s\), only received (\d+)$`)
)

// translateUsageMessage traduz para pt-BR as mensagens de erro de uso do
// cobra/pflag. Nomes de comandos e flags seguem em inglês; só o
// texto explicativo é traduzido. Mensagens não reconhecidas passam intactas.
func translateUsageMessage(msg string) string {
	switch {
	case strings.HasPrefix(msg, "unknown flag: "):
		return "flag desconhecida: " + strings.TrimPrefix(msg, "unknown flag: ")
	case strings.HasPrefix(msg, "unknown shorthand flag: "):
		return "flag curta desconhecida: " + strings.TrimPrefix(msg, "unknown shorthand flag: ")
	case strings.HasPrefix(msg, "flag needs an argument: "):
		return "a flag precisa de um valor: " + strings.TrimPrefix(msg, "flag needs an argument: ")
	}
	if m := reUnknownCmd.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("comando desconhecido: %q (use %q)", m[1], m[2]+" --help")
	}
	if m := reInvalidArg.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("valor inválido %q para a flag %q: %s", m[1], m[2], m[3])
	}
	if m := reAcceptsRange.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("aceita entre %s e %s argumento(s), recebeu %s", m[1], m[2], m[3])
	}
	if m := reAcceptsMax.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("aceita no máximo %s argumento(s), recebeu %s", m[1], m[2])
	}
	if m := reAccepts.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("aceita %s argumento(s), recebeu %s", m[1], m[2])
	}
	if m := reRequiresMin.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("requer ao menos %s argumento(s), recebeu %s", m[1], m[2])
	}
	if m := reRequiresMax.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("requer no máximo %s argumento(s), recebeu %s", m[1], m[2])
	}
	return msg
}
