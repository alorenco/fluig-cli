package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"generic", errors.New("qualquer"), ExitGeneric},
		{"usage", Usagef("faltou argumento"), ExitUsage},
		{"auth", AuthFailedf("senha errada"), ExitAuth},
		{"not found", NotFoundf("sumiu"), ExitNotFound},
		{"server", ServerErrorf("deploy rejeitado"), ExitServer},
		{"partial", Partialf("2 de 5 falharam"), ExitPartial},
		{"helper", MissingHelperf("instale a widget"), ExitMissingHelper},
		{"wrapped", &wrapper{AuthFailedf("interno")}, ExitAuth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCodeFor(tc.err); got != tc.want {
				t.Errorf("ExitCodeFor(%v) = %d, quer %d", tc.err, got, tc.want)
			}
		})
	}
}

type wrapper struct{ err error }

func (w *wrapper) Error() string { return w.err.Error() }
func (w *wrapper) Unwrap() error { return w.err }

func decodeEnvelope(t *testing.T, buf *bytes.Buffer) Envelope {
	t.Helper()
	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("stdout não é um JSON válido: %v\n%s", err, buf.String())
	}
	return env
}

func TestPrinterJSONSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := &Printer{JSON: true, Command: "server test", Server: "homolog", Stdout: &stdout, Stderr: &stderr}

	p.Successf("ignorado em modo json")
	p.Infof("vai para o stderr")
	p.Done(map[string]string{"status": "ok"})

	env := decodeEnvelope(t, &stdout)
	if !env.OK || env.Command != "server test" || env.Server != "homolog" || env.Error != nil {
		t.Errorf("envelope inesperado: %+v", env)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("vai para o stderr")) {
		t.Errorf("Infof deveria ir para o stderr em modo JSON; stderr=%q", stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("ignorado")) {
		t.Errorf("Successf não pode poluir o stdout em modo JSON")
	}
}

func TestPrinterJSONFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := &Printer{JSON: true, Command: "server test", Stdout: &stdout, Stderr: &stderr}

	exit := p.Fail(AuthFailedf("usuário ou senha inválidos"))
	if exit != ExitAuth {
		t.Errorf("exit = %d, quer %d", exit, ExitAuth)
	}
	env := decodeEnvelope(t, &stdout)
	if env.OK || env.Error == nil || env.Error.Code != CodeAuthFailed {
		t.Errorf("envelope de erro inesperado: %+v", env)
	}
}

// O contrato exige exatamente um documento JSON no stdout.
func TestPrinterEmitsSingleEnvelope(t *testing.T) {
	var stdout bytes.Buffer
	p := &Printer{JSON: true, Command: "x", Stdout: &stdout, Stderr: &bytes.Buffer{}}
	p.Done("primeiro")
	p.Done("segundo")
	p.Fail(Genericf("depois do done"))

	dec := json.NewDecoder(&stdout)
	var env Envelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("primeiro documento inválido: %v", err)
	}
	if dec.More() {
		t.Errorf("stdout contém mais de um documento JSON")
	}
}

func TestPrinterHumanFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := &Printer{JSON: false, Command: "server test", Stdout: &stdout, Stderr: &stderr}

	exit := p.Fail(NotFoundf("servidor %q não encontrado", "homolog"))
	if exit != ExitNotFound {
		t.Errorf("exit = %d, quer %d", exit, ExitNotFound)
	}
	if stdout.Len() != 0 {
		t.Errorf("erro humano deve ir para o stderr, stdout=%q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("não encontrado")) {
		t.Errorf("mensagem pt-BR ausente no stderr: %q", stderr.String())
	}
}
