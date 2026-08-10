package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// escreve cria o arquivo relativo à raiz e devolve a raiz.
func projetoComArquivo(t *testing.T, rel, conteudo string) string {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A chave do baseline NÃO usa o número da linha: inserir linhas antes do achado
// não pode transformar dívida antiga em achado novo (ROADMAP §5.5).
func TestBaselineIgnoraNumeroDaLinha(t *testing.T) {
	root := projetoComArquivo(t, "datasets/ds.js", "var a = 1;\nif (x === 'y') {}\n")
	achado := Finding{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 2}
	base := NewBaseline(root, []Finding{achado})
	if len(base.Entries) != 1 || base.Entries[0].Count != 1 {
		t.Fatalf("entradas = %+v", base.Entries)
	}
	if base.Entries[0].Sample != "if (x === 'y') {}" {
		t.Errorf("sample = %q", base.Entries[0].Sample)
	}

	// O arquivo ganhou duas linhas antes: o achado agora está na linha 4.
	if err := os.WriteFile(filepath.Join(root, "datasets", "ds.js"),
		[]byte("// comentario novo\nvar b = 2;\nvar a = 1;\n    if (x === 'y') {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	movido := Finding{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 4}
	conhecidos, novos, resolvidos := base.Partition(root, []Finding{movido}, []string{"datasets/ds.js"})
	if len(conhecidos) != 1 || len(novos) != 0 {
		t.Errorf("conhecidos=%d novos=%d (a indentação e o número da linha não podem contar)", len(conhecidos), len(novos))
	}
	if !conhecidos[0].Baseline {
		t.Error("o achado conhecido não foi marcado com Baseline")
	}
	if resolvidos != 0 {
		t.Errorf("resolvidos=%d, quer 0", resolvidos)
	}
}

// Achado em linha de texto DIFERENTE é novo, mesmo na mesma regra e arquivo.
func TestBaselineTextoDiferenteEhNovo(t *testing.T) {
	root := projetoComArquivo(t, "datasets/ds.js", "if (a === 'x') {}\nif (b === 'y') {}\n")
	base := NewBaseline(root, []Finding{
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 1},
	})
	conhecidos, novos, _ := base.Partition(root, []Finding{
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 1},
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 2},
	}, []string{"datasets/ds.js"})
	if len(conhecidos) != 1 || len(novos) != 1 {
		t.Fatalf("conhecidos=%d novos=%d", len(conhecidos), len(novos))
	}
	if novos[0].Line != 2 {
		t.Errorf("o achado novo devia ser o da linha 2, veio %d", novos[0].Line)
	}
}

// Linhas de texto IDÊNTICO contam: o baseline segura só a quantidade gravada.
func TestBaselineContaOcorrenciasIdenticas(t *testing.T) {
	root := projetoComArquivo(t, "datasets/ds.js", "if (a === 'x') {}\nif (a === 'x') {}\nif (a === 'x') {}\n")
	base := NewBaseline(root, []Finding{
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 1},
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 2},
	})
	if base.Entries[0].Count != 2 {
		t.Fatalf("count = %d, quer 2", base.Entries[0].Count)
	}
	conhecidos, novos, _ := base.Partition(root, []Finding{
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 1},
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 2},
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 3},
	}, []string{"datasets/ds.js"})
	if len(conhecidos) != 2 || len(novos) != 1 {
		t.Errorf("conhecidos=%d novos=%d — a 3ª ocorrência tem de barrar", len(conhecidos), len(novos))
	}
}

// Dívida quitada é contada, mas só nos arquivos desta rodada.
func TestBaselineContaResolvidosSoDosArquivosAuditados(t *testing.T) {
	root := projetoComArquivo(t, "datasets/ds.js", "if (a === 'x') {}\n")
	base := NewBaseline(root, []Finding{
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 1},
		{Rule: RuleConstInLoop, Severity: SeverityError, File: "datasets/outro.js", Line: 1},
	})
	// Rodada só em ds.js, e o achado dele sumiu.
	_, _, resolvidos := base.Partition(root, nil, []string{"datasets/ds.js"})
	if resolvidos != 1 {
		t.Errorf("resolvidos=%d, quer 1 (o de outro.js não foi auditado)", resolvidos)
	}
	// Rodada completa: os dois contam.
	_, _, resolvidos = base.Partition(root, nil, nil)
	if resolvidos != 2 {
		t.Errorf("resolvidos=%d, quer 2", resolvidos)
	}
}

// Ida e volta pelo disco, com recusa de versão desconhecida.
func TestBaselineSaveLoad(t *testing.T) {
	root := projetoComArquivo(t, "datasets/ds.js", "if (a === 'x') {}\n")
	if b, err := LoadBaseline(root); err != nil || b != nil {
		t.Fatalf("projeto sem baseline: b=%v err=%v", b, err)
	}
	base := NewBaseline(root, []Finding{
		{Rule: RuleJavaStrictEq, Severity: SeverityWarning, File: "datasets/ds.js", Line: 1},
	})
	if err := base.Save(root); err != nil {
		t.Fatal(err)
	}
	lido, err := LoadBaseline(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(lido.Entries) != 1 || lido.Entries[0].Rule != RuleJavaStrictEq {
		t.Errorf("entradas lidas = %+v", lido.Entries)
	}
	if err := os.WriteFile(BaselinePath(root), []byte(`{"version":"9.9.9","entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(root); err == nil {
		t.Error("versão desconhecida deveria ser recusada")
	}
}
