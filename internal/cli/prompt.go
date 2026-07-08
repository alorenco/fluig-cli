package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/alorenco/fluig-cli/internal/output"
)

var stdinReader = bufio.NewReader(os.Stdin)

// promptLine pergunta um valor em texto; def é usado quando o usuário só dá Enter.
func promptLine(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	line, err := stdinReader.ReadString('\n')
	if err != nil {
		return "", output.Genericf("falha ao ler a entrada: %v", err)
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return def, nil
	}
	return value, nil
}

// promptInt pergunta um número inteiro.
func promptInt(label string, def int) (int, error) {
	value, err := promptLine(label, strconv.Itoa(def))
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, output.Usagef("valor inválido: %q (esperado um número)", value)
	}
	return n, nil
}

// promptYesNo pergunta sim/não (s/n).
func promptYesNo(label string, def bool) (bool, error) {
	hint := "s/N"
	if def {
		hint = "S/n"
	}
	value, err := promptLine(fmt.Sprintf("%s (%s)", label, hint), "")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(value) {
	case "":
		return def, nil
	case "s", "sim", "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// promptPassword lê a senha com o eco do terminal desligado.
func promptPassword(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", output.Genericf("falha ao ler a senha: %v", err)
	}
	return string(pw), nil
}

// confirm pede confirmação para ações destrutivas, respeitando --yes e o modo
// não-interativo.
func (a *App) confirm(question string) error {
	if a.Yes {
		return nil
	}
	if !a.Interactive() {
		return output.Usagef("%s — confirme com --yes em modo não-interativo", question)
	}
	ok, err := promptYesNo(question, false)
	if err != nil {
		return err
	}
	if !ok {
		return output.Usagef("operação cancelada pelo usuário")
	}
	return nil
}
