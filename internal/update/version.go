// Package update descobre, baixa e instala novas versões do fluigcli a partir
// das releases do GitHub. Não conhece cobra nem faz I/O de terminal — quem
// decide o que imprimir é a CLI.
package update

import (
	"strconv"
	"strings"
)

// Compare compara duas versões semânticas ("0.2.0", "v0.2.0", "1.0.0-rc1").
// Retorna negativo se a < b, zero se iguais e positivo se a > b. Versões
// vazias ou não numéricas contam como 0.
func Compare(a, b string) int {
	aCore, aPre := splitVersion(a)
	bCore, bPre := splitVersion(b)
	for i := 0; i < 3; i++ {
		if d := numAt(aCore, i) - numAt(bCore, i); d != 0 {
			return d
		}
	}
	// Núcleos iguais: release final vence pré-release (1.0.0 > 1.0.0-rc1).
	switch {
	case aPre == "" && bPre != "":
		return 1
	case aPre != "" && bPre == "":
		return -1
	default:
		return strings.Compare(aPre, bPre)
	}
}

// IsNewer informa se latest é mais nova que a versão em execução. Builds de
// desenvolvimento ("dev" ou vazio) nunca são consideradas desatualizadas.
func IsNewer(latest, current string) bool {
	if current == "" || current == "dev" {
		return false
	}
	return Compare(latest, current) > 0
}

func splitVersion(v string) (core, pre string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ = strings.Cut(v, "-")
	return core, pre
}

func numAt(core string, i int) int {
	parts := strings.Split(core, ".")
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}
