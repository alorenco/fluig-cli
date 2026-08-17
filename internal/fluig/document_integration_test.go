//go:build integration

package fluig

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// Integração do download do GED. Opt-in via FLUIGCLI_TEST_*. O teste cria uma
// pasta e um arquivo com prefixo zz_fluigcli_test_ e apaga tudo ao final.
//
// O ponto medido é o que o comando `document download` precisa para nomear o
// arquivo: o stream devolve o nome FÍSICO no Content-Disposition e o tipo no
// Content-Type. Medido na homologação e em produção em 2026-08-17: os dois
// headers vêm sempre, o nome sai entre aspas e o Content-Disposition aparece
// DUPLICADO na resposta (o Get pega o primeiro, que basta).
func TestIntegrationDownloadGEDDocumentHeaders(t *testing.T) {
	opts := integrationOptions(t)
	c, err := NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	raizes, err := c.ListGEDFolders(ctx, 0)
	if err != nil {
		t.Fatalf("ListGEDFolders: %v", err)
	}
	if len(raizes) == 0 {
		t.Skip("nenhuma pasta raiz no servidor")
	}

	pasta, err := c.CreateGEDFolder(ctx, raizes[0].ID, "zz_fluigcli_test_ged_dl")
	if err != nil {
		t.Fatalf("CreateGEDFolder: %v", err)
	}
	defer func() {
		if err := c.DeleteGEDDocument(ctx, int(pasta.ID)); err != nil {
			t.Errorf("limpeza da pasta %d: %v", pasta.ID, err)
		}
	}()

	const nomeArquivo = "zz_fluigcli_test_download.pdf"
	conteudo := []byte("%PDF-1.4 zz_fluigcli_test\n")
	doc, err := c.UploadGEDDocument(ctx, int(pasta.ID), nomeArquivo, conteudo)
	if err != nil {
		t.Fatalf("UploadGEDDocument: %v", err)
	}
	defer func() {
		if err := c.DeleteGEDDocument(ctx, int(doc.ID)); err != nil {
			t.Errorf("limpeza do documento %d: %v", doc.ID, err)
		}
	}()

	baixado, err := c.DownloadGEDDocument(ctx, int(doc.ID))
	if err != nil {
		t.Fatalf("DownloadGEDDocument: %v", err)
	}
	if !bytes.Equal(baixado.Content, conteudo) {
		t.Errorf("round-trip não é byte a byte: %q", baixado.Content)
	}
	t.Logf("fileName=%q mimeType=%q", baixado.FileName, baixado.MimeType)
	if baixado.FileName != nomeArquivo {
		t.Errorf("FileName = %q, quer %q (o Content-Disposition mudou?)", baixado.FileName, nomeArquivo)
	}
	if !strings.HasPrefix(baixado.MimeType, "application/pdf") {
		t.Errorf("MimeType = %q, quer application/pdf", baixado.MimeType)
	}
	// O nome tem de servir como nome de arquivo local sem ajuste.
	if filepath.Base(baixado.FileName) != baixado.FileName {
		t.Errorf("FileName tem separador de caminho: %q", baixado.FileName)
	}
}
