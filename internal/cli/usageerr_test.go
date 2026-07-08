package cli

import "testing"

func TestTranslateUsageMessage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"unknown flag: --xyz", "flag desconhecida: --xyz"},
		{"unknown shorthand flag: 'z' in -z", "flag curta desconhecida: 'z' in -z"},
		{"flag needs an argument: --server", "a flag precisa de um valor: --server"},
		{`unknown command "foo" for "fluigcli"`, `comando desconhecido: "foo" (use "fluigcli --help")`},
		{"accepts 1 arg(s), received 2", "aceita 1 argumento(s), recebeu 2"},
		{"accepts at most 1 arg(s), received 3", "aceita no máximo 1 argumento(s), recebeu 3"},
		{"requires at least 1 arg(s), only received 0", "requer ao menos 1 argumento(s), recebeu 0"},
		{`invalid argument "x" for "--timeout" flag: time: invalid duration "x"`, `valor inválido "x" para a flag "--timeout": time: invalid duration "x"`},
		{"mensagem desconhecida qualquer", "mensagem desconhecida qualquer"},
	}
	for _, tc := range cases {
		if got := translateUsageMessage(tc.in); got != tc.want {
			t.Errorf("translateUsageMessage(%q)\n  = %q\n  quer %q", tc.in, got, tc.want)
		}
	}
}
