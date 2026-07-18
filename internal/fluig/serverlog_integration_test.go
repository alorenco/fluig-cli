//go:build integration

package fluig

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Leitura dos logs do servidor (read-only) contra a homologação. Requer o
// fluigcliHelper ≥ 0.3.0 instalado.
func TestIntegrationServerLogs(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	files, err := c.ListServerLogs(ctx)
	if err != nil {
		if errors.Is(err, ErrHelperMissing) || errors.Is(err, ErrHelperOutdated) {
			t.Skipf("fluigcliHelper indisponível: %v", err)
		}
		t.Fatalf("ListServerLogs: %v", err)
	}
	var hasServerLog bool
	for _, f := range files {
		if f.Name == DefaultServerLog {
			hasServerLog = true
			if f.Size == 0 || f.LastModified == nil {
				t.Errorf("server.log sem tamanho/data: %+v", f)
			}
		}
	}
	t.Logf("%d arquivos no diretório de log", len(files))
	if !hasServerLog {
		t.Fatal("server.log não apareceu na listagem")
	}

	tail, err := c.TailServerLog(ctx, ServerLogTailOptions{Lines: 5})
	if err != nil {
		t.Fatalf("TailServerLog: %v", err)
	}
	if len(tail.Entries) == 0 || tail.Size == 0 {
		t.Fatalf("tail vazio: %+v", tail)
	}
	t.Logf("tail: %d entradas, arquivo com %d bytes", len(tail.Entries), tail.Size)

	// Acompanhamento: ler do offset devolvido pelo tail não pode falhar.
	chunk, err := c.ReadServerLog(ctx, DefaultServerLog, tail.Size)
	if err != nil {
		t.Fatalf("ReadServerLog: %v", err)
	}
	if chunk.Size < tail.Size {
		t.Errorf("arquivo encolheu entre tail (%d) e read (%d)?", tail.Size, chunk.Size)
	}

	// Download em streaming.
	var sb strings.Builder
	n, err := c.DownloadServerLog(ctx, DefaultServerLog, &sb)
	if err != nil {
		t.Fatalf("DownloadServerLog: %v", err)
	}
	if n == 0 || sb.Len() == 0 {
		t.Error("download vazio")
	}
	t.Logf("download: %d bytes", n)

	// Inexistente: 404 → ErrNotFound (não ErrHelperOutdated, pois a versão
	// do helper tem as rotas).
	if _, err := c.TailServerLog(ctx, ServerLogTailOptions{File: "zz-fluigcli-nao-existe.log"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("tail de arquivo inexistente deveria dar ErrNotFound, veio %v", err)
	}
}
